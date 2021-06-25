# EasyTCP

[![Run Actions](https://github.com/DarthPestilane/easytcp/actions/workflows/actions.yml/badge.svg?branch=master&event=push)](https://github.com/DarthPestilane/easytcp/actions/workflows/actions.yml)
[![codecov](https://codecov.io/gh/DarthPestilane/easytcp/branch/master/graph/badge.svg?token=002KJ5IV4Z)](https://codecov.io/gh/DarthPestilane/easytcp)

## Introduction

`EasyTCP` is a light-weight TCP framework written in Go (Golang), features with:

- Non-invasive design
- Pipelined middlewares for route handler
- Customizable message packer and codec
- Handy functions to handle request data and send response

`EasyTCP` helps you build a TCP server easily and fast.

## Install

This package, so far, has been tested in

- go1.14.x
- go1.15.x
- go1.16.x

Use the below Go command to install EasyTCP.

```sh
$ go get -u github.com/DarthPestilane/easytcp
```

## Quick start

```go
package main

import (
	"fmt"
	"github.com/DarthPestilane/easytcp"
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/DarthPestilane/easytcp/router"
	"github.com/DarthPestilane/easytcp/server"
)

func main() {
	// create a new server
	s := easytcp.NewTCPServer(&server.TCPOption{})

	// add a route to message id
	s.AddRoute(uint(1001), func(ctx *router.Context) (packet.Message, error) {
		fmt.Printf("[server] request received | id: %d; size: %d; data: %s\n", ctx.MsgID(), ctx.MsgSize(), ctx.MsgRawData())
		return ctx.Response(uint(1002), []byte("copy that"))
	})

	// listen and serve
	if err := s.Serve(":5896"); err != nil && err != server.ErrServerStopped {
		fmt.Println("serve error: ", err.Error())
	}
}
```

Above is the server side example. There are client and more detailed examples in [examples/tcp](./examples/tcp)

## API

### Architecture

### Routing

EasyTCP considers every message has a `ID` segment.
A message will be routed, according to it's id, to the handler through middelwares.

```sh
# request flow:

+----------+    +--------------+    +--------------+    +---------+
| request  |--->|              |--->|              |--->|         |
+----------+    |              |    |              |    |         |
                | middleware 1 |    | middleware 2 |    | handler |
+----------+    |              |    |              |    |         |
| response |<---|              |<---|              |<---|         |
+----------+    +--------------+    +--------------+    +---------+
```

#### Register a route

```go
s.AddRoute(reqID, func(ctx *router.Context) (packet.Message, error) {
	// handle the request via ctx
	fmt.Printf("[server] request received | id: %d; size: %d; data: %s\n", ctx.MsgID(), ctx.MsgSize(), ctx.MsgRawData())

	// do things...

	// return response
	return ctx.Response(respID, []byte("copy that"))
})
```

#### Using middleware

```go
// register global middlewares.
// global middlewares are priorer than per-route middlewares, they will be invoked first
s.Use(recoverMiddleware, logMiddleware, ...)

// register middlewares for one route
s.AddRoute(reqID, handler, middleware1, middleware2)

// a middleware looks like:
var exampleMiddleware router.MiddlewareFunc = func(next router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) (resp packet.Message, err error) {
		// do things before...
		resp, err := next(ctx)
		// do things after...
		return resp, err
	}
}
```

### Packer

A packer is to pack and unpack packets' payload. We can set the Packer when creating the server.

```go
s := easytcp.NewTCPServer(&server.TCPOption{
	MsgPacker: new(MyPacker), // this is optional, the default one is packet.DefaultPacker
})
```

We can set our own Packer or EasyTCP uses [`DefaultPacker`](./packet/packer.go).

The `DefaultPacker` considers packet's payload as a `ID|Size|Data` format.
This may not covery most cases, fortunately, we can create our own Packer.

```go
// Msg16bit implements packet.Message
type Msg16bit struct {
	Size uint16
	ID   uint16
	Data []byte
}

func (m *Msg16bit) Setup(id uint, data []byte) {
	m.ID = uint16(id)
	m.Data = data
	m.Size = uint16(len(data))
}

func (m *Msg16bit) Duplicate() packet.Message {
	return &Msg16bit{}
}

func (m *Msg16bit) GetID() uint {
	return uint(m.ID)
}

func (m *Msg16bit) GetSize() uint {
	return uint(m.Size)
}

func (m *Msg16bit) GetData() []byte {
	return m.Data
}

// Packer16bit is a custom packer, implements packet.Packer
// packet format: size[2]id[2]data
type Packer16bit struct{}

func (p *Packer16bit) bytesOrder() binary.ByteOrder {
	return binary.BigEndian
}

func (p *Packer16bit) Pack(msg packet.Message) ([]byte, error) {
	buff := bytes.NewBuffer(make([]byte, 0, 2+2+msg.GetSize()))
	if err := binary.Write(buff, p.bytesOrder(), uint16(msg.GetSize())); err != nil {
		return nil, fmt.Errorf("write size err: %s", err)
	}
	if err := binary.Write(buff, p.bytesOrder(), uint16(msg.GetID())); err != nil {
		return nil, fmt.Errorf("write id err: %s", err)
	}
	if err := binary.Write(buff, p.bytesOrder(), msg.GetData()); err != nil {
		return nil, fmt.Errorf("write data err: %s", err)
	}
	return buff.Bytes(), nil
}

func (p *Packer16bit) Unpack(reader io.Reader) (packet.Message, error) {
	sizeBuff := make([]byte, 2)
	if _, err := io.ReadFull(reader, sizeBuff); err != nil {
		return nil, fmt.Errorf("read size err: %s", err)
	}
	size := p.bytesOrder().Uint16(sizeBuff)

	idBuff := make([]byte, 2)
	if _, err := io.ReadFull(reader, idBuff); err != nil {
		return nil, fmt.Errorf("read id err: %s", err)
	}
	id := p.bytesOrder().Uint16(idBuff)

	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, fmt.Errorf("read data err: %s", err)
	}
	msg := &Msg16bit{
		Size: size,
		ID:   id,
		Data: data,
	}
	return msg, nil
}
```

### Codec

A Codec is to encode and decode message data. The Codec is optional, EasyTCP won't encode/decode message data if Codec is not set.
We can set Codec when creating the server.

```go
s := easytcp.NewTCPServer(&server.TCPOption{
	MsgCodec: &packet.JsonCodec{}, // this is optional. The JsonCodec is a built-in codec
})
```

Since we set the codec, we may want to decode the request data in route handler.

```go
s.AddRoute(reqID, func(ctx *router.Context) (packet.Message, error) {
	var reqData map[string]interface{}
	if err := ctx.Bind(&reqData); err != nil { // here we decode message data and bind to reqData
		// handle error...
	}
	fmt.Printf("[server] request received | id: %d; size: %d; data-decoded: %+v\n", ctx.MsgID(), ctx.MsgSize(), reqData)
	respData := map[string]string{"key": "value"}
	return ctx.Response(respID, respData)
})
```

## Contribute

Check out a new branch for the job, and make sure git action passed.
