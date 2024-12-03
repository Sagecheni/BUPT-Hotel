package scheduler

import (
	"backend/internal/events"
	"container/heap"
	"fmt"
	"sync"
	"time"
)

// QueueManager 队列管理器
type QueueManager struct {
	mu             sync.RWMutex
	serviceQueue   map[int]*ServiceItem
	waitQueue      *PriorityQueue
	waitQueueIndex map[int]*PriorityItem
	currentService int
	eventBus       *events.EventBus // 改为指针类型以避免复制
}

// PriorityQueue 优先级队列实现
type PriorityQueue []*PriorityItem

// PriorityItem 优先级队列项
type PriorityItem struct {
	roomID    int
	priority  int
	waitObj   *WaitItem
	indexHeap int
}

// NewQueueManager 创建新的队列管理器
func NewQueueManager(eventBus *events.EventBus) *QueueManager {
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	return &QueueManager{
		serviceQueue:   make(map[int]*ServiceItem),
		waitQueue:      &pq,
		waitQueueIndex: make(map[int]*PriorityItem),
		currentService: 0,
		eventBus:       eventBus,
	}
}

// 实现优先级队列所需的heap.Interface方法
func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	if pq[i].priority == pq[j].priority {
		return pq[i].waitObj.WaitDuration < pq[j].waitObj.WaitDuration
	}
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

// Queue Management Methods

func (qm *QueueManager) AddToServiceQueue(item *ServiceItem) bool {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if qm.currentService >= MaxServices {
		return false
	}

	qm.serviceQueue[item.RoomID] = item
	qm.currentService++
	return true
}

func (qm *QueueManager) AddToWaitQueue(waitItem *WaitItem) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	item := &PriorityItem{
		roomID:   waitItem.RoomID,
		priority: waitItem.Priority,
		waitObj:  waitItem,
	}

	heap.Push(qm.waitQueue, item)
	qm.waitQueueIndex[waitItem.RoomID] = item
}

func (qm *QueueManager) RemoveFromServiceQueue(roomID int) *ServiceItem {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if item, exists := qm.serviceQueue[roomID]; exists {
		delete(qm.serviceQueue, roomID)
		qm.currentService--
		return item
	}
	return nil
}

func (qm *QueueManager) RemoveFromWaitQueue(roomID int) *WaitItem {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if item, exists := qm.waitQueueIndex[roomID]; exists {
		heap.Remove(qm.waitQueue, item.indexHeap)
		delete(qm.waitQueueIndex, roomID)
		return item.waitObj
	}
	return nil
}

// GetServiceQueue 获取服务队列快照
func (qm *QueueManager) GetServiceQueue() map[int]*ServiceItem {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	result := make(map[int]*ServiceItem)
	for k, v := range qm.serviceQueue {
		copied := *v // 深拷贝
		copied.Duration = float32(time.Since(v.StartTime).Seconds())
		result[k] = &copied
	}
	return result
}

// GetWaitQueue 获取等待队列快照
func (qm *QueueManager) GetWaitQueue() []*WaitItem {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	var items []*WaitItem
	for _, item := range *qm.waitQueue {
		items = append(items, item.waitObj)
	}
	return items
}

// IsInService 检查房间是否在服务队列中
func (qm *QueueManager) IsInService(roomID int) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	_, exists := qm.serviceQueue[roomID]
	return exists
}

// IsWaiting 检查房间是否在等待队列中
func (qm *QueueManager) IsWaiting(roomID int) bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	_, exists := qm.waitQueueIndex[roomID]
	return exists
}

// GetServiceCount 获取当前服务队列中的服务数量
func (qm *QueueManager) GetServiceCount() int {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.currentService
}

// GetServiceItem 获取指定房间的服务项
func (qm *QueueManager) GetServiceItem(roomID int) *ServiceItem {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	if item, exists := qm.serviceQueue[roomID]; exists {
		// 返回一个副本以避免并发修改
		copied := *item
		return &copied
	}
	return nil
}

