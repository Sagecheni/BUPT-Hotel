// internal/service/ac_service.go

package service

import (
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/types"
	"fmt"
	"sync"
)

// DefaultConfig 默认空调配置
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

var (
	acService *ACService
	acOnce    sync.Once
)

// ACService 集成空调控制和服务功能
type ACService struct {
	mu         sync.RWMutex
	config     types.Config
	roomRepo   *db.RoomRepository
	detailRepo *db.DetailRepository
	scheduler  *Scheduler
	billing    *BillingService

	// 中央空调状态
	centralACState struct {
		isOn bool
		mode types.Mode
	}
}

// ACStatus 空调状态结构体
type ACStatus struct {
	CurrentTemp  float32
	TargetTemp   float32
	CurrentSpeed types.Speed
	Mode         types.Mode
	CurrentFee   float32
	TotalFee     float32
	PowerState   bool
}

// GetACService 获取 ACService 单例
func GetACService() *ACService {
	acOnce.Do(func() {
		scheduler := GetScheduler()
		acService = &ACService{
			config:     DefaultConfig,
			roomRepo:   db.NewRoomRepository(),
			detailRepo: db.NewDetailRepository(),
			scheduler:  scheduler,
			billing:    GetBillingService(),
			centralACState: struct {
				isOn bool
				mode types.Mode
			}{
				isOn: false,
				mode: types.ModeCooling,
			},
		}
	})
	return acService
}

// StartCentralAC 启动中央空调
func (s *ACService) StartCentralAC(mode types.Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.centralACState.isOn {
		return fmt.Errorf("中央空调已经开启")
	}

	if mode != types.ModeCooling && mode != types.ModeHeating {
		return fmt.Errorf("无效的工作模式")
	}

	if err := s.roomRepo.SetACMode(string(mode)); err != nil {
		return fmt.Errorf("设置工作模式失败: %v", err)
	}

	s.centralACState.isOn = true
	s.centralACState.mode = mode
	StartMonitorService()
	logger.Info("中央空调启动成功，工作模式：%s", mode)
	return nil
}

// StopCentralAC 关闭中央空调
func (s *ACService) StopCentralAC() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.centralACState.isOn {
		return fmt.Errorf("中央空调已经关闭")
	}

	rooms, err := s.roomRepo.GetOccupiedRooms()
	if err != nil {
		return fmt.Errorf("获取已入住房间失败: %v", err)
	}

	for _, room := range rooms {
		if room.ACState == 1 {
			if err := s.PowerOff(room.RoomID); err != nil {
				logger.Error("关闭房间 %d 空调失败: %v", room.RoomID, err)
			}
		}
	}

	s.scheduler.ClearAllQueues()
	s.centralACState.isOn = false
	logger.Info("中央空调关闭成功")
	return nil
}

// SetCentralACMode 设置中央空调模式
func (s *ACService) SetCentralACMode(mode types.Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	if mode != types.ModeCooling && mode != types.ModeHeating {
		return fmt.Errorf("无效的工作模式")
	}

	if err := s.roomRepo.SetACMode(string(mode)); err != nil {
		return fmt.Errorf("设置工作模式失败: %v", err)
	}

	s.scheduler.ClearAllQueues()
	s.centralACState.mode = mode
	logger.Info("中央空调模式更改为：%s", mode)
	return nil
}

// PowerOn 开启房间空调
func (s *ACService) PowerOn(roomID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.State != 1 {
		return fmt.Errorf("房间未入住")
	}

	if room.ACState == 1 {
		return fmt.Errorf("空调已开启")
	}

	if err := s.roomRepo.PowerOnAC(roomID, string(s.centralACState.mode), s.config.DefaultTemp); err != nil {
		return fmt.Errorf("开启空调失败: %v", err)
	}

	inService, err := s.scheduler.HandleRequest(
		roomID,
		s.config.DefaultSpeed,
		s.config.DefaultTemp,
		room.CurrentTemp,
	)
	if err != nil {
		return fmt.Errorf("调度失败: %v", err)
	}

	if !inService {
		logger.Info("房间 %d 已加入等待队列", roomID)
	}

	return nil
}

// PowerOff 关闭房间空调
func (s *ACService) PowerOff(roomID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间状态失败: %v", err)
	}

	s.scheduler.RemoveRoom(roomID)

	if err := s.roomRepo.PowerOffAC(roomID); err != nil {
		return fmt.Errorf("关闭空调失败: %v", err)
	}

	logger.Info("房间 %d 空调关机成功", roomID)
	return nil
}

// SetTemperature 设置目标温度
func (s *ACService) SetTemperature(roomID int, targetTemp float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.ACState != 1 {
		return fmt.Errorf("空调未开启")
	}

	if !s.isValidTemp(types.Mode(room.Mode), targetTemp) {
		return fmt.Errorf("温度 %.1f°C 超出当前模式允许范围", targetTemp)
	}

	// 更新房间的目标温度
	if err := s.roomRepo.UpdateRoom(&db.RoomInfo{
		RoomID:     roomID,
		TargetTemp: targetTemp,
	}); err != nil {
		return fmt.Errorf("更新目标温度失败: %v", err)
	}

	// 将温度调节请求发送给调度器
	inService, err := s.scheduler.HandleRequest(
		roomID,
		types.Speed(room.CurrentSpeed),
		targetTemp,
		room.CurrentTemp,
	)
	if err != nil {
		return fmt.Errorf("处理温度调节请求失败: %v", err)
	}

	if !inService {
		logger.Info("房间 %d 温度调节请求已加入等待队列 (目标温度: %.1f°C)",
			roomID, targetTemp)
		return nil
	}

	logger.Info("房间 %d 温度调节请求已开始处理 (目标温度: %.1f°C)",
		roomID, targetTemp)
	return nil
}

