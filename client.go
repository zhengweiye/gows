package gows

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Client struct {
	clientIp          string
	clientType        string // 服务名称
	connWrap          *ConnectionWrap
	handler           Handler // 处理器
	ctx               context.Context
	globalWaitGroup   *sync.WaitGroup // 全局WaitGroup，确保所有Client执行完成
	response          http.ResponseWriter
	request           *http.Request
	urlParameters     []string
	urlParameterMap   map[string]string
	headerNames       []string
	headerMap         map[string]string
	responseQueueSize int
	poolWorkerSize    int
	poolQueueSize     int
	heartBeatSeconds  int64
}

var clientLock sync.Mutex

type Option func(c *Client)

/**
 * 接受（监听）客户端
 * （1）客户端连接进来，调用 AcceptClient()函数 --> 会被拦截器拦截
 * （2）客户端发送数据进来，不会再调用 AcceptClient()函数 --> 不会经过拦截器
 * （3）客户端分类管理
 * 		（1）不同 “业务”，创建不同的 Client，通过name去区分Client
 *  	（2）通过group对Connection进行分组管理，例如：使用userId作为group，一个账号在不同pc建立Connection，通过userId作为group，可以批量给该分组进行推送数据
 * （4）客户端断开
 *		（1）外部的web服务（如：gin、net/http）停止
 *		（2）客户端主动断开
 */

func AcceptClient(
	clientType string,
	ctx context.Context,
	wg *sync.WaitGroup,
	response http.ResponseWriter,
	request *http.Request,
	opts ...Option,
) *Client {
	// 创建server
	client := &Client{
		clientType:        clientType,
		ctx:               ctx,
		globalWaitGroup:   wg,
		response:          response,
		request:           request,
		urlParameterMap:   make(map[string]string),
		headerMap:         make(map[string]string),
		responseQueueSize: 50,
		poolWorkerSize:    5,
		poolQueueSize:     50,
		heartBeatSeconds:  0,
	}

	// 给字段赋值
	for _, opt := range opts {
		opt(client)
	}

	// 监听conn
	client.listenConn()

	// 定时发送心跳
	if client.heartBeatSeconds > 0 {
		newHeartBeat(clientType, client.heartBeatFunc)
	}

	fmt.Printf("[gows] [%s] [%s] [%s] connect success\n", time.Now().Format("2006-01-02 15:04:05"), client.clientType, client.clientIp)
	return client
}

/**
 * url携带参数
 * 如：ws://127.0.0.1:8080/ws/test?apiKey=xxx&userId=xxx
 * paramNames 参数名称（数组）
 */
func UrlParamOption(paramNames []string) Option {
	return func(client *Client) {
		client.urlParameters = paramNames
	}
}

/*
 * 请求头参数
 * headerNames 请求头名称（数组）
 */
func HeaderOption(headerNames []string) Option {
	return func(client *Client) {
		client.headerNames = headerNames
	}
}

/*
 * 协程池
 * workerSize 协程数量
 * queueSize 每个协程负责监听的工作队列大小
 */
func PoolOption(workerSize, queueSize int) Option {
	return func(client *Client) {
		client.poolWorkerSize = workerSize
		client.poolQueueSize = queueSize
	}
}

/*
 * 响应客户端：服务端个客户端发送数据时，不是实时发送，而是把数据放到队列里面，由独立协程负责读取队列数据并且发送给客户端
 * queueSize 队列长度
 */
func ResponseOption(queueSize int) Option {
	return func(client *Client) {
		client.responseQueueSize = queueSize
	}
}

/*
 * 心跳
 * seconds：每隔{seconds}秒往客户端，发送一个心跳包
 */
func HeartBeatOption(seconds int64) Option {
	return func(client *Client) {
		client.heartBeatSeconds = seconds
	}
}

/*
 * 回调handler
 */
func HandlerOption(handler Handler) Option {
	return func(client *Client) {
		client.handler = handler
	}
}

/**
 * 获取客户端的 “连接”
 */
func (c *Client) GetConnection() *ConnectionWrap {
	return c.connWrap
}

func (c *Client) listenConn() {
	// 服务升级
	upgrader := &websocket.Upgrader{
		//ReadBufferSize:  1024,// 读缓冲区大小
		//WriteBufferSize: 1024,// 写缓冲区大小
		CheckOrigin: func(r *http.Request) bool { // 检查请求来源
			/**
			if r.Method != "GET" {
				fmt.Println("method is not GET")
			    return false
			}
			if r.URL.Path != "/ws" {
			    fmt.Println("path error")
			    return false
			}
			*/
			return true
		},
	}
	conn, err := upgrader.Upgrade(c.response, c.request, nil)
	if err != nil {
		panic(err)
	}

	// clientIp
	c.clientIp = conn.RemoteAddr().String()

	// url参数
	if len(c.urlParameters) > 0 {
		url, err2 := url.Parse(c.request.RequestURI)
		if err2 != nil {
			m := map[string]any{
				"isSuccess": false,
				"msg":       fmt.Sprint(err2),
			}
			conn.WriteJSON(m)
			conn.Close()
			return
		}
		for _, parameterName := range c.urlParameters {
			c.urlParameterMap[parameterName] = url.Query().Get(parameterName)
		}
	}

	// 请求头
	if len(c.headerNames) > 0 {
		for _, headerName := range c.headerNames {
			c.headerMap[headerName] = c.request.Header.Get(headerName)
		}
	}

	// 创建conn的包装类
	c.connWrap = newConnectionWrap(conn, c)
}

func (c *Client) heartBeatFunc() {
	go func() {
		ticker := time.NewTicker(time.Duration(c.heartBeatSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.connWrap.connectionManager.check()
			}
		}
	}()
}
