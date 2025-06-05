package gows

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/zhengweiye/gopool"
	"io"
	"sync"
	"time"
)

type ConnectionWrap struct {
	groupId           string
	connId            string
	conn              *websocket.Conn
	client            *Client
	lastTime          time.Time       // 更新该字段三个地方：（1）readLoop()；（2）writeLoop()；（3）心跳成功
	readWg            *sync.WaitGroup // 主要用于Close()，等待 “当前conn” 的 readLoop() 执行完成
	writeWg           *sync.WaitGroup // 主要用于Close()，等待 “当前conn” 的 writeLoop() 执行完成
	disconnectWg      *sync.WaitGroup // 主要用于Close()，等待 “当前conn” 的 “回调” 执行完成
	responseChan      chan Data
	quitChan          chan int
	close             bool
	closeLock         sync.Mutex
	pool              *gopool.Pool       // 每种业务 “clientType” 独立一份 “协程池”
	connectionManager *ConnectionManager // 每种业务 “clientType” 独立一份 “连接管理器”
}

func newConnectionWrap(conn *websocket.Conn, client *Client) (connWrap *ConnectionWrap) {
	readWg := &sync.WaitGroup{}
	writeWg := &sync.WaitGroup{}
	disconnectWg := &sync.WaitGroup{}

	// 创建ConnWrap
	connWrap = &ConnectionWrap{
		groupId:           client.clientIp,
		connId:            client.clientIp,
		conn:              conn,
		client:            client,
		lastTime:          time.Now(),
		readWg:            readWg,
		writeWg:           writeWg,
		disconnectWg:      disconnectWg,
		responseChan:      make(chan Data, client.responseQueueSize),
		quitChan:          make(chan int),
		pool:              newPool(client.clientType, client.ctx, client.globalWaitGroup, client.poolQueueSize, client.poolWorkerSize),
		connectionManager: newConnectionManager(client.clientType),
	}

	// 加入管理器
	connWrap.connectionManager.add(connWrap)

	// 监听
	connWrap.listen()

	/*
	 * （1）这里加1
	 * （2）Close()时Done()
	 * （3）main()函数的信号量执行wait()
	 */
	client.globalWaitGroup.Add(1)

	// 回调触发
	connWrap.connectedCallback()
	return
}

/*
 * 设置连接分组Id
 * 作用：把连接分组管理，例如：同一个账号在不同设备登陆。如果给该用户发送通知时，所有设备都会收到消息
 */
func (c *ConnectionWrap) SetGroupId(groupId string) {
	if len(groupId) > 0 {
		c.connectionManager.moveToNewGroup(groupId, c)
	}
}

/**
 * 获取连接分组Id
 */
func (c *ConnectionWrap) GetGroupId() string {
	return c.groupId
}

/*
 * 设置连接Id
 */
/*func (c *ConnectionWrap) SetId(id string) {
	if len(id) > 0 {
		c.connId = id
	}
}*/

/*
 * 获取连接Id
 */
func (c *ConnectionWrap) GetId() string {
	return c.connId
}

/**
 * 获取客户端ip:port信息
 */
func (c *ConnectionWrap) GetIp() string {
	return c.client.clientIp
}

/**
 * 获取协程池
 */
func (c *ConnectionWrap) GetPool() *gopool.Pool {
	return c.pool
}

/*
 * 获取连接管理器
 */
func (c *ConnectionWrap) GetConnectionManager() *ConnectionManager {
	return c.connectionManager
}

/**
 * 发送数据
 */
func (c *ConnectionWrap) Send(data Data) (err error) {
	switch data.MessageType {
	case websocket.TextMessage:
	case websocket.BinaryMessage:
	case websocket.CloseMessage:
	case websocket.PingMessage:
	case websocket.PongMessage:
		err = fmt.Errorf("不支持messageType[%d]", data.MessageType)
		return
	}
	c.writeWg.Add(1)
	c.responseChan <- data
	return
}

func (c *ConnectionWrap) listen() {
	go c.quitLoop()
	go c.writeLoop()
	go c.readLoop()
}

func (c *ConnectionWrap) quitLoop() {
	defer c.Close()
	for {
		select {
		case <-c.client.ctx.Done():
			return
		case <-c.quitChan:
			/*
			 * 触发条件
			 * 	（1）processResponse()报错 c.quitChan<-1
			 *  （2）Close()立马close(quitChan)
			 */
			return
		}
	}
}

