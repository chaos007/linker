package linker

type Packet struct {
	nType         uint32
	nSequence     int64
	nHeaderLength uint32
	nBodyLength   uint32
	bHeader       []byte
	bBody         []byte
}

func NewPack(operator uint32, sequence int64, header, body []byte) Packet {
	return Packet{
		nType:         operator,
		nSequence:     sequence,
		nHeaderLength: uint32(len(header)),
		nBodyLength:   uint32(len(body)),
		bHeader:       header,
		bBody:         body,
	}
}

// 得到序列化后的Packet
func (p Packet) Bytes() (buf []byte) {
	buf = append(buf, Uint32ToBytes(p.nType)...)
	buf = append(buf, Int64ToBytes(p.nSequence)...)
	buf = append(buf, Uint32ToBytes(p.nHeaderLength)...)
	buf = append(buf, Uint32ToBytes(p.nBodyLength)...)
	buf = append(buf, p.bHeader...)
	buf = append(buf, p.bBody...)

	return buf
}

func (p Packet) OperateType() uint32 {
	return p.nType
}

func (p Packet) Sequence() int64 {
	return p.nSequence
}

func (p Packet) Header() []byte {
	return p.bHeader
}

func (p Packet) Body() []byte {
	return p.bBody
}
