package scheduler

import (
	"backend/internal/db"
	"backend/internal/events"
	"backend/internal/logger"
	"fmt"
	"math"
	"sync"
	"time"
)

type Scheduler struct {
	mu          sync.RWMutex
	queueMgr    *QueueManager
	strategy    *CompositeStrategy
	eventBus    *events.EventBus
	roomRepo    db.IRoomRepository
	serviceRepo db.ServiceRepositoryInterface
	config      *Config
	stopChan    chan struct{}
}

type Config struct {
	MaxServices    int
	BaseWaitTime   float32
	DefaultSpeed   string
	DefaultTemp    float32
	TempThreshold  float32
	ServiceTimeout float32
}

func NewScheduler(eventBus *events.EventBus, roomRepo db.IRoomRepository, config *Config, serviceRepo db.ServiceRepositoryInterface) *Scheduler {
	if config == nil {
		config = &Config{
			MaxServices:    3,
			BaseWaitTime:   20,
			DefaultSpeed:   SpeedMedium,
			DefaultTemp:    25.0,
			TempThreshold:  0.1,
			ServiceTimeout: 300, // 5分钟超时
		}
	}
	s := &Scheduler{
		queueMgr:    NewQueueManager(eventBus),
		strategy:    NewCompositeStrategy(),
		eventBus:    eventBus,
		roomRepo:    roomRepo,
		serviceRepo: serviceRepo,
		config:      config,
		stopChan:    make(chan struct{}),
	}

	// 订阅相关事件
	eventBus.Subscribe(events.EventServiceRequest, s.handleServiceRequest)
	eventBus.Subscribe(events.EventTemperatureChange, s.handleTemperatureChange)
	eventBus.Subscribe(events.EventSpeedChange, s.handleSpeedChange)
	eventBus.Subscribe(events.EventServiceComplete, s.handleServiceComplete)

	// 启动监控协程
	go s.monitorQueues()

	return s
}

