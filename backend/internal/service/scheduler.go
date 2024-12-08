// internal/service/scheduler.go

package service

import (
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/types"
	"container/heap"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	MaxServices = 3  // 最大服务对象
	WaitTime    = 20 // 时间片
)

// ServiceObject 服务对象
type ServiceObject struct {
	RoomID      int
	StartTime   time.Time // 当前服务开始时间
	PowerOnTime time.Time // 开机时间，用于费用计算
	Speed       types.Speed
	Duration    float32
	TargetTemp  float32
	CurrentTemp float32
	IsCompleted bool
}

// WaitObject 等待对象
type WaitObject struct {
	RoomID       int
	RequestTime  time.Time
	Speed        types.Speed
	WaitDuration float32
	TargetTemp   float32
	CurrentTemp  float32
}

// PriorityItem 优先级队列项
type PriorityItem struct {
	roomID    int
	priority  int
	waitObj   *WaitObject
	indexHeap int
}

// PriorityQueue 实现
type PriorityQueue []*PriorityItem

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority > pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].indexHeap = i
	pq[j].indexHeap = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PriorityItem)
	item.indexHeap = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.indexHeap = -1
	*pq = old[0 : n-1]
	return item
}

// Scheduler 调度器
type Scheduler struct {
	mu               sync.RWMutex
	serviceQueue     map[int]*ServiceObject
	waitQueue        *PriorityQueue
	waitQueueIndex   map[int]*PriorityItem
	currentService   int
	stopChan         chan struct{}
	billingService   *BillingService
	enableLogging    bool
	roomTemp         map[int]float32 // 用于缓存房间温度
	tempRecoveryRate float32         // 回温速率(每10秒)
	tempTicker       *time.Ticker
	roomRepo         *db.RoomRepository
}

// 速度优先级映射
var speedPriority = map[types.Speed]int{
	types.SpeedLow:    1,
	types.SpeedMedium: 2,
	types.SpeedHigh:   3,
}

func NewScheduler() *Scheduler {
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	s := &Scheduler{
		serviceQueue:     make(map[int]*ServiceObject),
		waitQueue:        &pq,
		waitQueueIndex:   make(map[int]*PriorityItem),
		currentService:   0,
		stopChan:         make(chan struct{}),
		roomRepo:         db.NewRoomRepository(),
		enableLogging:    false,
		roomTemp:         make(map[int]float32), // 初始化 roomTemp map
		tempRecoveryRate: 0.05,                  // 设置默认回温速率
	}

	go s.monitorServiceStatus()
	go s.monitorRoomTemperature()
	return s
}

// SetBillingService 设置billing service的方法
func (s *Scheduler) SetBillingService(billing *BillingService) {
	s.mu.Lock()
	s.billingService = billing
	s.mu.Unlock()
}

// 回温处理
func (s *Scheduler) monitorRoomTemperature() {
	s.tempTicker = time.NewTicker(1 * time.Second) // 每秒检查一次

	go func() {
		for {
			select {
			case <-s.tempTicker.C:
				s.handleTemperatureRecovery()
			case <-s.stopChan:
				s.tempTicker.Stop()
				return
			}
		}
	}()
}

// HandleRequest 处理空调请求
func (s *Scheduler) HandleRequest(roomID int, speed types.Speed, targetTemp, currentTemp float32) (bool, error) {
	s.mu.RLock()
	// 检查是否已在服务队列
	if service, exists := s.serviceQueue[roomID]; exists {
		s.mu.RUnlock()
		s.mu.Lock()
		if service.Speed != speed {
			// 记录当前服务的详单
			if err := s.billingService.CreateDetail(roomID, service, db.DetailTypeSpeedChange); err != nil {
				logger.Error("创建风速切换详单失败 - 房间ID: %d, 错误: %v", roomID, err)
			}
			// 更新服务对象
			service.StartTime = time.Now()
			service.Speed = speed
			service.TargetTemp = targetTemp
			// 更新房间风速
			if err := s.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
				logger.Error("更新房间风速失败: %v", err)
			}
		}
		s.mu.Unlock()
		return true, nil
	}
	s.mu.RUnlock()

	// 检查是否在等待队列
	if item, exists := s.waitQueueIndex[roomID]; exists {
		s.mu.Lock()
		if s.shouldReschedule(roomID, speed) {
			delete(s.waitQueueIndex, roomID)
			heap.Remove(s.waitQueue, item.indexHeap)
			result, err := s.schedule(roomID, speed, targetTemp, currentTemp)
			s.mu.Unlock()
			return result, err
		}
		item.waitObj.Speed = speed
		item.waitObj.TargetTemp = targetTemp
		item.priority = speedPriority[speed]
		heap.Fix(s.waitQueue, item.indexHeap)
		s.mu.Unlock()
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentService < MaxServices {
		// 首次加入服务队列时创建初始详单
		if err := s.addToServiceQueue(roomID, speed, targetTemp, currentTemp); err != nil {
			return false, err
		}
		return true, nil
	}

	return s.schedule(roomID, speed, targetTemp, currentTemp)
}

