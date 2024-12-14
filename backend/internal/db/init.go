package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const DB_NAME = "hotel.db"

var Init bool
var SQLDB *sql.DB
var DB *gorm.DB

func Init_DB() {
	if _, err := os.Stat(DB_NAME); os.IsNotExist(err) {
		Init = true
	} else {
		fmt.Println("database already exists")
	}
	db, err := gorm.Open(sqlite.Open(DB_NAME), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get db")
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	DB = db
	SQLDB = sqlDB
	err = db.AutoMigrate(&RoomInfo{}, &Detail{}, &User{})
	if err != nil {
		panic("failed to migrate database")
	}
	if Init {
		InitBaseData()
		InitRooms()
	}

}

func InitBaseData() {
	// 添加管理员用户
	var adminCount int64
	DB.Model(&User{}).Where("identity = ?", "administrator").Count(&adminCount)
	if adminCount == 0 {
		admin := User{
			Username: "admin",
			Password: "admin123",
			Identity: "administrator",
		}
		DB.Create(&admin)
	}

	// 添加经理用户
	var managerCount int64
	DB.Model(&User{}).Where("identity = ?", "manager").Count(&managerCount)
	if managerCount == 0 {
		manager := User{
			Username: "manager",
			Password: "manager123",
			Identity: "manager",
		}
		DB.Create(&manager)
	}

	// 添加前台用户
	var receptionCount int64
	DB.Model(&User{}).Where("identity = ?", "reception").Count(&receptionCount)
	if receptionCount == 0 {
		reception := User{
			Username: "reception",
			Password: "reception123",
			Identity: "reception",
		}
		DB.Create(&reception)
	}

	// 添加示例客户
	var customerCount int64
	DB.Model(&User{}).Where("identity = ?", "customer").Count(&customerCount)
	if customerCount == 0 {
		customer := User{
			Username: "customer",
			Password: "customer123",
			Identity: "customer",
		}
		DB.Create(&customer)
	}
}

func GetDB() *gorm.DB {
	return DB
}

func InitRooms() {
	var count int64
	DB.Model(&RoomInfo{}).Count(&count)
	if count == 0 {
		rooms := []RoomInfo{
			{
				RoomID:      1,
				State:       0, // 0表示空闲
				CurrentTemp: 32.0,
				ACState:     0, // 0: 关闭 1: 开启
				InitialTemp: 32.0,
				DailyRate:   100.0,
			},
			{
				RoomID:      2,
				State:       0,
				CurrentTemp: 28.0,
				ACState:     0,
				InitialTemp: 28.0,
				DailyRate:   125.0,
			},
			{
				RoomID:      3,
				State:       0,
				CurrentTemp: 30.0,
				ACState:     0,
				InitialTemp: 30.0,
				DailyRate:   150.0,
			},
			{
				RoomID:      4,
				State:       0,
				CurrentTemp: 29.0,
				ACState:     0,
				InitialTemp: 29.0,
				DailyRate:   200.0,
			},
			{
				RoomID:      5,
				State:       0,
				CurrentTemp: 35.0,
				ACState:     0,
				InitialTemp: 35.0,
				DailyRate:   100.0,
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
