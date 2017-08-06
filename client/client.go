package client

import (
	"errors"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wpajqz/linker"
	"github.com/wpajqz/linker/coder"
)

const (
	MaxPayload = uint32(2048)
	TIMEOUT    = 30
)

var (
	ErrPacketLength = errors.New("the packet is big than " + strconv.Itoa(int(MaxPayload)))
)

type Handler func(*Context)
type ErrorHandler func(error)

type RequestStatusCallback struct {
	OnSuccess  func(ctx *Context)
	OnProgress func(progress int, status string)
	OnError    func(code int, message string)
}

type Client struct {
	mutex                   *sync.Mutex
	rwMutex                 *sync.RWMutex
	Context                 *Context
	timeout                 time.Duration
	conn                    net.Conn
	handlerContainer        sync.Map
	packet                  chan linker.Packet
	cancelHeartbeat         chan bool
	closeClient             chan bool
	running                 chan bool
	OnConnectionStateChange func(status bool)
	destructHandler         Handler
	errorHandler            ErrorHandler
}

func NewClient() *Client {
	c := &Client{
		mutex:            new(sync.Mutex),
		rwMutex:          new(sync.RWMutex),
		Context:          &Context{Request: &Request{}, Response: Response{}},
		timeout:          TIMEOUT * time.Second,
		packet:           make(chan linker.Packet, 1024),
		handlerContainer: sync.Map{},
		cancelHeartbeat:  make(chan bool, 1),
		closeClient:      make(chan bool, 1),
		running:          make(chan bool, 1),
	}

	return c
}

func (c *Client) Connect(server string, port int) {
	address := strings.Join([]string{server, strconv.Itoa(port)}, ":")
	conn, err := net.Dial("tcp", address)
	if err != nil {
		panic(err)
	}

	c.conn = conn

	go func(string, net.Conn) {
		for {
			err := c.handleConnection(c.conn)
			if err != nil {
				c.running <- false
			}

			select {
			case r := <-c.running:
				if !r {
					// 把在线状态传递出去,方便调用方给用户提示信息
					if c.OnConnectionStateChange != nil {
						c.OnConnectionStateChange(r)
					}

					for {
						//服务端timeout设置影响链接延时时间
						conn, err := net.Dial("tcp", address)
						if err == nil {
							c.conn = conn
							if c.OnConnectionStateChange != nil {
								c.OnConnectionStateChange(true)
							}

							break
						}
					}
				}
			case <-c.closeClient:
				return
			}
		}
	}(address, c.conn)
}

func (c *Client) StartHeartbeat(interval time.Duration, param interface{}) error {
	t := c.Context.Request.GetRequestProperty("Content-Type")
	r, err := coder.NewCoder(t)
	if err != nil {
		return err
	}

	pbData, err := r.Encoder(param)
	if err != nil {
		return err
	}

	sequence := time.Now().UnixNano()
	p := linker.NewPack(linker.OPERATOR_HEARTBEAT, sequence, c.Context.Request.Header, pbData)

	// 建立连接以后就发送心跳包建立会话信息，后面的定期发送
	c.packet <- p
	ticker := time.NewTicker(interval * time.Second)
	for {
		select {
		case <-ticker.C:
			c.packet <- p
		case <-c.cancelHeartbeat:
			return nil
		}
	}
}

func (c *Client) StopHeartbeat() {
	c.cancelHeartbeat <- true
}

func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

func (c *Client) Close() {
	c.cancelHeartbeat <- true
	c.closeClient <- true
	close(c.cancelHeartbeat)
	close(c.closeClient)
}

// 向服务端发送请求，同步处理服务端返回结果
func (c *Client) SyncCall(operator string, param interface{}, callback RequestStatusCallback) {
	t := c.Context.Request.GetRequestProperty("Content-Type")
	r, err := coder.NewCoder(t)
	if err != nil {
		if callback.OnError != nil {
			callback.OnError(500, err.Error())
		}
	}

	pbData, err := r.Encoder(param)
	if err != nil {
		if callback.OnError != nil {
			callback.OnError(500, err.Error())
		}
	}

	nType := crc32.ChecksumIEEE([]byte(operator))
	sequence := time.Now().UnixNano()
	listener := int64(nType) + sequence

	// 对数据请求的返回状态进行处理,同步阻塞处理机制
	c.mutex.Lock()
	quit := make(chan bool)

	if callback.OnProgress != nil {
		callback.OnProgress(0, "proecssing...")
	}

	var handler Handler = func(ctx *Context) {
		code := ctx.Response.GetResponseProperty("code")
		if code != "" {
			message := ctx.Response.GetResponseProperty("message")
			if callback.OnError != nil {
				v, _ := strconv.Atoi(code)
				callback.OnError(v, message)
			}
		} else {
			if callback.OnSuccess != nil {
				callback.OnSuccess(ctx)
			}

			if callback.OnProgress != nil {
				callback.OnProgress(100, "successful")
			}
		}

		c.handlerContainer.Delete(listener)
		quit <- true
	}

	c.handlerContainer.Store(listener, handler)

	p := linker.NewPack(nType, sequence, c.Context.Request.Header, pbData)
	c.packet <- p
	<-quit
	c.mutex.Unlock()
}

// 向服务端发送请求，异步处理服务端返回结果
func (c *Client) AsyncCall(operator string, param interface{}, callback RequestStatusCallback) {
	t := c.Context.Request.GetRequestProperty("Content-Type")
	r, err := coder.NewCoder(t)
	if err != nil {
		if callback.OnError != nil {
			callback.OnError(500, err.Error())
		}
	}

	pbData, err := r.Encoder(param)
	if err != nil {
		if callback.OnError != nil {
			callback.OnError(500, err.Error())
		}
	}

	nType := crc32.ChecksumIEEE([]byte(operator))
	sequence := time.Now().UnixNano()

	listener := int64(nType) + sequence
	if callback.OnProgress != nil {
		callback.OnProgress(0, "proecssing...")
	}

	var handler Handler = func(ctx *Context) {
		code := ctx.Response.GetResponseProperty("code")
		if code != "" {
			message := ctx.Response.GetResponseProperty("message")
			if callback.OnError != nil {
				v, _ := strconv.Atoi(code)
				callback.OnError(v, message)
			}
		} else {
			if callback.OnSuccess != nil {
				callback.OnSuccess(ctx)
			}

			if callback.OnProgress != nil {
				callback.OnProgress(100, "successful")
			}
		}

		c.handlerContainer.Delete(listener)
	}
	c.handlerContainer.Store(listener, handler)

	p := linker.NewPack(nType, sequence, c.Context.Request.Header, pbData)
	c.packet <- p
}

// 添加事件监听器
func (c *Client) AddMessageListener(operator string, callback Handler) {
	c.handlerContainer.Store(int64(crc32.ChecksumIEEE([]byte(operator))), callback)
}

// 移除事件监听器
func (c *Client) RemoveMessageListener(operator string) {
	c.handlerContainer.Delete(int64(crc32.ChecksumIEEE([]byte(operator))))
}

// 链接断开以后执行回收操作
func (c *Client) OnClose(handler Handler) {
	c.destructHandler = handler
}

// 设置默认错误处理方法
func (c *Client) OnError(errorHandler ErrorHandler) {
	c.errorHandler = errorHandler
}
