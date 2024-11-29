// internal/service/service.go

package service

import (
	"backend/internal/scheduler"
	"sync"
)

var (
	schedulerService *scheduler.Scheduler
	once             sync.Once
)

// InitServices 初始化所有服务
func InitServices() {
	once.Do(func() {
		schedulerService = scheduler.NewScheduler()
	})
}

// GetScheduler 获取调度器实例
func GetScheduler() *scheduler.Scheduler {
	return schedulerService
}

// StopServices 停止所有服务
func StopServices() {
	if schedulerService != nil {
		schedulerService.Stop()
	}
}
