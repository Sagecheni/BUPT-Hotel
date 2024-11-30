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
	// 温度变化速率 (每秒变化的温度)
	TempChangeRateLow    = 0.4 // 低速温度变化率
	TempChangeRateMedium = 0.5 // 中速温度变化率
	TempChangeRateHigh   = 0.6 // 高速温度变化率
)
const tempThreshold = 0.1

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

type ServiceObject struct {
	RoomID      int
	StartTime   time.Time
	Speed       string
	Duration    float32 //服务时长
	TargetTemp  float32 //目标温度
	CurrentTemp float32 //当前温度
	IsCompleted bool    //是否完成
}

type WaitObject struct {
	RoomID       int
	RequestTime  time.Time
	Speed        string
	WaitDuration float32 //等待时长
	TargetTemp   float32 //目标温度
	CurrentTemp  float32 //当前温度
}

// 调度器结构
type Scheduler struct {
	mu             sync.Mutex
	serviceQueue   map[int]*ServiceObject //服务队列
	waitQueue      map[int]*WaitObject    //等待队列
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
	go s.monitorServiceStatus() //启动监控协程
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

// 处理请求
func (s *Scheduler) HandleRequest(roomID int, speed string, targetTemp, currentTemp float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger.Debug("Handling request for Room %d with speed %s, target temperature %.1f",
		roomID, speed, targetTemp)
	s.logQueueStatus("Before handling request")

	//检查是否已在服务队列
	if service, exists := s.serviceQueue[roomID]; exists {
		logger.Info("Room %d already in service queue, updating parameters", roomID)
		// 更新服务参数
		service.Speed = speed
		service.TargetTemp = targetTemp
		// 不更新CurrentTemp，因为它应该是连续变化的
		// 不重置StartTime和Duration，保持服务时长的连续性
		return true, nil
	}

	// 检查是否在等待队列
	if _, exists := s.waitQueue[roomID]; exists {
		logger.Info("Room %d already in wait queue, updating parameters", roomID)
		// 如果新请求的优先级更高，尝试重新调度
		if s.shouldReschedule(roomID, speed) {
			delete(s.waitQueue, roomID)
			return s.schedule(roomID, speed, targetTemp, currentTemp)
		}
		// 否则只更新参数
		s.updateWaitingRequest(roomID, speed, targetTemp)
		return false, nil
	}

	//如果当前服务对象小于最大服务对象，直接分配
	if s.currentService < MaxServices {
		logger.Debug("Direct assignment available, adding Room %d to service queue", roomID)
		s.addToServiceQueue(roomID, speed, targetTemp, currentTemp)
		s.logQueueStatus("After direct assignment")
		return true, nil
	}

	//否则启动调度
	result, err := s.schedule(roomID, speed, targetTemp, currentTemp)
	if err != nil {
		logger.Error("Scheduling failed for Room %d: %v", roomID, err)
	}
	s.logQueueStatus("After scheduling")
	return result, err
}

// 调度核心
func (s *Scheduler) schedule(roomID int, speed string, targetTemp float32, currentTemp float32) (bool, error) {
	requestPriority := speedPriority[speed]
	logger.Debug("Starting scheduling for Room %d with priority %d", roomID, requestPriority)

	//1.优先级调度
	lowPriorityServices := s.findLowPriorityServices(requestPriority)
	if len(lowPriorityServices) > 0 {
		victim := s.selectVictim(lowPriorityServices)
		if victim != nil {
			logger.Info("Selected Room %d as victim for preemption", victim.RoomID)

			// 将被抢占的服务对象添加到等待队列，保留其当前状态
			s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)

			// 从服务队列中移除被抢占的对象
			delete(s.serviceQueue, victim.RoomID)
			s.currentService--

			// 将新请求加入服务队列
			s.addToServiceQueue(roomID, speed, targetTemp, currentTemp)
			logger.Debug("Room %d preempted Room %d", roomID, victim.RoomID)
			return true, nil
		}
	}

	//2.时间片调度
	logger.Debug("No low priority services found, trying time slice scheduling")
	s.addToWaitQueue(roomID, speed, targetTemp, currentTemp)
	return false, nil

}

