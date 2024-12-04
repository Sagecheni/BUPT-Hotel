// internal/ac/ac.go

package ac

import (
	"backend/internal/db"
	"backend/internal/events"
	"backend/internal/logger"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	ModeCooling  = "cooling"
	ModeHeating  = "heating"
	DefaultSpeed = "medium" // 缺省风速
)

// ACService 定义空调服务接口
type ACService interface {
	// PowerOn 开机
	PowerOn(roomID int) error
	// PowerOff 关机
	PowerOff(roomID int) error
	// SetTemperature 设置温度
	SetTemperature(roomID int, targetTemp float32) error
	// SetFanSpeed 设置风速
	SetFanSpeed(roomID int, speed string) error
	// GetACState 获取空调状态
	GetACState(roomID int) (*ACState, error)
	// SetMode 设置工作模式（制冷/制热）
	SetMode(mode string) error

	// 新增管理员方法
	PowerOnMainUnit() error                                                       // 开启中央空调
	PowerOffMainUnit() error                                                      // 关闭中央空调
	GetMainUnitState() (bool, error)                                              // 获取中央空调状态
	SetTemperatureRange(mode string, minTemp, maxTemp, defaultTemp float32) error // 设置温度范围
	GetTemperatureRange(mode string) (*TempRange, error)                          // 获取温度范围配置
}

// ACState 空调状态
type ACState struct {
	RoomID      int     `json:"room_id"`
	IsOn        bool    `json:"is_on"`
	Mode        string  `json:"mode"`
	CurrentTemp float32 `json:"current_temp"`
	TargetTemp  float32 `json:"target_temp"`
	Speed       string  `json:"speed"`
	MainUnitOn  bool    `json:"main_unit_on"` // 主机状态
}

// TempRange 温度范围配置
type TempRange struct {
	Mode        string  `json:"mode"`
	MinTemp     float32 `json:"min_temp"`
	MaxTemp     float32 `json:"max_temp"`
	DefaultTemp float32 `json:"default_temp"`
}

type acService struct {
	mu          sync.RWMutex
	roomRepo    db.IRoomRepository
	eventBus    *events.EventBus
	serviceRepo db.ServiceRepositoryInterface
	configRepo  db.IACConfigRepository // 配置仓库接口
}

// NewACService 创建新的空调服务实例

func NewACService(
	roomRepo db.IRoomRepository,
	eventBus *events.EventBus,
	serviceRepo db.ServiceRepositoryInterface,
	configRepo db.IACConfigRepository,
) ACService {
	service := &acService{
		roomRepo:    roomRepo,
		eventBus:    eventBus,
		serviceRepo: serviceRepo,
		configRepo:  configRepo,
	}

	// 订阅温度变化事件
	eventBus.Subscribe(events.EventTemperatureChange, service.handleTemperatureChange)

	return service
}

func (s *acService) handleTemperatureChange(e events.Event) {
	data := e.Data.(events.TemperatureEventData)

	// 更新房间温度
	if err := s.roomRepo.UpdateTemperature(data.RoomID, data.CurrentTemp); err != nil {
		logger.Error("Failed to update room temperature: %v", err)
		return
	}

	// 如果达到目标温度，可以在这里处理相关逻辑
	if math.Abs(float64(data.CurrentTemp-data.TargetTemp)) <= 0.1 {
		// 处理达到目标温度的情况
		s.eventBus.Publish(events.Event{
			Type:      events.EventTargetTempReached,
			RoomID:    data.RoomID,
			Timestamp: time.Now(),
			Data:      data,
		})
	}
}

// PowerOnMainUnit 开启中央空调
func (s *acService) PowerOnMainUnit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 更新中央空调状态
	if err := s.configRepo.SetMainUnitState(true); err != nil {
		return err
	}

	// 发布中央空调开机事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventSystemStartup,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"main_unit": "on",
		},
	})

	return nil
}

// PowerOffMainUnit 关闭中央空调
func (s *acService) PowerOffMainUnit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有房间的空调
	rooms, err := s.roomRepo.GetAllRooms()
	if err != nil {
		return err
	}

	for _, room := range rooms {
		if room.ACState == 1 {
			if err := s.PowerOff(room.RoomID); err != nil {
				logger.Error("Failed to power off AC in room %d: %v", room.RoomID, err)
				// Continue with other rooms even if one fails
				continue
			}
		}
	}

	// 更新中央空调状态
	if err := s.configRepo.SetMainUnitState(false); err != nil {
		return err
	}

	// 发布中央空调关机事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventSystemShutdown,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"main_unit": "off",
		},
	})

	return nil
}

// GetMainUnitState 获取中央空调状态
func (s *acService) GetMainUnitState() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.configRepo.GetMainUnitState()
}

// SetTemperatureRange 设置温度范围
func (s *acService) SetTemperatureRange(mode string, minTemp, maxTemp, defaultTemp float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mode != ModeCooling && mode != ModeHeating {
		return errors.New("invalid mode")
	}

	if minTemp >= maxTemp {
		return errors.New("invalid temperature range")
	}

	if defaultTemp < minTemp || defaultTemp > maxTemp {
		return errors.New("default temperature out of range")
	}

	// 根据模式验证温度范围的合理性
	if mode == ModeCooling {
		if minTemp < 16 || maxTemp > 30 {
			return errors.New("cooling mode temperature range must be between 16-30°C")
		}
	} else {
		if minTemp < 16 || maxTemp > 30 {
			return errors.New("heating mode temperature range must be between 16-30°C")
		}
	}

	// 更新温度范围配置
	config := &db.ACConfig{
		Mode:        mode,
		MinTemp:     minTemp,
		MaxTemp:     maxTemp,
		DefaultTemp: defaultTemp,
	}

	if err := s.configRepo.SetTemperatureRange(config); err != nil {
		return err
	}

	// 发布配置更新事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventConfigChanged,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"mode":         mode,
			"min_temp":     minTemp,
			"max_temp":     maxTemp,
			"default_temp": defaultTemp,
		},
	})

	return nil
}

