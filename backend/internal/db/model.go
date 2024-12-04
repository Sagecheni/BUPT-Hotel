package db

import "time"

// 房间表
type RoomInfo struct {
	RoomID       int    `gorm:"primaryKey"`
	State        int    `gorm:"default:0"` // 0:空闲 1:占用
	ClientID     string `gorm:"type:varchar(50)"`
	ClientName   string `gorm:"type:varchar(50)"`
	CheckinTime  time.Time
	CheckoutTime time.Time
	ACState      int     `gorm:"default:0"`       // 空调状态 0:关闭 1:开启
	Mode         string  `gorm:"default:cooling"` // cooling/heating
	CurrentSpeed string  // 当前风速
	CurrentTemp  float32 // 当前温度
	TargetTemp   float32 `gorm:"default:26"` // 目标温度
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

// ServiceDetail 服务详情表 - 记录每次服务的完整生命周期
type ServiceDetail struct {
	ID              int `gorm:"primaryKey;autoIncrement"`
	RoomID          int
	StartTime       time.Time `gorm:"type:datetime;index"`
	EndTime         time.Time `gorm:"type:datetime;index"`
	InitialTemp     float32   `gorm:"type:float(5,2)"`
	TargetTemp      float32   `gorm:"type:float(5,2)"`
	FinalTemp       float32   `gorm:"type:float(5,2)"`
	Speed           string    `gorm:"type:varchar(20)"`
	ServiceState    string    `gorm:"type:varchar(20)"` // active, paused, completed, preempted
	PreemptedBy     *int      // 被哪个房间抢占
	WaitDuration    float32   `gorm:"type:float(7,2)"` // 等待时长(秒)
	ServiceDuration float32   `gorm:"type:float(7,2)"` // 服务时长(秒)
	Cost            float32   `gorm:"type:float(7,2)"` // 当前费用
	TotalFee        float32   `gorm:"type:float(7,2)"` // 总费用
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

// ServiceQueue 服务队列表 - 记录当前服务和等待状态
type ServiceQueue struct {
	ID          int       `gorm:"primaryKey;autoIncrement"`
	RoomID      int       `gorm:"uniqueIndex"`
	QueueType   string    `gorm:"type:varchar(20);index"` // service, waiting
	Priority    int       `gorm:"index"`
	EnterTime   time.Time `gorm:"type:datetime;index"`
	Speed       string    `gorm:"type:varchar(20)"`
	TargetTemp  float32   `gorm:"type:float(5,2)"`
	CurrentTemp float32   `gorm:"type:float(5,2)"`
	Position    int       // 在等待队列中的位置
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// User 用户表
type User struct {
	ID       int    `gorm:"primaryKey;autoIncrement"`
	Account  string `gorm:"type:varchar(50);unique"`
	Password string `gorm:"type:varchar(50)"`
	Identity string `gorm:"type:varchar(20)"` // manager, reception, admin
}
