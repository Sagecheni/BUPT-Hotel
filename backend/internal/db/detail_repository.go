// internal/db/detail_repository.go
package db

import (
	"backend/internal/logger"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DetailRepository struct {
	db *gorm.DB
}

func NewDetailRepository() *DetailRepository {
	return &DetailRepository{db: DB}
}

// CreateDetail 创建新的详单记录
func (r *DetailRepository) CreateDetail(detail *Detail) error {
	err := r.db.Create(detail).Error
	if err != nil {
		logger.Error("创建详单记录失败 - 房间ID: %d, 错误: %v", detail.RoomID, err)
		return fmt.Errorf("创建详单记录失败: %v", err)
	}
	logger.Info("成功创建详单记录 - 房间ID: %d, 开始时间: %v, 服务时长: %.1f分钟, 费用: %.2f元, 风速: %s",
		detail.RoomID, detail.StartTime.Format("15:04:05"), detail.ServeTime, detail.Cost, detail.Speed)
	return nil
}

// GetDetailsByRoomAndTimeRange 获取指定房间在时间范围内的所有详单
func (r *DetailRepository) GetDetailsByRoomAndTimeRange(roomID int, startTime, endTime time.Time) ([]Detail, error) {
	var details []Detail
	err := r.db.Where("room_id = ? AND query_time BETWEEN ? AND ?",
		roomID, startTime, endTime).
		Order("query_time ASC").
		Find(&details).Error
	if err != nil {
		logger.Error("获取详单记录失败 - 房间ID: %d, 时间范围: %v 到 %v, 错误: %v",
			roomID, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"), err)
		return nil, fmt.Errorf("获取详单记录失败: %v", err)
	}
	return details, nil
}

// GetDetailsByRoom 获取指定房间的所有详单
func (r *DetailRepository) GetDetailsByRoom(roomID int) ([]Detail, error) {
	var details []Detail
	err := r.db.Where("room_id = ?", roomID).
		Order("query_time ASC").
		Find(&details).Error
	if err != nil {
		logger.Error("获取房间详单失败 - 房间ID: %d, 错误: %v", roomID, err)
		return nil, fmt.Errorf("获取房间详单失败: %v", err)
	}
	logger.Info("成功获取房间所有详单 - 房间ID: %d, 总记录数: %d", roomID, len(details))
	return details, nil
}

// GetLatestDetail 获取最新的详单记录
func (r *DetailRepository) GetLatestDetail(roomID int) (*Detail, error) {
	var detail Detail
	err := r.db.Where("room_id = ?", roomID).
		Order("query_time DESC").
		First(&detail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		logger.Error("获取最新详单失败 - 房间ID: %d, 错误: %v", roomID, err)
		return nil, fmt.Errorf("获取最新详单失败: %v", err)
	}
	return &detail, nil
}

// GetTotalCost 获取指定房间在时间范围内的总费用（不包括当前开机的费用）
func (r *DetailRepository) GetTotalCost(roomID int, startTime, endTime time.Time) (float32, error) {
	var totalCost float32
	err := r.db.Model(&Detail{}).
		Where("room_id = ? AND query_time BETWEEN ? AND ?", roomID, startTime, endTime).
		Select("COALESCE(SUM(cost), 0) as total_cost").
		Scan(&totalCost).Error
	if err != nil {
		logger.Error("计算总费用失败 - 房间ID: %d, 时间范围: %v 到 %v, 错误: %v",
			roomID, startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"), err)
		return 0, fmt.Errorf("计算总费用失败: %v", err)
	}
	return totalCost, nil
}

// DeleteDetails 删除指定房间的所有详单
func (r *DetailRepository) DeleteDetails(roomID int) error {
	result := r.db.Where("room_id = ?", roomID).Delete(&Detail{})
	if result.Error != nil {
		logger.Error("删除房间详单失败 - 房间ID: %d, 错误: %v", roomID, result.Error)
		return fmt.Errorf("删除房间详单失败: %v", result.Error)
	}
	logger.Info("成功删除房间详单 - 房间ID: %d, 删除记录数: %d", roomID, result.RowsAffected)
	return nil
}
