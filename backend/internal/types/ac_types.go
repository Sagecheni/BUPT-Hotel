// internal/types/ac_types.go

package types

import "time"

// Mode 空调工作模式
type Mode string

const (
	ModeCooling Mode = "cooling"
	ModeHeating Mode = "heating"
)

// Speed 风速
type Speed string

const (
	SpeedLow    Speed = "low"
	SpeedMedium Speed = "medium"
	SpeedHigh   Speed = "high"
)

// State 空调状态
type State struct {
	PowerState   bool      // 开关状态
	Mode         Mode      // 当前模式
	CurrentTemp  float32   // 当前温度
	TargetTemp   float32   // 目标温度
	Speed        Speed     // 当前风速
	LastModified time.Time // 最后修改时间
}

// TempRange 温度范围
type TempRange struct {
	Min float32
	Max float32
}

// Config 空调配置
type Config struct {
	DefaultTemp  float32            // 默认温度
	DefaultSpeed Speed              // 默认风速
	TempRanges   map[Mode]TempRange // 不同模式的温度范围
	Rates        map[Speed]float32  // 不同风速的费率
}
