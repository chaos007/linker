package linker

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/wpajqz/linker/utils/convert"
)

func (s *Server) handleTCPConnection(conn *net.TCPConn) error {
	ctx := &ContextTcp{common: common{Context: context.Background()}, Conn: conn}
	if s.options.constructHandler != nil {
		s.options.constructHandler.Handle(ctx)
	}

	ctx.Set(nodeID, uuid.NewV4().String())

	defer func() {
		if s.options.destructHandler != nil {
			s.options.destructHandler.Handle(ctx)
		}

		_ = ctx.unSubscribe()
		_ = conn.Close()
	}()

	if s.options.ReadBufferSize > 0 {
		err := conn.SetReadBuffer(s.options.ReadBufferSize)
		if err != nil {
			return err
		}
	}

	if s.options.WriteBufferSize > 0 {
		err := conn.SetWriteBuffer(s.options.WriteBufferSize)
		if err != nil {
			return err
		}
	}

	var (
		bType         = make([]byte, 4)
		bSequence     = make([]byte, 8)
		bHeaderLength = make([]byte, 4)
		bBodyLength   = make([]byte, 4)
		sequence      int64
		headerLength  uint32
		bodyLength    uint32
	)

	for {
		if s.options.Timeout != 0 {
			err := conn.SetDeadline(time.Now().Add(s.options.Timeout))
			if err != nil {
				return err
			}
		}

		if _, err := io.ReadFull(conn, bType); err != nil {
			return err
		}

		if _, err := io.ReadFull(conn, bSequence); err != nil {
			return err
		}

		if _, err := io.ReadFull(conn, bHeaderLength); err != nil {
			return err
		}

		if _, err := io.ReadFull(conn, bBodyLength); err != nil {
			return err
		}

		sequence = convert.BytesToInt64(bSequence)
		headerLength = convert.BytesToUint32(bHeaderLength)
		bodyLength = convert.BytesToUint32(bBodyLength)
		pacLen := headerLength + bodyLength + uint32(20)

		if pacLen > s.options.MaxPayload {
			_, file, line, _ := runtime.Caller(1)
			return SystemError{time.Now(), file, line, "packet larger than MaxPayload"}
		}

		header := make([]byte, headerLength)
		if _, err := io.ReadFull(conn, header); err != nil {
			return err
		}

		body := make([]byte, bodyLength)
		if _, err := io.ReadFull(conn, body); err != nil {
			return err
		}

		rp, err := NewPacket(convert.BytesToUint32(bType), sequence, header, body, s.options.PluginForPacketReceiver)
		if err != nil {
			return err
		}

		ctx = NewContextTcp(ctx.Context, conn, rp.Operator, rp.Sequence, rp.Header, rp.Body, s.options)
		go s.handleTCPPacket(ctx, rp)
	}
}

func (s *Server) handleTCPPacket(ctx Context, rp Packet) {
	defer func() {
		if r := recover(); r != nil {
			var errMsg string

			switch v := r.(type) {
			case string:
				errMsg = v
			case error:
				errMsg = v.Error()
			default:
				errMsg = StatusText(StatusInternalServerError)
			}

			ctx.Set(errorTag, errMsg)

			if s.options.errorHandler != nil {
				s.options.errorHandler.Handle(ctx)
			}

			ctx.Error(StatusInternalServerError, errMsg)
		}
	}()

	if rp.Operator == OperatorHeartbeat {
		if s.options.pingHandler != nil {
			s.options.pingHandler.Handle(ctx)
		}

		ctx.Success(nil)
	}

	handler, ok := s.router.handlerContainer[rp.Operator]
	if !ok {
		ctx.Error(StatusInternalServerError, "server don't register your request.")
	}

	if rm, ok := s.router.routerMiddleware[rp.Operator]; ok {
		for _, v := range rm {
			ctx = v.Handle(ctx)
		}
	}

	for _, v := range s.router.middleware {
		ctx = v.Handle(ctx)
		if tm, ok := v.(TerminateMiddleware); ok {
			tm.Terminate(ctx)
		}
	}

	handler.Handle(ctx)
	ctx.Success(nil) // If it don't call the function of Success or Error, deal it by default
}

// RunTCP 开始运行Tcp服务
func (s *Server) RunTCP(name, address string) error {
	tcpAddr, err := net.ResolveTCPAddr(name, address)
	if err != nil {
		return err
	}

	listener, err := net.ListenTCP(name, tcpAddr)
	if err != nil {
		return err
	}

	defer listener.Close()

	fmt.Printf("Listening and serving TCP on %s\n", address)
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			continue
		}

		go func(conn *net.TCPConn) {
			err := s.handleTCPConnection(conn)
			if err != nil && err != io.EOF {
				fmt.Printf("tcp connection error: %s\n", err.Error())
			}
		}(conn)
	}
}
