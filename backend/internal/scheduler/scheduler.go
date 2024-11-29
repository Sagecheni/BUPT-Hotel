package scheduler

import (
	"backend/internal/db"
	"backend/internal/logger"
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

const (
	DefaultSpeed = SpeedMedium   // 默认中速
	DefaultTemp  = float32(25.0) // 默认25度
)

// 添加默认配置结构体
type DefaultConfig struct {
	DefaultSpeed string  `json:"default_speed"`
	DefaultTemp  float32 `json:"default_temp"`
}

const (
	// 温度变化速率 (每秒变化的温度)
	TempChangeRateLow    = 1.0      // 低速温度变化率
	TempChangeRateMedium = 0.5      // 中速温度变化率
	TempChangeRateHigh   = 0.333333 // 高速温度变化率
)

const tempThreshold = 0.5

// 速度与温度变化率的映射
var speedTempRate = map[string]float32{
	SpeedLow:    TempChangeRateLow,
	SpeedMedium: TempChangeRateMedium,
	SpeedHigh:   TempChangeRateHigh,
}

const (
	SpeedLow    = "low"
	SpeedMedium = "medium"
	SpeedHigh   = "high"
)

var speedPriority = map[string]int{
	SpeedLow:    1,
	SpeedMedium: 2,
	SpeedHigh:   3,
}

// PriorityQueue 实现
type PriorityItem struct {
	roomID    int
	priority  int
	waitObj   *WaitObject
	indexHeap int
}

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

// 服务对象
type ServiceObject struct {
	RoomID      int
	StartTime   time.Time
	Speed       string
	Duration    float32
	TargetTemp  float32
	CurrentTemp float32
	IsCompleted bool
}

// 等待对象
type WaitObject struct {
	RoomID       int
	RequestTime  time.Time
	Speed        string
	WaitDuration float32
	TargetTemp   float32
	CurrentTemp  float32
}

// 调度器结构
type Scheduler struct {
	mu             sync.RWMutex
	serviceQueue   map[int]*ServiceObject
	waitQueue      *PriorityQueue
	waitQueueIndex map[int]*PriorityItem // 用于快速查找
	currentService int
	stopChan       chan struct{}
	roomRepo       *db.RoomRepository
	defaultConfig  DefaultConfig // 默认配置
}

func NewScheduler() *Scheduler {
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	s := &Scheduler{
		serviceQueue:   make(map[int]*ServiceObject), // 服务队列
		waitQueue:      &pq,
		waitQueueIndex: make(map[int]*PriorityItem), // 等待队列
		currentService: 0,
		stopChan:       make(chan struct{}),
		roomRepo:       db.NewRoomRepository(),
		defaultConfig: DefaultConfig{
			DefaultSpeed: DefaultSpeed,
			DefaultTemp:  DefaultTemp,
		},
	}

	go s.monitorServiceStatus()
	return s
}

// 监控服务状态
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

func (s *Scheduler) HandleRequest(roomID int, speed string, targetTemp, currentTemp float32) (bool, error) {
	s.mu.RLock()
	// 检查是否已在服务队列
	if service, exists := s.serviceQueue[roomID]; exists {
		s.mu.RUnlock()
		s.mu.Lock()
		service.Speed = speed
		service.TargetTemp = targetTemp
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
		// 更新等待队列中的参数
		item.waitObj.Speed = speed
		item.waitObj.TargetTemp = targetTemp
		item.priority = speedPriority[speed]
		heap.Fix(s.waitQueue, item.indexHeap) // 重新调整堆
		s.mu.Unlock()
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果当前服务对象小于最大服务对象，直接分配
	if s.currentService < MaxServices {
		s.addToServiceQueue(roomID, speed, targetTemp, currentTemp)
		return true, nil
	}

	// 否则启动调度
	return s.schedule(roomID, speed, targetTemp, currentTemp)
}

func (s *Scheduler) schedule(roomID int, speed string, targetTemp, currentTemp float32) (bool, error) {
	requestPriority := speedPriority[speed]

	// 1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 {
		victim := s.selectVictim(lowPriorityServices)
		if victim != nil {
			//更新被抢占房间的风速
			if err := s.roomRepo.UpdateSpeed(victim.RoomID, speed); err != nil {
				logger.Error("Failed to update room %d speed: %v", victim.RoomID, err)
			}
			// 将被抢占的服务对象添加到等待队列
			s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)
			delete(s.serviceQueue, victim.RoomID)
			s.currentService--

			// 将新请求加入服务队列
			s.addToServiceQueue(roomID, speed, targetTemp, currentTemp)
			return true, nil
		}
	}

	// 2.时间片调度
	s.addToWaitQueue(roomID, speed, targetTemp, currentTemp)
	return false, nil
}

