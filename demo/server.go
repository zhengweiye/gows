package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var shutdownSignal chan os.Signal

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}

	// 路由
	http.Handle("/ws/test1", NewTest1Handler(ctx, &wg))
	http.Handle("/ws/test2", NewTest2Handler(ctx, &wg))

	// 启动服务
	//http.ListenAndServe(":6666", nil)
	httpServer := &http.Server{
		Addr: ":8080",
	}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				// 处理监听失败的错误
			}
		}
	}()

	// 信号量创建
	shutdownSignal = make(chan os.Signal)

	// 等待中断信号，以优雅地关闭服务器
	//TODO 无法监听kill -9 ${pid} ==> 触发的SIGKILL信号无法发送到对应进程，因为进程已经被系统强制退出
	signal.Notify(shutdownSignal,
		syscall.SIGHUP, // 挂起信号：通常由终端关闭触发
		syscall.SIGINT, // 中断信号：通常由 Ctrl+C 触发
		syscall.SIGQUIT,
		syscall.SIGILL,
		syscall.SIGTRAP,
		syscall.SIGABRT,
		syscall.SIGBUS,
		syscall.SIGFPE,
		syscall.SIGKILL, // 强制终止信号：不可捕获，通常由 kill -9 触发
		syscall.SIGSEGV,
		syscall.SIGPIPE,
		syscall.SIGALRM,
		syscall.SIGTERM, // 终止信号：通常由 kill 命令触发  | docker重启项目会触发该信号
	)
	<-shutdownSignal

	stop(ctx, cancel, &wg, httpServer)
}

func stop(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, httpServer *http.Server) {
	// 停止监听信号量
	signal.Stop(shutdownSignal)

	// ctx.Done()触发，等connect里面的list()执行完成
	cancel()

	// 等所有的connect都执行完成
	wg.Wait()

	fmt.Println("所有的conn都执行完成, 关闭net/http服务器...")

	// 关闭服务器：阻止新的连接进入并等待活跃连接处理完成后再终止程序，达到优雅退出的目的
	err := httpServer.Shutdown(ctx)
	if err != nil {
		panic(err)
	}
}
