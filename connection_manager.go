package gows

import (
	"fmt"
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type ConnectionManager struct {
	clientType      string
	connectionWraps map[string][]*ConnectionWrap
	connectionLock  sync.RWMutex
}

var connectionManagers map[string]*ConnectionManager
var connectionManagerLock sync.Mutex

func init() {
	connectionManagers = make(map[string]*ConnectionManager)
}

func newConnectionManager(clientType string) *ConnectionManager {
	connectionManagerLock.Lock()
	defer connectionManagerLock.Unlock()

	manager, ok := connectionManagers[clientType]
	if ok && manager != nil {
		return manager
	}
	manager = &ConnectionManager{
		clientType:      clientType,
		connectionWraps: make(map[string][]*ConnectionWrap),
	}
	connectionManagers[clientType] = manager
	return manager
}

/*
 * 添加连接
 */

func (c *ConnectionManager) add(connWrap *ConnectionWrap) (err error) {
	c.connectionLock.Lock()
	defer c.connectionLock.Unlock()

	connList, ok := c.connectionWraps[connWrap.groupId]
	if !ok {
		connList = make([]*ConnectionWrap, 0, 1)
	}

	exist := false
	for _, item := range connList {
		if item.connId == connWrap.connId {
			exist = true
			break
		}
	}
	if !exist {
		connList = append(connList, connWrap)
		c.connectionWraps[connWrap.groupId] = connList
	}

	// 打印日志
	//fmt.Printf("[gows] connection_manager client [%s] num is [%d]\n", connWrap.client.clientType, len(c.connectionWraps))
	return
}

/*
 * 移除连接
 */

func (c *ConnectionManager) remove(connWrap *ConnectionWrap) (err error) {
	c.connectionLock.Lock()
	defer c.connectionLock.Unlock()

	defer func() {
		if err2 := recover(); err2 != nil {
			fmt.Printf("[gows] [%s] [%s] remove from connectionManager error, errMsg=%v\n", connWrap.client.clientType, connWrap.client.clientIp, err2)
		}
	}()

	connList, ok := c.connectionWraps[connWrap.groupId]
	if ok {
		connIndex := -1
		for index, item := range connList {
			if item.connId == connWrap.connId {
				connIndex = index
				break
			}
		}
		if connIndex > -1 {
			conn := connList[connIndex]
			// 触发readLoop()退出监听
			conn.conn.Close()

			// 从管理器移除
			connList = append(connList[:connIndex], connList[connIndex+1:]...)
		}
		if len(connList) == 0 {
			delete(c.connectionWraps, connWrap.groupId)
		} else {
			c.connectionWraps[connWrap.groupId] = connList
		}
	}

	// 打印日志
	//fmt.Printf("[gows] client [%s] num is [%d]\n", connWrap.client.clientType, len(c.connectionWraps))
	return
}

/**
 * 移动分组
 */
func (c *ConnectionManager) moveToNewGroup(newGroupId string, connWrap *ConnectionWrap) {
	if newGroupId == connWrap.groupId {
		return
	}

	c.connectionLock.Lock()
	defer c.connectionLock.Unlock()

	// 把conn从原来的分组移除
	connList, ok := c.connectionWraps[connWrap.groupId]
	if ok && len(connList) > 0 {
		connIndex := -1
		for index, conn := range connList {
			if conn.connId == connWrap.connId {
				connIndex = index
				break
			}
		}
		if connIndex > -1 {
			connList = append(connList[:connIndex], connList[connIndex+1:]...)
		}
		if len(connList) == 0 {
			delete(c.connectionWraps, connWrap.groupId)
		} else {
			c.connectionWraps[connWrap.groupId] = connList
		}
	}

	// 修改groupId
	connWrap.groupId = newGroupId

	// 把conn添加到新的分组
	newConnList, ok := c.connectionWraps[connWrap.groupId]
	if !ok {
		newConnList = make([]*ConnectionWrap, 0, 1)
	}
	exist := false
	for _, conn := range newConnList {
		if conn.connId == connWrap.connId {
			exist = true
			break
		}
	}
	if !exist {
		newConnList = append(newConnList, connWrap)
	}
	c.connectionWraps[connWrap.groupId] = newConnList
}

/*
 * 获取所有连接
 */

func (c *ConnectionManager) GetConnections() map[string][]*ConnectionWrap {
	c.connectionLock.RLock()
	defer c.connectionLock.RUnlock()

	return c.connectionWraps
}

/*
 * 心跳检查
 */
func (c *ConnectionManager) check() {
	c.connectionLock.RLock()
	defer c.connectionLock.RUnlock()

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("[gows] [%s] heart beat is error, errMsg=%v\n", c.clientType, err)
		}
	}()
	nowTime := time.Now()
	for _, connList := range c.connectionWraps {
		for _, conn := range connList {
			//fmt.Printf("[%s] lastTime=%s\n", conn.connId, conn.lastTime.Format("2006-01-02 15:04:05"))

			// 为了提高性能，90秒之内有过通信的，不需要发送心跳包
			if nowTime.Sub(conn.lastTime).Seconds() < 90 {
				continue
			}

			// 设置 PingHandler 和 PongHandler 来处理 WebSocket 的 Ping/Pong 帧
			// Gorilla WebSocket 库本身并没有直接提供一个方法来设置 Ping 的发送频率
			/*conn.conn.SetPingHandler(func(appData string) error {
				return nil
			})
			conn.conn.SetPongHandler(func(appData string) error {
				return nil
			})*/

			// 发送 Ping 帧
			err := conn.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			if err != nil {
				conn.Close()
			} else {
				conn.lastTime = nowTime
			}
		}
	}
}
