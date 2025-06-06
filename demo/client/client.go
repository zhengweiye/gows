package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/zhengweiye/gows"
	"sync"
	"time"
)

func main() {
	url := "ws://127.0.0.1:8080/ws/test1?apiKey=test&userId=1"
	ctx := context.Background()
	wg := &sync.WaitGroup{}
	gows.ConnectServer(url, ctx, wg, MyHandler{})
	for {

	}
}

type MyHandler struct {
}

func (m MyHandler) Connected(conn *gows.ClientConnection) {
	fmt.Println("连接成功........................")
	conn.Send(gows.Data{
		MessageType: websocket.TextMessage,
		MessageData: []byte("hello world"),
	})
}

func (m MyHandler) Do(conn *gows.ClientConnection, data gows.Data) {
	fmt.Println("接受数据=", string(data.MessageData))
}

func (m MyHandler) Disconnected(conn *gows.ClientConnection) {
	fmt.Println("连接断开........................")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			err := conn.Reconnect()
			fmt.Println("重新连接：err=", err)
			// 如果连接成功，则退出
			if err == nil {
				return
			}
		}
	}
}
