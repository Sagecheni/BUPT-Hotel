// internal/service/monitor.go

package service

import (
	"backend/internal/db"
	"backend/internal/logger"
	"time"
)

type MonitorService struct {
	scheduler    *Scheduler
	stopChan     chan struct{}
	tempTicker   *time.Ticker
	queuesTicker *time.Ticker
	roomRepo     *db.RoomRepository
}

func NewMonitorService(scheduler *Scheduler) *MonitorService {
	return &MonitorService{
		scheduler: scheduler,
		stopChan:  make(chan struct{}),
		roomRepo:  db.NewRoomRepository(),
	}
}

// 开始监控房间温度
func (s *MonitorService) StartRoomTempMonitor(interval time.Duration) {
	s.tempTicker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.tempTicker.C:
				s.logAllRoomStatus()
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

func (s *MonitorService) logAllRoomStatus() {
	// 获取所有房间信息
	rooms, err := s.roomRepo.GetAllRooms()
	if err != nil {
		logger.Error("获取房间信息失败: %v", err)
		return
	}

	// 获取服务队列和计费服务
	serviceQueue := s.scheduler.GetServiceQueue()
	billingService := GetBillingService()

	logger.Info("=== 所有房间状态 (时间: %s) ===", time.Now().Format("15:04:05"))

	for _, room := range rooms {
		// 获取账单信息
		var currentFee, totalFee float32 = 0, 0
		if billingService != nil {
			if bill, err := billingService.CalculateCurrentFee(room.RoomID); err == nil {
				currentFee = bill.CurrentFee
				totalFee = bill.TotalFee
			}
		}

		// 获取服务状态和当前风速
		status := "空闲"
		currentSpeed := "无"
		if room.State == 1 {
			if room.ACState == 1 {
				if service, exists := serviceQueue[room.RoomID]; exists {
					status = "服务中"
					currentSpeed = string(service.Speed)
				} else {
					status = "等待中"
					currentSpeed = room.CurrentSpeed // 使用房间记录的风速
				}
			} else {
				status = "已入住(空调关闭)"
			}
		}

		logger.Info("房间 %d [%s]:", room.RoomID, status)
		logger.Info("  - 温度: 当前 %.2f°C / 目标 %.2f°C / 初始 %.2f°C",
			room.CurrentTemp, room.TargetTemp, room.InitialTemp)
		if room.ACState == 1 {
			logger.Info("  - 空调: 模式 %s / 风速 %s", room.Mode, currentSpeed)
			logger.Info("  - 费用: 当前 %.2f元 / 累计 %.2f元", currentFee, totalFee)
		}
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
