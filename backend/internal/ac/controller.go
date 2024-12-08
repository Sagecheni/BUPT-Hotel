// internal/ac/controller.go

package ac

import (
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/types"
	"fmt"
	"math"
	"sync"
	"time"
)

// DefaultConfig 默认配置
var DefaultConfig = types.Config{
	DefaultTemp:  25.0,
	DefaultSpeed: types.SpeedMedium,
	TempRanges: map[types.Mode]types.TempRange{
		types.ModeCooling: {Min: 16, Max: 24},
		types.ModeHeating: {Min: 22, Max: 28},
	},
	Rates: map[types.Speed]float32{
		types.SpeedLow:    0.5,
		types.SpeedMedium: 1.0,
		types.SpeedHigh:   2.0,
	},
}

// ACController 实现 Controller 接口
type ACController struct {
	mu       sync.RWMutex
	config   types.Config
	roomRepo *db.RoomRepository
	// 中央空调状态
	centralACState struct {
		isOn bool
		mode types.Mode
	}
}

// NewController 创建新的空调控制器
func NewController() Controller {
	return &ACController{
		config:   DefaultConfig,
		roomRepo: db.NewRoomRepository(),
		centralACState: struct {
			isOn bool
			mode types.Mode
		}{
			isOn: false,
			mode: types.ModeCooling,
		},
	}
}

// StartCentralAC 启动中央空调
func (c *ACController) StartCentralAC(mode types.Mode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.centralACState.isOn {
		return fmt.Errorf("中央空调已经开启")
	}

	// 验证模式
	if mode != types.ModeCooling && mode != types.ModeHeating {
		return fmt.Errorf("无效的工作模式")
	}

	// 更新所有房间的工作模式
	if err := c.roomRepo.SetACMode(string(mode)); err != nil {
		return fmt.Errorf("设置工作模式失败: %v", err)
	}

	c.centralACState.isOn = true
	c.centralACState.mode = mode
	logger.Info("中央空调启动成功，工作模式：%s", mode)
	return nil
}

// StopCentralAC 关闭中央空调
func (c *ACController) StopCentralAC() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.centralACState.isOn {
		return fmt.Errorf("中央空调已经关闭")
	}

	// 关闭所有房间的空调
	rooms, err := c.roomRepo.GetOccupiedRooms()
	if err != nil {
		return fmt.Errorf("获取已入住房间失败: %v", err)
	}

	for _, room := range rooms {
		if room.ACState == 1 {
			if err := c.roomRepo.PowerOffAC(room.RoomID); err != nil {
				logger.Error("关闭房间 %d 空调失败: %v", room.RoomID, err)
			}
		}
	}

	c.centralACState.isOn = false
	logger.Info("中央空调关闭成功")
	return nil
}

// SetCentralACMode 设置中央空调模式
func (c *ACController) SetCentralACMode(mode types.Mode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	if mode != types.ModeCooling && mode != types.ModeHeating {
		return fmt.Errorf("无效的工作模式")
	}

	// 更新所有房间的工作模式
	if err := c.roomRepo.SetACMode(string(mode)); err != nil {
		return fmt.Errorf("设置工作模式失败: %v", err)
	}

	c.centralACState.mode = mode
	logger.Info("中央空调模式更改为：%s", mode)
	return nil
}

// PowerOn 开启房间空调
func (c *ACController) PowerOn(roomID int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.State != 1 {
		return fmt.Errorf("房间未入住")
	}

	if room.ACState == 1 {
		return fmt.Errorf("空调已开启")
	}

	// 使用房间当前温度作为初始温度
	if err := c.roomRepo.PowerOnAC(roomID, string(c.centralACState.mode), c.config.DefaultTemp); err != nil {
		return fmt.Errorf("开启空调失败: %v", err)
	}

	logger.Info("房间 %d 空调开机成功，当前温度：%.1f，目标温度：%.1f",
		roomID, room.CurrentTemp, c.config.DefaultTemp)
	return nil
}

// PowerOff 关闭房间空调
func (c *ACController) PowerOff(roomID int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.ACState != 1 {
		return fmt.Errorf("空调未开启")
	}

	if err := c.roomRepo.PowerOffAC(roomID); err != nil {
		return fmt.Errorf("关闭空调失败: %v", err)
	}

	logger.Info("房间 %d 空调关机成功", roomID)
	return nil
}

