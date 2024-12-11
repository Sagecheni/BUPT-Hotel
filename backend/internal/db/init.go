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
	var adminCount int64
	DB.Model(&User{}).Where("identity = ?", "admin").Count(&adminCount)

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
			},
			{
				RoomID:      2,
				State:       0,
				CurrentTemp: 28.0,
				ACState:     0,
				InitialTemp: 28.0,
			},
			{
				RoomID:      3,
				State:       0,
				CurrentTemp: 30.0,
				ACState:     0,
				InitialTemp: 30.0,
			},
			{
				RoomID:      4,
				State:       0,
				CurrentTemp: 29.0,
				ACState:     0,
				InitialTemp: 29.0,
			},
			{
				RoomID:      5,
				State:       0,
				CurrentTemp: 35.0,
				ACState:     0,
				InitialTemp: 35.0,
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