func (s *Scheduler) handleServiceRequest(e events.Event) {
	// 解析服务请求数据
	eventReq := e.Data.(events.ServiceRequest)
	req := ServiceRequest{
		RoomID:      eventReq.RoomID,
		Speed:       eventReq.Speed,
		TargetTemp:  eventReq.TargetTemp,
		CurrentTemp: eventReq.CurrentTemp,
		RequestTime: eventReq.RequestTime,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 检查房间当前状态
	_, err := s.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		s.eventBus.Publish(events.Event{
			Type:      events.EventServiceComplete,
			RoomID:    req.RoomID,
			Timestamp: time.Now(),
			Data: events.ServiceEventData{
				RoomID: req.RoomID,
				Reason: "room_not_found",
			},
		})
		return
	}

	// 2. 检查房间是否已在服务队列
	if s.queueMgr.IsInService(req.RoomID) {
		// 更新现有服务的参数
		if service := s.queueMgr.GetServiceItem(req.RoomID); service != nil {
			// 更新服务参数（温度、风速等）
			service.Speed = req.Speed
			service.TargetTemp = req.TargetTemp
			//更新数据库中的服务记录
			activeService, err := s.serviceRepo.GetActiveServiceDetail(req.RoomID)
			if err != nil {
				logger.Error("Failed to get active service detail: %v", err)
				return
			}
			if activeService != nil {
				activeService.TargetTemp = req.TargetTemp
				activeService.Speed = req.Speed
				if err := s.serviceRepo.UpdateServiceDetail(activeService); err != nil {
					logger.Error("Failed to update service detail: %v", err)
					return
				}
			}
			// 更新房间状态
			if err := s.roomRepo.UpdateSpeed(req.RoomID, req.Speed); err != nil {
				logger.Error("Failed to update room speed: %v", err)
			}
			// 发布服务更新事件
			s.eventBus.Publish(events.Event{
				Type:      events.EventServiceStart,
				RoomID:    req.RoomID,
				Timestamp: time.Now(),
				Data: events.ServiceEventData{
					RoomID:      req.RoomID,
					Speed:       req.Speed,
					TargetTemp:  req.TargetTemp,
					CurrentTemp: req.CurrentTemp,
					StartTime:   time.Now(),
					Reason:      "service_updated",
				},
			})
		}
		return
	}

	// 3. 检查房间是否在等待队列
	if s.queueMgr.IsWaiting(req.RoomID) {
		waitItem := s.queueMgr.GetWaitItem(req.RoomID)
		shouldReschedule := s.strategy.ShouldPreempt(&req, &ServiceItem{
			RoomID:      waitItem.RoomID,
			Speed:       waitItem.Speed,
			TargetTemp:  waitItem.TargetTemp,
			CurrentTemp: waitItem.CurrentTemp,
		})
		if shouldReschedule {
			s.queueMgr.RemoveFromWaitQueue(req.RoomID)
			if err := s.serviceRepo.RemoveFromQueue(req.RoomID); err != nil {
				logger.Error("Failed to remove from queue: %v", err)
			}
		} else {
			return
		}

	}

	// 4. 尝试直接加入服务队列
	if s.queueMgr.GetServiceCount() < s.config.MaxServices {
		// 直接加入服务队列
		serviceDetail := &db.ServiceDetail{
			RoomID:      req.RoomID,
			StartTime:   time.Now(),
			InitialTemp: req.CurrentTemp,
			TargetTemp:  req.TargetTemp,
			Speed:       req.Speed,
		}
		if err := s.serviceRepo.CreateServiceDetail(serviceDetail); err != nil {
			logger.Error("Failed to create service detail: %v", err)
			return
		}
		// 添加到服务队列
		if err := s.serviceRepo.AddToServiceQueue(
			req.RoomID,
			req.Speed,
			req.TargetTemp,
			req.CurrentTemp,
		); err != nil {
			logger.Error("Failed to add to service queue: %v", err)
			return
		}

		// 更新内存队列
		s.queueMgr.AddToServiceQueue(&ServiceItem{
			RoomID:      req.RoomID,
			StartTime:   time.Now(),
			Speed:       req.Speed,
			TargetTemp:  req.TargetTemp,
			CurrentTemp: req.CurrentTemp,
		})

		// 更新房间状态
		if err := s.roomRepo.UpdateSpeed(req.RoomID, req.Speed); err != nil {
			logger.Error("Failed to update room speed: %v", err)
		}
		return
	}

	// 5. 执行调度策略
	needSchedule, victimID := s.strategy.Schedule(&req, s.queueMgr)
	if needSchedule && victimID > 0 {
		// 将受害者移到等待队列
		if victim := s.queueMgr.RemoveFromServiceQueue(victimID); victim != nil {
			// 更新被抢占服务的状态
			if err := s.serviceRepo.PreemptServiceDetail(victimID, req.RoomID); err != nil {
				logger.Error("Failed to preempt service: %v", err)
			}
			// 将被抢占服务加入等待队列
			if err := s.serviceRepo.AddToWaitQueue(
				victim.RoomID,
				victim.Speed,
				victim.TargetTemp,
				victim.CurrentTemp,
				SpeedPriorityMap[victim.Speed],
			); err != nil {
				logger.Error("Failed to add to wait queue: %v", err)
			}

			s.queueMgr.AddToWaitQueue(&WaitItem{
				RoomID:       victim.RoomID,
				RequestTime:  time.Now(),
				Speed:        victim.Speed,
				TargetTemp:   victim.TargetTemp,
				CurrentTemp:  victim.CurrentTemp,
				Priority:     SpeedPriorityMap[victim.Speed],
				WaitDuration: s.strategy.CalculateWaitTime(s.queueMgr.GetWaitQueueLength()),
			})

			// 创建新服务的记录
			serviceDetail := &db.ServiceDetail{
				RoomID:       req.RoomID,
				StartTime:    time.Now(),
				InitialTemp:  req.CurrentTemp,
				TargetTemp:   req.TargetTemp,
				Speed:        req.Speed,
				ServiceState: "active",
			}
			if err := s.serviceRepo.CreateServiceDetail(serviceDetail); err != nil {
				logger.Error("Failed to create service detail: %v", err)
			}
			// 添加新服务到队列
			if err := s.serviceRepo.AddToServiceQueue(
				req.RoomID,
				req.Speed,
				req.TargetTemp,
				req.CurrentTemp,
			); err != nil {
				logger.Error("Failed to add to service queue: %v", err)
			}

			s.queueMgr.AddToServiceQueue(&ServiceItem{
				RoomID:      req.RoomID,
				StartTime:   time.Now(),
				Speed:       req.Speed,
				TargetTemp:  req.TargetTemp,
				CurrentTemp: req.CurrentTemp,
			})
		}
	} else {
		// 加入等待队列
		if err := s.serviceRepo.AddToWaitQueue(
			req.RoomID,
			req.Speed,
			req.TargetTemp,
			req.CurrentTemp,
			SpeedPriorityMap[req.Speed],
		); err != nil {
			logger.Error("Failed to add to wait queue: %v", err)
		}

		s.queueMgr.AddToWaitQueue(&WaitItem{
			RoomID:       req.RoomID,
			RequestTime:  time.Now(),
			Speed:        req.Speed,
			TargetTemp:   req.TargetTemp,
			CurrentTemp:  req.CurrentTemp,
			Priority:     SpeedPriorityMap[req.Speed],
			WaitDuration: s.strategy.CalculateWaitTime(s.queueMgr.GetWaitQueueLength()),
		})
	}

}

