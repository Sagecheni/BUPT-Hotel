package db

import (
	"database/sql"
	"log"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const DB_NAME = "hotel.db"

var (
	Init  bool
	SQLDB *sql.DB
	DB    *gorm.DB
)

func Init_DB() {
	// 检查数据库是否存在
	if _, err := os.Stat(DB_NAME); os.IsNotExist(err) {
		Init = true
	} else {
		log.Println("database already exists")
	}

	// 打开数据库连接
	db, err := gorm.Open(sqlite.Open(DB_NAME), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get db")
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db
	SQLDB = sqlDB

	// 记录旧表是否存在
	hasOldDetailTable := db.Migrator().HasTable("details")

	// 自动迁移
	err = db.AutoMigrate(
		&RoomInfo{},
		&User{},
		&ServiceDetail{},
		&ServiceQueue{},
	)
	if err != nil {
		panic("failed to migrate database")
	}

	if Init {
		InitBaseData()
		InitRooms()
	} else if hasOldDetailTable {
		// 执行数据迁移
		MigrateDetailData(db)
	}
}

// MigrateDetailData 迁移旧的详单数据到新的服务详情表
func MigrateDetailData(db *gorm.DB) {
	var oldDetails []struct {
		ID         int
		RoomID     int
		QueryTime  time.Time
		StartTime  time.Time
		EndTime    time.Time
		ServeTime  float32
		Speed      string
		Cost       float32
		Rate       float32
		TempChange float32
	}

	if err := db.Table("details").Find(&oldDetails).Error; err != nil {
		log.Printf("获取旧详单数据失败: %v", err)
		return
	}

	// 开始迁移数据
	for _, old := range oldDetails {
		serviceDetail := &ServiceDetail{
			RoomID:          old.RoomID,
			StartTime:       old.StartTime,
			EndTime:         old.EndTime,
			ServiceDuration: old.ServeTime,
			Speed:           old.Speed,
			ServiceState:    "completed",
			InitialTemp:     0, // 旧数据中没有这个信息
			FinalTemp:       0, // 旧数据中没有这个信息
			TotalFee:        old.Cost,
		}

		if err := db.Create(serviceDetail).Error; err != nil {
			log.Printf("迁移详单数据失败, ID: %d, 错误: %v", old.ID, err)
			continue
		}
	}

	// 迁移完成后删除旧表
	if err := db.Migrator().DropTable("details"); err != nil {
		log.Printf("删除旧详单表失败: %v", err)
	}
}

func InitBaseData() {
	var adminCount int64
	DB.Model(&User{}).Where("identity = ?", "manager").Count(&adminCount)

	if adminCount == 0 {
		admin := User{
			Account:  "manager",
			Password: "manager",
			Identity: "manager",
		}
		DB.Create(&admin)
	}

	var receptionCount int64
	DB.Model(&User{}).Where("identity = ?", "reception").Count(&receptionCount)

	if receptionCount == 0 {
		reception := User{
			Account:  "reception",
			Password: "123456",
			Identity: "reception",
		}
		DB.Create(&reception)
	}
}

func InitRooms() {
	var count int64
	DB.Model(&RoomInfo{}).Count(&count)
	if count == 0 {
		rooms := []RoomInfo{
			{
				RoomID:      1,
				State:       0,
				CurrentTemp: 32.0,
			},
			{
				RoomID:      2,
				State:       0,
				CurrentTemp: 28.0,
			},
			{
				RoomID:      3,
				State:       0,
				CurrentTemp: 30.0,
			},
			{
				RoomID:      4,
				State:       0,
				CurrentTemp: 29.0,
			},
			{
				RoomID:      5,
				State:       0,
				CurrentTemp: 35.0,
			},
		}

		for _, room := range rooms {
			if err := DB.Create(&room).Error; err != nil {
				log.Printf("创建房间 %d 失败: %v\n", room.RoomID, err)
			} else {
				log.Printf("成功创建房间: %d\n", room.RoomID)
			}
		}
	}
}
