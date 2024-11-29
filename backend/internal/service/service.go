// internal/service/service.go

package service

import (
	"backend/internal/scheduler"
	"sync"
	"time"
)

var (
	schedulerService *scheduler.Scheduler
	monitorService   *MonitorService
	once             sync.Once
)

// InitServices 初始化所有服务
func InitServices() {
	once.Do(func() {
		schedulerService = scheduler.NewScheduler()
		schedulerService.SetLogging(false) // 关闭scheduler的日志

		monitorService = NewMonitorService(schedulerService)
		// 启动监控服务
		monitorService.StartRoomTempMonitor(10 * time.Second)
		monitorService.StartQueuesMonitor(5 * time.Second)
	})
}

// GetScheduler 获取调度器实例
func GetScheduler() *scheduler.Scheduler {
	return schedulerService
}

// GetMonitor 获取监控服务实例
func GetMonitor() *MonitorService {
	return monitorService
}

// StopServices 停止所有服务
func StopServices() {
	if monitorService != nil {
		monitorService.Stop()
	}
	if schedulerService != nil {
		schedulerService.Stop()
	}
}
