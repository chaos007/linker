package export

import (
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wpajqz/linker"
)

const MaxPayload = 2048

type Handler interface {
	Handle(header, body []byte)
}

type ErrorHandler interface {
	Handle(err string)
}

type RequestStatusCallback interface {
	OnSuccess(header, body []byte)
	OnError(status int, message string)
	OnStart()
	OnEnd()
}

type Client struct {
	running                chan bool
	mutex                  *sync.Mutex
	rwMutex                *sync.RWMutex
	timeout, retryInterval time.Duration
	conn                   net.Conn
	handlerContainer       sync.Map
	packet                 chan linker.Packet
	constructHandler       Handler
	destructHandler        Handler
	errorHandler           ErrorHandler
	request, response      struct {
		Header, Body []byte
	}
}

type handlerFunc func(header, body []byte)

func (f handlerFunc) Handle(header, body []byte) {
	f(header, body)
}

func NewClient() *Client {
	c := &Client{
		running:          make(chan bool, 1),
		mutex:            new(sync.Mutex),
		rwMutex:          new(sync.RWMutex),
		timeout:          30 * time.Second,
		retryInterval:    5 * time.Second,
		packet:           make(chan linker.Packet, 1024),
		handlerContainer: sync.Map{},
	}

	return c
}

func (c *Client) Connect(server string, port int) error {
	address := strings.Join([]string{server, strconv.Itoa(port)}, ":")
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}

	c.conn = conn

	if c.constructHandler != nil {
		c.constructHandler.Handle(nil, nil)
	}

	// 检测conn的状态，断线以后进行重连操作
	go func() {
		err := c.handleConnection(conn)
		for {
			if err != nil {
				conn, err = net.Dial("tcp", address)
				if err != nil {
					if c.errorHandler != nil {
						c.errorHandler.Handle(err.Error())
						time.Sleep(c.retryInterval) // 重连失败以后休息一会再干活
					}
				} else {
					c.conn = conn
					err = c.handleConnection(conn)
				}
			}
		}
	}()

	return nil
}

// 心跳处理，客户端与服务端保持长连接
func (c *Client) Ping(interval int64, param []byte, callback RequestStatusCallback) {
	sequence := time.Now().UnixNano()
	listener := int64(linker.OPERATOR_HEARTBEAT) + sequence

	c.handlerContainer.Store(listener, handlerFunc(func(header, body []byte) {
		code := c.GetResponseProperty("code")
		if code != "" {
			message := c.GetResponseProperty("message")
			if callback.OnError != nil {
				v, _ := strconv.Atoi(code)
				callback.OnError(v, message)
			}
		} else {
			if callback.OnSuccess != nil {
				callback.OnSuccess(header, body)
			}

			if callback.OnEnd != nil {
				callback.OnEnd()
			}
		}
	}))

	// 建立连接以后就发送心跳包建立会话信息，后面的定期发送
	p := linker.NewPack(linker.OPERATOR_HEARTBEAT, sequence, c.request.Header, param)
	c.packet <- p
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		select {
		case <-ticker.C:
			c.packet <- p
		}
	}
}

// 向服务端发送请求，同步处理服务端返回结果
func (c *Client) SyncSend(operator string, param []byte, callback RequestStatusCallback) {
	nType := crc32.ChecksumIEEE([]byte(operator))
	sequence := time.Now().UnixNano()
	listener := int64(nType) + sequence

	// 对数据请求的返回状态进行处理,同步阻塞处理机制
	c.mutex.Lock()
	quit := make(chan bool)

	if callback.OnStart != nil {
		callback.OnStart()
	}

	c.handlerContainer.Store(listener, handlerFunc(func(header, body []byte) {
		code := c.GetResponseProperty("code")
		if code != "" {
			message := c.GetResponseProperty("message")
			if callback.OnError != nil {
				v, _ := strconv.Atoi(code)
				callback.OnError(v, message)
			}
		} else {
			if callback.OnSuccess != nil {
				callback.OnSuccess(header, body)
			}

			if callback.OnEnd != nil {
				callback.OnEnd()
			}
		}

		c.handlerContainer.Delete(listener)
		quit <- true
	}))

	p := linker.NewPack(nType, sequence, c.request.Header, param)
	c.packet <- p
	<-quit
	c.mutex.Unlock()
}

// 向服务端发送请求，异步处理服务端返回结果
func (c *Client) AsyncSend(operator string, param []byte, callback RequestStatusCallback) {
	nType := crc32.ChecksumIEEE([]byte(operator))
	sequence := time.Now().UnixNano()

	listener := int64(nType) + sequence
	if callback.OnStart != nil {
		callback.OnStart()
	}

	c.handlerContainer.Store(listener, handlerFunc(func(header, body []byte) {
		code := c.GetResponseProperty("code")
		if code != "" {
			message := c.GetResponseProperty("message")
			if callback.OnError != nil {
				v, _ := strconv.Atoi(code)
				callback.OnError(v, message)
			}
		} else {
			if callback.OnSuccess != nil {
				callback.OnSuccess(header, body)
			}

			if callback.OnEnd != nil {
				callback.OnEnd()
			}
		}

		c.handlerContainer.Delete(listener)
	}))

	p := linker.NewPack(nType, sequence, c.request.Header, param)
	c.packet <- p
}

// 添加事件监听器
func (c *Client) AddMessageListener(listener string, callback Handler) {
	c.handlerContainer.Store(int64(crc32.ChecksumIEEE([]byte(listener))), callback)
}

// 移除事件监听器
func (c *Client) RemoveMessageListener(listener string) {
	c.handlerContainer.Delete(int64(crc32.ChecksumIEEE([]byte(listener))))
}

// 链接建立以后执行的操作
func (c *Client) OnOpen(handler Handler) {
	c.constructHandler = handler
}

// 链接断开以后执行回收操作
func (c *Client) OnClose(handler Handler) {
	c.destructHandler = handler
}

// 设置默认错误处理方法
func (c *Client) OnError(errorHandler ErrorHandler) {
	c.errorHandler = errorHandler
}

// 设置请求属性
func (c *Client) SetRequestProperty(key, value string) {
	c.request.Header = append(c.request.Header, []byte(key+"="+value+";")...)
}

// 获取请求属性
func (c *Client) GetRequestProperty(key string) string {
	values := strings.Split(string(c.request.Header), ";")
	for _, value := range values {
		kv := strings.Split(value, "=")
		if kv[0] == key {
			return kv[1]
		}
	}

	return ""
}

// 获取响应属性
func (c *Client) GetResponseProperty(key string) string {
	values := strings.Split(string(c.response.Header), ";")
	for _, value := range values {
		kv := strings.Split(value, "=")
		if kv[0] == key {
			return kv[1]
		}
	}

	return ""
}

// 设置响应属性
func (c *Client) SetResponseProperty(key, value string) {
	c.response.Header = append(c.response.Header, []byte(key+"="+value+";")...)
}

// 设置断线重连的间隔时间, 单位s
func (c *Client) SetRetryInterval(interval int) {
	c.retryInterval = time.Duration(interval) * time.Second
}

// 设置服务端默认超时时间, 单位s
func (c *Client) SetTimeout(timeout int) {
	c.timeout = time.Duration(timeout) * time.Second
}