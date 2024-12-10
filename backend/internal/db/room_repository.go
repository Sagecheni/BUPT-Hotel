package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type RoomRepository struct {
	db *gorm.DB
}

func NewRoomRepository() *RoomRepository {
	return &RoomRepository{db: DB}
}

// GetRoomByID 通过房间号获取房间信息
func (r *RoomRepository) GetRoomByID(roomID int) (*RoomInfo, error) {
	var room RoomInfo
	err := r.db.Where("room_id = ?", roomID).First(&room).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("room not found")
		}
		return nil, err
	}
	return &room, nil
}

// UpdateRoom 更新房间信息
func (r *RoomRepository) UpdateRoom(room *RoomInfo) error {
	// 只更新指定的字段,避免覆盖其他字段
	updates := make(map[string]interface{})

	if room.TargetTemp != 0 {
		updates["target_temp"] = room.TargetTemp
	}
	if room.CurrentTemp != 0 {
		updates["current_temp"] = room.CurrentTemp
	}
	if room.CurrentSpeed != "" {
		updates["current_speed"] = room.CurrentSpeed
	}
	if room.ACState != 0 {
		updates["ac_state"] = room.ACState
	}

	return r.db.Model(&RoomInfo{}).
		Where("room_id = ?", room.RoomID).
		Updates(updates).Error
}

// CheckIn 入住
func (r *RoomRepository) CheckIn(roomID int, clientID, clientName string) error {
	now := time.Now()
	return r.db.Model(&RoomInfo{}).Where("room_id = ? AND state = ?", roomID, 0).Updates(map[string]interface{}{
		"client_id":     clientID,
		"client_name":   clientName,
		"checkin_time":  now,
		"state":         1,
		"ac_state":      0,           // 空调初始为关闭状态
		"mode":          "cooling",   // 默认制冷模式
		"current_speed": "",          // 清空风速
		"target_temp":   float32(24), // 默认目标温度
	}).Error
}

func (r *RoomRepository) CheckOut(roomID int) error {
	now := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 获取房间信息
		var room RoomInfo
		if err := tx.Where("room_id = ?", roomID).First(&room).Error; err != nil {
			return err
		}

		// 更新房间状态
		return tx.Model(&RoomInfo{}).Where("room_id = ? AND state = ?", roomID, 1).Updates(map[string]interface{}{
			"client_id":     "",
			"client_name":   "",
			"checkout_time": now,
			"state":         0,
			"ac_state":      0,    // 确保空调关闭
			"current_speed": "",   // 清空风速
			"target_temp":   26.0, // 重置目标温度
		}).Error
	})
}

// UpdateRoomState 更新房间状态
func (r *RoomRepository) UpdateRoomState(roomID, state int) error {
	return r.db.Model(&RoomInfo{}).Where("room_id = ?", roomID).Update("state", state).Error
}

// UpdateRoomSpeed 更新房间环境
func (r *RoomRepository) UpdateRoomEnvironment(roomID int, temp float32, speed string) error {
	return r.db.Model(&RoomInfo{}).Where("room_id = ?", roomID).Updates(map[string]interface{}{
		"current_speed": speed,
		"current_temp":  temp,
	}).Error
}

// GetOccupiedRooms 获取所有已入住房间
func (r *RoomRepository) GetOccupiedRooms() ([]RoomInfo, error) {
	var rooms []RoomInfo
	err := r.db.Where("state = ?", 1).Find(&rooms).Error
	return rooms, err
}

// GetAvailableRooms 获取所有可入住房间
func (r *RoomRepository) GetAvailableRooms() ([]RoomInfo, error) {
	var rooms []RoomInfo
	err := r.db.Where("state = ?", 0).Find(&rooms).Error
	return rooms, err
}

func (r *RoomRepository) GetDB() *gorm.DB {
	return r.db
}

// UpdateTemperature 更新房间温度
func (r *RoomRepository) UpdateTemperature(roomID int, targetTemp float32) error {
	result := r.db.Model(&RoomInfo{}).
		Where("room_id = ?", roomID).
		Update("current_temp", targetTemp)
	if result.Error != nil {
		return fmt.Errorf("更新房间温度失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("房间不存在")
	}
	return nil
}

// UpdateSpeed 更新房间风速
func (r *RoomRepository) UpdateSpeed(roomID int, speed string) error {
	result := r.db.Model(&RoomInfo{}).
		Where("room_id = ?", roomID).
		Update("current_speed", speed)
	if result.Error != nil {
		return fmt.Errorf("更新房间风速失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("房间不存在")
	}
	return nil
}
func (r *RoomRepository) PowerOnAC(roomID int, mode string, defaultTemp float32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 更新房间空调状态
		updates := map[string]interface{}{
			"ac_state":      1,           // 开机状态
			"mode":          mode,        // 工作模式
			"target_temp":   defaultTemp, // 目标温度设为默认温度
			"current_speed": "中",         // 初始中风速
		}

		if err := tx.Model(&RoomInfo{}).Where("room_id = ?", roomID).Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *RoomRepository) PowerOffAC(roomID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 更新房间空调状态
		updates := map[string]interface{}{
			"ac_state":      0,  // 关机状态
			"current_speed": "", // 清除风速
		}

		if err := tx.Model(&RoomInfo{}).Where("room_id = ?", roomID).Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *RoomRepository) SetACMode(mode string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 更新所有房间的工作模式
		if err := tx.Model(&RoomInfo{}).Where("1 = 1").Updates(map[string]interface{}{
			"mode": mode,
		}).Error; err != nil {
			return err
		}

		return nil
	})
}

// GetAllRooms 获取所有房间信息
func (r *RoomRepository) GetAllRooms() ([]RoomInfo, error) {
	var rooms []RoomInfo
	result := r.db.Find(&rooms)
	if result.Error != nil {
		return nil, fmt.Errorf("获取所有房间失败: %v", result.Error)
	}
	if len(rooms) == 0 {
		return nil, fmt.Errorf("没有房间")
	}
	return rooms, nil
}
