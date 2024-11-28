package db

import (
	"errors"
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
	return r.db.Model(&RoomInfo{}).Where("room_id = ?", room.RoomID).Where("room_id=?", room.RoomID).Updates(map[string]interface{}{
		"client_id":     room.ClientID,
		"client_name":   room.ClientName,
		"checkin_time":  room.CheckinTime,
		"checkout_time": room.CheckoutTime,
		"state":         room.State,
		"current_speed": room.CurrentSpeed,
		"current_temp":  room.CurrentTemp,
	}).Error
}

// CheckIn 入住
func (r *RoomRepository) CheckIn(roomID int, clientID, clientName string) error {
	now := time.Now()
	return r.db.Model(&RoomInfo{}).Where("room_id = ? AND state = ?", roomID, 0).Updates(map[string]interface{}{
		"client_id":     clientID,
		"client_name":   clientName,
		"checkin_time":  now,
		"state":         1,
		"current_speed": "",
		"current_temp":  26.0, //作为一个环境温度
	}).Error
}

// CheckOut 退房
func (r *RoomRepository) CheckOut(roomID int) error {
	now := time.Now()
	return r.db.Model(&RoomInfo{}).Where("room_id = ? AND state = ?", roomID, 1).Updates(map[string]interface{}{
		"client_id":     "",
		"client_name":   "",
		"checkout_time": now,
		"state":         0,
		"current_speed": "",
		"current_temp":  26.0,
	}).Error
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
