// internal/ac/ac_service.go

package ac

import (
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/service"
	"backend/internal/types"
	"fmt"
	"sync"
	"time"
)

var (
	acService *ACService
	acOnce    sync.Once
)

type ACService struct {
	controller Controller
	scheduler  *service.Scheduler
	detailRepo *db.DetailRepository
	roomRepo   *db.RoomRepository
}

// GetACService 获取 ACService 单例
func GetACService() *ACService {
	acOnce.Do(func() {
		acService = &ACService{
			controller: NewController(),
			scheduler:  service.GetScheduler(), // 使用已有的调度器单例
			detailRepo: db.NewDetailRepository(),
			roomRepo:   db.NewRoomRepository(),
		}
	})
	return acService
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

	// 获取房间状态
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 创建开机详单
	if err := s.createPowerOnDetail(roomID, room.CurrentTemp); err != nil {
		return fmt.Errorf("创建开机详单失败: %v", err)
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
	// 获取房间当前状态
	state, err := s.controller.GetState(roomID)
	if err != nil {
		return fmt.Errorf("获取房间状态失败: %v", err)
	}

	// 创建关机详单
	if err := s.createPowerOffDetail(roomID, state.CurrentTemp, state.Speed); err != nil {
		return fmt.Errorf("创建关机详单失败: %v", err)
	}

	// 从调度器中移除
	s.scheduler.RemoveRoom(roomID)

	// 关闭空调
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

// GetQueueInfo 获取队列状态
func (s *ACService) GetQueueInfo() (map[int]*service.ServiceObject, []*service.WaitObject) {
	return s.scheduler.GetServiceQueue(), s.scheduler.GetWaitQueue()
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

// createPowerOnDetail 创建开机详单
func (s *ACService) createPowerOnDetail(roomID int, currentTemp float32) error {
	detail := &db.Detail{
		RoomID:      roomID,
		QueryTime:   time.Now(),
		StartTime:   time.Now(),
		EndTime:     time.Now(),
		ServeTime:   0,
		Speed:       "",
		Cost:        0,
		Rate:        0,
		TempChange:  0,
		CurrentTemp: currentTemp,
		DetailType:  db.DetailTypePowerOn,
	}

	if err := s.detailRepo.CreateDetail(detail); err != nil {
		logger.Error("创建开机详单失败 - 房间ID: %d, 错误: %v", roomID, err)
		return err
	}

	logger.Info("创建开机详单成功 - 房间ID: %d", roomID)
	return nil
}

// createPowerOffDetail 创建关机详单
func (s *ACService) createPowerOffDetail(roomID int, currentTemp float32, speed types.Speed) error {
	// 获取最后一条详单记录
	lastDetail, err := s.detailRepo.GetLatestDetail(roomID)
	if err != nil {
		logger.Error("获取最近详单失败 - 房间ID: %d, 错误: %v", roomID, err)
		return err
	}

	// 计算相关数据
	now := time.Now()
	var serveTime float32 = 0
	var tempChange float32 = 0
	var startTime time.Time = now

	if lastDetail != nil {
		serveTime = float32(now.Sub(lastDetail.StartTime).Minutes())
		tempChange = currentTemp - lastDetail.CurrentTemp
		startTime = lastDetail.StartTime
	}

	// 获取费率
	config := s.controller.GetConfig()
	rate := config.Rates[speed]

	// 计算费用
	cost := rate * float32(tempChange)
	if cost < 0 {
		cost = -cost // 确保费用为正数
	}
	cost *= serveTime // 乘以服务时长

	detail := &db.Detail{
		RoomID:      roomID,
		QueryTime:   now,
		StartTime:   startTime,
		EndTime:     now,
		ServeTime:   serveTime,
		Speed:       string(speed),
		Cost:        cost,
		Rate:        rate,
		TempChange:  tempChange,
		CurrentTemp: currentTemp,
		DetailType:  db.DetailTypePowerOff,
	}

	if err := s.detailRepo.CreateDetail(detail); err != nil {
		logger.Error("创建关机详单失败 - 房间ID: %d, 错误: %v", roomID, err)
		return err
	}

	logger.Info("创建关机详单成功 - 房间ID: %d, 服务时长: %.1f分钟, 费用: %.2f元",
		roomID, serveTime, cost)
	return nil
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
