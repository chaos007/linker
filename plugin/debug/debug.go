package debug

import (
	"fmt"

	"github.com/wpajqz/linker/utils/encrypt"
)

type Debug struct {
	Sender bool
}

func (d *Debug) Handle(header, body []byte) (h, b []byte) {
	if d.Sender {
		th, _ := encrypt.Decrypt(header)
		tb, _ := encrypt.Decrypt(body)

		fmt.Println("[send packet]", "header:", string(th), "body:", string(tb))
	} else {
		fmt.Println("[receive packet]", "header:", string(header), "body:", string(body))
	}

	return header, body
}

func NewPlugin(sender bool) *Debug {
	return &Debug{Sender: sender}
}
