// internal/service/scheduler.go
// Package service 提供酒店空调系统的核心服务实现
// 包括空调控制、调度管理、计费等功能
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

// ServiceObject 表示一个正在服务中的空调对象
type ServiceObject struct {
	RoomID      int         // 房间唯一标识
	StartTime   time.Time   // 当前服务周期的开始时间
	PowerOnTime time.Time   // 本次开机的时间点,用于费用计算
	Speed       types.Speed // 当前风速设置
	Duration    float32     // 当前服务时长(秒)
	TargetTemp  float32     // 目标温度
	CurrentTemp float32     // 当前温度
	IsCompleted bool        // 是否已完成服务
}

// WaitObject 表示一个等待服务的请求对象
// 用于管理未能立即得到服务的空调请求
type WaitObject struct {
	RoomID       int         // 请求房间号
	RequestTime  time.Time   // 发起请求的时间
	Speed        types.Speed // 请求的风速
	WaitDuration float32     // 剩余等待时间
	TargetTemp   float32     // 请求的目标温度
	CurrentTemp  float32     // 请求时的当前温度
}

// PriorityQueue 优先级队列实现
// 用于管理等待队列中的请求，支持基于优先级的排序
type PriorityItem struct {
	roomID    int         // 房间号
	priority  int         // 优先级值
	waitObj   *WaitObject // 等待对象
	indexHeap int         // 在堆中的索引
}

// 优先级队列必需的接口方法实现
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

