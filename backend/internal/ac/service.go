// internal/ac/service.go

package ac

import (
	"backend/internal/logger"
	"backend/internal/service"
	"backend/internal/types"
	"fmt"
	"time"
)

// ExtendedState 扩展状态信息
type ExtendedState struct {
	RoomState     *types.State
	CentralACOn   bool
	CentralACMode types.Mode
	InService     bool
	QueueStatus   string
}

// ACService 协调 Controller 和 Scheduler
type ACService struct {
	controller Controller
	scheduler  *service.Scheduler
}

// NewACService 创建新的 ACService
func NewACService() *ACService {
	controller := NewController()
	scheduler := service.NewScheduler()
	return &ACService{
		controller: controller,
		scheduler:  scheduler,
	}
}

// StartCentralAC 启动中央空调
func (s *ACService) StartCentralAC(mode types.Mode) error {
	return s.controller.StartCentralAC(mode)
}

// StopCentralAC 关闭中央空调
func (s *ACService) StopCentralAC() error {
	s.scheduler.ClearAllQueues()
	return s.controller.StopCentralAC()
}

// SetCentralACMode 设置中央空调模式
func (s *ACService) SetCentralACMode(mode types.Mode) error {
	// 切换模式时清空所有队列
	s.scheduler.ClearAllQueues()
	return s.controller.SetCentralACMode(mode)
}

// PowerOn 开启房间空调
func (s *ACService) PowerOn(roomID int) error {
	// 检查中央空调状态
	isOn, _ := s.controller.GetCentralACState()
	if !isOn {
		return fmt.Errorf("中央空调未开启")
	}

	// 开启房间空调
	if err := s.controller.PowerOn(roomID); err != nil {
		return err
	}

	// 获取房间当前状态
	state, err := s.controller.GetState(roomID)
	if err != nil {
		// 如果获取状态失败，尝试关闭空调
		if powerOffErr := s.controller.PowerOff(roomID); powerOffErr != nil {
			logger.Error("关闭空调失败: %v", powerOffErr)
		}
		return fmt.Errorf("获取空调状态失败: %v", err)
	}

	// 获取默认配置
	config := s.controller.GetConfig()

	// 通过调度器处理请求
	inService, err := s.scheduler.HandleRequest(
		roomID,
		config.DefaultSpeed,
		config.DefaultTemp,
		state.CurrentTemp, // 使用实际的当前温度
	)
	if err != nil {
		// 如果调度失败，关闭空调
		if powerOffErr := s.controller.PowerOff(roomID); powerOffErr != nil {
			logger.Error("关闭空调失败: %v", powerOffErr)
		}
		return fmt.Errorf("调度失败: %v", err)
	}

	if !inService {
		logger.Info("房间 %d 已加入等待队列", roomID)
	}

	return nil
}

// PowerOff 关闭房间空调
func (s *ACService) PowerOff(roomID int) error {
	// 先从调度器中移除
	s.scheduler.RemoveRoom(roomID)
	// 再关闭空调
	return s.controller.PowerOff(roomID)
}

// SetTemperature 设置温度
func (s *ACService) SetTemperature(roomID int, temp float32) error {
	// 获取当前状态
	state, err := s.controller.GetState(roomID)
	if err != nil {
		return err
	}

	// 检查中央空调状态
	isOn, mode := s.controller.GetCentralACState()
	if !isOn {
		return fmt.Errorf("中央空调未开启")
	}

	// 验证温度是否在当前模式的有效范围内
	if !s.controller.IsValidTemp(mode, temp) {
		return fmt.Errorf("温度 %.1f°C 超出当前模式允许范围", temp)
	}

	// 通过调度器处理请求
	inService, err := s.scheduler.HandleRequest(
		roomID,
		state.Speed,
		temp,
		state.CurrentTemp,
	)
	if err != nil {
		return err
	}

	if !inService {
		logger.Info("房间 %d 温度调节请求已加入等待队列", roomID)
		return nil
	}

	// 如果在服务队列中，更新控制器温度
	return s.controller.SetTemperature(roomID, temp)
}

// SetFanSpeed 设置风速
func (s *ACService) SetFanSpeed(roomID int, speed types.Speed) error {
	// 检查中央空调状态
	isOn, _ := s.controller.GetCentralACState()
	if !isOn {
		return fmt.Errorf("中央空调未开启")
	}

	// 获取当前状态
	state, err := s.controller.GetState(roomID)
	if err != nil {
		return err
	}

	// 通过调度器处理请求
	inService, err := s.scheduler.HandleRequest(
		roomID,
		speed,
		state.TargetTemp,
		state.CurrentTemp,
	)
	if err != nil {
		return err
	}

	if !inService {
		logger.Info("房间 %d 风速调节请求已加入等待队列", roomID)
		return nil
	}

	// 如果在服务队列中，更新控制器风速
	return s.controller.SetFanSpeed(roomID, speed)
}

// GetState 获取空调状态（包括中央空调和房间空调状态）
func (s *ACService) GetState(roomID int) (*ExtendedState, error) {
	centralACOn, centralACMode := s.controller.GetCentralACState()
	roomState, err := s.controller.GetState(roomID)
	if err != nil {
		return nil, err
	}

	inService, queueStatus, err := s.getQueueStatus(roomID)
	if err != nil {
		return nil, err
	}

	return &ExtendedState{
		RoomState:     roomState,
		CentralACOn:   centralACOn,
		CentralACMode: centralACMode,
		InService:     inService,
		QueueStatus:   queueStatus,
	}, nil
}

// GetQueueInfo 获取队列状态
func (s *ACService) GetQueueInfo() (map[int]*service.ServiceObject, []*service.WaitObject) {
	return s.scheduler.GetServiceQueue(), s.scheduler.GetWaitQueue()
}

// getQueueStatus 获取指定房间的队列状态
func (s *ACService) getQueueStatus(roomID int) (bool, string, error) {
	serviceQueue := s.scheduler.GetServiceQueue()
	waitQueue := s.scheduler.GetWaitQueue()

	if _, inService := serviceQueue[roomID]; inService {
		return true, "服务中", nil
	}

	for _, wait := range waitQueue {
		if wait.RoomID == roomID {
			return false, fmt.Sprintf("等待中，预计等待时间 %.1f 秒", wait.WaitDuration), nil
		}
	}

	return false, "未在队列中", nil
}

// CalculateFee 计算费用
func (s *ACService) CalculateFee(roomID int) (float32, error) {
	state, err := s.controller.GetState(roomID)
	if err != nil {
		return 0, err
	}

	if !state.PowerState {
		return 0, nil
	}

	return s.controller.CalculateFee(roomID, time.Second)
}

// GetConfig 获取空调配置
func (s *ACService) GetConfig() types.Config {
	return s.controller.GetConfig()
}

// SetConfig 设置空调配置
func (s *ACService) SetConfig(config types.Config) error {
	return s.controller.SetConfig(config)
}

// GetController 获取控制器实例
func (s *ACService) GetController() Controller {
	return s.controller
}

// GetScheduler 获取调度器实例
func (s *ACService) GetScheduler() *service.Scheduler {
	return s.scheduler
}