// ClearAllQueues 清空所有队列
func (s *Scheduler) ClearAllQueues() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清空服务队列
	for roomID := range s.serviceQueue {
		delete(s.serviceQueue, roomID)
	}
	s.currentService = 0

	// 清空等待队列
	s.waitQueue = &PriorityQueue{}
	heap.Init(s.waitQueue)
	s.waitQueueIndex = make(map[int]*PriorityItem)
}

func (s *Scheduler) schedule(roomID int, speed types.Speed, targetTemp, currentTemp float32) (bool, error) {
	requestPriority := speedPriority[speed]

	// 1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 {
		victim := s.selectVictim(lowPriorityServices)
		if victim != nil {

			// 将被抢占的服务对象添加到等待队列
			s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)
			delete(s.serviceQueue, victim.RoomID)
			s.currentService--

			// 将新请求加入服务队列
			if err := s.addToServiceQueue(roomID, speed, targetTemp, currentTemp); err != nil {
				return false, err
			}
			return true, nil
		}
	}

	// 2.时间片调度
	s.addToWaitQueue(roomID, speed, targetTemp, currentTemp)
	return false, nil
}

func (s *Scheduler) monitorServiceStatus() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			s.updateServiceStatus()
			s.checkWaitQueue()
			s.mu.Unlock()
		case <-s.stopChan:
			return
		}
	}
}

func (s *Scheduler) updateServiceStatus() {
	for roomID, service := range s.serviceQueue {
		service.Duration = float32(time.Since(service.StartTime).Seconds())

		// 计算温度变化
		tempDiff := service.TargetTemp - service.CurrentTemp

		if math.Abs(float64(tempDiff)) < 0.05 {
			// 温度达到目标
			if s.billingService != nil {
				if err := s.billingService.CreateDetail(roomID, service, db.DetailTypeTargetReached); err != nil {
					logger.Error("创建目标温度达到详单失败: %v", err)
				}
			}
			// 更新房间温度
			if err := s.roomRepo.UpdateTemperature(roomID, service.TargetTemp); err != nil {
				logger.Error("更新房间温度失败: %v", err)
			}

			// 更新缓存
			s.roomTemp[roomID] = service.TargetTemp

			// 从服务队列移除并处理下一个请求
			delete(s.serviceQueue, roomID)
			s.currentService--
			//如果等待队列不为空，处理下一个请求
			if s.waitQueue.Len() > 0 {
				item := heap.Pop(s.waitQueue).(*PriorityItem)
				wait := item.waitObj
				delete(s.waitQueueIndex, wait.RoomID)

				if err := s.addToServiceQueue(wait.RoomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp); err != nil {
					logger.Error("添加新服务失败: %v", err)
				}
			}
		} else {
			// 温度未达目标继续调节
			var tempChange float32
			if tempDiff > 0 {
				tempChange = 0.1
			} else {
				tempChange = -0.1
			}
			service.CurrentTemp += tempChange

			// 更新房间温度和缓存
			if err := s.roomRepo.UpdateTemperature(roomID, service.CurrentTemp); err != nil {
				logger.Error("更新房间温度失败: %v", err)
			}
			s.roomTemp[roomID] = service.CurrentTemp
		}
	}
}

