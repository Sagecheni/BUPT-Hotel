package scheduler

import (
	"backend/internal/logger"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	MaxServices = 3 //最大服务对象
	WaitTime    = 2 //时间片
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
	RoomID     int
	StartTime  time.Time
	Speed      string
	Duration   float32 //服务时长
	TargetTemp float32 //目标温度
}

type WaitObject struct {
	RoomID       int
	RequestTime  time.Time
	Speed        string
	WaitDuration float32 //等待时长
	TargetTemp   float32 //目标温度
}

// 调度器
type Scheduler struct {
	mu sync.Mutex

	serviceQueue map[int]*ServiceObject
	waitQueue    map[int]*WaitObject

	currentService int
	stopChan       chan struct{} //用来停止监控的通道
}

func NewScheduler() *Scheduler {
	s := &Scheduler{
		serviceQueue:   make(map[int]*ServiceObject),
		waitQueue:      make(map[int]*WaitObject),
		currentService: 0,
		stopChan:       make(chan struct{}),
	}
	go s.monitorServiceStatus()
	return s

}

func (s *Scheduler) HandleRequest(roomID int, speed string, targetTemp float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger.Debug("Handling request for Room %d with speed %s, target temperature %.1f",
		roomID, speed, targetTemp)
	s.logQueueStatus("Before handling request")

	if _, exists := s.serviceQueue[roomID]; exists {
		logger.Info("Room %d already in service queue", roomID)
		return true, nil
	}
	//如果当前服务对象小于最大服务对象，直接分配
	if s.currentService < MaxServices {
		logger.Debug("Direct assignment available, adding Room %d to service queue", roomID)
		s.addToServiceQueue(roomID, speed, targetTemp)
		s.logQueueStatus("After direct assignment")
		return true, nil
	}
	//否则调度
	result, err := s.scheduler(roomID, speed, targetTemp)
	if err != nil {
		logger.Error("Scheduling failed for Room %d: %v", roomID, err)
	}
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

// 增加等待服务时长分配
func (s *Scheduler) assignWaitDuration() float32 {
	// 可以基于当前负载、队列长度等因素计算
	baseDuration := float32(2 * time.Second) // 基础等待时间2分钟
	return baseDuration
}

// 找出服务时间最长的对象
func (s *Scheduler) findLongestService() *ServiceObject {
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
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	//有多个候选者时，检查是否有相同优先级的
	var lowPriority = math.MaxInt32
	var sameSpeedServices []*ServiceObject

	//找出最低优先级
	for _, service := range candidates {
		priority := speedPriority[service.Speed]
		if priority < lowPriority {
			lowPriority = speedPriority[service.Speed]
			sameSpeedServices = []*ServiceObject{service}
		} else if priority == lowPriority {
			sameSpeedServices = append(sameSpeedServices, service)
		}
	}
	// 在最低优先级中选择服务时间最长的
	var victim *ServiceObject
	var maxDuration float32 = 0
	for _, service := range sameSpeedServices {
		duration := float32(time.Since(service.StartTime).Seconds())
		if duration > maxDuration {
			maxDuration = duration
			victim = service
		}
	}
	return victim

}

// 找到等待时间最短的请求
func (s *Scheduler) findShortestWaitingRequest() int {
	var shortestRoom int
	shortestWait := float32(math.MaxFloat32)

	for roomID, wait := range s.waitQueue {
		if wait.WaitDuration < shortestWait {
			shortestWait = wait.WaitDuration
			shortestRoom = roomID
		}
	}
	return shortestRoom
}

// 辅助方法：计算等待时长
func (s *Scheduler) calculateWaitDuration() float32 {
	baseDuration := float32(WaitTime)
	queueLength := len(s.waitQueue)

	// 根据等待队列长度调整等待时间
	if queueLength > 0 {
		return baseDuration * (1 + float32(queueLength)*0.5)
	}
	return baseDuration
}

// 辅助方法：添加到服务队列
func (s *Scheduler) addToServiceQueue(roomID int, speed string, targetTemp float32) {
	s.serviceQueue[roomID] = &ServiceObject{
		RoomID:     roomID,
		StartTime:  time.Now(),
		Speed:      speed,
		Duration:   0,
		TargetTemp: targetTemp,
	}
	s.currentService++
}

// 辅助方法：添加到等待队列
func (s *Scheduler) addToWaitQueue(roomID int, speed string, targetTemp float32) {
	waitDuration := s.calculateWaitDuration()
	s.waitQueue[roomID] = &WaitObject{
		RoomID:       roomID,
		RequestTime:  time.Now(),
		Speed:        speed,
		WaitDuration: waitDuration,
		TargetTemp:   targetTemp,
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
		s.addToServiceQueue(roomID, wait.Speed, wait.TargetTemp)
		delete(s.waitQueue, roomID)
	}
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

// 更新服务状态
func (s *Scheduler) updateServiceStatus() {
	for _, service := range s.serviceQueue {
		service.Duration = float32(time.Since(service.StartTime).Seconds())
		// 这里可以添加费用计算逻辑
	}
}

// 检查等待队列
func (s *Scheduler) checkWaitQueue() {
	// 更新等待时间
	for roomID, wait := range s.waitQueue {
		wait.WaitDuration -= 1
		// 当等待时间到期时，尝试进行时间片轮转
		if wait.WaitDuration <= 0 {
			if victim := s.findLongestService(); victim != nil {
				s.moveToWaitQueue(victim.RoomID)
				s.promoteFromWaitQueue(roomID)
			}
		}
	}
}

// 调度器
func (s *Scheduler) scheduler(roomID int, speed string, targetTemp float32) (bool, error) {
	requestPriority := speedPriority[speed]
	logger.Debug("Starting scheduling for Room %d with priority %d", roomID, requestPriority)

	//1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 { //有低优先级服务对象
		victim := s.selectVictim(lowPriorityServices)
		if victim != nil {
			logger.Info("Selected Room %d as victim for preemption", victim.RoomID)
			s.moveToWaitQueue(victim.RoomID)
			s.addToServiceQueue(roomID, speed, targetTemp)
			return true, nil
		}
	}
	//2.时间片调度
	logger.Debug("No low priority services found, applying time-slice scheduling")
	s.addToWaitQueue(roomID, speed, targetTemp)
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

// 日志记录
func (s *Scheduler) logQueueStatus(action string) {
	var status string
	status += fmt.Sprintf("\n=== %s ===\n", action)

	status += "Service Queue:\n"
	for roomID, service := range s.serviceQueue {
		status += fmt.Sprintf("Room %d: Speed=%s, Duration=%.2fs, Target=%.1f°C\n",
			roomID,
			service.Speed,
			service.Duration,
			service.TargetTemp)
	}

	status += "\nWait Queue:\n"
	for roomID, wait := range s.waitQueue {
		status += fmt.Sprintf("Room %d: Speed=%s, WaitTime=%.2fs, Target=%.1f°C\n",
			roomID,
			wait.Speed,
			wait.WaitDuration,
			wait.TargetTemp)
	}

	logger.Debug(status)
}

// 停止调度器
func (s *Scheduler) Stop() {
	close(s.stopChan)
}