// 时间片调度
func (s *Scheduler) checkWaitQueue() {
	if s.waitQueue.Len() == 0 {
		return
	}
	if s.waitQueue.Len() > 0 {
		logger.Info("Current wait queue length: %d", s.waitQueue.Len())
		// 打印等待队列中的房间信息
		for _, item := range *s.waitQueue {
			logger.Info("Waiting Room %d: Speed %s, Wait duration %.1f",
				item.roomID, item.waitObj.Speed, item.waitObj.WaitDuration)
		}
	}

	// 更新等待时间并检查是否需要轮转
	for _, item := range *s.waitQueue {
		wait := item.waitObj
		wait.WaitDuration -= 1

		if wait.WaitDuration <= 0 {
			var longestServiceRoom int
			var maxDuration float32 = 0

			// 找到相同风速中服务时间最长的
			for sRoomID, service := range s.serviceQueue {
				if service.Speed == wait.Speed && service.Duration > maxDuration {
					longestServiceRoom = sRoomID
					maxDuration = service.Duration
				}
			}

			if longestServiceRoom != 0 {
				victim := s.serviceQueue[longestServiceRoom]
				s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)
				delete(s.serviceQueue, longestServiceRoom)
				s.currentService--

				s.addToServiceQueue(wait.RoomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp)
				delete(s.waitQueueIndex, wait.RoomID)
				heap.Remove(s.waitQueue, item.indexHeap)
			} else {
				// 重置等待时间
				wait.WaitDuration = s.calculateWaitDuration()
			}
		}
	}
}

// 选择牺牲者(最低优先级后时间最长)
func (s *Scheduler) selectVictim(candidates []*ServiceObject) *ServiceObject {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	var lowPriority = math.MaxInt32
	var sameSpeedServices []*ServiceObject

	// 找出最低优先级
	for _, service := range candidates {
		priority := speedPriority[service.Speed]
		if priority < lowPriority {
			lowPriority = priority
			sameSpeedServices = []*ServiceObject{service}
		} else if priority == lowPriority {
			sameSpeedServices = append(sameSpeedServices, service)
		}
	}

	// 在最低优先级中选择服务时间最长的
	var victim *ServiceObject = sameSpeedServices[0]
	var maxDuration float32 = sameSpeedServices[0].Duration
	for _, service := range sameSpeedServices {
		if service.Duration > maxDuration {
			maxDuration = service.Duration
			victim = service
		}
	}
	return victim
}

