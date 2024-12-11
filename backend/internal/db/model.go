package db

import "time"

// DetailType 详单类型
type DetailType string

const (
	DetailTypePowerOn       DetailType = "power_on"       // 开机
	DetailTypePowerOff      DetailType = "power_off"      // 关机
	DetailTypeSpeedChange   DetailType = "speed_change"   // 调整风速
	DetailTypeTargetReached DetailType = "target_reached" // 达到目标温度
)

// 房间信息表
type RoomInfo struct {
	RoomID       int       `gorm:"primaryKey"`
	ClientID     string    `gorm:"type:varchar(255)"`
	ClientName   string    `gorm:"type:varchar(255)"`
	CheckinTime  time.Time `gorm:"type:datetime"`
	CheckoutTime time.Time `gorm:"type:datetime"`
	State        int
	CurrentSpeed string  `gorm:"type:varchar(255)"`
	CurrentTemp  float32 `gorm:"type:float"`
	ACState      int     // 0: 关闭 1: 开启
	Mode         string  `gorm:"type:varchar(20)"` // cooling/heating
	TargetTemp   float32 `gorm:"type:float(5, 2)"`
	InitialTemp  float32 `gorm:"type:float(5,2)"`
}

// Detail 详单表
type Detail struct {
	ID          int        `gorm:"primary_key"`
	RoomID      int        `gorm:"type:int"`
	QueryTime   time.Time  `gorm:"type:datetime"`
	StartTime   time.Time  `gorm:"type:datetime"`
	EndTime     time.Time  `gorm:"type:datetime"`
	ServeTime   float32    `gorm:"type:float(7,2)"` // 服务时长(分钟)
	Speed       string     `gorm:"type:varchar(255)"`
	Cost        float32    `gorm:"type:float(7,2)"`  // 费用(元)
	Rate        float32    `gorm:"type:float(5,2)"`  // 每分钟费率(元/分钟)
	TempChange  float32    `gorm:"type:float(5,2)"`  // 温度变化
	CurrentTemp float32    `gorm:"type:float(5,2)"`  // 当前温度
	DetailType  DetailType `gorm:"type:varchar(20)"` // 详单类型
}

// 用户表
type User struct {
	Account  string `gorm:"primary_key;type:varchar(255)"`
	Password string `gorm:"type:varchar(255)"`
	Identity string `gorm:"type:varchar(255)"`
}
