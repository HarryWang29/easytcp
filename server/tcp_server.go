package server

import (
	"fmt"
	"github.com/DarthPestilane/easytcp/logger"
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/DarthPestilane/easytcp/router"
	"github.com/DarthPestilane/easytcp/session"
	"github.com/sirupsen/logrus"
	"net"
)

type TcpServer struct {
	rwBufferSize int
	listener     *net.TCPListener
	log          *logrus.Entry
	msgPacker    packet.Packer
	msgCodec     packet.Codec
	accepting    chan struct{}
	router       *router.Router
}

var _ Server = &TcpServer{}

type TcpOption struct {
	RWBufferSize int           // socket 读写 buffer
	MsgPacker    packet.Packer // 消息封包/拆包器
	MsgCodec     packet.Codec  // 消息编码/解码器
}

func NewTcpServer(opt TcpOption) *TcpServer {
	if opt.MsgPacker == nil {
		opt.MsgPacker = &packet.DefaultPacker{}
	}
	if opt.MsgCodec == nil {
		opt.MsgCodec = &packet.StringCodec{}
	}
	return &TcpServer{
		log:          logger.Default.WithField("scope", "server.TcpServer"),
		rwBufferSize: opt.RWBufferSize,
		msgPacker:    opt.MsgPacker,
		msgCodec:     opt.MsgCodec,
		accepting:    make(chan struct{}),
		router:       router.New(),
	}
}

func (s *TcpServer) Serve(addr string) error {
	address, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	lis, err := net.ListenTCP("tcp", address)
	if err != nil {
		return err
	}
	s.listener = lis

	return s.acceptLoop()
}

func (s *TcpServer) acceptLoop() error {
	close(s.accepting)
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			return fmt.Errorf("accept err: %s", err)
		}
		if s.rwBufferSize > 0 {
			if err := conn.SetReadBuffer(s.rwBufferSize); err != nil {
				return fmt.Errorf("conn set read buffer err: %s", err)
			}
			if err := conn.SetWriteBuffer(s.rwBufferSize); err != nil {
				return fmt.Errorf("conn set write buffer err: %s", err)
			}
		}

		// handle conn in a new goroutine
		go s.handleConn(conn)
	}
}

// handleConn
// create a new session and save it to memory
// read/write loop
// route incoming message to handler
// wait for session to close
// remove session from memory
func (s *TcpServer) handleConn(conn *net.TCPConn) {
	sess := session.NewTcp(conn, s.msgPacker, s.msgCodec)
	session.Sessions().Add(sess)
	go s.router.Loop(sess)
	go sess.ReadLoop()
	go sess.WriteLoop()
	sess.WaitUntilClosed()
	session.Sessions().Remove(sess.ID()) // session has been closed, remove it
	s.log.WithField("sid", sess.ID()).Tracef("session closed")
	if err := conn.Close(); err != nil {
		s.log.Tracef("connection close err: %s", err)
	}
}

// Stop stops server and closes all the tcp sessions
func (s *TcpServer) Stop() error {
	closedNum := 0
	session.Sessions().Range(func(id string, sess session.Session) (next bool) {
		if tcpSess, ok := sess.(*session.TcpSession); ok {
			tcpSess.Close()
			closedNum++
		}
		return true
	})
	s.log.Tracef("%d session(s) closed", closedNum)
	return s.listener.Close()
}

func (s *TcpServer) AddRoute(msgId uint, handler router.HandlerFunc, middlewares ...router.MiddlewareFunc) {
	s.router.Register(msgId, handler, middlewares...)
}

func (s *TcpServer) Use(middlewares ...router.MiddlewareFunc) {
	s.router.RegisterMiddleware(middlewares...)
}
