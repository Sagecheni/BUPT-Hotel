package scheduler

import "time"

// Speed Constants
const (
	SpeedLow    = "low"
	SpeedMedium = "medium"
	SpeedHigh   = "high"
)

// Scheduler Constants
const (
	MaxServices = 3  // 最大服务数量
	WaitTime    = 20 // 基础等待时间(秒)
)

// Temperature Change Rates
const (
	TempChangeRateLow    = 0.03333 // 低速温度变化率
	TempChangeRateMedium = 0.05    // 中速温度变化率
	TempChangeRateHigh   = 0.1     // 高速温度变化率
	TempThreshold        = 0.1     // 温度变化阈值
)

// ServiceRequest 定义服务请求
type ServiceRequest struct {
	RoomID      int
	Speed       string
	TargetTemp  float32
	CurrentTemp float32
	RequestTime time.Time
}

// ServiceItem 定义服务项
type ServiceItem struct {
	RoomID      int
	StartTime   time.Time
	Speed       string
	Duration    float32
	TargetTemp  float32
	CurrentTemp float32
	IsCompleted bool
}

// WaitItem 定义等待项
type WaitItem struct {
	RoomID       int
	RequestTime  time.Time
	Speed        string
	WaitDuration float32
	TargetTemp   float32
	CurrentTemp  float32
	Priority     int
}

// DefaultConfig 定义默认配置
type DefaultConfig struct {
	DefaultSpeed string  `json:"default_speed"`
	DefaultTemp  float32 `json:"default_temp"`
}

// SpeedPriorityMap 定义风速优先级映射
var SpeedPriorityMap = map[string]int{
	SpeedLow:    1,
	SpeedMedium: 2,
	SpeedHigh:   3,
}

// SpeedTempRateMap 定义风速温度变化率映射
var SpeedTempRateMap = map[string]float32{
	SpeedLow:    TempChangeRateLow,
	SpeedMedium: TempChangeRateMedium,
	SpeedHigh:   TempChangeRateHigh,
}
