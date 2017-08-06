package client

import (
	"context"
	"io"
	"net"

	"github.com/wpajqz/linker"
)

// 处理客户端连接
func (c *Client) handleConnection(conn net.Conn) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err := recover(); err != nil {
			if c.errorHandler != nil {
				c.errorHandler(err.(error))
			}
		}

		cancel()
	}()

	go c.handleSendPackets(ctx, conn)

	return c.handleReceivedPackets(conn)
}

// 对发送的数据包进行处理
func (c *Client) handleSendPackets(ctx context.Context, conn net.Conn) {
	for {
		select {
		case p := <-c.packet:
			_, err := conn.Write(p.Bytes())
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// 对接收到的数据包进行处理
func (c *Client) handleReceivedPackets(conn net.Conn) error {
	var (
		bType         []byte = make([]byte, 4)
		bSequence     []byte = make([]byte, 8)
		bHeaderLength []byte = make([]byte, 4)
		bBodyLength   []byte = make([]byte, 4)
		sequence      int64
		headerLength  uint32
		bodyLength    uint32
		pacLen        uint32
	)

	for {
		if n, err := io.ReadFull(conn, bType); err != nil && n != 4 {
			return err
		}

		if n, err := io.ReadFull(conn, bSequence); err != nil && n != 8 {
			return err
		}

		if n, err := io.ReadFull(conn, bHeaderLength); err != nil && n != 4 {
			return err
		}

		if n, err := io.ReadFull(conn, bBodyLength); err != nil && n != 4 {
			return err
		}

		nType := linker.BytesToUint32(bType)
		sequence = linker.BytesToInt64(bSequence)
		headerLength = linker.BytesToUint32(bHeaderLength)
		bodyLength = linker.BytesToUint32(bBodyLength)

		pacLen = headerLength + bodyLength + 20
		if pacLen > MaxPayload {
			return ErrPacketLength
		}

		header := make([]byte, headerLength)
		if n, err := io.ReadFull(conn, header); err != nil && n != int(headerLength) {
			return err
		}

		body := make([]byte, bodyLength)
		if n, err := io.ReadFull(conn, body); err != nil && n != int(bodyLength) {
			return err
		}

		operator := int64(nType) + sequence
		if handler, ok := c.handlerContainer.Load(operator); ok {
			if v, ok := handler.(Handler); ok {
				req := &Request{Conn: conn, OperateType: nType, Sequence: sequence, Header: c.Context.Request.Header, Body: c.Context.Request.Body}
				res := Response{Conn: conn, OperateType: nType, Sequence: sequence, Header: header, Body: body}

				ctx := &Context{req, res}
				v(ctx)
			}
		}
	}
}
