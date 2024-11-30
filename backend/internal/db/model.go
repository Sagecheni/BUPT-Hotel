package db

import "time"

// 房间信息表
type RoomInfo struct {
	RoomID       int       `gorm:"primaryKey"`
	ClientID     string    `gorm:"type:varchar(255)"`
	ClientName   string    `gorm:"type:varchar(255)"`
	CheckinTime  time.Time `gorm:"type:datetime"`
	CheckoutTime time.Time `gorm:"type:datetime"`
	State        int
	CurrentSpeed string  `gorm:"type:varchar(255)"`
	CurrentTemp  float32 `gorm:"type:floadd dd dd ddt"`
	ACState      int     // 0: 关闭 1: 开启
	Mode         string  `gorm:"type:varchar(20)"` // cooling/heating
	TargetTemp   float32 `gorm:"type:float(5, 2)"`
}

// 操作日志表
type OperationLog struct {
	ID     int `gorm:"primaryKey"`
	RoomID int
	OpTime time.Time `gorm:"type:datetime"`
	OpType int
	Old    string `gorm:"type:varchar(255)"`
	New    string `gorm:"type:varchar(255)"`
}

// 详单表
type Detail struct {
	ID         int `gorm:"primary_key"`
	RoomID     int
	QueryTime  time.Time `gorm:"type:datetime"`
	StartTime  time.Time `gorm:"type:datetime"`
	EndTime    time.Time `gorm:"type:datetime"`
	ServeTime  float32   `gorm:"type:float(7, 2)"`
	Speed      string    `gorm:"type:varchar(255)"`
	Cost       float32   `gorm:"type:float(7, 2)"`
	Rate       float32   `gorm:"type:float(5, 2)"`
	TempChange float32   `gorm:"type:float(5, 2)"`
}

// 用户表
type User struct {
	Account  string `gorm:"primary_key;type:varchar(255)"`
	Password string `gorm:"type:varchar(255)"`
	Identity string `gorm:"type:varchar(255)"`
}

// 调度表
type SchedulerBoard struct {
	ID       int `gorm:"primary_key"`
	RoomID   int
	Duration float32 `gorm:"type:float(5, 2)"`
	Speed    string  `gorm:"type:varchar(255)"`
	Cost     float32 `gorm:"type:float(7, 2)"`
}