// SetFanSpeed 设置风速
func (s *ACService) SetFanSpeed(roomID int, speed types.Speed) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.centralACState.isOn {
		return fmt.Errorf("中央空调未开启")
	}

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	if room.ACState != 1 {
		return fmt.Errorf("空调未开启")
	}

	inService, err := s.scheduler.HandleRequest(
		roomID,
		speed,
		room.TargetTemp,
		room.CurrentTemp,
	)
	if err != nil {
		return err
	}

	if !inService {
		logger.Info("房间 %d 风速调节请求已加入等待队列", roomID)
		return nil
	}

	if err := s.roomRepo.UpdateSpeed(roomID, string(speed)); err != nil {
		return fmt.Errorf("设置风速失败: %v", err)
	}

	logger.Info("房间 %d 设置风速为 %s 成功", roomID, speed)
	return nil
}

// GetACStatus 获取空调状态
func (s *ACService) GetACStatus(roomID int) (*ACStatus, error) {
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %v", err)
	}

	var currentFee, totalFee float32 = 0, 0
	if room.ACState == 1 {
		// 获取当前费用
		currentFee, err = s.billing.CalculateCurrentSessionFee(roomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		// 获取总费用
		totalFee, err = s.billing.CalculateTotalFee(roomID)
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	status := &ACStatus{
		CurrentTemp:  room.CurrentTemp,
		TargetTemp:   room.TargetTemp,
		CurrentSpeed: types.Speed(room.CurrentSpeed),
		Mode:         types.Mode(room.Mode),
		CurrentFee:   currentFee,
		TotalFee:     totalFee,
		PowerState:   room.ACState == 1,
	}

	return status, nil
}

// GetCentralACState 获取中央空调状态
func (s *ACService) GetCentralACState() (bool, types.Mode) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.centralACState.isOn, s.centralACState.mode
}

// GetConfig 获取空调配置
func (s *ACService) GetConfig() types.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetConfig 设置空调配置的方法已存在，但我们需要确保它能正确处理温度范围的更新
func (s *ACService) SetConfig(config types.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证配置
	if err := s.validateConfig(config); err != nil {
		return err
	}

	// 更新配置
	s.config = config
	logger.Info("空调配置已更新")

	// 检查所有房间的目标温度是否在新范围内
	rooms, err := s.roomRepo.GetOccupiedRooms()
	if err != nil {
		logger.Error("获取已入住房间失败: %v", err)
		return err
	}

	// 遍历所有房间，将超出范围的目标温度调整到范围内
	for _, room := range rooms {
		if room.ACState == 1 {
			currentMode := types.Mode(room.Mode)
			tempRange := config.TempRanges[currentMode]

			if room.TargetTemp < tempRange.Min {
				if err := s.SetTemperature(room.RoomID, tempRange.Min); err != nil {
					logger.Error("调整房间 %d 温度失败: %v", room.RoomID, err)
				}
			} else if room.TargetTemp > tempRange.Max {
				if err := s.SetTemperature(room.RoomID, tempRange.Max); err != nil {
					logger.Error("调整房间 %d 温度失败: %v", room.RoomID, err)
				}
			}
		}
	}

	return nil
}

// 内部辅助方法

func (s *ACService) isValidTemp(mode types.Mode, temp float32) bool {
	if tempRange, ok := s.config.TempRanges[mode]; ok {
		return temp >= tempRange.Min && temp <= tempRange.Max
	}
	return false
}

func (s *ACService) validateConfig(config types.Config) error {
	// 验证默认温度
	if !s.isValidTemp(types.ModeCooling, config.DefaultTemp) &&
		!s.isValidTemp(types.ModeHeating, config.DefaultTemp) {
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

// GetQueueInfo 获取队列状态
func (s *ACService) GetQueueInfo() (map[int]*ServiceObject, []*WaitObject) {
	return s.scheduler.GetServiceQueue(), s.scheduler.GetWaitQueue()
}

// GetScheduler 获取调度器实例
func (s *ACService) GetScheduler() *Scheduler {
	return s.scheduler
}

// 以下是一些用于测试和调试的辅助方法

// ResetState 重置服务状态（仅用于测试）
func (s *ACService) ResetState() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.centralACState.isOn = false
	s.centralACState.mode = types.ModeCooling
	s.config = DefaultConfig
	s.scheduler.ClearAllQueues()
}

// SetLogging 设置是否启用服务日志
func (s *ACService) SetLogging(enable bool) {
	s.scheduler.SetLogging(enable)
}

// GetRoomRepo 获取房间存储库（用于测试）
func (s *ACService) GetRoomRepo() *db.RoomRepository {
	return s.roomRepo
}

// GetDetailRepo 获取详单存储库（用于测试）
func (s *ACService) GetDetailRepo() *db.DetailRepository {
	return s.detailRepo
}
