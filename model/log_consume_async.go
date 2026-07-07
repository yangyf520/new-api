package model

import (
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

var (
	consumeLogCh     chan *Log
	consumeLogOnce   sync.Once
	consumeLogDrop   atomic.Uint64
	consumeLogClosed atomic.Bool
)

// InitConsumeLogWriter starts fixed workers draining a bounded consume-log queue.
func InitConsumeLogWriter() {
	if !common.LogConsumeEnabled || !common.LogConsumeAsyncEnabled {
		return
	}
	consumeLogOnce.Do(func() {
		size := common.LogConsumeQueueSize
		if size <= 0 {
			size = 10000
		}
		workers := common.LogConsumeWorkers
		if workers <= 0 {
			workers = 4
		}
		consumeLogCh = make(chan *Log, size)
		for i := 0; i < workers; i++ {
			go consumeLogWorker()
		}
		common.SysLog("consume log async writer started")
	})
}

func consumeLogWorker() {
	for log := range consumeLogCh {
		if log == nil {
			continue
		}
		if err := LOG_DB.Create(log).Error; err != nil {
			common.SysLog("failed to async record consume log: " + err.Error())
		}
	}
}

func enqueueConsumeLog(log *Log) bool {
	if log == nil {
		return true
	}
	InitConsumeLogWriter()
	if consumeLogCh == nil {
		return false
	}
	select {
	case consumeLogCh <- log:
		return true
	default:
		consumeLogDrop.Add(1)
		return false
	}
}

// ShutdownConsumeLogWriter drains pending consume logs (best-effort on process exit).
func ShutdownConsumeLogWriter() {
	if consumeLogCh == nil || consumeLogClosed.Swap(true) {
		return
	}
	close(consumeLogCh)
}

func writeConsumeLogSync(log *Log) {
	if log == nil {
		return
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record consume log: " + err.Error())
	}
}
