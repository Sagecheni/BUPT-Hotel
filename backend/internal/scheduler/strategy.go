package scheduler

import (
	"math"
	"time"
)

// SchedulingStrategy 调度策略接口
type SchedulingStrategy interface {
	// Schedule 执行调度,返回是否需要调度切换和被调度的房间ID
	Schedule(request *ServiceRequest, qm *QueueManager) (bool, int)
	// CalculateWaitTime 计算等待时间
	CalculateWaitTime(queueLength int) float32
}

// PriorityStrategy 优先级调度策略
type PriorityStrategy struct{}

// TimeSliceStrategy 时间片轮转策略
type TimeSliceStrategy struct{}

// CompositeStrategy 组合策略(优先级+时间片)
type CompositeStrategy struct {
	priority  *PriorityStrategy
	timeSlice *TimeSliceStrategy
}

// NewCompositeStrategy 创建新的组合策略
func NewCompositeStrategy() *CompositeStrategy {
	return &CompositeStrategy{
		priority:  &PriorityStrategy{},
		timeSlice: &TimeSliceStrategy{},
	}
}

// Schedule 实现组合调度策略
func (cs *CompositeStrategy) Schedule(request *ServiceRequest, qm *QueueManager) (bool, int) {
	requestPriority := SpeedPriorityMap[request.Speed]

	serviceQueue := qm.GetServiceQueue()

	// 1. 优先级调度
	// 查找所有优先级低于请求的服务
	lowPriorityServices := make([]*ServiceItem, 0)
	for roomID, service := range serviceQueue {
		if SpeedPriorityMap[service.Speed] < requestPriority {
			serviceCopy := *service
			service.RoomID = roomID // 设置房间ID
			lowPriorityServices = append(lowPriorityServices, &serviceCopy)
		}
	}

	// 如果存在低优先级服务，执行优先级调度
	if len(lowPriorityServices) > 0 {
		victim := cs.selectVictim(lowPriorityServices)
		return true, victim.RoomID
	}

	// 2. 时间片调度
	// 如果所有服务优先级相同，使用时间片策略
	samePriorityServices := make([]*ServiceItem, 0)
	for roomID, service := range serviceQueue {
		if SpeedPriorityMap[service.Speed] == requestPriority {
			serviceCopy := *service
			service.RoomID = roomID
			samePriorityServices = append(samePriorityServices, &serviceCopy)
		}
	}

	if len(samePriorityServices) > 0 {
		longestRunning := cs.findLongestRunning(samePriorityServices)
		return true, longestRunning.RoomID
	}

	return false, 0
}

// selectVictim 选择优先级最低且运行时间最长的服务
func (cs *CompositeStrategy) selectVictim(candidates []*ServiceItem) *ServiceItem {
	var victim *ServiceItem
	lowestPriority := math.MaxInt32
	longestDuration := float32(0)

	// 首先找出最低优先级
	for _, service := range candidates {
		priority := SpeedPriorityMap[service.Speed]
		if priority < lowestPriority {
			lowestPriority = priority
		}
	}

	// 在最低优先级中选择运行时间最长的
	for _, service := range candidates {
		if SpeedPriorityMap[service.Speed] == lowestPriority && service.Duration > longestDuration {
			longestDuration = service.Duration
			victim = service
		}
	}

	return victim
}

// findLongestRunning 找出运行时间最长的服务
func (cs *CompositeStrategy) findLongestRunning(services []*ServiceItem) *ServiceItem {
	var longest *ServiceItem
	maxDuration := float32(0)

	for _, service := range services {
		duration := float32(time.Since(service.StartTime).Seconds())
		if duration > maxDuration {
			maxDuration = duration
			longest = service
		}
	}

	return longest
}

// CalculateWaitTime 计算等待时间
func (cs *CompositeStrategy) CalculateWaitTime(queueLength int) float32 {
	// 基础等待时间
	baseWaitTime := float32(WaitTime)

	// 根据队列长度调整等待时间，避免等待时间过长
	if queueLength > 0 {
		// 每多一个等待者，增加50%的基础等待时间
		return baseWaitTime * (1 + float32(queueLength)*0.5)
	}

	return baseWaitTime
}

// IsHigherPriority 检查是否有更高优先级
func (cs *CompositeStrategy) IsHigherPriority(newSpeed string, currentSpeed string) bool {
	return SpeedPriorityMap[newSpeed] > SpeedPriorityMap[currentSpeed]
}

// ShouldPreempt 判断是否应该进行抢占
func (cs *CompositeStrategy) ShouldPreempt(request *ServiceRequest, service *ServiceItem) bool {
	requestPriority := SpeedPriorityMap[request.Speed]
	servicePriority := SpeedPriorityMap[service.Speed]

	// 如果请求优先级更高，应该抢占
	if requestPriority > servicePriority {
		return true
	}

	// 如果优先级相同，检查服务时间
	if requestPriority == servicePriority {
		// 如果当前服务运行时间超过时间片，应该切换
		return service.Duration >= float32(WaitTime)
	}

	return false
}

// GetNextFromWaitQueue 从等待队列中获取下一个要服务的请求
func (cs *CompositeStrategy) GetNextFromWaitQueue(qm *QueueManager) *WaitItem {
	waitQueue := qm.GetWaitQueue()
	if len(waitQueue) == 0 {
		return nil
	}

	// 找出优先级最高的请求
	var highest *WaitItem
	highestPriority := -1

	for _, item := range waitQueue {
		priority := SpeedPriorityMap[item.Speed]
		if priority > highestPriority {
			highestPriority = priority
			highest = item
		}
	}

	return highest
}
