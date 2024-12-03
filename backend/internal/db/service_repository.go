package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ServiceRepository 定义
type ServiceRepository struct {
	db *gorm.DB
}

// ServiceRepositoryInterface 接口定义
type ServiceRepositoryInterface interface {
	// 服务详情管理
	CreateServiceDetail(detail *ServiceDetail) error
	UpdateServiceDetail(detail *ServiceDetail) error
	GetActiveServiceDetail(roomID int) (*ServiceDetail, error)
	GetServiceHistory(roomID int, startTime, endTime time.Time) ([]*ServiceDetail, error)
	CompleteServiceDetail(roomID int, finalTemp float32) error
	PauseServiceDetail(roomID int) error
	ResumeServiceDetail(roomID int) error
	PreemptServiceDetail(roomID, preemptedByRoomID int) error

	// 队列管理
	AddToServiceQueue(roomID int, speed string, targetTemp, currentTemp float32) error
	AddToWaitQueue(roomID int, speed string, targetTemp, currentTemp float32, priority int) error
	RemoveFromQueue(roomID int) error
	UpdateQueueItemSpeed(roomID int, speed string) error
	UpdateQueueItemTemp(roomID int, currentTemp, targetTemp float32) error
	GetQueueStatus(roomID int) (*ServiceQueue, error)
	GetServiceQueueItems() ([]*ServiceQueue, error)
	GetWaitQueueItems() ([]*ServiceQueue, error)

	// 费用统计
	CalculateServiceFee(roomID int) (float32, error)
	GetServiceStats(roomID int, startTime, endTime time.Time) (map[string]float32, error)
}

func NewServiceRepository(db *gorm.DB) ServiceRepositoryInterface {
	return &ServiceRepository{db: db}
}

// CreateServiceDetail 创建新的服务详情记录
func (r *ServiceRepository) CreateServiceDetail(detail *ServiceDetail) error {
	detail.ServiceState = "active"
	return r.db.Create(detail).Error
}

// UpdateServiceDetail 更新现有服务详情
func (r *ServiceRepository) UpdateServiceDetail(detail *ServiceDetail) error {
	if detail.ID == 0 {
		return errors.New("invalid service detail ID")
	}
	return r.db.Save(detail).Error
}

// GetActiveServiceDetail 获取房间当前活动的服务详情
func (r *ServiceRepository) GetActiveServiceDetail(roomID int) (*ServiceDetail, error) {
	var detail ServiceDetail
	err := r.db.Where("room_id = ? AND service_state = 'active'", roomID).First(&detail).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("查询服务详情失败: %v", err)
	}
	return &detail, nil
}

// GetServiceHistory 获取服务历史记录
func (r *ServiceRepository) GetServiceHistory(roomID int, startTime, endTime time.Time) ([]*ServiceDetail, error) {
	var details []*ServiceDetail
	err := r.db.Where("room_id = ? AND start_time >= ? AND start_time <= ?",
		roomID, startTime, endTime).
		Order("start_time DESC").
		Find(&details).Error
	if err != nil {
		return nil, fmt.Errorf("查询服务历史失败: %v", err)
	}
	return details, nil
}

// CompleteServiceDetail 完成服务记录
func (r *ServiceRepository) CompleteServiceDetail(roomID int, finalTemp float32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var detail ServiceDetail
		if err := tx.Where("room_id = ? AND service_state = 'active'", roomID).
			First(&detail).Error; err != nil {
			return err
		}

		now := time.Now()
		detail.EndTime = now
		detail.FinalTemp = finalTemp
		detail.ServiceState = "completed"
		detail.ServiceDuration = float32(now.Sub(detail.StartTime).Seconds())

		// 计算最终费用
		if err := tx.Save(&detail).Error; err != nil {
			return err
		}

		return nil
	})
}

// PauseServiceDetail 暂停服务
func (r *ServiceRepository) PauseServiceDetail(roomID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var detail ServiceDetail
		if err := tx.Where("room_id = ? AND service_state = 'active'", roomID).
			First(&detail).Error; err != nil {
			return err
		}

		now := time.Now()
		serviceDuration := float32(now.Sub(detail.StartTime).Seconds())
		detail.ServiceDuration = serviceDuration
		detail.ServiceState = "paused"

		return tx.Save(&detail).Error
	})
}

// ResumeServiceDetail 恢复服务
func (r *ServiceRepository) ResumeServiceDetail(roomID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var detail ServiceDetail
		if err := tx.Where("room_id = ? AND service_state = 'paused'", roomID).
			First(&detail).Error; err != nil {
			return err
		}

		// 创建新的服务记录
		newDetail := ServiceDetail{
			RoomID:       detail.RoomID,
			StartTime:    time.Now(),
			InitialTemp:  detail.FinalTemp,
			TargetTemp:   detail.TargetTemp,
			Speed:        detail.Speed,
			ServiceState: "active",
		}

		return tx.Create(&newDetail).Error
	})
}

// PreemptServiceDetail 处理服务抢占
func (r *ServiceRepository) PreemptServiceDetail(roomID, preemptedByRoomID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var detail ServiceDetail
		if err := tx.Where("room_id = ? AND service_state = 'active'", roomID).
			First(&detail).Error; err != nil {
			return err
		}

		now := time.Now()
		detail.EndTime = now
		detail.ServiceState = "preempted"
		detail.PreemptedBy = &preemptedByRoomID
		detail.ServiceDuration = float32(now.Sub(detail.StartTime).Seconds())

		return tx.Save(&detail).Error
	})
}