// handleTemperatureChange 处理温度变化事件
func (s *Scheduler) handleTemperatureChange(e events.Event) {
	data := e.Data.(events.TemperatureEventData)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 检查房间是否在服务队列中
	if !s.queueMgr.IsInService(data.RoomID) {
		return
	}

	serviceItem := s.queueMgr.GetServiceItem(data.RoomID)
	if serviceItem == nil {
		return
	}
	// 获取当前活动的服务记录
	activeService, err := s.serviceRepo.GetActiveServiceDetail(data.RoomID)
	if err != nil {
		logger.Error("Failed to get active service: %v", err)
		return
	}

	// 2. 计算温度变化
	tempDiff := math.Abs(float64(serviceItem.TargetTemp - data.CurrentTemp))

	// 3. 检查是否达到目标温度
	if tempDiff <= float64(s.config.TempThreshold) {
		// 达到目标温度
		s.eventBus.Publish(events.Event{
			Type:      events.EventTargetTempReached,
			RoomID:    data.RoomID,
			Timestamp: time.Now(),
			Data: events.TemperatureEventData{
				RoomID:      data.RoomID,
				CurrentTemp: data.CurrentTemp,
				TargetTemp:  serviceItem.TargetTemp,
				Speed:       serviceItem.Speed,
			},
		})

		// 完成服务记录
		if err := s.serviceRepo.CompleteServiceDetail(data.RoomID, data.CurrentTemp); err != nil {
			logger.Error("Failed to complete service: %v", err)
		}

		// 从服务队列移除
		s.queueMgr.RemoveFromServiceQueue(data.RoomID)
		if err := s.serviceRepo.RemoveFromQueue(data.RoomID); err != nil {
			logger.Error("Failed to remove from queue: %v", err)
		}

		// 服务完成，从服务队列移除
		s.queueMgr.RemoveFromServiceQueue(data.RoomID)

		// 处理等待队列
		if s.queueMgr.GetWaitQueueLength() > 0 {
			if nextItem := s.strategy.GetNextFromWaitQueue(s.queueMgr); nextItem != nil {
				// 创建新的服务记录
				newService := &db.ServiceDetail{
					RoomID:      nextItem.RoomID,
					StartTime:   time.Now(),
					InitialTemp: nextItem.CurrentTemp,
					TargetTemp:  nextItem.TargetTemp,
					Speed:       nextItem.Speed,
				}
				if err := s.serviceRepo.CreateServiceDetail(newService); err != nil {
					logger.Error("Failed to create service detail: %v", err)
				}

				// 从等待队列移动到服务队列
				if err := s.serviceRepo.RemoveFromQueue(nextItem.RoomID); err != nil {
					logger.Error("Failed to remove from wait queue: %v", err)
				}
				if err := s.serviceRepo.AddToServiceQueue(
					nextItem.RoomID,
					nextItem.Speed,
					nextItem.TargetTemp,
					nextItem.CurrentTemp,
				); err != nil {
					logger.Error("Failed to add to service queue: %v", err)
				}

				s.queueMgr.AddToServiceQueue(&ServiceItem{
					RoomID:      nextItem.RoomID,
					StartTime:   time.Now(),
					Speed:       nextItem.Speed,
					TargetTemp:  nextItem.TargetTemp,
					CurrentTemp: nextItem.CurrentTemp,
				})
				s.queueMgr.RemoveFromWaitQueue(nextItem.RoomID)
			}
		}
	} else {
		// 更新当前温度
		s.queueMgr.UpdateServiceItem(data.RoomID, func(item *ServiceItem) {
			item.CurrentTemp = data.CurrentTemp
		})
		if err := s.serviceRepo.UpdateQueueItemTemp(data.RoomID, data.CurrentTemp, serviceItem.TargetTemp); err != nil {
			logger.Error("Failed to update queue item temperature: %v", err)
		}
		if activeService != nil {
			activeService.FinalTemp = data.CurrentTemp
			if err := s.serviceRepo.UpdateServiceDetail(activeService); err != nil {
				logger.Error("Failed to update service detail: %v", err)
			}
		}
	}
}

