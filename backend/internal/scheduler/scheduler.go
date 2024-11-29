package scheduler

import (
	"fmt"
	"sync"
	"time"
)

const (
	MaxServices = 3 //最大服务对象
	WaitTime    = 2 //等待时间
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
}

type WaitObject struct {
	RoomID      int
	RequestTime time.Time
	Speed       string
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
		serviceQueue:   make(map[int]*ServiceObject),
		waitQueue:      make(map[int]*WaitObject),
		currentService: 0,
	}
}

func (s *Scheduler) HandleRequest(roomID int, speed string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logQueueStatus(fmt.Sprintf("Before handling request: Room %d, Speed %s", roomID, speed))

	if _, exists := s.serviceQueue[roomID]; exists {
		return true, nil
	}
	//如果当前服务对象小于最大服务对象，直接分配
	if s.currentService < MaxServices {
		s.addToServiceQueue(roomID, speed)
		s.logQueueStatus("After direct assignment")
		return true, nil
	}
	//否则调度
	result, err := s.scheduler(roomID, speed)
	s.logQueueStatus("After scheduling")
	return result, err
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
	maxDuration := float64(0)

	for _, service := range s.serviceQueue {
		duration := time.Since(service.StartTime).Seconds()
		if duration > maxDuration {
			longest = service
			maxDuration = duration
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
		duration := time.Since(service.StartTime).Seconds()
		if priority < lowestPriority {
			victim = service
			lowestPriority = priority
			longestDuration = float32(duration)
		} else if priority == lowestPriority && float32(duration) > longestDuration {
			victim = service
			longestDuration = float32(duration)
		}
	}
	return victim
}

// 辅助方法：添加到服务队列
func (s *Scheduler) addToServiceQueue(roomID int, speed string) {
	s.serviceQueue[roomID] = &ServiceObject{
		RoomID:    roomID,
		StartTime: time.Now(),
		Speed:     speed,
	}
	s.currentService++
}

// 辅助方法：添加到等待队列
func (s *Scheduler) addToWaitQueue(roomID int, speed string) {
	s.waitQueue[roomID] = &WaitObject{
		RoomID:      roomID,
		RequestTime: time.Now(),
		Speed:       speed,
	}
}

// 辅助方法：从服务队列移到等待队列
func (s *Scheduler) moveToWaitQueue(roomID int) {
	if service, exists := s.serviceQueue[roomID]; exists {
		s.waitQueue[roomID] = &WaitObject{
			RoomID:      roomID,
			RequestTime: time.Now(),
			Speed:       service.Speed,
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logQueueStatus(fmt.Sprintf("Time slice check for Room %d", roomID))
	//如果请求还在等待队列
	if _, exists := s.waitQueue[roomID]; exists {
		victim := s.findLongestServices()
		if victim != nil {
			s.logQueueStatus(fmt.Sprintf("Found longest service: Room %d", victim.RoomID))
			s.moveToWaitQueue(victim.RoomID)
			s.promoteFromWaitQueue(roomID)
			s.logQueueStatus("After time slice rotation")
		}
	}
}

// 调度器
func (s *Scheduler) scheduler(roomID int, speed string) (bool, error) {
	requestPriority := speedPriority[speed]

	//1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 {
		s.logQueueStatus(fmt.Sprintf("Found %d low priority services", len(lowPriorityServices)))
		if len(lowPriorityServices) == 1 {
			//只有一个低优先级
			victim := lowPriorityServices[0]
			s.logQueueStatus(fmt.Sprintf("Selected victim: Room %d", victim.RoomID))
			s.moveToWaitQueue(victim.RoomID)
			s.addToServiceQueue(roomID, speed)
			return true, nil
		} else {
			//多个优先级
			victim := s.selectVictim(lowPriorityServices)
			s.logQueueStatus(fmt.Sprintf("Selected victim: Room %d", victim.RoomID))
			s.moveToWaitQueue(victim.RoomID)
			s.addToServiceQueue(roomID, speed)
			return true, nil
		}
	}
	//2.时间片调度
	s.addToWaitQueue(roomID, speed)
	s.logQueueStatus("No low priority services found, adding to wait queue")
	go func() {
		time.Sleep(WaitTime * time.Second)
		s.timeSliceCheck(roomID)
	}()
	return false, nil
}

// 获取服务队列
func (s *Scheduler) GetServiceQueue() map[int]*ServiceObject {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serviceQueue
}

// 获取等待队列
func (s *Scheduler) GetWaitQueue() map[int]*WaitObject {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitQueue
}

/*-------------*/
// 添加日志方法到Scheduler结构体
func (s *Scheduler) logQueueStatus(action string) {
	fmt.Printf("\n=== %s ===\n", action)
	fmt.Println("Service Queue:")
	for roomID, service := range s.serviceQueue {
		fmt.Printf("Room %d: Speed=%s, Duration=%.2fs\n",
			roomID,
			service.Speed,
			time.Since(service.StartTime).Seconds())
	}

	fmt.Println("\nWait Queue:")
	for roomID, wait := range s.waitQueue {
		fmt.Printf("Room %d: Speed=%s, WaitTime=%.2fs\n",
			roomID,
			wait.Speed,
			time.Since(wait.RequestTime).Seconds())
	}
	fmt.Println("================\n ")
}