func (s *Scheduler) checkWaitQueue() {
	if s.waitQueue.Len() == 0 {
		return
	}

	for _, item := range *s.waitQueue {
		item.waitObj.WaitDuration -= 1

		if item.waitObj.WaitDuration <= 0 {
			var longestServiceRoom int
			var maxDuration float32 = 0

			for sRoomID, service := range s.serviceQueue {
				if service.Speed == item.waitObj.Speed && service.Duration > maxDuration {
					longestServiceRoom = sRoomID
					maxDuration = service.Duration
				}
			}

			if longestServiceRoom != 0 {
				victim := s.serviceQueue[longestServiceRoom]

				s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)
				delete(s.serviceQueue, longestServiceRoom)
				s.currentService--

				if err := s.addToServiceQueue(item.waitObj.RoomID, item.waitObj.Speed,
					item.waitObj.TargetTemp, item.waitObj.CurrentTemp); err != nil {
					logger.Error("添加轮转服务失败: %v", err)
					// 重置等待时间
					item.waitObj.WaitDuration = s.calculateWaitDuration()
					continue
				}

				delete(s.waitQueueIndex, item.waitObj.RoomID)
				heap.Remove(s.waitQueue, item.indexHeap)
			} else {
				item.waitObj.WaitDuration = s.calculateWaitDuration()
			}
		}
	}
}

func (s *Scheduler) addToServiceQueue(roomID int, speed types.Speed, targetTemp, currentTemp float32) error {
	if err := s.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
		return fmt.Errorf("更新房间风速失败: %v", err)
	}

	// 查找房间的开机时间
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	s.serviceQueue[roomID] = &ServiceObject{
		RoomID:      roomID,
		StartTime:   time.Now(),       // 当前服务的开始时间
		PowerOnTime: room.CheckinTime, // 保存开机时间
		Speed:       speed,
		Duration:    0,
		TargetTemp:  targetTemp,
		CurrentTemp: currentTemp,
		IsCompleted: false,
	}
	s.currentService++
	return nil
}

func (s *Scheduler) addToWaitQueue(roomID int, speed types.Speed, targetTemp, currentTemp float32) {
	waitObj := &WaitObject{
		RoomID:       roomID,
		RequestTime:  time.Now(),
		Speed:        speed,
		WaitDuration: s.calculateWaitDuration(),
		TargetTemp:   targetTemp,
		CurrentTemp:  currentTemp,
	}

	item := &PriorityItem{
		roomID:   roomID,
		priority: speedPriority[speed],
		waitObj:  waitObj,
	}

	heap.Push(s.waitQueue, item)
	s.waitQueueIndex[roomID] = item
}

func (s *Scheduler) calculateWaitDuration() float32 {
	baseDuration := float32(WaitTime)
	queueLength := s.waitQueue.Len()

	if queueLength > 0 {
		return baseDuration * (1 + float32(queueLength)*0.5)
	}
	return baseDuration
}

func (s *Scheduler) findLowPriorityServices(requestPriority int) []*ServiceObject {
	services := make([]*ServiceObject, 0)
	for _, service := range s.serviceQueue {
		if speedPriority[service.Speed] < requestPriority {
			services = append(services, service)
		}
	}
	return services
}

func (s *Scheduler) selectVictim(candidates []*ServiceObject) *ServiceObject {
	if len(candidates) == 0 {
		return nil
	}

	var victim *ServiceObject = candidates[0]
	var minPriority = speedPriority[victim.Speed]
	var maxDuration float32 = victim.Duration

	for _, service := range candidates {
		priority := speedPriority[service.Speed]
		if priority < minPriority ||
			(priority == minPriority && service.Duration > maxDuration) {
			victim = service
			minPriority = priority
			maxDuration = service.Duration
		}
	}
	return victim
}

func (s *Scheduler) shouldReschedule(roomID int, newSpeed types.Speed) bool {
	item := s.waitQueueIndex[roomID]
	oldPriority := speedPriority[item.waitObj.Speed]
	newPriority := speedPriority[newSpeed]
	return newPriority > oldPriority
}

func (s *Scheduler) GetServiceQueue() map[int]*ServiceObject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serviceQueue
}

func (s *Scheduler) GetWaitQueue() []*WaitObject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*WaitObject, 0, s.waitQueue.Len())
	for _, item := range *s.waitQueue {
		result = append(result, item.waitObj)
	}
	return result
}

