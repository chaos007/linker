package coder

import (
	"errors"

	"github.com/golang/protobuf/proto"
)

type ProtoBufCoder struct{}

func (c *ProtoBufCoder) Encoder(data interface{}) ([]byte, error) {
	v, ok := data.(proto.Message)
	if !ok {
		return nil, errors.New("Unsupported data format")
	}

	return proto.Marshal(v)
}

func (c *ProtoBufCoder) Decoder(data []byte, des interface{}) error {
	v, ok := des.(proto.Message)
	if !ok {
		return errors.New("Unsupported data format")
	}

	return proto.Unmarshal(data, v)
}

func init() {
	register(PROTOBUF, &ProtoBufCoder{})
}
