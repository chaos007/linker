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
	quit := make(chan bool)

	defer func() {
		if err := recover(); err != nil {
			s.errorHandler(err.(error))
		}

		conn.Close()
		quit <- true
	}()

	receivePackets := make(chan Packet, 100)
	go s.handlePacket(conn, receivePackets, quit)

	var (
		bType         []byte = make([]byte, 4)
		bHeaderLength []byte = make([]byte, 4)
		bBodyLength   []byte = make([]byte, 4)
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

		headerLength = BytesToUint32(bHeaderLength)
		bodyLength = BytesToUint32(bBodyLength)
		pacLen := headerLength + bodyLength + uint32(12)

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

		receivePackets <- s.protocolPacket.Pack(BytesToUint32(bType), header, body)
	}
}

func (s *Server) handlePacket(conn net.Conn, receivePackets <-chan Packet, quit <-chan bool) {
	defer func() {
		if err := recover(); err != nil {
			s.errorHandler(err.(error))
		}
	}()

	ctx := NewContext(context.Background(), &request{Conn: conn, Packet: s.protocolPacket}, response{Conn: conn, Packet: s.protocolPacket})
	if s.constructHandler != nil {
		s.constructHandler(ctx)
	}

	for {
		select {
		case p := <-receivePackets:
			handler, ok := s.handlerContainer[p.OperateType()]
			if !ok {
				continue
			}

			go func(handler Handler) {
				defer func() {
					if err := recover(); err != nil {
						s.errorHandler(err.(error))
					}
				}()

				req := &request{Conn: conn, Packet: p}
				res := response{Conn: conn, Packet: s.protocolPacket}
				ctx = NewContext(context.Background(), req, res)

				if rm, ok := s.int32Middleware[p.OperateType()]; ok {
					for _, v := range rm {
						ctx = v.Handle(ctx)
					}
				}

				for _, v := range s.middleware {
					ctx = v.Handle(ctx)
				}

				handler(ctx)
			}(handler)

		case <-quit:
			// 执行链接退出以后回收操作
			if s.destructHandler != nil {
				s.destructHandler(ctx)
			}

			return
		}
	}
}
