package codec

import "fmt"

const (
	JSON     = "text/json"
	PROTOBUF = "text/protobuf"
)

type Coder interface {
	Encoder(data interface{}) ([]byte, error)
	Decoder(data []byte, v interface{}) error
}

var adapters = make(map[string]Coder)

func Register(name string, adapter Coder) {
	if adapter == nil {
		panic("codec: Register adapter is nil")
	}

	if _, ok := adapters["name"]; ok {
		panic("codec: Register called twice for adapter " + name)
	}

	adapters[name] = adapter
}

func NewCoder(adapterName string) (Coder, error) {
	if v, ok := adapters[adapterName]; ok {
		return v, nil
	}

	return nil, fmt.Errorf("codec: unknown adapter name %q (forgot to import?)", adapterName)
}