// internal/db/ac_config.go

package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	SpeedLow    = "low"
	SpeedMedium = "medium"
	SpeedHigh   = "high"
)

// ACConfig 空调配置表
type ACConfig struct {
	ID              int       `gorm:"primaryKey"`
	Mode            string    `gorm:"type:varchar(20)"` // cooling/heating
	MinTemp         float32   `gorm:"type:float(5,2)"`
	MaxTemp         float32   `gorm:"type:float(5,2)"`
	DefaultTemp     float32   `gorm:"type:float(5,2)"`
	DefaultSpeed    string    `gorm:"type:varchar(10)"` // 默认风速
	LowSpeedRate    float32   `gorm:"type:float(5,2)"`
	MediumSpeedRate float32   `gorm:"type:float(5,2)"`
	HighSpeedRate   float32   `gorm:"type:float(5,2)"`
	MainUnitOn      bool      `gorm:"default:false"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

// IACConfigRepository 空调配置仓库接口
type IACConfigRepository interface {
	SetMainUnitState(state bool) error
	GetMainUnitState() (bool, error)
	SetTemperatureRange(config *ACConfig) error
	GetTemperatureRange(mode string) (*ACConfig, error)
	SetSpeedRates(lowRate, mediumRate, highRate float32) error
	GetSpeedRates() (float32, float32, float32, error)
	SetDefaultSpeed(speed string) error
	GetDefaultSpeed() (string, error)
}

type ACConfigRepository struct {
	db *gorm.DB
}

func NewACConfigRepository(db *gorm.DB) IACConfigRepository {
	return &ACConfigRepository{db: db}
}

// 新增默认风速设置方法
func (r *ACConfigRepository) SetDefaultSpeed(speed string) error {
	if speed != SpeedLow && speed != SpeedMedium && speed != SpeedHigh {
		return errors.New("invalid speed value")
	}
	return r.db.Model(&ACConfig{}).Where("1 = 1").Update("default_speed", speed).Error
}

// 默认风速获取方法
func (r *ACConfigRepository) GetDefaultSpeed() (string, error) {
	var config ACConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SpeedMedium, nil // 返回默认中速
		}
		return "", err
	}
	return config.DefaultSpeed, nil
}

// 初始化方法
func (r *ACConfigRepository) Init() error {
	var count int64
	r.db.Model(&ACConfig{}).Count(&count)
	if count == 0 {
		// 创建默认配置
		configs := []ACConfig{
			{
				Mode:            "cooling",
				MinTemp:         16,
				MaxTemp:         30,
				DefaultTemp:     24,
				DefaultSpeed:    SpeedMedium,
				LowSpeedRate:    0.5,
				MediumSpeedRate: 1.0,
				HighSpeedRate:   2.0,
				MainUnitOn:      false,
			},
			{
				Mode:            "heating",
				MinTemp:         16,
				MaxTemp:         30,
				DefaultTemp:     26,
				DefaultSpeed:    SpeedMedium,
				LowSpeedRate:    0.5,
				MediumSpeedRate: 1.0,
				HighSpeedRate:   2.0,
				MainUnitOn:      false,
			},
		}

		return r.db.Create(&configs).Error
	}
	return nil
}

// 费率设置方法
func (r *ACConfigRepository) SetSpeedRates(lowRate, mediumRate, highRate float32) error {
	return r.db.Model(&ACConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"low_speed_rate":    lowRate,
		"medium_speed_rate": mediumRate,
		"high_speed_rate":   highRate,
	}).Error
}

// 新增费率获取方法
func (r *ACConfigRepository) GetSpeedRates() (float32, float32, float32, error) {
	var config ACConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认费率
			return 0.5, 1.0, 2.0, nil
		}
		return 0, 0, 0, err
	}
	return config.LowSpeedRate, config.MediumSpeedRate, config.HighSpeedRate, nil
}

// 修改获取温度范围方法，同时返回费率信息
func (r *ACConfigRepository) GetTemperatureRange(mode string) (*ACConfig, error) {
	var config ACConfig
	err := r.db.Where("mode = ?", mode).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 返回默认配置
			defaultTemp := float32(24)
			if mode == "heating" {
				defaultTemp = 26
			}
			return &ACConfig{
				Mode:            mode,
				MinTemp:         16,
				MaxTemp:         30,
				DefaultTemp:     defaultTemp,
				LowSpeedRate:    0.5,
				MediumSpeedRate: 1.0,
				HighSpeedRate:   2.0,
			}, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *ACConfigRepository) SetMainUnitState(state bool) error {
	// 更新所有配置记录的主机状态
	return r.db.Model(&ACConfig{}).Where("1 = 1").Update("main_unit_on", state).Error
}

func (r *ACConfigRepository) GetMainUnitState() (bool, error) {
	var config ACConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return config.MainUnitOn, nil
}

func (r *ACConfigRepository) SetTemperatureRange(config *ACConfig) error {
	// 查找是否存在该模式的配置
	var existing ACConfig
	err := r.db.Where("mode = ?", config.Mode).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不存在则创建
			return r.db.Create(config).Error
		}
		return err
	}

	// 存在则更新
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"min_temp":     config.MinTemp,
		"max_temp":     config.MaxTemp,
		"default_temp": config.DefaultTemp,
	}).Error
}
