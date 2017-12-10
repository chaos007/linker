package linker

import (
	"hash/crc32"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/wpajqz/linker/codec"
)

type (
	Handler      func(Context)
	ErrorHandler func(error)
	Server       struct {
		debug            bool
		contentType      string
		timeout          time.Duration
		handlerContainer map[uint32]Handler
		middleware       []Middleware
		routerMiddleware map[uint32][]Middleware
		maxPayload       uint32
		errorHandler     ErrorHandler
		heartbeatHandler Handler
		constructHandler Handler
		destructHandler  Handler
	}
)

func NewServer() *Server {
	return &Server{
		contentType:      codec.JSON,
		timeout:          TIMEOUT,
		maxPayload:       MaxPayload,
		handlerContainer: make(map[uint32]Handler),
		routerMiddleware: make(map[uint32][]Middleware),
		errorHandler: func(err error) {
			if err != io.EOF {
				log.Println(err.Error())
			}
		},
	}
}

// 设置所有请求的序列化数据类型
func (s *Server) SetDebug(bool bool) {
	s.debug = bool
}

// 设置所有请求的序列化数据类型
func (s *Server) SetContentType(contentType string) {
	s.contentType = contentType
}

// 设置默认超时时间
func (s *Server) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// 设置可处理的数据包的最大长度
func (s *Server) SetMaxPayload(maxPayload uint32) {
	s.maxPayload = maxPayload
}

// 在服务中注册要处理的handler
func (s *Server) HandleFunc(pattern string, handler Handler) {
	data := []byte(pattern)
	operator := crc32.ChecksumIEEE(data)

	if _, ok := s.handlerContainer[operator]; !ok {
		s.handlerContainer[operator] = handler
	}
}

// 绑定Server需要处理的router
func (s *Server) BindRouter(routers []Router) {
	for _, router := range routers {
		operator := crc32.ChecksumIEEE([]byte(router.Operator))
		if operator <= OPERATOR_MAX {
			panic("Unavailable operator, the value of crc32 need less than " + strconv.Itoa(OPERATOR_MAX))
		}

		for _, m := range router.Middleware {
			s.routerMiddleware[operator] = append(s.routerMiddleware[operator], m)
		}

		s.HandleFunc(router.Operator, router.Handler)
	}
}

// 添加请求需要进行处理的中间件
func (s *Server) Use(middleware ...Middleware) {
	s.middleware = append(s.middleware, middleware...)
}

// 设置默认错误处理方法
func (s *Server) OnError(errorHandler ErrorHandler) {
	s.errorHandler = errorHandler
}

// 客户端链接断开以后执行回收操作
func (s *Server) OnClose(handler Handler) {
	s.destructHandler = handler
}

// 客户端建立连接以后初始化操作
func (s *Server) OnOpen(handler Handler) {
	s.constructHandler = handler
}

// 设置心跳包的handler,需要客户端发送心跳包才能够触发
// 客户端发送心跳包，服务端未调用此方法时只起到建立长连接的作用
func (s *Server) OnPing(handler Handler) {
	s.handlerContainer[OPERATOR_HEARTBEAT] = handler
}
