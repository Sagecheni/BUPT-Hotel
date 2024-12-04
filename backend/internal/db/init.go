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

	// 自动迁移所有表结构
	err = db.AutoMigrate(
		&RoomInfo{},
		&User{},
		&ServiceDetail{},
		&ServiceQueue{},
		&ACConfig{},
	)
	if err != nil {
		panic("failed to migrate database")
	}

	if Init {
		initBaseData(db)
		initRooms(db)
		initACConfig(db)
	}
}

func initBaseData(db *gorm.DB) {
	// 初始化管理员账户
	var adminCount int64
	db.Model(&User{}).Where("identity = ?", "manager").Count(&adminCount)
	if adminCount == 0 {
		admin := User{
			Account:  "manager",
			Password: "manager",
			Identity: "manager",
		}
		db.Create(&admin)
	}

	// 初始化前台账户
	var receptionCount int64
	db.Model(&User{}).Where("identity = ?", "reception").Count(&receptionCount)
	if receptionCount == 0 {
		reception := User{
			Account:  "reception",
			Password: "123456",
			Identity: "reception",
		}
		db.Create(&reception)
	}

	// 初始化空调管理员账户
	var acAdminCount int64
	db.Model(&User{}).Where("identity = ?", "ac_admin").Count(&acAdminCount)
	if acAdminCount == 0 {
		acAdmin := User{
			Account:  "ac_admin",
			Password: "123456",
			Identity: "ac_admin",
		}
		db.Create(&acAdmin)
	}
}

func initRooms(db *gorm.DB) {
	var count int64
	db.Model(&RoomInfo{}).Count(&count)
	if count == 0 {
		// 初始化40间房间
		for i := 1; i <= 5; i++ {
			room := RoomInfo{
				RoomID:      i,
				State:       1,
				CurrentTemp: 31.0,
				TargetTemp:  26.0,
				Mode:        "cooling",
			}
			if err := db.Create(&room).Error; err != nil {
				log.Printf("创建房间 %d 失败: %v\n", i, err)
			} else {
				log.Printf("成功创建房间: %d\n", i)
			}
		}
	}
}

func initACConfig(db *gorm.DB) {
	var count int64
	db.Model(&ACConfig{}).Count(&count)
	if count == 0 {
		// 创建默认空调配置
		configs := []ACConfig{
			{
				Mode:        "cooling",
				MinTemp:     16,
				MaxTemp:     30,
				DefaultTemp: 24,
				MainUnitOn:  false,
			},
			{
				Mode:        "heating",
				MinTemp:     16,
				MaxTemp:     30,
				DefaultTemp: 26,
				MainUnitOn:  false,
			},
		}

		for _, config := range configs {
			if err := db.Create(&config).Error; err != nil {
				log.Printf("创建空调配置失败: %v\n", err)
			}
		}
	}
}
