// internal/monitor/monitor.go

package monitor

import (
	"backend/internal/db"
	"backend/internal/events"
	"backend/internal/logger"
	"sync"
	"time"
)

// MonitorMetrics 监控指标
type MonitorMetrics struct {
	Timestamp     time.Time             `json:"timestamp"`
	ServiceQueue  map[int]*QueueMetrics `json:"service_queue"`
	WaitQueue     map[int]*QueueMetrics `json:"wait_queue"`
	RoomStates    map[int]*RoomMetrics  `json:"room_states"`
	SystemMetrics *SystemMetrics        `json:"system_metrics"`
}

// QueueMetrics 队列指标
type QueueMetrics struct {
	RoomID       int       `json:"room_id"`
	EnterTime    time.Time `json:"enter_time"`
	Duration     float32   `json:"duration"`
	TargetTemp   float32   `json:"target_temp"`
	CurrentTemp  float32   `json:"current_temp"`
	Speed        string    `json:"speed"`
	Priority     int       `json:"priority,omitempty"`
	WaitDuration float32   `json:"wait_duration,omitempty"`
}

// RoomMetrics 房间指标
type RoomMetrics struct {
	RoomID      int       `json:"room_id"`
	IsOccupied  bool      `json:"is_occupied"`
	ACState     bool      `json:"ac_state"`
	Mode        string    `json:"mode"`
	CurrentTemp float32   `json:"current_temp"`
	TargetTemp  float32   `json:"target_temp"`
	Speed       string    `json:"speed"`
	LastUpdate  time.Time `json:"last_update"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	TotalRooms         int     `json:"total_rooms"`
	ActiveRooms        int     `json:"active_rooms"`
	ServiceQueueLength int     `json:"service_queue_length"`
	WaitQueueLength    int     `json:"wait_queue_length"`
	AvgServiceTime     float32 `json:"avg_service_time"`
	AvgWaitTime        float32 `json:"avg_wait_time"`
	MainUnitState      bool    `json:"main_unit_state"`
}

type Monitor struct {
	mu              sync.RWMutex
	eventBus        *events.EventBus
	roomRepo        db.IRoomRepository
	serviceRepo     db.ServiceRepositoryInterface
	acConfigRepo    db.IACConfigRepository
	monitorInterval time.Duration
	metrics         *MonitorMetrics
	stopChan        chan struct{}
}

func NewMonitor(
	eventBus *events.EventBus,
	roomRepo db.IRoomRepository,
	serviceRepo db.ServiceRepositoryInterface,
	acConfigRepo db.IACConfigRepository,
	interval time.Duration,
) *Monitor {
	if interval == 0 {
		interval = 5 * time.Second // 默认5秒更新一次
	}

	return &Monitor{
		eventBus:        eventBus,
		roomRepo:        roomRepo,
		serviceRepo:     serviceRepo,
		acConfigRepo:    acConfigRepo,
		monitorInterval: interval,
		metrics: &MonitorMetrics{
			ServiceQueue: make(map[int]*QueueMetrics),
			WaitQueue:    make(map[int]*QueueMetrics),
			RoomStates:   make(map[int]*RoomMetrics),
		},
		stopChan: make(chan struct{}),
	}
}

func (m *Monitor) Start() {
	go m.run()
	logger.Info("Monitor started with interval: %v", m.monitorInterval)
}

func (m *Monitor) Stop() {
	close(m.stopChan)
	logger.Info("Monitor stopped")
}

func (m *Monitor) run() {
	ticker := time.NewTicker(m.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.updateMetrics(); err != nil {
				logger.Error("Failed to update metrics: %v", err)
			}
			m.publishMetrics()
		case <-m.stopChan:
			return
		}
	}
}

func (m *Monitor) updateMetrics() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.metrics.Timestamp = now

	// 更新服务队列状态
	serviceItems, err := m.serviceRepo.GetServiceQueueItems()
	if err != nil {
		return err
	}
	m.metrics.ServiceQueue = make(map[int]*QueueMetrics)
	for _, item := range serviceItems {
		m.metrics.ServiceQueue[item.RoomID] = &QueueMetrics{
			RoomID:      item.RoomID,
			EnterTime:   item.EnterTime,
			Duration:    float32(now.Sub(item.EnterTime).Seconds()),
			TargetTemp:  item.TargetTemp,
			CurrentTemp: item.CurrentTemp,
			Speed:       item.Speed,
		}
	}

	// 更新等待队列状态
	waitItems, err := m.serviceRepo.GetWaitQueueItems()
	if err != nil {
		return err
	}
	m.metrics.WaitQueue = make(map[int]*QueueMetrics)
	for _, item := range waitItems {
		m.metrics.WaitQueue[item.RoomID] = &QueueMetrics{
			RoomID:       item.RoomID,
			EnterTime:    item.EnterTime,
			WaitDuration: float32(now.Sub(item.EnterTime).Seconds()),
			TargetTemp:   item.TargetTemp,
			CurrentTemp:  item.CurrentTemp,
			Speed:        item.Speed,
			Priority:     item.Priority,
		}
	}

	// 更新房间状态
	rooms, err := m.roomRepo.GetAllRooms()
	if err != nil {
		return err
	}
	m.metrics.RoomStates = make(map[int]*RoomMetrics)
	var activeRooms int
	for _, room := range rooms {
		m.metrics.RoomStates[room.RoomID] = &RoomMetrics{
			RoomID:      room.RoomID,
			IsOccupied:  room.State == 1,
			ACState:     room.ACState == 1,
			Mode:        room.Mode,
			CurrentTemp: room.CurrentTemp,
			TargetTemp:  room.TargetTemp,
			Speed:       room.CurrentSpeed,
			LastUpdate:  now,
		}
		if room.ACState == 1 {
			activeRooms++
		}
	}

	// 更新系统指标
	mainUnitState, err := m.acConfigRepo.GetMainUnitState()
	if err != nil {
		return err
	}

	var avgServiceTime, avgWaitTime float32
	if len(serviceItems) > 0 {
		var totalServiceTime float32
		for _, item := range m.metrics.ServiceQueue {
			totalServiceTime += item.Duration
		}
		avgServiceTime = totalServiceTime / float32(len(serviceItems))
	}
	if len(waitItems) > 0 {
		var totalWaitTime float32
		for _, item := range m.metrics.WaitQueue {
			totalWaitTime += item.WaitDuration
		}
		avgWaitTime = totalWaitTime / float32(len(waitItems))
	}

	m.metrics.SystemMetrics = &SystemMetrics{
		TotalRooms:         len(rooms),
		ActiveRooms:        activeRooms,
		ServiceQueueLength: len(serviceItems),
		WaitQueueLength:    len(waitItems),
		AvgServiceTime:     avgServiceTime,
		AvgWaitTime:        avgWaitTime,
		MainUnitState:      mainUnitState,
	}

	return nil
}

func (m *Monitor) publishMetrics() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 打印系统概况
	logger.Info("=== System Status Report ===")
	logger.Info("Total Rooms: %d, Active Rooms: %d",
		m.metrics.SystemMetrics.TotalRooms,
		m.metrics.SystemMetrics.ActiveRooms)
	logger.Info("Service Queue Length: %d, Wait Queue Length: %d",
		m.metrics.SystemMetrics.ServiceQueueLength,
		m.metrics.SystemMetrics.WaitQueueLength)
	logger.Info("Average Service Time: %.2fs, Average Wait Time: %.2fs",
		m.metrics.SystemMetrics.AvgServiceTime,
		m.metrics.SystemMetrics.AvgWaitTime)
	logger.Info("Main Unit State: %v", m.metrics.SystemMetrics.MainUnitState)

	// 打印服务队列状态
	if len(m.metrics.ServiceQueue) > 0 {
		logger.Info("=== Service Queue Status ===")
		for roomID, metrics := range m.metrics.ServiceQueue {
			logger.Info("Room %d: Speed=%s, Current=%.1f°C, Target=%.1f°C, Duration=%.1fs",
				roomID,
				metrics.Speed,
				metrics.CurrentTemp,
				metrics.TargetTemp,
				metrics.Duration)
		}
	}

	// 打印等待队列状态
	if len(m.metrics.WaitQueue) > 0 {
		logger.Info("=== Wait Queue Status ===")
		for roomID, metrics := range m.metrics.WaitQueue {
			logger.Info("Room %d: Priority=%d, Speed=%s, Current=%.1f°C, Target=%.1f°C, Wait=%.1fs",
				roomID,
				metrics.Priority,
				metrics.Speed,
				metrics.CurrentTemp,
				metrics.TargetTemp,
				metrics.WaitDuration)
		}
	}

	// 打印活跃房间状态
	logger.Info("=== Active Rooms Status ===")
	for roomID, state := range m.metrics.RoomStates {
		if state.ACState {
			logger.Info("Room %d: Mode=%s, Speed=%s, Current=%.1f°C, Target=%.1f°C",
				roomID,
				state.Mode,
				state.Speed,
				state.CurrentTemp,
				state.TargetTemp)
		}
	}

	// 添加分隔线
	logger.Info("======================================\n")
	// 发布监控指标事件
	m.eventBus.Publish(events.Event{
		Type:      events.EventMetricsUpdate,
		Timestamp: m.metrics.Timestamp,
		Data: events.MetricsEventData{
			Timestamp:          m.metrics.Timestamp,
			TotalRooms:         m.metrics.SystemMetrics.TotalRooms,
			OccupiedRooms:      m.metrics.SystemMetrics.ActiveRooms,
			ActiveACs:          m.metrics.SystemMetrics.ServiceQueueLength,
			AvgServiceTime:     m.metrics.SystemMetrics.AvgServiceTime,
			AvgWaitTime:        m.metrics.SystemMetrics.AvgWaitTime,
			ServiceQueueLength: m.metrics.SystemMetrics.ServiceQueueLength,
			WaitQueueLength:    m.metrics.SystemMetrics.WaitQueueLength,
		},
	})

	// 发布房间状态变更事件
	for _, roomState := range m.metrics.RoomStates {
		m.eventBus.Publish(events.Event{
			Type:      events.EventRoomStateChange,
			RoomID:    roomState.RoomID,
			Timestamp: m.metrics.Timestamp,
			Data: events.RoomStateEventData{
				RoomID:      roomState.RoomID,
				State:       boolToInt(roomState.IsOccupied),
				ACState:     boolToInt(roomState.ACState),
				CurrentTemp: roomState.CurrentTemp,
				TargetTemp:  roomState.TargetTemp,
				Speed:       roomState.Speed,
				Mode:        roomState.Mode,
			},
		})
	}
}

// GetMetrics 获取当前监控指标
func (m *Monitor) GetMetrics() *MonitorMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.metrics
}

// GetRoomMetrics 获取指定房间的监控指标
func (m *Monitor) GetRoomMetrics(roomID int) *RoomMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.metrics.RoomStates[roomID]
}

// 辅助函数: bool转int
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