// 更新服务状态
func (s *Scheduler) updateServiceStatus() {
	for roomID, service := range s.serviceQueue {
		// 更新服务时长
		service.Duration = float32(time.Since(service.StartTime).Seconds())

		// 计算温度变化
		tempRate := speedTempRate[service.Speed]
		tempDiff := service.TargetTemp - service.CurrentTemp
		oldTemp := service.CurrentTemp

		//日志记录
		logger.Info("Room %d: Current temp %.1f, Target temp %.1f, Speed %s",
			roomID, service.CurrentTemp, service.TargetTemp, service.Speed)
		if math.Abs(float64(tempDiff)) > tempThreshold {
			if tempDiff > 0 {
				service.CurrentTemp += tempRate
			} else {
				service.CurrentTemp -= tempRate
			}
			if err := s.roomRepo.UpdateTemperature(roomID, service.CurrentTemp); err != nil {
				logger.Error("Failed to update room %d temperature: %v", roomID, err)
			}
		} else {
			// 温度达到目标值
			logger.Info("Room %d has reached target temperature", roomID)
			if oldTemp != service.TargetTemp {
				service.CurrentTemp = service.TargetTemp
				if err := s.roomRepo.UpdateTemperature(roomID, service.CurrentTemp); err != nil {
					logger.Error("Failed to update room %d temperature: %v", roomID, err)
				}
			}
			service.IsCompleted = true

			// 从等待队列中选择下一个请求
			if s.waitQueue.Len() > 0 {
				item := heap.Pop(s.waitQueue).(*PriorityItem)
				wait := item.waitObj
				delete(s.waitQueueIndex, wait.RoomID)

				// 更新房间风速
				if err := s.roomRepo.UpdateSpeed(wait.RoomID, wait.Speed); err != nil {
					logger.Error("Failed to update room %d speed: %v", wait.RoomID, err)
				}

				delete(s.serviceQueue, roomID)
				s.currentService--
				s.addToServiceQueue(wait.RoomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp)
				if err := s.roomRepo.UpdateSpeed(wait.RoomID, wait.Speed); err != nil {
					logger.Error("Failed to update room %d speed: %v", wait.RoomID, err)
				}
			} else {
				if err := s.roomRepo.UpdateSpeed(roomID, ""); err != nil {
					logger.Error("Failed to update room %d speed: %v", roomID, err)
				}
				delete(s.serviceQueue, roomID)
				s.currentService--
				logger.Info("Room %d service completed and released, no waiting requests", roomID)
			}
		}
	}
}

// 辅助方法：计算等待时间
func (s *Scheduler) calculateWaitDuration() float32 {
	baseDuration := float32(WaitTime)
	queueLength := s.waitQueue.Len()

	if queueLength > 0 {
		return baseDuration * (1 + float32(queueLength)*0.5)
	}
	return baseDuration
}

// 辅助方法：查找低优先级服务对象
func (s *Scheduler) findLowPriorityServices(requestPriority int) []*ServiceObject {
	services := make([]*ServiceObject, 0, len(s.serviceQueue))
	for _, service := range s.serviceQueue {
		if speedPriority[service.Speed] < requestPriority {
			services = append(services, service)
		}
	}
	return services
}

// 辅助方法：添加服务对象到服务队列
func (s *Scheduler) addToServiceQueue(roomID int, speed string, targetTemp, currentTemp float32) {
	s.serviceQueue[roomID] = &ServiceObject{
		RoomID:      roomID,
		StartTime:   time.Now(),
		Speed:       speed,
		Duration:    0,
		TargetTemp:  targetTemp,
		CurrentTemp: currentTemp,
		IsCompleted: false,
	}
	s.currentService++
	if err := s.roomRepo.UpdateSpeed(roomID, speed); err != nil {
		logger.Error("Failed to update room %d speed: %v", roomID, err)
	}
}

// 辅助方法：添加等待对象到等待队列
func (s *Scheduler) addToWaitQueue(roomID int, speed string, targetTemp, currentTemp float32) {
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

// 辅助方法：判断是否需要重新调度
func (s *Scheduler) shouldReschedule(roomID int, newSpeed string) bool {
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

// 添加获取和设置默认配置的方法
func (s *Scheduler) GetDefaultConfig() DefaultConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultConfig
}

func (s *Scheduler) SetDefaultConfig(config DefaultConfig) error {
	// 验证风速值
	if config.DefaultSpeed != SpeedLow &&
		config.DefaultSpeed != SpeedMedium &&
		config.DefaultSpeed != SpeedHigh {
		return fmt.Errorf("无效的默认风速值")
	}

	// 验证温度值
	if config.DefaultTemp < 16 || config.DefaultTemp > 28 {
		return fmt.Errorf("默认温度必须在16-28度之间")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultConfig = config
	return nil
}

func (s *Scheduler) Stop() {
	close(s.stopChan)
}
