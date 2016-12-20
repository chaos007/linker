package packet

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/wpajqz/linker"
)

type ProtoPacket struct {
	Type         uint32
	HeaderLength uint32
	BodyLength   uint32
	bHeader      []byte
	bBody        []byte
}

// 得到序列化后的Packet
func (p ProtoPacket) Bytes() (buf []byte) {
	buf = append(buf, linker.Uint32ToBytes(p.Type)...)
	buf = append(buf, linker.Uint32ToBytes(p.HeaderLength)...)
	buf = append(buf, linker.Uint32ToBytes(p.BodyLength)...)
	buf = append(buf, p.bHeader...)
	buf = append(buf, p.bBody...)

	return buf
}

// 将数据包类型和pb数据结构一起打包成Packet，并加密Packet.Data
func (p ProtoPacket) Pack(operator uint32, header []byte, body interface{}) (linker.Packet, error) {
	p.Type = operator
	pbData, err := proto.Marshal(body.(proto.Message))
	if err != nil {
		return ProtoPacket{}, fmt.Errorf("Pack error: %v", err.Error())
	}

	p.HeaderLength = uint32(len(header))
	p.bHeader = header

	p.bBody = pbData
	p.BodyLength = uint32(len(p.bBody))

	return p, nil
}

func (p ProtoPacket) UnPack(pb interface{}) error {
	err := proto.Unmarshal(p.bBody, pb.(proto.Message))
	if err != nil {
		return fmt.Errorf("Unpack error: %v", err.Error())
	}

	return nil
}

func (p ProtoPacket) New(operator uint32, header, body []byte) linker.Packet {
	return ProtoPacket{
		Type:         operator,
		HeaderLength: uint32(len(header)),
		BodyLength:   uint32(len(body)),
		bHeader:      header,
		bBody:        body,
	}
}

func (p ProtoPacket) OperateType() uint32 {
	return p.Type
}

func (p ProtoPacket) Header() []byte {
	return p.bHeader
}