// RemoveRoom 从调度器中移除指定房间的所有请求
func (s *Scheduler) RemoveRoom(roomID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 从服务队列中移除
	if _, exists := s.serviceQueue[roomID]; exists {
		delete(s.serviceQueue, roomID)
		s.currentService--
		logger.Info("房间 %d 从服务队列中移除", roomID)
	}

	// 从等待队列中移除
	if item, exists := s.waitQueueIndex[roomID]; exists {
		heap.Remove(s.waitQueue, item.indexHeap)
		delete(s.waitQueueIndex, roomID)
		logger.Info("房间 %d 从等待队列中移除", roomID)
	}

	// 尝试从等待队列中选择下一个请求
	if s.currentService < MaxServices && s.waitQueue.Len() > 0 {
		item := heap.Pop(s.waitQueue).(*PriorityItem)
		wait := item.waitObj
		delete(s.waitQueueIndex, wait.RoomID)

		if err := s.addToServiceQueue(wait.RoomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp); err != nil {
			logger.Error("添加新服务失败 - 房间ID: %d, 错误: %v", wait.RoomID, err)
		} else {
			logger.Info("房间 %d 从等待队列提升至服务队列", wait.RoomID)
		}
	}
}

// SetLogging 设置是否启用日志
func (s *Scheduler) SetLogging(enable bool) {
	s.mu.Lock()
	s.enableLogging = enable
	s.mu.Unlock()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	if s.tempTicker != nil {
		s.tempTicker.Stop()
	}
	close(s.stopChan)
}

// handleTemperatureRecovery 处理回温
func (s *Scheduler) handleTemperatureRecovery() {
	// 1. 获取当前在服务队列中的房间列表
	s.mu.RLock()
	serviceRooms := make(map[int]struct{})
	for roomID := range s.serviceQueue {
		serviceRooms[roomID] = struct{}{}
	}
	s.mu.RUnlock()

	// 2. 获取所有房间信息
	rooms, err := s.roomRepo.GetAllRooms()
	if err != nil {
		logger.Error("获取房间列表失败: %v", err)
		return
	}

	for _, room := range rooms {
		// 3. 如果房间在服务队列中，跳过处理
		if _, inService := serviceRooms[room.RoomID]; inService {
			continue
		}

		s.mu.Lock()

		// 4. 计算房间温度与初始温度的差值
		currentTemp := room.CurrentTemp
		initialTemp := room.InitialTemp
		tempDiff := currentTemp - initialTemp // 正值表示高于初始温度，需要降温；负值表示低于初始温度，需要回暖

		// 6. 按照回温速率调整温度
		var newTemp float32
		if tempDiff > 0 { // 当前温度高于初始温度，需要降温
			newTemp = currentTemp - s.tempRecoveryRate
			if newTemp < initialTemp {
				newTemp = initialTemp
			}
		} else { // 当前温度低于初始温度，需要回暖
			newTemp = currentTemp + s.tempRecoveryRate
			if newTemp > initialTemp {
				newTemp = initialTemp
			}
		}

		// 7. 更新房间温度
		if err := s.roomRepo.UpdateTemperature(room.RoomID, newTemp); err != nil {
			logger.Error("更新房间温度失败 - 房间ID: %d, 错误: %v", room.RoomID, err)
			s.mu.Unlock()
			continue
		}

		s.mu.Unlock()

		// 8. 如果房间开着空调且温差>=1度，尝试申请服务
		if room.ACState == 1 && math.Abs(float64(currentTemp-room.TargetTemp)) >= 1.0 {
			s.mu.RLock()
			// 确认不在等待队列中才尝试申请服务
			if _, waiting := s.waitQueueIndex[room.RoomID]; !waiting {
				s.mu.RUnlock()

				// 获取当前风速或使用默认中速
				speed := types.SpeedMedium
				if room.CurrentSpeed != "" {
					speed = types.Speed(room.CurrentSpeed)
				}

				// 尝试申请服务
				if _, err := s.HandleRequest(room.RoomID, speed, room.TargetTemp, newTemp); err != nil {
					logger.Error("房间 %d 自动请求服务失败: %v", room.RoomID, err)
				} else {
					logger.Info("房间 %d 空调开启且温差超过1度，自动请求服务 (当前: %.1f°C, 目标: %.1f°C)",
						room.RoomID, newTemp, room.TargetTemp)
				}
			} else {
				s.mu.RUnlock()
			}
		}

	}
}
