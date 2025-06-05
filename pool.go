package gows

import (
	"context"
	"fmt"
	"github.com/zhengweiye/gopool"
	"sync"
)

var poolMap map[string]*gopool.Pool
var poolLock sync.Mutex

func init() {
	poolMap = make(map[string]*gopool.Pool)
}

func newPool(clientType string, ctx context.Context, wg *sync.WaitGroup, queueSize, workSize int) *gopool.Pool {
	poolLock.Lock()
	defer poolLock.Unlock()

	pool, ok := poolMap[clientType]
	if ok && pool != nil {
		return pool
	}

	pool = gopool.NewPool(queueSize, workSize, ctx, wg)
	poolMap[clientType] = pool

	fmt.Printf("[gows] [%s] pool pointer addr is [%p]\n", clientType, pool)
	return pool
}
