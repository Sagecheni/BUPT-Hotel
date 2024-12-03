package events

import "time"

// EventType 事件类型定义
type EventType int

const (
	// 系统事件 (0-9)
	EventSystemStartup EventType = iota
	EventSystemShutdown
	EventConfigChanged

	// 空调控制事件 (10-29)
	EventPowerOn
	EventPowerOff
	EventTemperatureChange
	EventSpeedChange
	EventModeChange
	EventTargetTempReached

	// 房间状态事件 (30-49)
	EventRoomCheckIn
	EventRoomCheckOut
	EventRoomStateChange
	EventRoomTempUpdate

	// 调度事件 (50-79)
	EventServiceRequest   // 服务请求
	EventServiceStart     // 服务开始
	EventServiceComplete  // 服务完成
	EventServicePreempted // 服务被抢占
	EventServicePaused    // 服务暂停
	EventServiceResumed   // 服务恢复

	// 队列事件 (80-99)
	EventAddToWaitQueue
	EventRemoveFromWaitQueue
	EventQueueStatusChange
	EventSchedulerStatusChange

	// 监控事件 (100-119)
	EventMetricsUpdate
	EventResourceUsageUpdate
	EventPerformanceAlert
)

// Event 事件结构
type Event struct {
	Type      EventType   `json:"type"`
	RoomID    int         `json:"room_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Handler 事件处理函数类型
type Handler func(Event)

// Subscription 事件订阅信息
type Subscription struct {
	EventType EventType
	Handler   Handler
}

// 服务相关数据结构
type ServiceRequest struct {
	RoomID      int       `json:"room_id"`
	RequestTime time.Time `json:"request_time"`
	Speed       string    `json:"speed"`
	TargetTemp  float32   `json:"target_temp"`
	CurrentTemp float32   `json:"current_temp"`
	Priority    int       `json:"priority"`
}

type ServiceEventData struct {
	RoomID      int       `json:"room_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Duration    float32   `json:"duration"`
	Speed       string    `json:"speed"`
	TargetTemp  float32   `json:"target_temp"`
	CurrentTemp float32   `json:"current_temp"`
	IsCompleted bool      `json:"is_completed"`
	Reason      string    `json:"reason"`
}

type WaitQueueEventData struct {
	RoomID       int       `json:"room_id"`
	RequestTime  time.Time `json:"request_time"`
	Speed        string    `json:"speed"`
	WaitDuration float32   `json:"wait_duration"`
	Position     int       `json:"position"`
	Priority     int       `json:"priority"`
	TargetTemp   float32   `json:"target_temp"`
	CurrentTemp  float32   `json:"current_temp"`
}

type SchedulerStatusData struct {
	Timestamp         time.Time              `json:"timestamp"`
	ServiceCount      int                    `json:"service_count"`
	WaitingCount      int                    `json:"waiting_count"`
	ServiceQueue      map[string]interface{} `json:"service_queue"`
	WaitQueue         []interface{}          `json:"wait_queue"`
	TotalRequests     int64                  `json:"total_requests"`
	CompletedRequests int64                  `json:"completed_requests"`
}

// 温度控制相关数据结构
type TemperatureEventData struct {
	RoomID          int     `json:"room_id"`
	PreviousTemp    float32 `json:"previous_temp"`
	CurrentTemp     float32 `json:"current_temp"`
	TargetTemp      float32 `json:"target_temp"`
	Speed           string  `json:"speed"`
	Mode            string  `json:"mode"`
	ChangeRate      float32 `json:"change_rate"`
	TimeSinceUpdate float32 `json:"time_since_update"`
}

// 房间状态相关数据结构
type RoomStateEventData struct {
	RoomID       int       `json:"room_id"`
	State        int       `json:"state"`
	ClientID     string    `json:"client_id,omitempty"`
	ClientName   string    `json:"client_name,omitempty"`
	CheckInTime  time.Time `json:"check_in_time,omitempty"`
	CheckOutTime time.Time `json:"check_out_time,omitempty"`
	ACState      int       `json:"ac_state"`
	CurrentTemp  float32   `json:"current_temp"`
	TargetTemp   float32   `json:"target_temp"`
	Speed        string    `json:"speed"`
	Mode         string    `json:"mode"`
}

// 系统配置相关数据结构
type ConfigEventData struct {
	DefaultSpeed    string   `json:"default_speed"`
	DefaultTemp     float32  `json:"default_temp"`
	MaxServices     int      `json:"max_services"`
	BaseWaitTime    float32  `json:"base_wait_time"`
	TempThreshold   float32  `json:"temp_threshold"`
	ServiceTimeout  float32  `json:"service_timeout"`
	MetricsInterval int      `json:"metrics_interval"`
	ChangedSettings []string `json:"changed_settings"`
}

// 监控指标相关数据结构
type MetricsEventData struct {
	Timestamp          time.Time `json:"timestamp"`
	TotalRooms         int       `json:"total_rooms"`
	OccupiedRooms      int       `json:"occupied_rooms"`
	ActiveACs          int       `json:"active_acs"`
	AvgServiceTime     float32   `json:"avg_service_time"`
	AvgWaitTime        float32   `json:"avg_wait_time"`
	ServiceQueueLength int       `json:"service_queue_length"`
	WaitQueueLength    int       `json:"wait_queue_length"`
	TotalRequests      int64     `json:"total_requests"`
	CompletedRequests  int64     `json:"completed_requests"`
	PreemptedServices  int64     `json:"preempted_services"`
	ResourceUsage      struct {
		CPUUsage    float32 `json:"cpu_usage"`
		MemoryUsage float32 `json:"memory_usage"`
		DiskUsage   float32 `json:"disk_usage"`
	} `json:"resource_usage"`
}

// EventNames 提供事件类型的字符串表示
var EventNames = map[EventType]string{
	EventSystemStartup:         "SystemStartup",
	EventSystemShutdown:        "SystemShutdown",
	EventConfigChanged:         "ConfigChanged",
	EventPowerOn:               "PowerOn",
	EventPowerOff:              "PowerOff",
	EventTemperatureChange:     "TemperatureChange",
	EventSpeedChange:           "SpeedChange",
	EventModeChange:            "ModeChange",
	EventTargetTempReached:     "TargetTempReached",
	EventRoomCheckIn:           "RoomCheckIn",
	EventRoomCheckOut:          "RoomCheckOut",
	EventRoomStateChange:       "RoomStateChange",
	EventRoomTempUpdate:        "RoomTempUpdate",
	EventServiceRequest:        "ServiceRequest",
	EventServiceStart:          "ServiceStart",
	EventServiceComplete:       "ServiceComplete",
	EventServicePreempted:      "ServicePreempted",
	EventServicePaused:         "ServicePaused",
	EventServiceResumed:        "ServiceResumed",
	EventAddToWaitQueue:        "AddToWaitQueue",
	EventRemoveFromWaitQueue:   "RemoveFromWaitQueue",
	EventQueueStatusChange:     "QueueStatusChange",
	EventSchedulerStatusChange: "SchedulerStatusChange",
	EventMetricsUpdate:         "MetricsUpdate",
	EventResourceUsageUpdate:   "ResourceUsageUpdate",
	EventPerformanceAlert:      "PerformanceAlert",
}
