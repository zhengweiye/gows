package gows

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/zhengweiye/gopool"
	"sync"
	"time"
)

/**
 * 客户端连接服务端
 * @param url
 * @param ctx
 * @param wg
 * @param handler 业务处理
 */
func ConnectServer(url string, ctx context.Context, wg *sync.WaitGroup, handler ClientHandler) (err error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return
	}
	newConnection(url, conn, ctx, wg, handler)
	return
}

type ClientConnection struct {
	url           string
	heartBeatTime int64
	pool          *gopool.Pool
	conn          *websocket.Conn
	ctx           context.Context
	wg            *sync.WaitGroup
	handler       ClientHandler
	readWg        *sync.WaitGroup
	writeWg       *sync.WaitGroup
	disconnectWg  *sync.WaitGroup
	writeChan     chan Data
	quitChan      chan int
	close         bool
	closeLock     sync.Mutex
}

func newConnection(url string, conn *websocket.Conn, ctx context.Context, wg *sync.WaitGroup, handler ClientHandler) *ClientConnection {
	readWg := &sync.WaitGroup{}
	writeWg := &sync.WaitGroup{}
	disconnectWg := &sync.WaitGroup{}
	connWrap := &ClientConnection{
		url:           url,
		heartBeatTime: 15,
		pool:          newPool("ws_client", ctx, wg, 50, 3),
		conn:          conn,
		ctx:           ctx,
		wg:            wg,
		handler:       handler,
		readWg:        readWg,
		writeWg:       writeWg,
		disconnectWg:  disconnectWg,
		writeChan:     make(chan Data, 50),
		quitChan:      make(chan int),
	}

	// 监听
	connWrap.listen()

	// 回调触发
	connWrap.connectedCallback()

	/*
	 * （1）这里加1
	 * （2）Close()时Done()
	 * （3）main()函数的信号量执行wait()
	 */
	connWrap.wg.Add(1)

	return connWrap
}

func (c *ClientConnection) listen() {
	go c.quitLoop()
	go c.readLoop()
	go c.writeLoop()
}

func (c *ClientConnection) quitLoop() {
	defer c.Close()
	for {
		select {
		case <-c.ctx.Done():
			/*
			 * 调用cancel()时，触发
			 */
			return
		case <-c.quitChan:
			/*
			 * 触发条件
			 * 	（1）processWrite()报错 c.quitChan<-1
			 *  （2）Close()立马close(quitChan)
			 */
			return
		}
	}
}

func (c *ClientConnection) readLoop() {
	defer c.conn.Close()
	for {
		select {
		default:
			/*
			 * （1）这里一直会堵塞，导致其它分支无法被及时触发
			 * （2）这里退出监听的唯一条件是 c.conn.Close()-->连接断开
			 * （3）心跳包数据不会被读取
			 */
			messageType, messageData, err := c.conn.ReadMessage()
			if err != nil {
				fmt.Printf("[gows client] [%s] read data err, errMsg=%v\n", time.Now().Format("2006-01-02 15:04:05"), err)
				return
			}

			// 协程池处理请求
			c.pool.ExecTask(gopool.Job{
				JobName: "client_process",
				JobFunc: c.processRead,
				JobParam: map[string]any{
					"messageType": messageType,
					"messageData": messageData,
				},
			})
		}
	}
}

func (c *ClientConnection) writeLoop() {
	ticker := time.NewTicker(time.Duration(c.heartBeatTime) * time.Second)
	defer c.Close()
	defer ticker.Stop()
	for {
		select {
		case <-c.quitChan:
			/*
			 * 触发条件
			 * 	（1）processWrite()报错 c.quitChan<-1
			 *  （2）Close()立马close(quitChan)
			 */
			return
		case <-ticker.C:
			err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			if err != nil {
				fmt.Printf("[gows client] write ping message err, errMsg=%v\n", err)
				return
			}
		case data := <-c.writeChan:
			/*
			 * 触发条件
			 * （1）Close()里面执行close(c.writeChan)时，会触发这里
			 * （2）Send()方法调用
			 */
			if data.MessageType > 0 { // 防止close(c.responseChan)时，还往客户端响应数据
				c.processWrite(data)
			}
		}
	}
}

func (c *ClientConnection) processRead(workerId int, jobName string, param map[string]any) (err error) {
	c.readWg.Add(1)
	defer c.readWg.Done()

	messageType := param["messageType"].(int)
	messageData := param["messageData"].([]byte)

	// 业务函数回调
	c.doCallback(messageType, messageData)
	return
}

func (c *ClientConnection) processWrite(data Data) {
	defer c.writeWg.Done()
	err := c.conn.WriteMessage(data.MessageType, data.MessageData)
	// 响应错误时，证明连接断开了，退出监听
	if err != nil {
		c.quitChan <- 1
	}
	return
}

/*
 * 发送数据
 */
func (c *ClientConnection) Send(data Data) (err error) {
	switch data.MessageType {
	case websocket.TextMessage:
	case websocket.BinaryMessage:
		err = fmt.Errorf("不支持messageType[%d]", data.MessageType)
		return
	}
	c.writeWg.Add(1)
	c.writeChan <- data
	return
}

/*
 * 关闭连接
 */
func (c *ClientConnection) Close() {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()

	if c.close {
		return
	}
	c.close = true

	fmt.Printf("[gows client] [%s] closing\n", time.Now().Format("2006-01-02 15:04:05"))

	// 请求处理堵塞
	c.readWg.Wait()

	// 响应堵塞
	c.writeWg.Wait() // 必须放在最后，因为上面两个Wait()都可能需要往客户端发送数据
	fmt.Printf("[gows client] [%s] close finish\n", time.Now().Format("2006-01-02 15:04:05"))

	// 关闭通道
	close(c.writeChan) // 触发writeLoop()退出监听
	close(c.quitChan)  // 触发writeLoop()、quitLoop()退出监听

	// 关闭底层conn
	c.conn.Close()

	// 回调函数执行-->c.conn.Close()之后才能回调-->因为如果conn没有关闭，如何用户重连，那么就出问题
	c.disconnectCallback()

	// 标记完成
	c.wg.Done()

	// 回调堵塞
	c.disconnectWg.Wait()
}

/*
 * 重连
 */
func (c *ClientConnection) Reconnect() (err error) {
	if !c.close {
		return
	}
	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		return
	}
	newConnection(c.url, conn, c.ctx, c.wg, c.handler)
	return
}

func (c *ClientConnection) connectedCallback() {
	if c.handler != nil {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("[gows client] Connected() function callback error, errMsg=%v\n", err)
			}
		}()
		c.handler.Connected(c)
	}
}

func (c *ClientConnection) doCallback(messageType int, messageData []byte) {
	if c.handler != nil {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("[gows client] [%s] do() function callback err, errMsg=%v \n",
					time.Now().Format("2006-01-02 15:04:05"),
					err,
				)
			}
		}()
		c.handler.Do(c, Data{
			MessageType: messageType,
			MessageData: messageData,
		})
	}
}

func (c *ClientConnection) disconnectCallback() {
	if c.handler != nil {
		c.disconnectWg.Add(1)
		defer c.disconnectWg.Done()
		c.handler.Disconnected(c)
	}
}