// Scheduler 空调调度器
// 负责管理所有房间的空调请求,实现服务队列和等待队列的调度
type Scheduler struct {
	mu               sync.RWMutex           // 并发安全锁
	serviceQueue     map[int]*ServiceObject // 服务队列,key为房间号
	waitQueue        *PriorityQueue         // 等待队列,基于优先级排序
	waitQueueIndex   map[int]*PriorityItem  // 等待队列索引,用于快速查找
	currentService   int                    // 当前服务数量
	stopChan         chan struct{}          // 停止信号通道
	billingService   *BillingService        // 计费服务
	enableLogging    bool                   // 是否启用日志
	roomTemp         map[int]float32        // 房间温度缓存
	tempRecoveryRate float32                // 温度回温率(每100ms)
	tempTicker       *time.Ticker           // 温度更新定时器
	roomRepo         *db.RoomRepository     // 房间数据访问对象
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
		tempRecoveryRate: 0.005,                 // 设置默认回温速率
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
	s.tempTicker = time.NewTicker(100 * time.Millisecond) // 每100毫秒检查一次

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

// HandleRequest 处理新的空调请求
// 实现请求的优先级调度和时间片轮转调度
// roomID: 请求的房间号
// speed: 请求的风速
// targetTemp: 目标温度
// currentTemp: 当前温度
// 返回值:
//   - bool: 是否直接进入服务队列
//   - error: 错误信息
func (s *Scheduler) HandleRequest(roomID int, speed types.Speed, targetTemp, currentTemp float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 检查是否已在服务队列
	if service, exists := s.serviceQueue[roomID]; exists {
		service.TargetTemp = targetTemp
		if service.Speed != speed {
			// 记录当前服务的详单
			if err := s.billingService.CreateDetail(roomID, service, db.DetailTypeSpeedChange); err != nil {
				logger.Error("创建风速切换详单失败 - 房间ID: %d, 错误: %v", roomID, err)
			}
			// 更新服务对象
			service.StartTime = time.Now()
			service.Speed = speed
			// 更新房间风速
			if err := s.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
				logger.Error("更新房间风速失败: %v", err)
			}
		}
		return true, nil
	}

	// 检查是否在等待队列
	if item, exists := s.waitQueueIndex[roomID]; exists {
		oldSpeed := item.waitObj.Speed
		if oldSpeed != speed {
			// 获取房间信息，用于创建详单
			room, err := s.roomRepo.GetRoomByID(roomID)
			if err != nil {
				logger.Error("获取房间信息失败: %v", err)
				return false, err
			}

			// 创建一个临时的服务对象用于记录详单
			tempService := &ServiceObject{
				RoomID:      roomID,
				StartTime:   time.Now(),
				PowerOnTime: room.CheckinTime,
				Speed:       oldSpeed, // 使用旧风速
				TargetTemp:  item.waitObj.TargetTemp,
				CurrentTemp: item.waitObj.CurrentTemp,
			}

			// 创建风速切换详单
			if s.billingService != nil {
				if err := s.billingService.CreateDetail(roomID, tempService, db.DetailTypeSpeedChange); err != nil {
					logger.Error("创建风速切换详单失败 - 房间ID: %d, 错误: %v", roomID, err)
				}
			}
		}
		if s.shouldReschedule(roomID, speed) {
			delete(s.waitQueueIndex, roomID)
			heap.Remove(s.waitQueue, item.indexHeap)
			result, err := s.schedule(roomID, speed, targetTemp, currentTemp)
			return result, err
		}
		item.waitObj.Speed = speed
		item.waitObj.TargetTemp = targetTemp
		item.priority = speedPriority[speed]
		return false, nil
	}

	if s.currentService < MaxServices {

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
			// 从服务队列中移除

			if s.billingService != nil {
				if err := s.billingService.CreateDetail(victim.RoomID, victim, db.DetailTypeServiceInterrupt); err != nil {
					logger.Error("创建服务中断详单失败 - 房间ID: %d, 错误: %v", roomID, err)
				}
			}
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
	tempChangeRates := map[types.Speed]float32{
		types.SpeedHigh:   0.1,    // 1度/10秒
		types.SpeedMedium: 0.05,   // 1度/20秒
		types.SpeedLow:    0.0333, // 1度/30秒
	}
	for roomID, service := range s.serviceQueue {
		service.Duration = float32(time.Since(service.StartTime).Seconds())

		// 计算温度变化
		tempDiff := service.TargetTemp - service.CurrentTemp

		if math.Abs(float64(tempDiff)) < 0.05 {
			// 温度达到目标
			if err := s.roomRepo.UpdateTemperature(roomID, service.TargetTemp); err != nil {
				logger.Error("更新房间温度失败: %v", err)
			}

			// 更新缓存
			s.roomTemp[roomID] = service.TargetTemp

			// 从服务队列移除并处理下一个请求
			if s.billingService != nil {
				if err := s.billingService.CreateDetail(roomID, service, db.DetailTypeServiceInterrupt); err != nil {
					logger.Error("创建服务中断详单失败 - 房间ID: %d, 错误: %v", roomID, err)
				}
			}
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
			// 根据风速获取温度变化率
			tempChangeRate := tempChangeRates[service.Speed]

			// 根据目标温度和当前温度的差值确定变化方向
			var tempChange float32
			if tempDiff > 0 {
				tempChange = tempChangeRate // 需要升温
			} else {
				tempChange = -tempChangeRate // 需要降温
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

// checkWaitQueue 检查等待队列中的请求
// 处理等待超时的请求，实现时间片轮转调度
func (s *Scheduler) checkWaitQueue() {
	if s.waitQueue.Len() == 0 {
		return
	}
	// 遍历等待队列中的所有请求
	for _, item := range *s.waitQueue {
		item.waitObj.WaitDuration -= 1 // 递减等待时间
		// 当等待时间到期时进行处理
		if item.waitObj.WaitDuration <= 0 {
			// 查找服务时间最长的相同风速级别的服务
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
				if s.billingService != nil {
					if err := s.billingService.CreateDetail(longestServiceRoom, victim, db.DetailTypeServiceInterrupt); err != nil {
						logger.Error("创建服务中断详单失败 - 房间ID: %d, 错误: %v", longestServiceRoom, err)
					}
				}
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

// addToServiceQueue 将请求添加到服务队列
// roomID: 房间号
// speed: 风速设置
// targetTemp: 目标温度
// currentTemp: 当前温度
// 返回值: 错误信息
func (s *Scheduler) addToServiceQueue(roomID int, speed types.Speed, targetTemp, currentTemp float32) error {
	if err := s.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
		return fmt.Errorf("更新房间风速失败: %v", err)
	}

	// 查找房间的开机时间
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	serviceObj := &ServiceObject{
		RoomID:      roomID,
		StartTime:   time.Now(),       // 当前服务的开始时间
		PowerOnTime: room.CheckinTime, // 保存开机时间
		Speed:       speed,
		Duration:    0,
		TargetTemp:  targetTemp,
		CurrentTemp: currentTemp,
		IsCompleted: false,
	}

	s.serviceQueue[roomID] = serviceObj
	s.currentService++
	// 创建服务开始详单
	if s.billingService != nil {
		if err := s.billingService.CreateDetail(roomID, serviceObj, db.DetailTypeServiceStart); err != nil {
			logger.Error("创建服务开始详单失败 - 房间ID: %d, 错误: %v", roomID, err)
			// 不要因为详单创建失败而影响正常服务
		}
	}

	return nil
}

// addToWaitQueue 将请求添加到等待队列
// roomID: 房间号
// speed: 请求的风速级别
// targetTemp: 目标温度
// currentTemp: 当前温度
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

// calculateWaitDuration 计算新请求的等待时间
// 根据当前等待队列长度动态调整等待时间
// 返回值: 计算得到的等待时间(秒)
func (s *Scheduler) calculateWaitDuration() float32 {
	baseDuration := float32(WaitTime)
	queueLength := s.waitQueue.Len()

	if queueLength > 0 {
		return baseDuration * (1 + float32(queueLength)*0.5)
	}
	return baseDuration
}

// findLowPriorityServices 查找优先级较低的服务
// requestPriority: 新请求的优先级
// 返回值: 优先级低于请求的服务对象列表
func (s *Scheduler) findLowPriorityServices(requestPriority int) []*ServiceObject {
	services := make([]*ServiceObject, 0)
	for _, service := range s.serviceQueue {
		if speedPriority[service.Speed] < requestPriority {
			services = append(services, service)
		}
	}
	return services
}

// selectVictim 在候选服务中选择被抢占的对象
// candidates: 候选服务列表
// 返回值: 被选中要抢占的服务对象
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
	if service, exists := s.serviceQueue[roomID]; exists {
		if s.billingService != nil {
			if err := s.billingService.CreateDetail(roomID, service, db.DetailTypeServiceInterrupt); err != nil {
				logger.Error("创建服务中断详单失败 - 房间ID: %d, 错误: %v", roomID, err)
			}
		}
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

// handleTemperatureRecovery 处理房间温度回温
// 当空调未在服务时，房间温度会逐渐恢复到初始温度
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
					switch room.CurrentSpeed {
					case "低":
						speed = types.SpeedLow
					case "中":
						speed = types.SpeedMedium
					case "高":
						speed = types.SpeedHigh
					default:
						speed = types.SpeedMedium
					}

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
