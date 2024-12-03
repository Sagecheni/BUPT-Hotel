package db

import (
	"time"

	"gorm.io/gorm"
)

// IBillingRepository 定义计费相关的数据访问接口
type IBillingRepository interface {
	// CreateDetail 创建新的详单记录
	CreateDetail(*Detail) error

	// GetDetails 获取指定房间的详单记录
	GetDetails(roomID int, startTime, endTime time.Time) ([]*Detail, error)

	// GetTotalFee 获取指定房间在指定时间段内的总费用
	GetTotalFee(roomID int, startTime, endTime time.Time) (float32, error)

	// GetCurrentServiceDetail 获取当前服务的详单
	GetCurrentServiceDetail(roomID int) (*Detail, error)

	// UpdateDetail 更新详单
	UpdateDetail(*Detail) error
}

// BillingRepository 计费数据访问实现
type BillingRepository struct {
	db *gorm.DB
}

// NewBillingRepository 创建新的计费数据访问实例
func NewBillingRepository(db *gorm.DB) IBillingRepository {
	return &BillingRepository{db: db}
}

func (r *BillingRepository) CreateDetail(detail *Detail) error {
	return r.db.Create(detail).Error
}

// GetDetails 获取指定房间的详单记录
func (r *BillingRepository) GetDetails(roomID int, startTime, endTime time.Time) ([]*Detail, error) {
	var details []*Detail
	err := r.db.Where("room_id = ? AND start_time >= ? AND end_time <= ?",
		roomID, startTime, endTime).Find(&details).Error
	return details, err
}

// GetTotalFee 获取指定房间在指定时间段内的总费用
func (r *BillingRepository) GetTotalFee(roomID int, startTime, endTime time.Time) (float32, error) {
	var total float32
	err := r.db.Model(&Detail{}).
		Where("room_id = ? AND start_time >= ? AND end_time <= ?",
			roomID, startTime, endTime).
		Select("COALESCE(SUM(cost), 0)").
		Scan(&total).Error
	return total, err
}

func (r *BillingRepository) GetCurrentServiceDetail(roomID int) (*Detail, error) {
	var detail Detail
	err := r.db.Where("room_id = ? AND end_time IS NULL", roomID).
		Order("start_time DESC").
		First(&detail).Error
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

// UpdateDetail 更新详单
func (r *BillingRepository) UpdateDetail(detail *Detail) error {
	return r.db.Save(detail).Error
}

// 需要在init.go中注册这个Repository
func NewBillingRepo() IBillingRepository {
	return NewBillingRepository(DB)
}