// GetTemperatureRange 获取温度范围配置
func (s *acService) GetTemperatureRange(mode string) (*TempRange, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if mode != ModeCooling && mode != ModeHeating {
		return nil, errors.New("invalid mode")
	}

	config, err := s.configRepo.GetTemperatureRange(mode)
	if err != nil {
		return nil, err
	}

	return &TempRange{
		Mode:        config.Mode,
		MinTemp:     config.MinTemp,
		MaxTemp:     config.MaxTemp,
		DefaultTemp: config.DefaultTemp,
	}, nil
}

func (s *acService) PowerOn(roomID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查中央空调状态
	mainUnitOn, err := s.configRepo.GetMainUnitState()
	if err != nil {
		return err
	}

	if !mainUnitOn {
		return errors.New("main AC unit is not powered on")
	}

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return err
	}

	if room.State != 1 {
		return errors.New("room is not occupied")
	}

	if room.ACState == 1 {
		return errors.New("ac is already on")
	}

	// 获取当前模式的温度范围配置
	config, err := s.configRepo.GetTemperatureRange(room.Mode)
	if err != nil {
		return err
	}

	// 使用缺省风速和默认温度开机
	if err := s.roomRepo.PowerOnAC(roomID, room.Mode, config.DefaultTemp, config.DefaultSpeed); err != nil {
		return err
	}

	// 设置缺省风速
	if err := s.roomRepo.UpdateSpeed(roomID, "medium"); err != nil {
		return err
	}

	// 发布空调开机事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventPowerOn,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"mode":         room.Mode,
			"current_temp": room.CurrentTemp,
			"target_temp":  config.DefaultTemp,
			"speed":        DefaultSpeed,
		},
	})
	// 发送服务请求事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventServiceRequest,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: events.ServiceRequest{
			RoomID:      roomID,
			RequestTime: time.Now(),
			Speed:       DefaultSpeed,
			TargetTemp:  config.DefaultTemp,
			CurrentTemp: room.CurrentTemp,
		},
	})

	return nil
}

func (s *acService) PowerOff(roomID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return err
	}

	if room.ACState == 0 {
		return errors.New("ac is already off")
	}

	// 1. 完成当前服务记录
	activeService, err := s.serviceRepo.GetActiveServiceDetail(roomID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get active service: %v", err)
	}

	if activeService != nil {
		if err := s.serviceRepo.CompleteServiceDetail(roomID, room.CurrentTemp); err != nil {
			return fmt.Errorf("failed to complete service detail: %v", err)
		}
	}

	// 2. 从服务队列中移除
	s.eventBus.Publish(events.Event{
		Type:      events.EventServiceComplete,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: events.ServiceEventData{
			RoomID:  roomID,
			EndTime: time.Now(),
			Reason:  "power_off",
		},
	})

	// 3. 更新房间状态
	if err := s.roomRepo.PowerOffAC(roomID); err != nil {
		return fmt.Errorf("failed to update room state: %v", err)
	}

	// 4. 发送关机事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventPowerOff,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"final_temp": room.CurrentTemp,
		},
	})

	return nil
}

func (s *acService) SetTemperature(roomID int, targetTemp float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return err
	}

	if room.ACState == 0 {
		return errors.New("ac is not turned on")
	}

	// 获取当前模式的温度范围配置
	config, err := s.configRepo.GetTemperatureRange(room.Mode)
	if err != nil {
		return err
	}

	// 验证目标温度是否在允许范围内
	if targetTemp < config.MinTemp || targetTemp > config.MaxTemp {
		return fmt.Errorf("target temperature must be between %.1f-%.1f°C", config.MinTemp, config.MaxTemp)
	}

	// 发布温度变化事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventTemperatureChange,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: events.TemperatureEventData{
			RoomID:      roomID,
			CurrentTemp: room.CurrentTemp,
			TargetTemp:  targetTemp,
			Speed:       room.CurrentSpeed,
			Mode:        room.Mode,
		},
	})

	return nil
}

func (s *acService) SetFanSpeed(roomID int, speed string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return err
	}

	if room.ACState == 0 {
		return errors.New("ac is not turned on")
	}

	// 验证风速值
	validSpeeds := map[string]bool{"low": true, "medium": true, "high": true}
	if !validSpeeds[speed] {
		return errors.New("invalid fan speed")
	}

	// 更新房间风速
	if err := s.roomRepo.UpdateSpeed(roomID, speed); err != nil {
		return err
	}

	// 发布风速变化事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventSpeedChange,
		RoomID:    roomID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"speed":        speed,
			"current_temp": room.CurrentTemp,
		},
	})

	return nil
}

func (s *acService) GetACState(roomID int) (*ACState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, err
	}

	return &ACState{
		RoomID:      roomID,
		IsOn:        room.ACState == 1,
		Mode:        room.Mode,
		CurrentTemp: room.CurrentTemp,
		TargetTemp:  room.TargetTemp,
		Speed:       room.CurrentSpeed,
	}, nil
}

func (s *acService) SetMode(mode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if mode != ModeCooling && mode != ModeHeating {
		return errors.New("invalid mode")
	}

	// 更新所有房间的工作模式
	if err := s.roomRepo.SetACMode(mode); err != nil {
		return err
	}

	// 发布模式变化事件
	s.eventBus.Publish(events.Event{
		Type:      events.EventModeChange,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"mode": mode,
		},
	})

	return nil
}
