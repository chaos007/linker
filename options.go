package linker

import (
	"time"

	"github.com/wpajqz/linker/broker"
	"github.com/wpajqz/linker/plugin"
)

type (
	Options struct {
		Debug                                                        bool
		ReadBufferSize                                               int
		WriteBufferSize                                              int
		Timeout                                                      time.Duration
		MaxPayload                                                   uint32
		ContentType                                                  string
		Broker                                                       broker.Broker
		PluginForPacketSender                                        []plugin.PacketPlugin
		PluginForPacketReceiver                                      []plugin.PacketPlugin
		errorHandler, constructHandler, destructHandler, pingHandler Handler
	}

	Option func(o *Options)
)

func Debug() Option {
	return func(o *Options) {
		o.Debug = true
	}
}

func ReadBufferSize(size int) Option {
	return func(o *Options) {
		o.ReadBufferSize = size
	}
}

func WriteBufferSize(size int) Option {
	return func(o *Options) {
		o.WriteBufferSize = size
	}
}

func Timeout(d time.Duration) Option {
	return func(o *Options) {
		o.Timeout = d
	}
}

func MaxPayload(maxPayload uint32) Option {
	return func(o *Options) {
		o.MaxPayload = maxPayload
	}
}

func ContentType(mime string) Option {
	return func(o *Options) {
		o.ContentType = mime
	}
}

func Broker(broker broker.Broker) Option {
	return func(o *Options) {
		o.Broker = broker
	}
}

func PluginForPacketSender(plugins ...plugin.PacketPlugin) Option {
	return func(o *Options) {
		o.PluginForPacketSender = append(o.PluginForPacketSender, plugins...)
	}
}

func PluginForPacketReceiver(plugins ...plugin.PacketPlugin) Option {
	return func(o *Options) {
		o.PluginForPacketReceiver = append(o.PluginForPacketReceiver, plugins...)
	}
}

func WithOnError(handler Handler) Option {
	return func(o *Options) {
		o.errorHandler = handler
	}
}

func WithOnClose(handler Handler) Option {
	return func(o *Options) {
		o.destructHandler = handler
	}
}

func WithOnOpen(handler Handler) Option {
	return func(o *Options) {
		o.constructHandler = handler
	}
}

func WithOnPing(handler Handler) Option {
	return func(o *Options) {
		o.pingHandler = handler
	}
}