// SetTemperature 设置温度
func (c *ACController) SetTemperature(roomID int, temp float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.ACState != 1 {
		return fmt.Errorf("空调未开启")
	}

	if !c.IsValidTemp(types.Mode(room.Mode), temp) {
		return fmt.Errorf("温度超出有效范围")
	}

	if err := c.roomRepo.UpdateTemperature(roomID, temp); err != nil {
		return fmt.Errorf("设置温度失败: %v", err)
	}

	logger.Info("房间 %d 设置温度为 %.1f°C 成功", roomID, temp)
	return nil
}

// SetFanSpeed 设置风速
func (c *ACController) SetFanSpeed(roomID int, speed types.Speed) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.ACState != 1 {
		return fmt.Errorf("空调未开启")
	}

	if err := c.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
		return fmt.Errorf("设置风速失败: %v", err)
	}

	logger.Info("房间 %d 设置风速为 %s 成功", roomID, speed)
	return nil
}

// GetState 获取空调状态
func (c *ACController) GetState(roomID int) (*types.State, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %v", err)
	}

	state := &types.State{
		PowerState:   room.ACState == 1,
		Mode:         types.Mode(room.Mode),
		CurrentTemp:  room.CurrentTemp,
		TargetTemp:   room.TargetTemp,
		Speed:        types.Speed(room.CurrentSpeed),
		LastModified: room.CheckinTime,
	}

	return state, nil
}

// GetCentralACState 获取中央空调状态
func (c *ACController) GetCentralACState() (bool, types.Mode) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.centralACState.isOn, c.centralACState.mode
}

// IsValidTemp 检查温度是否在有效范围内
func (c *ACController) IsValidTemp(mode types.Mode, temp float32) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if tempRange, ok := c.config.TempRanges[mode]; ok {
		return temp >= tempRange.Min && temp <= tempRange.Max
	}
	return false
}

// GetConfig 获取当前配置
func (c *ACController) GetConfig() types.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// SetConfig 设置新配置
func (c *ACController) SetConfig(config types.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 验证配置
	if err := c.validateConfig(config); err != nil {
		return err
	}

	c.config = config
	logger.Info("更新空调配置成功")
	return nil
}

// CalculateFee 计算费用
func (c *ACController) CalculateFee(roomID int, duration time.Duration) (float32, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	room, err := c.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return 0, fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.CurrentSpeed == "" {
		return 0, nil
	}

	speed := types.Speed(room.CurrentSpeed)
	rate := c.config.Rates[speed]
	tempDiff := math.Abs(float64(room.CurrentTemp - room.TargetTemp))

	return rate * float32(tempDiff) * float32(duration.Seconds()), nil
}

// validateConfig 验证配置
func (c *ACController) validateConfig(config types.Config) error {
	// 验证默认温度
	if !c.IsValidTemp(types.ModeCooling, config.DefaultTemp) &&
		!c.IsValidTemp(types.ModeHeating, config.DefaultTemp) {
		return fmt.Errorf("默认温度超出有效范围")
	}

	// 验证温度范围
	for mode, tempRange := range config.TempRanges {
		if tempRange.Min >= tempRange.Max {
			return fmt.Errorf("模式 %s 的温度范围无效", mode)
		}
	}

	// 验证费率
	for speed, rate := range config.Rates {
		if rate <= 0 {
			return fmt.Errorf("风速 %s 的费率无效", speed)
		}
	}

	return nil
}

func (s *ACService) GetACStatus(roomID int) (*ACStatus, error) {
	// 获取房间状态
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 获取空调控制器状态
	state, err := s.controller.GetState(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取空调状态失败: %v", err)
	}

	// 计算当前费用
	var currentFee, totalFee float32 = 0, 0
	if room.ACState == 1 {
		// 计算当前开机的费用
		currentFee, err = s.CalculateFee(roomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		// 获取总费用(从入住时间到现在的所有费用)
		totalFee, err = s.detailRepo.GetTotalCost(roomID, room.CheckinTime, time.Now())
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	status := &ACStatus{
		CurrentTemp:  state.CurrentTemp,
		TargetTemp:   state.TargetTemp,
		CurrentSpeed: state.Speed,
		Mode:         state.Mode,
		CurrentFee:   currentFee,
		TotalFee:     totalFee,
		PowerState:   state.PowerState,
	}

	return status, nil
}