// handleSpeedChange 处理风速变化事件
func (s *Scheduler) handleSpeedChange(e events.Event) {
	data := e.Data.(events.Event)
	speedData := struct {
		RoomID      int
		Speed       string
		CurrentTemp float32
	}{
		RoomID:      data.RoomID,
		Speed:       data.Data.(map[string]interface{})["speed"].(string),
		CurrentTemp: data.Data.(map[string]interface{})["current_temp"].(float32),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 检查房间是否在服务队列中
	if s.queueMgr.IsInService(speedData.RoomID) {
		// 获取当前服务详情
		activeService, err := s.serviceRepo.GetActiveServiceDetail(speedData.RoomID)
		if err != nil {
			logger.Error("Failed to get active service: %v", err)
			return
		}

		oldSpeed := ""
		if activeService != nil {
			oldSpeed = activeService.Speed
			// 完成当前服务记录
			if err := s.serviceRepo.CompleteServiceDetail(speedData.RoomID, speedData.CurrentTemp); err != nil {
				logger.Error("Failed to complete service detail: %v", err)
			}

			// 创建新的服务记录
			newService := &db.ServiceDetail{
				RoomID:      speedData.RoomID,
				StartTime:   time.Now(),
				InitialTemp: speedData.CurrentTemp,
				TargetTemp:  activeService.TargetTemp,
				Speed:       speedData.Speed,
			}
			if err := s.serviceRepo.CreateServiceDetail(newService); err != nil {
				logger.Error("Failed to create new service detail: %v", err)
			}
		}

		// 更新服务队列
		if err := s.serviceRepo.UpdateQueueItemSpeed(speedData.RoomID, speedData.Speed); err != nil {
			logger.Error("Failed to update queue item speed: %v", err)
		}

		// 更新内存中的队列状态
		s.queueMgr.UpdateServiceItem(speedData.RoomID, func(item *ServiceItem) {
			item.Speed = speedData.Speed
		})

		// 发布风速变化事件
		s.eventBus.Publish(events.Event{
			Type:      events.EventSpeedChange,
			RoomID:    speedData.RoomID,
			Timestamp: time.Now(),
			Data: struct {
				OldSpeed    string
				NewSpeed    string
				TargetTemp  float32
				CurrentTemp float32
			}{
				OldSpeed:    oldSpeed,
				NewSpeed:    speedData.Speed,
				TargetTemp:  activeService.TargetTemp,
				CurrentTemp: speedData.CurrentTemp,
			},
		})
		return
	}

	// 2. 检查房间是否在等待队列中
	if s.queueMgr.IsWaiting(speedData.RoomID) {
		waitItem := s.queueMgr.GetWaitItem(speedData.RoomID)
		if waitItem == nil {
			return
		}

		// 获取新的优先级
		newPriority := SpeedPriorityMap[speedData.Speed]
		oldPriority := SpeedPriorityMap[waitItem.Speed]

		// 如果新的优先级更高，尝试抢占
		if newPriority > oldPriority {
			// 创建新的服务请求
			req := &ServiceRequest{
				RoomID:      speedData.RoomID,
				Speed:       speedData.Speed,
				TargetTemp:  waitItem.TargetTemp,
				CurrentTemp: speedData.CurrentTemp,
				RequestTime: time.Now(),
			}

			// 尝试执行调度
			needSchedule, victimID := s.strategy.Schedule(req, s.queueMgr)
			if needSchedule && victimID > 0 {
				// 发布等待队列移除事件
				s.eventBus.Publish(events.Event{
					Type:      events.EventRemoveFromWaitQueue,
					RoomID:    speedData.RoomID,
					Timestamp: time.Now(),
					Data: events.WaitQueueEventData{
						RoomID: speedData.RoomID,
					},
				})

				// 处理被抢占的服务
				if victim := s.queueMgr.RemoveFromServiceQueue(victimID); victim != nil {
					// 更新被抢占服务的状态
					if err := s.serviceRepo.PreemptServiceDetail(victimID, speedData.RoomID); err != nil {
						logger.Error("Failed to preempt service: %v", err)
					}

					// 将被抢占服务加入等待队列
					if err := s.serviceRepo.AddToWaitQueue(
						victim.RoomID,
						victim.Speed,
						victim.TargetTemp,
						victim.CurrentTemp,
						SpeedPriorityMap[victim.Speed],
					); err != nil {
						logger.Error("Failed to add to wait queue: %v", err)
					}

					// 发布服务抢占事件
					s.eventBus.Publish(events.Event{
						Type:      events.EventServicePreempted,
						RoomID:    victimID,
						Timestamp: time.Now(),
						Data: events.ServiceEventData{
							RoomID:  victimID,
							EndTime: time.Now(),
							Reason:  "preempted_by_speed_change",
						},
					})
				}

				// 创建新的服务记录
				serviceDetail := &db.ServiceDetail{
					RoomID:      speedData.RoomID,
					StartTime:   time.Now(),
					InitialTemp: speedData.CurrentTemp,
					TargetTemp:  waitItem.TargetTemp,
					Speed:       speedData.Speed,
				}
				if err := s.serviceRepo.CreateServiceDetail(serviceDetail); err != nil {
					logger.Error("Failed to create service detail: %v", err)
				}

				// 发布服务开始事件
				s.eventBus.Publish(events.Event{
					Type:      events.EventServiceStart,
					RoomID:    speedData.RoomID,
					Timestamp: time.Now(),
					Data: events.ServiceEventData{
						RoomID:      speedData.RoomID,
						StartTime:   time.Now(),
						Speed:       speedData.Speed,
						TargetTemp:  waitItem.TargetTemp,
						CurrentTemp: speedData.CurrentTemp,
					},
				})
			} else {
				// 仅更新等待队列中的风速
				if err := s.serviceRepo.UpdateQueueItemSpeed(speedData.RoomID, speedData.Speed); err != nil {
					logger.Error("Failed to update queue item speed: %v", err)
				}

				s.queueMgr.UpdateWaitItem(speedData.RoomID, func(item *WaitItem) {
					item.Speed = speedData.Speed
					item.Priority = newPriority
				})

				// 发布等待队列更新事件
				s.eventBus.Publish(events.Event{
					Type:      events.EventQueueStatusChange,
					RoomID:    speedData.RoomID,
					Timestamp: time.Now(),
					Data: events.WaitQueueEventData{
						RoomID:   speedData.RoomID,
						Speed:    speedData.Speed,
						Priority: newPriority,
					},
				})
			}
		}
	}
}

func (s *Scheduler) monitorQueues() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.updateQueueStatus()
			s.updateTemperatures()
			s.checkTimeouts()
		case <-s.stopChan:
			return
		}
	}
}