// GetWaitItem 获取指定房间的等待项
func (qm *QueueManager) GetWaitItem(roomID int) *WaitItem {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	if item, exists := qm.waitQueueIndex[roomID]; exists {
		// 返回一个副本以避免并发修改
		copied := *item.waitObj
		return &copied
	}
	return nil
}

// UpdateServiceItem 更新服务队列中的服务项
func (qm *QueueManager) UpdateServiceItem(roomID int, updater func(*ServiceItem)) bool {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if item, exists := qm.serviceQueue[roomID]; exists {
		updater(item)
		return true
	}
	return false
}

// UpdateWaitItem 更新等待队列中的等待项
func (qm *QueueManager) UpdateWaitItem(roomID int, updater func(*WaitItem)) bool {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if item, exists := qm.waitQueueIndex[roomID]; exists {
		updater(item.waitObj)
		// 可能需要重新调整堆
		heap.Fix(qm.waitQueue, item.indexHeap)
		return true
	}
	return false
}

// GetWaitQueueLength 获取等待队列长度
func (qm *QueueManager) GetWaitQueueLength() int {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.waitQueue.Len()
}

// GetServiceStatus 获取服务队列状态
func (qm *QueueManager) GetServiceStatus() map[int]ServiceStatus {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	status := make(map[int]ServiceStatus)
	for roomID, item := range qm.serviceQueue {
		status[roomID] = ServiceStatus{
			RoomID:      roomID,
			InService:   true,
			StartTime:   item.StartTime,
			Duration:    float32(time.Since(item.StartTime).Seconds()),
			Speed:       item.Speed,
			TargetTemp:  item.TargetTemp,
			CurrentTemp: item.CurrentTemp,
		}
	}
	return status
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	RoomID      int
	InService   bool
	StartTime   time.Time
	Duration    float32
	Speed       string
	TargetTemp  float32
	CurrentTemp float32
}

// ValidateRequest 验证服务请求的合法性
func (qm *QueueManager) ValidateRequest(req *ServiceRequest) error {
	// 检查风速是否合法
	if _, exists := SpeedPriorityMap[req.Speed]; !exists {
		return fmt.Errorf("invalid speed: %s", req.Speed)
	}

	// 检查温度范围是否合法
	if req.TargetTemp < 16 || req.TargetTemp > 30 {
		return fmt.Errorf("target temperature out of range: %.1f", req.TargetTemp)
	}

	return nil
}

// GetQueueMetrics 获取队列指标
func (qm *QueueManager) GetQueueMetrics() QueueMetrics {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	metrics := QueueMetrics{
		ServiceCount:    qm.currentService,
		WaitingCount:    qm.waitQueue.Len(),
		ServiceCapacity: MaxServices,
		AvgServiceTime:  qm.calculateAvgServiceTime(),
		AvgWaitTime:     qm.calculateAvgWaitTime(),
	}

	return metrics
}

// QueueMetrics 队列指标
type QueueMetrics struct {
	ServiceCount    int
	WaitingCount    int
	ServiceCapacity int
	AvgServiceTime  float32
	AvgWaitTime     float32
}

func (qm *QueueManager) calculateAvgServiceTime() float32 {
	if len(qm.serviceQueue) == 0 {
		return 0
	}

	var totalTime float32
	for _, item := range qm.serviceQueue {
		totalTime += float32(time.Since(item.StartTime).Seconds())
	}
	return totalTime / float32(len(qm.serviceQueue))
}

func (qm *QueueManager) calculateAvgWaitTime() float32 {
	waitQueue := qm.GetWaitQueue()
	if len(waitQueue) == 0 {
		return 0
	}

	var totalTime float32
	for _, item := range waitQueue {
		totalTime += item.WaitDuration
	}
	return totalTime / float32(len(waitQueue))
}
