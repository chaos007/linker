package linker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wpajqz/linker/utils/convert"
	"github.com/wpajqz/linker/utils/encrypt"
)

func (s *Server) handleWebSocketConnection(ctx context.Context, conn *websocket.Conn) error {
	receivePackets := make(chan Packet, 100)
	go s.handleWebSocketPacket(ctx, conn, receivePackets)

	var (
		bType         = make([]byte, 4)
		bSequence     = make([]byte, 8)
		bHeaderLength = make([]byte, 4)
		bBodyLength   = make([]byte, 4)
		sequence      int64
		headerLength  uint32
		bodyLength    uint32
	)

	conn.SetReadLimit(MaxPayload)
	conn.SetReadDeadline(time.Now().Add(s.timeout))

	for {
		_, r, err := conn.NextReader()
		if err != nil {
			return err
		}

		if n, err := io.ReadFull(r, bType); err != nil && n != 4 {
			return err
		}

		if n, err := io.ReadFull(r, bSequence); err != nil && n != 8 {
			return err
		}

		if n, err := io.ReadFull(r, bHeaderLength); err != nil && n != 4 {
			return err
		}

		if n, err := io.ReadFull(r, bBodyLength); err != nil && n != 4 {
			return err
		}

		sequence = convert.BytesToInt64(bSequence)
		headerLength = convert.BytesToUint32(bHeaderLength)
		bodyLength = convert.BytesToUint32(bBodyLength)
		pacLen := headerLength + bodyLength + uint32(20)

		if pacLen > s.maxPayload {
			_, file, line, _ := runtime.Caller(1)
			return SystemError{time.Now(), file, line, "packet larger than MaxPayload"}
		}

		header := make([]byte, headerLength)
		if n, err := io.ReadFull(r, header); err != nil && n != int(headerLength) {
			return err

		}

		body := make([]byte, bodyLength)
		if n, err := io.ReadFull(r, body); err != nil && n != int(bodyLength) {
			return err
		}

		header, err = encrypt.Decrypt(header)
		if err != nil {
			return err
		}

		body, err = encrypt.Decrypt(body)
		if err != nil {
			return err
		}

		receivePackets <- NewPack(convert.BytesToUint32(bType), sequence, header, body)
	}
}

func (s *Server) handleWebSocketPacket(ctx context.Context, conn *websocket.Conn, receivePackets <-chan Packet) {
	var c Context = &ContextWebsocket{Conn: conn}
	for {
		select {
		case p := <-receivePackets:
			c = NewContextWebsocket(conn, p.OperateType(), p.Sequence(), s.contentType, p.Header(), p.Body())
			if p.OperateType() == OPERATOR_HEARTBEAT && s.pingHandler != nil {
				go func() {
					s.pingHandler.Handle(c)
					c.Success(nil)
				}()

				continue
			}

			handler, ok := s.router.handlerContainer[p.OperateType()]
			if !ok {
				continue
			}

			go func(c Context, handler Handler) {
				if rm, ok := s.router.routerMiddleware[p.OperateType()]; ok {
					for _, v := range rm {
						c = v.Handle(c)
					}
				}

				for _, v := range s.router.middleware {
					c = v.Handle(c)
					if tm, ok := v.(TerminateMiddleware); ok {
						tm.Terminate(c)
					}
				}

				handler.Handle(c)
				c.Success(nil) // If it don't call the function of Success or Error, deal it by default
			}(c, handler)
		case <-ctx.Done():
			// 执行链接退出以后回收操作
			if s.destructHandler != nil {
				s.destructHandler.Handle(c)
			}

			return
		}
	}
}

// 开始运行webocket服务
func (s *Server) RunWebSocket(address string) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var upgrade = websocket.Upgrader{
			HandshakeTimeout:  s.timeout,
			ReadBufferSize:    int(s.maxPayload),
			WriteBufferSize:   int(s.maxPayload),
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}

		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer func() {
			if r := recover(); r != nil {
				if s.errorHandler != nil {
					switch v := r.(type) {
					case error:
						s.errorHandler(v)
					case string:
						s.errorHandler(errors.New(v))
					}
				}
			}

			cancel()
			conn.Close()
		}()

		if s.constructHandler != nil {
			s.constructHandler.Handle(nil)
		}

		err = s.handleWebSocketConnection(ctx, conn)
		if err != nil {
			if s.errorHandler != nil {
				s.errorHandler(err)
			}
		}
	})

	fmt.Printf("websocket server running on %s\n", address)

	return http.ListenAndServe(address, nil)
}
