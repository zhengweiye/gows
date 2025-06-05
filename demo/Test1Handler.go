package main

import (
	"context"
	"fmt"
	"github.com/zhengweiye/gows"
	"net/http"
	"sync"
	"time"
)

type Test1Handler struct {
	ctx context.Context
	wg  *sync.WaitGroup
}

func NewTest1Handler(ctx context.Context, wg *sync.WaitGroup) Test1Handler {
	return Test1Handler{
		ctx: ctx,
		wg:  wg,
	}
}

func (w Test1Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	gows.AcceptClient("test1",
		w.ctx,
		w.wg,
		response,
		request,
		gows.HandlerOption(NewCustom1Handler()),
		gows.UrlParamOption([]string{"apiKey", "userId"}),
		gows.HeaderOption([]string{"Authorization"}),
		gows.HeartBeatOption(30),
	)
}

type Custom1Handler struct {
}

func NewCustom1Handler() Custom1Handler {
	return Custom1Handler{}
}

func (c Custom1Handler) Connected(clientType string, conn *gows.ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Connected()：clientType=%s, param=%v, header=%v\n", clientType, urlParameterMap, headerMap)
	conn.SetGroupId(urlParameterMap["userId"])

	for key, value := range conn.GetConnectionManager().GetConnections() {
		fmt.Println("key=", key, "=================================================")
		for _, item := range value {
			fmt.Println(item.GetId())
		}
	}
}

func (c Custom1Handler) Do(clientType string, conn *gows.ConnectionWrap, data gows.Data, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Do()：clientType=%s, param=%v, content=%s\n", clientType, urlParameterMap, string(data.MessageData))

	time.Sleep(10 * time.Second)

	conn.Send(gows.Data{
		MessageType: data.MessageType,
		MessageData: []byte(fmt.Sprintf("响应：%s", string(data.MessageData))),
	})
}

func (c Custom1Handler) Disconnected(clientType string, conn *gows.ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Disconnected()：clientType=%s, param=%v\n", clientType, urlParameterMap)
}
