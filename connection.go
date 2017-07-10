package linker

import (
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"time"
)

func (s *Server) handleConnection(conn net.Conn) {
	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		if err := recover(); err != nil {
			s.errorHandler(err.(error))
		}

		conn.Close()
		cancel()
	}()

	receivePackets := make(chan Packet, 100)
	go s.handlePacket(ctx, conn, receivePackets)

	var (
		bType         []byte = make([]byte, 4)
		bSequence     []byte = make([]byte, 8)
		bHeaderLength []byte = make([]byte, 4)
		bBodyLength   []byte = make([]byte, 4)
		sequence      int64
		headerLength  uint32
		bodyLength    uint32
	)

	for {

		conn.SetDeadline(time.Now().Add(s.timeout))

		if n, err := io.ReadFull(conn, bType); err != nil && n != 4 {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})
		}

		if n, err := io.ReadFull(conn, bSequence); err != nil && n != 8 {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})
		}

		if n, err := io.ReadFull(conn, bHeaderLength); err != nil && n != 4 {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})
		}

		if n, err := io.ReadFull(conn, bBodyLength); err != nil && n != 4 {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})
		}

		sequence = BytesToInt64(bSequence)
		headerLength = BytesToUint32(bHeaderLength)
		bodyLength = BytesToUint32(bBodyLength)
		pacLen := headerLength + bodyLength + uint32(20)

		if pacLen > s.MaxPayload {
			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, "packet larger than MaxPayload"})
		}

		header := make([]byte, headerLength)
		if n, err := io.ReadFull(conn, header); err != nil && n != int(headerLength) {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})

		}

		body := make([]byte, bodyLength)
		if n, err := io.ReadFull(conn, body); err != nil && n != int(bodyLength) {
			if err == io.EOF {
				return
			}

			_, file, line, _ := runtime.Caller(1)
			panic(SystemError{time.Now(), file, line, fmt.Sprintf("Read packetLength failed: %v", err)})
		}

		receivePackets <- NewPack(BytesToUint32(bType), sequence, header, body)
	}
}

func (s *Server) handlePacket(ctx context.Context, conn net.Conn, receivePackets <-chan Packet) {
	defer func() {
		if err := recover(); err != nil {
			s.errorHandler(err.(error))
		}
	}()

	c := NewContext(ctx, &request{Conn: conn}, response{Conn: conn})
	if s.constructHandler != nil {
		s.constructHandler(c)
	}

	for {
		select {
		case p := <-receivePackets:
			handler, ok := s.handlerContainer[p.OperateType()]
			if !ok {
				continue
			}

			req := &request{Conn: conn, OperateType: p.OperateType(), Sequence: p.Sequence(), Header: p.Header(), Body: p.Body()}
			res := response{Conn: conn, OperateType: p.OperateType(), Sequence: p.Sequence()}
			c = NewContext(c, req, res)

			if rm, ok := s.routerMiddleware[p.OperateType()]; ok {
				for _, v := range rm {
					c = v.Handle(c)
				}
			}

			for _, v := range s.middleware {
				c = v.Handle(c)
				if tm, ok := v.(TerminateMiddleware); ok {
					tm.Terminate(c)
				}
			}

			go func(handler Handler, ctx *Context) {
				defer func() {
					if err := recover(); err != nil {
						s.errorHandler(err.(error))
					}
				}()

				handler(c)
			}(handler, c)

		case <-ctx.Done():
			// 执行链接退出以后回收操作
			if s.destructHandler != nil {
				s.destructHandler(c)
			}

			return
		}
	}
}