// AddToServiceQueue 添加到服务队列
func (r *ServiceRepository) AddToServiceQueue(roomID int, speed string, targetTemp, currentTemp float32) error {
	queue := &ServiceQueue{
		RoomID:      roomID,
		QueueType:   "service",
		EnterTime:   time.Now(),
		Speed:       speed,
		TargetTemp:  targetTemp,
		CurrentTemp: currentTemp,
		Priority:    getPriority(speed),
	}
	return r.db.Create(queue).Error
}

// AddToWaitQueue 添加到等待队列
func (r *ServiceRepository) AddToWaitQueue(roomID int, speed string, targetTemp, currentTemp float32, priority int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 获取当前等待队列长度
		var count int64
		if err := tx.Model(&ServiceQueue{}).
			Where("queue_type = ?", "waiting").
			Count(&count).Error; err != nil {
			return err
		}

		queue := &ServiceQueue{
			RoomID:      roomID,
			QueueType:   "waiting",
			EnterTime:   time.Now(),
			Speed:       speed,
			TargetTemp:  targetTemp,
			CurrentTemp: currentTemp,
			Priority:    priority,
			Position:    int(count + 1),
		}
		return tx.Create(queue).Error
	})
}

// RemoveFromQueue 从队列中移除
func (r *ServiceRepository) RemoveFromQueue(roomID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var queue ServiceQueue
		if err := tx.Where("room_id = ?", roomID).First(&queue).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		// 如果是等待队列，需要更新其他项的位置
		if queue.QueueType == "waiting" {
			if err := tx.Model(&ServiceQueue{}).
				Where("queue_type = ? AND position > ?", "waiting", queue.Position).
				UpdateColumn("position", gorm.Expr("position - 1")).
				Error; err != nil {
				return err
			}
		}

		return tx.Delete(&queue).Error
	})
}

// UpdateQueueItemSpeed 更新队列项的风速
func (r *ServiceRepository) UpdateQueueItemSpeed(roomID int, speed string) error {
	return r.db.Model(&ServiceQueue{}).
		Where("room_id = ?", roomID).
		Updates(map[string]interface{}{
			"speed":    speed,
			"priority": getPriority(speed),
		}).Error
}

// UpdateQueueItemTemp 更新队列项的温度
func (r *ServiceRepository) UpdateQueueItemTemp(roomID int, currentTemp, targetTemp float32) error {
	return r.db.Model(&ServiceQueue{}).
		Where("room_id = ?", roomID).
		Updates(map[string]interface{}{
			"current_temp": currentTemp,
			"target_temp":  targetTemp,
		}).Error
}

// GetQueueStatus 获取房间在队列中的状态
func (r *ServiceRepository) GetQueueStatus(roomID int) (*ServiceQueue, error) {
	var queue ServiceQueue
	err := r.db.Where("room_id = ?", roomID).First(&queue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &queue, nil
}

// GetServiceQueueItems 获取服务队列中的所有项
func (r *ServiceRepository) GetServiceQueueItems() ([]*ServiceQueue, error) {
	var items []*ServiceQueue
	err := r.db.Where("queue_type = ?", "service").
		Order("enter_time ASC").
		Find(&items).Error
	return items, err
}

// GetWaitQueueItems 获取等待队列中的所有项
func (r *ServiceRepository) GetWaitQueueItems() ([]*ServiceQueue, error) {
	var items []*ServiceQueue
	err := r.db.Where("queue_type = ?", "waiting").
		Order("priority DESC, enter_time ASC").
		Find(&items).Error
	return items, err
}

// CalculateServiceFee 计算服务费用
func (r *ServiceRepository) CalculateServiceFee(roomID int) (float32, error) {
	var total float32
	err := r.db.Model(&ServiceDetail{}).
		Where("room_id = ? AND service_state = 'completed'", roomID).
		Select("COALESCE(SUM(total_fee), 0)").
		Scan(&total).Error
	return total, err
}

// GetServiceStats 获取服务统计信息
func (r *ServiceRepository) GetServiceStats(roomID int, startTime, endTime time.Time) (map[string]float32, error) {
	stats := make(map[string]float32)

	// 计算总服务时间
	var totalServiceTime float32
	err := r.db.Model(&ServiceDetail{}).
		Where("room_id = ? AND start_time >= ? AND start_time <= ?", roomID, startTime, endTime).
		Select("COALESCE(SUM(service_duration), 0)").
		Scan(&totalServiceTime).Error
	if err != nil {
		return nil, err
	}
	stats["total_service_time"] = totalServiceTime

	// 计算总等待时间
	var totalWaitTime float32
	err = r.db.Model(&ServiceDetail{}).
		Where("room_id = ? AND start_time >= ? AND start_time <= ?", roomID, startTime, endTime).
		Select("COALESCE(SUM(wait_duration), 0)").
		Scan(&totalWaitTime).Error
	if err != nil {
		return nil, err
	}
	stats["total_wait_time"] = totalWaitTime

	// 计算总费用
	var totalFee float32
	err = r.db.Model(&ServiceDetail{}).
		Where("room_id = ? AND start_time >= ? AND start_time <= ?", roomID, startTime, endTime).
		Select("COALESCE(SUM(total_fee), 0)").
		Scan(&totalFee).Error
	if err != nil {
		return nil, err
	}
	stats["total_fee"] = totalFee

	return stats, nil
}

// 辅助函数：获取风速对应的优先级
func getPriority(speed string) int {
	switch speed {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
