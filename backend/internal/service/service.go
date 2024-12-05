// internal/service/service.go

package service

import (
	"sync"
	"time"
)

var (
	schedulerService *Scheduler
	monitorService   *MonitorService
	billingService   *BillingService
	once             sync.Once
)

// InitServices 初始化所有服务
func InitServices() {
	once.Do(func() {
		schedulerService = NewScheduler()
		schedulerService.SetLogging(true) // 关闭scheduler的日志
		billingService = NewBillingService(schedulerService)
		schedulerService.SetBillingService(billingService)
		monitorService = NewMonitorService(schedulerService)
		// 启动监控服务
		monitorService.StartRoomTempMonitor(1 * time.Second)
		//monitorService.StartQueuesMonitor(5 * time.Second)
	})
}

// GetScheduler 获取调度器实例
func GetScheduler() *Scheduler {
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