func (c *ConnectionWrap) writeLoop() {
	defer c.Close()
	for {
		select {
		case <-c.quitChan:
			/*
			 * 触发条件
			 * 	（1）processResponse()报错 c.quitChan<-1
			 *  （2）Close()立马close(quitChan)
			 */
			return
		case responseData := <-c.responseChan:
			/*
			 * 触发条件
			 * （1）Close()里面执行close(c.responseChan)时，会触发这里
			 * （2）Send()方法调用
			 */
			if responseData.MessageType > 0 { // 防止close(c.responseChan)时，还往客户端响应数据
				// 更新时间
				c.lastTime = time.Now()

				// 处理响应
				c.processResponse(responseData)
			}
		}
	}
}

func (c *ConnectionWrap) readLoop() {
	defer c.Close()
	for {
		select {
		default:
			/*
			 * （1）这里一直会堵塞，导致其它分支无法被及时触发
			 * （2）这里退出监听的唯一条件是 c.conn.Close()-->连接断开
			 * （3）心跳包数据不会被读取
			 */
			messageType, messageData, err := c.conn.ReadMessage()
			if err != nil || err == io.EOF {
				fmt.Printf("[gows] [%s] [%s] [%s] read data err, errMsg=%v\n", time.Now().Format("2006-01-02 15:04:05"), c.client.clientType, c.client.clientIp, err)
				return
			}

			// 更新时间
			c.lastTime = time.Now()

			// 协程池处理请求
			c.pool.ExecTask(gopool.Job{
				JobName: fmt.Sprintf("processRequest_%s", c.client.clientType),
				JobFunc: c.processRequest,
				JobParam: map[string]any{
					"messageType": messageType,
					"messageData": messageData,
				},
			})
		}
	}
}

func (c *ConnectionWrap) processResponse(responseData Data) (err error) {
	defer c.writeWg.Done()
	err = c.conn.WriteMessage(responseData.MessageType, responseData.MessageData)
	// 响应错误时，证明连接断开了，退出监听
	if err != nil {
		c.quitChan <- 1
	}
	return
}

func (c *ConnectionWrap) processRequest(workerId int, jobName string, param map[string]any) (err error) {
	c.readWg.Add(1)
	defer c.readWg.Done()
	messageType := param["messageType"].(int)
	messageData := param["messageData"].([]byte)

	// 业务函数回调
	c.doCallback(messageType, messageData)
	return
}

/*
 * 关闭连接
 */
func (c *ConnectionWrap) Close() {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()
	if c.close {
		return
	}
	c.close = true

	fmt.Printf("[gows] [%s] [%s] [%s] stoping\n", time.Now().Format("2006-01-02 15:04:05"), c.client.clientType, c.client.clientIp)

	// 请求处理堵塞
	c.readWg.Wait()
	// 回调函数执行
	c.disconnectCallback()

	// 回调堵塞
	c.disconnectWg.Wait()

	// 响应堵塞
	c.writeWg.Wait() // 必须放在最后，因为上面两个Wait()都可能需要往客户端发送数据
	fmt.Printf("[gows] [%s] [%s] [%s] stop finish\n", time.Now().Format("2006-01-02 15:04:05"), c.client.clientType, c.client.clientIp)

	// 关闭通道
	close(c.responseChan) // 触发writeLoop()退出监听
	close(c.quitChan)     // 触发writeLoop()退出监听

	// 关闭连接
	c.connectionManager.remove(c)

	// 标记完成
	c.client.globalWaitGroup.Done()
}

func (c *ConnectionWrap) connectedCallback() {
	if c.client.handler != nil {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("[gows] [%s] Connected() function callback error, errMsg=%v\n", c.client.clientType, err)
			}
		}()
		c.client.handler.Connected(c, c.client.urlParameterMap, c.client.headerMap)
	}
}

func (c *ConnectionWrap) doCallback(messageType int, messageData []byte) {
	if c.client.handler != nil {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("[gows] [%s] [%s] [%s] do() function callback err, errMsg=%v \n",
					time.Now().Format("2006-01-02 15:04:05"),
					c.client.clientType,
					c.client.clientIp,
					err,
				)
			}
		}()
		c.client.handler.Do(c, Data{
			MessageType: messageType,
			MessageData: messageData,
		}, c.client.urlParameterMap, c.client.headerMap)
	}
}

func (c *ConnectionWrap) disconnectCallback() {
	if c.client.handler != nil {
		c.disconnectWg.Add(1)
		defer c.disconnectWg.Done()
		c.client.handler.Disconnected(c, c.client.urlParameterMap, c.client.headerMap)
	}
}

/*
func (c *ConnectionClient) Reconnect() {
	c.isCloseLock.Lock()
	defer c.isCloseLock.Unlock()

	if c.isClose {
		ticker := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-ticker.C:
				err := c.client.connect(c.url) // 其实还是共用同一个wsClient,只是里面的conn更改了而已
				if err == nil {
					ticker.Stop()
					return
				}
			}
		}
	}
}
*/