// 时间片轮转
func (s *Scheduler) checkWaitQueue() {
	// 更新等待时间
	for roomID, wait := range s.waitQueue {
		wait.WaitDuration -= 1
		// 当等待时间到期时，尝试进行时间片轮转
		if wait.WaitDuration <= 0 {
			var longestServiceRoom int
			var maxDuration float32 = 0
			//遍历服务队列找相同风速的服务对象
			for sRoomID, service := range s.serviceQueue {
				if service.Speed == wait.Speed && service.Duration > maxDuration {
					longestServiceRoom = sRoomID
					maxDuration = service.Duration
				}
			}
			//如果找到了相同风速的服务对象
			if longestServiceRoom != 0 {
				logger.Info("Time slice rotation: Room %d replacing Room %d", roomID, longestServiceRoom)

				// 获取要被替换的服务对象
				victim := s.serviceQueue[longestServiceRoom]

				// 将被替换的服务对象放入等待队列
				s.addToWaitQueue(victim.RoomID, victim.Speed, victim.TargetTemp, victim.CurrentTemp)

				// 从服务队列中移除被替换的对象
				delete(s.serviceQueue, longestServiceRoom)
				s.currentService--

				// 将等待的请求加入服务队列
				s.addToServiceQueue(roomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp)

				// 从等待队列中移除该请求
				delete(s.waitQueue, roomID)

				logger.Debug("Time slice rotation completed")
			} else {
				// 如果没有找到相同风速的服务对象，重新分配等待时间
				wait.WaitDuration = s.calculateWaitDuration()
				logger.Info("No same speed service found for Room %d, reassigning wait duration: %.2f",
					roomID, wait.WaitDuration)
			}
		}
	}
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
		//更新服务时长
		service.Duration = float32(time.Since(service.StartTime).Seconds())

		// 计算温度变化
		tempRate := speedTempRate[service.Speed]
		tempDiff := service.TargetTemp - service.CurrentTemp

		if math.Abs(float64(tempDiff)) > tempThreshold {
			if tempDiff > 0 {
				service.CurrentTemp += tempRate
			} else {
				service.CurrentTemp -= tempRate
			}
		} else {
			//温度达到目标值
			service.CurrentTemp = service.TargetTemp
			service.IsCompleted = true
			if nextRoom := s.findShortestWaitingRequest(service.Speed); nextRoom != 0 {
				delete(s.serviceQueue, roomID)
				s.currentService--
				s.promoteFromWaitQueue(nextRoom)
			}
		}
	}
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

// 辅助函数：找出低优先级的服务对象
func (s *Scheduler) findLowPriorityServices(requestPriority int) []*ServiceObject {
	var services []*ServiceObject
	for _, service := range s.serviceQueue {
		if speedPriority[service.Speed] < requestPriority {
			services = append(services, service)
		}
	}
	return services
}

// 辅助方法：添加到服务队列
func (s *Scheduler) addToServiceQueue(roomID int, speed string, targetTemp float32, currentTemp float32) {
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
}

// 辅助方法：添加到等待队列
func (s *Scheduler) addToWaitQueue(roomID int, speed string, targetTemp, currentTemp float32) {
	waitDuration := s.calculateWaitDuration()
	s.waitQueue[roomID] = &WaitObject{
		RoomID:       roomID,
		RequestTime:  time.Now(),
		Speed:        speed,
		WaitDuration: waitDuration,
		TargetTemp:   targetTemp,
		CurrentTemp:  currentTemp, // 保存当前温度，以便恢复服务时继续从这个温度开始
	}
}

// 辅助方法：从等待队列提升到服务队列
func (s *Scheduler) promoteFromWaitQueue(roomID int) {
	if wait, exists := s.waitQueue[roomID]; exists {
		s.addToServiceQueue(roomID, wait.Speed, wait.TargetTemp, wait.CurrentTemp)
		delete(s.waitQueue, roomID)
	}
}

// 辅助方法：找到相同温度上等待时间最短的请求
func (s *Scheduler) findShortestWaitingRequest(speed string) int {
	var shortestRoom int
	shortestWait := float32(math.MaxFloat32)

	// 先找相同风速的请求
	for roomID, wait := range s.waitQueue {
		if wait.Speed == speed && wait.WaitDuration < shortestWait {
			shortestWait = wait.WaitDuration
			shortestRoom = roomID
		}
	}

	return shortestRoom
}

// 判断是否需要重新调度
func (s *Scheduler) shouldReschedule(roomID int, newSpeed string) bool {
	waitObj := s.waitQueue[roomID]
	oldPriority := speedPriority[waitObj.Speed]
	newPriority := speedPriority[newSpeed]
	return newPriority > oldPriority
}

// 更新等待队列中的请求
func (s *Scheduler) updateWaitingRequest(roomID int, speed string, targetTemp float32) {
	wait := s.waitQueue[roomID]
	wait.Speed = speed
	wait.TargetTemp = targetTemp
	// 如果改变了优先级，可能需要调整等待时间
	if speedPriority[speed] != speedPriority[wait.Speed] {
		wait.WaitDuration = s.calculateWaitDuration()
	}
}

// 辅助方法：获取服务队列
func (s *Scheduler) GetServiceQueue() map[int]*ServiceObject {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serviceQueue
}

// 辅助方法：获取等待队列
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
		status += fmt.Sprintf("Room %d: Speed=%s, Duration=%.2fs, Current=%.1f°C, Target=%.1f°C\n",
			roomID,
			service.Speed,
			service.Duration,
			service.CurrentTemp,
			service.TargetTemp)
	}

	status += "\nWait Queue:\n"
	for roomID, wait := range s.waitQueue {
		status += fmt.Sprintf("Room %d: Speed=%s, WaitTime=%.2fs, Current=%.1f°C, Target=%.1f°C\n",
			roomID,
			wait.Speed,
			wait.WaitDuration,
			wait.CurrentTemp,
			wait.TargetTemp)
	}

	logger.Debug(status)
}

// 停止调度器
func (s *Scheduler) Stop() {
	close(s.stopChan)
}
