package linker

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/golang/glog"
	"github.com/wpajqz/linker/utils"
)

func (s *Server) handleConnection(conn net.Conn) {
	quit := make(chan bool)

	defer func() {
		conn.Close()
		quit <- true
	}()

	receivePackets := make(chan Packet, 100)
	go s.handlePacket(conn, receivePackets, quit)

	// 只有设置有超时的情况下才进行心跳检测进行长连接
	heartbeatPackets := make(chan Packet, 100)
	if s.writeTimeout > 0 || s.readTimeout > 0 {
		go s.checkHeartbeat(conn, heartbeatPackets, quit)
	}

	var (
		bLen   []byte = make([]byte, 4)
		bType  []byte = make([]byte, 4)
		pacLen int32
	)

	for {
		if n, err := io.ReadFull(conn, bLen); err != nil && n != 4 {
			glog.Errorf("Read packetLength failed: %v", err)
			return
		}

		if n, err := io.ReadFull(conn, bType); err != nil && n != 4 {
			glog.Errorf("Read packetType failed: %v", err)
			return
		}

		if pacLen = utils.BytesToInt32(bLen); pacLen > s.MaxPayload {
			glog.Errorf("packet larger than MaxPayload")
			return
		}

		dataLength := pacLen - 8
		data := make([]byte, dataLength)
		if n, err := io.ReadFull(conn, data); err != nil && n != int(dataLength) {
			glog.Errorf("Read packetData failed: %v", err)
		}

		// 0号包预留为心跳包使用,其他handler不能够使用
		operator := utils.BytesToInt32(bType)
		if operator == 0 {
			// 只有设置有超时的情况下才进行心跳检测进行长连接
			if s.writeTimeout > 0 || s.readTimeout > 0 {
				heartbeatPackets <- s.protocolPacket.New(pacLen, utils.BytesToInt32(bType), data)
			}

		} else {
			receivePackets <- s.protocolPacket.New(pacLen, utils.BytesToInt32(bType), data)
		}
	}
}

func (s *Server) handlePacket(conn net.Conn, receivePackets <-chan Packet, quit <-chan bool) {
	for {
		select {
		case p := <-receivePackets:
			handler, ok := s.handlerContainer[p.OperateType()]
			if !ok {
				continue
			}

			go func(handler Handler) {
				req := &Request{conn, p.OperateType(), p}
				res := Response{conn, 200, ""}
				ctx := NewContext(context.Background(), req, res)

				for _, v := range s.middleware {
					ctx = v.Handle(ctx)
				}

				if rm, ok := s.int32Middleware[p.OperateType()]; ok {
					for _, v := range rm {
						ctx = v.Handle(ctx)
					}
				}

				handler(ctx)
			}(handler)

		case <-quit:
			glog.Info("Stop handle receivePackets.")
			return
		}
	}
}

func (s *Server) checkHeartbeat(conn net.Conn, heartbeatPackets <-chan Packet, quit <-chan bool) {
	for {
		select {
		case <-heartbeatPackets:
			if s.writeTimeout != 0 {
				conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
			}

			if s.readTimeout != 0 {
				conn.SetReadDeadline(time.Now().Add(s.readTimeout))
			}
		case <-time.After(s.writeTimeout):
			// todo:添加心跳断开以后的处理逻辑
			conn.Close()
		case <-time.After(s.readTimeout):
			conn.Close()
		case <-quit:
			return
		}
	}
}
