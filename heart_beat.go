package gows

import (
	"sync"
)

var heartBeatMap map[string]func()
var heartBeatLock sync.Mutex

func init() {
	heartBeatMap = make(map[string]func())
}

func newHeartBeat(clientType string, f func()) {
	heartBeatLock.Lock()
	defer heartBeatLock.Unlock()

	_, ok := heartBeatMap[clientType]
	if ok {
		return
	}
	heartBeatMap[clientType] = f
	f()
}