// updateQueueStatus 更新队列状态并发布监控事件
func (s *Scheduler) updateQueueStatus() {
	s.mu.Lock()
	defer s.mu.Unlock()

	metrics := s.queueMgr.GetQueueMetrics()

	// 发布状态更新事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventQueueStatusChange,
		Timestamp: time.Now(),
		Data: events.SchedulerStatusData{
			Timestamp:    time.Now(),
			ServiceCount: metrics.ServiceCount,
			WaitingCount: metrics.WaitingCount,
			ServiceQueue: s.getServiceQueueSnapshot(),
			WaitQueue:    s.getWaitQueueSnapshot(),
		},
	})

	// 发布性能指标事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventMetricsUpdate,
		Timestamp: time.Now(),
		Data: events.MetricsEventData{
			Timestamp: time.Now(),

			ActiveACs:          metrics.ServiceCount,
			AvgServiceTime:     metrics.AvgServiceTime,
			AvgWaitTime:        metrics.AvgWaitTime,
			ServiceQueueLength: metrics.ServiceCount,
			WaitQueueLength:    metrics.WaitingCount,
		},
	})
}

// updateTemperatures 更新所有服务中房间的温度
func (s *Scheduler) updateTemperatures() {
	s.mu.Lock()
	defer s.mu.Unlock()

	serviceQueue := s.queueMgr.GetServiceQueue()
	for roomID, service := range serviceQueue {
		// 计算温度变化
		tempRate := SpeedTempRateMap[service.Speed]
		tempDiff := service.TargetTemp - service.CurrentTemp

		if math.Abs(float64(tempDiff)) > float64(s.config.TempThreshold) {
			// 更新温度
			var newTemp float32
			if tempDiff > 0 {
				newTemp = service.CurrentTemp + tempRate
			} else {
				newTemp = service.CurrentTemp - tempRate
			}

			// 发布温度变化事件
			s.eventBus.Publish(events.Event{
				Type:      events.EventTemperatureChange,
				RoomID:    roomID,
				Timestamp: time.Now(),
				Data: events.TemperatureEventData{
					RoomID:          roomID,
					PreviousTemp:    service.CurrentTemp,
					CurrentTemp:     newTemp,
					TargetTemp:      service.TargetTemp,
					Speed:           service.Speed,
					ChangeRate:      tempRate,
					TimeSinceUpdate: float32(time.Since(service.StartTime).Seconds()),
				},
			})
		}
	}
}

