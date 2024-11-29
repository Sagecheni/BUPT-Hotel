// internal/service/monitor.go

package service

import (
	"backend/internal/logger"
	"backend/internal/scheduler"
	"time"
)

type MonitorService struct {
	scheduler    *scheduler.Scheduler
	stopChan     chan struct{}
	tempTicker   *time.Ticker
	queuesTicker *time.Ticker
}

func NewMonitorService(scheduler *scheduler.Scheduler) *MonitorService {
	return &MonitorService{
		scheduler: scheduler,
		stopChan:  make(chan struct{}),
	}
}

// 开始监控房间温度
func (s *MonitorService) StartRoomTempMonitor(interval time.Duration) {
	s.tempTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.tempTicker.C:
				s.logRoomTemperatures()
			case <-s.stopChan:
				return
			}
		}
	}()
}

// 开始监控调度队列
func (s *MonitorService) StartQueuesMonitor(interval time.Duration) {
	s.queuesTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.queuesTicker.C:
				s.logSchedulerQueues()
			case <-s.stopChan:
				return
			}
		}
	}()
}

// 记录房间温度信息
func (s *MonitorService) logRoomTemperatures() {
	serviceQueue := s.scheduler.GetServiceQueue()

	logger.Info("=== 房间温度状态 (时间: %s) ===", time.Now().Format("15:04:05"))
	if len(serviceQueue) == 0 {
		logger.Info("当前没有房间在使用空调")
		return
	}

	for roomID, service := range serviceQueue {
		logger.Info("房间 %d: 当前温度 %.1f°C, 目标温度 %.1f°C, 风速: %s",
			roomID,
			service.CurrentTemp,
			service.TargetTemp,
			service.Speed,
		)
	}
	logger.Info("=============================")
}

// 记录调度队列信息
func (s *MonitorService) logSchedulerQueues() {
	serviceQueue := s.scheduler.GetServiceQueue()
	waitQueue := s.scheduler.GetWaitQueue()

	logger.Info("=== 调度队列状态 (时间: %s) ===", time.Now().Format("15:04:05"))

	// 打印服务队列信息
	logger.Info("--- 服务队列 (共 %d 个房间) ---", len(serviceQueue))
	if len(serviceQueue) == 0 {
		logger.Info("服务队列为空")
	} else {
		for roomID, service := range serviceQueue {
			logger.Info("房间 %d: 温度 %.1f°C -> %.1f°C, 风速: %s, 已服务时长: %.1f秒",
				roomID,
				service.CurrentTemp,
				service.TargetTemp,
				service.Speed,
				service.Duration,
			)
		}
	}

	// 打印等待队列信息
	logger.Info("--- 等待队列 (共 %d 个房间) ---", len(waitQueue))
	if len(waitQueue) == 0 {
		logger.Info("等待队列为空")
	} else {
		for _, wait := range waitQueue {
			logger.Info("房间 %d: 温度 %.1f°C -> %.1f°C, 风速: %s, 剩余等待时间: %.1f秒",
				wait.RoomID,
				wait.CurrentTemp,
				wait.TargetTemp,
				wait.Speed,
				wait.WaitDuration,
			)
		}
	}
	logger.Info("=============================")
}

// 停止监控
func (s *MonitorService) Stop() {
	if s.tempTicker != nil {
		s.tempTicker.Stop()
	}
	if s.queuesTicker != nil {
		s.queuesTicker.Stop()
	}
	close(s.stopChan)
}
