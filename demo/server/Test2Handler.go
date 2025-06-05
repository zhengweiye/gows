package main

import (
	"context"
	"fmt"
	"github.com/zhengweiye/gows"
	"net/http"
	"sync"
)

type Test2Handler struct {
	ctx context.Context
	wg  *sync.WaitGroup
}

func NewTest2Handler(ctx context.Context, wg *sync.WaitGroup) Test2Handler {
	return Test2Handler{
		ctx: ctx,
		wg:  wg,
	}
}

func (w Test2Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	gows.AcceptClient("test2",
		w.ctx,
		w.wg,
		response,
		request,
		gows.HandlerOption(NewCustom2Handler()),
		gows.UrlParamOption([]string{"apiKey"}),
		gows.HeartBeatOption(30),
	)
}

type Custom2Handler struct {
}

func NewCustom2Handler() Custom2Handler {
	return Custom2Handler{}
}

func (c Custom2Handler) Connected(conn *gows.ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Connected() param=%v\n", urlParameterMap)
}

func (c Custom2Handler) Do(conn *gows.ConnectionWrap, data gows.Data, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Do() param=%v, content=%s\n", urlParameterMap, string(data.MessageData))

	//time.Sleep(15 * time.Second)

	conn.Send(gows.Data{
		MessageType: data.MessageType,
		MessageData: []byte(fmt.Sprintf("响应：%s", string(data.MessageData))),
	})
}

func (c Custom2Handler) Disconnected(conn *gows.ConnectionWrap, urlParameterMap map[string]string, headerMap map[string]string) {
	fmt.Printf("触发Disconnected()：param=%v\n", urlParameterMap)
}
