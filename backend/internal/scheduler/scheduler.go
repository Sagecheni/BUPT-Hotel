package scheduler

import (
	"sync"
	"time"
)

const (
	MaxServicecs = 3   //最大服务对象
	WaitTime     = 120 //等待时间
)

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

type ServiceObject struct {
	RoomID    int
	StartTime time.Time
	Speed     string
	Duration  float32
}

type WaitObject struct {
	RoomID       int
	RequestTime  time.Time
	Speed        string
	WaitDuration float32
}

// 调度器
type Scheduler struct {
	mu sync.Mutex

	serviceQueue map[int]*ServiceObject
	waitQueue    map[int]*WaitObject

	currentService int
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		serviceQueue: make(map[int]*ServiceObject),
		waitQueue:    make(map[int]*WaitObject),
	}
}

func (s *Scheduler) HandleRequest(roomID int, speed string, Duration float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	//如果当前服务对象小于最大服务对象，直接分配
	if s.currentService < MaxServicecs {
		s.currentService++
		s.addToServiceQueue(roomID, speed)
	}
	//否则调度
	return s.scheduler(roomID, speed)
}

// 找出低优先级的服务对象
func (s *Scheduler) findLowPriorityServices(requestPriority int) []*ServiceObject {
	var services []*ServiceObject
	for _, service := range s.serviceQueue {
		if speedPriority[service.Speed] < requestPriority {
			services = append(services, service)
		}
	}
	return services
}

// 找出服务时间最长的对象
func (s *Scheduler) findLongestServices() *ServiceObject {
	var longest *ServiceObject
	maxDuration := float32(0)

	for _, service := range s.serviceQueue {
		if service.Duration > maxDuration {
			longest = service
			maxDuration = service.Duration
		}
	}
	return longest
}

// 选择牺牲者（优先级最低，其次是持续时间最长）
func (s *Scheduler) selectVictim(candidates []*ServiceObject) *ServiceObject {
	var victim *ServiceObject
	lowestPriority := speedPriority[SpeedHigh]
	longestDuration := float32(0)
	for _, service := range candidates {
		priority := speedPriority[service.Speed]
		if priority < lowestPriority {
			victim = service
			lowestPriority = priority
			longestDuration = service.Duration
		} else if priority == lowestPriority && service.Duration > longestDuration {
			victim = service
			longestDuration = service.Duration
		}
	}
	return victim
}

func (s *Scheduler) addToServiceQueue(roomID int, speed string) {
	s.serviceQueue[roomID] = &ServiceObject{
		RoomID:    roomID,
		StartTime: time.Now(),
		Speed:     speed,
		Duration:  0,
	}
	s.currentService++
}

// 辅助方法：添加到等待队列
func (s *Scheduler) addToWaitQueue(roomID int, speed string) {
	s.waitQueue[roomID] = &WaitObject{
		RoomID:       roomID,
		RequestTime:  time.Now(),
		Speed:        speed,
		WaitDuration: 0,
	}
}

// 辅助方法：从服务队列移到等待队列
func (s *Scheduler) moveToWaitQueue(roomID int) {
	if service, exists := s.serviceQueue[roomID]; exists {
		s.waitQueue[roomID] = &WaitObject{
			RoomID:       roomID,
			RequestTime:  time.Now(),
			Speed:        service.Speed,
			WaitDuration: 0,
		}
		delete(s.serviceQueue, roomID)
		s.currentService--
	}
}

// 辅助方法：从等待队列提升到服务队列
func (s *Scheduler) promoteFromWaitQueue(roomID int) {
	if wait, exists := s.waitQueue[roomID]; exists {
		s.addToServiceQueue(roomID, wait.Speed)
		delete(s.waitQueue, roomID)
	}
}

// 时间片方法
func (s *Scheduler) timeSliceCheck(roomID int) {
	time.Sleep(WaitTime * time.Second)
	s.mu.Lock()
	defer s.mu.Unlock()
	//如果请求还在等待队列
	if _, exists := s.waitQueue[roomID]; exists {
		victim := s.findLongestServices()
		if victim != nil {
			s.moveToWaitQueue(victim.RoomID)
			s.promoteFromWaitQueue(roomID)
		}
	}
}

func (s *Scheduler) scheduler(roomID int, speed string) (bool, error) {
	requestPriority := speedPriority[speed]

	//1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 {
		if len(lowPriorityServices) == 1 {
			//只有一个低优先级
			victim := lowPriorityServices[0]
			s.moveToWaitQueue(victim.RoomID)
			s.addToServiceQueue(roomID, speed)
			return true, nil
		} else {
			//多个优先级
			victim := s.selectVictim(lowPriorityServices)
			s.moveToWaitQueue(victim.RoomID)
			s.addToServiceQueue(roomID, speed)
			return true, nil
		}
	}
	//2.时间片调度
	s.addToWaitQueue(roomID, speed)

	go s.timeSliceCheck(roomID)
	return false, nil
}