// checkTimeouts 检查服务超时和等待超时
func (s *Scheduler) checkTimeouts() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 检查服务队列超时
	serviceQueue := s.queueMgr.GetServiceQueue()
	for roomID, service := range serviceQueue {
		duration := now.Sub(service.StartTime).Seconds()
		if duration >= float64(s.config.ServiceTimeout) {
			// 发布服务超时事件
			s.eventBus.Publish(events.Event{
				Type:      events.EventServiceComplete,
				RoomID:    roomID,
				Timestamp: now,
				Data: events.ServiceEventData{
					RoomID:    roomID,
					StartTime: service.StartTime,
					EndTime:   now,
					Duration:  float32(duration),
					Reason:    "service_timeout",
				},
			})

			// 将服务移到等待队列
			if s.queueMgr.GetWaitQueueLength() > 0 {
				s.queueMgr.RemoveFromServiceQueue(roomID)
				s.moveNextToService()
			}
		}
	}

	// 检查等待队列超时
	waitQueue := s.queueMgr.GetWaitQueue()
	for _, waitItem := range waitQueue {
		if waitItem.WaitDuration <= 0 {
			// 重新计算等待时间
			waitItem.WaitDuration = s.strategy.CalculateWaitTime(len(waitQueue))

			// 发布等待更新事件
			s.eventBus.Publish(events.Event{
				Type:      events.EventQueueStatusChange,
				RoomID:    waitItem.RoomID,
				Timestamp: now,
				Data: struct {
					NewWaitDuration float32
					QueuePosition   int
				}{
					NewWaitDuration: waitItem.WaitDuration,
					QueuePosition:   len(waitQueue),
				},
			})
		}
	}
}

// 辅助方法: 获取服务队列快照
func (s *Scheduler) getServiceQueueSnapshot() map[string]interface{} {
	snapshot := make(map[string]interface{})
	serviceQueue := s.queueMgr.GetServiceQueue()

	for roomID, service := range serviceQueue {
		snapshot[fmt.Sprintf("%d", roomID)] = map[string]interface{}{
			"start_time":   service.StartTime,
			"duration":     time.Since(service.StartTime).Seconds(),
			"speed":        service.Speed,
			"target_temp":  service.TargetTemp,
			"current_temp": service.CurrentTemp,
			"is_completed": service.IsCompleted,
		}
	}
	return snapshot
}

// 辅助方法: 获取等待队列快照
func (s *Scheduler) getWaitQueueSnapshot() []interface{} {
	var snapshot []interface{}
	waitQueue := s.queueMgr.GetWaitQueue()

	for _, item := range waitQueue {
		snapshot = append(snapshot, map[string]interface{}{
			"room_id":       item.RoomID,
			"request_time":  item.RequestTime,
			"speed":         item.Speed,
			"wait_duration": item.WaitDuration,
			"target_temp":   item.TargetTemp,
			"current_temp":  item.CurrentTemp,
			"priority":      item.Priority,
		})
	}
	return snapshot
}

// 辅助方法: 将下一个等待项移到服务队列
func (s *Scheduler) moveNextToService() {
	if next := s.strategy.GetNextFromWaitQueue(s.queueMgr); next != nil {
		newService := &ServiceItem{
			RoomID:      next.RoomID,
			StartTime:   time.Now(),
			Speed:       next.Speed,
			TargetTemp:  next.TargetTemp,
			CurrentTemp: next.CurrentTemp,
		}

		s.queueMgr.AddToServiceQueue(newService)
		s.queueMgr.RemoveFromWaitQueue(next.RoomID)
	}
}

func (s *Scheduler) handleServiceComplete(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 从服务队列中移除
	s.queueMgr.RemoveFromServiceQueue(e.RoomID)

	// 从数据库队列中移除
	if err := s.serviceRepo.RemoveFromQueue(e.RoomID); err != nil {
		logger.Error("Failed to remove from queue: %v", err)
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.stopChan)
}
