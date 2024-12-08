// internal/service/billing.go
package service

import (
	"backend/internal/db"
	"fmt"
	"time"
)

// 电费费率 (元/度)
const PowerRate = 1.0

// 不同风速的费率 (元/分钟)
var speedToRate = map[string]float32{
	"high":   1.0,       // 1元/分钟 (1度电/分钟 * 1元/度)
	"medium": 1.0 / 2.0, // 0.5元/分钟 (0.5度电/分钟 * 1元/度)
	"low":    1.0 / 3.0, // 0.33元/分钟 (0.33度电/分钟 * 1元/度)
}

// BillingService 账单服务
type BillingService struct {
	roomRepo   *db.RoomRepository
	detailRepo *db.DetailRepository
	scheduler  *Scheduler
}

// BillResponse 账单响应
type BillResponse struct {
	RoomID        int         `json:"room_id"`
	CheckInTime   time.Time   `json:"check_in_time"`
	CheckOutTime  time.Time   `json:"check_out_time"`
	TotalDuration float32     `json:"total_duration"` // 总使用时长(分钟)
	TotalCost     float32     `json:"total_cost"`     // 总费用(元)
	Details       []db.Detail `json:"details"`        // 详单列表
}

// CurrentBill 实时费用计算结果
type CurrentBill struct {
	RoomID      int       `json:"room_id"`
	CurrentFee  float32   `json:"current_fee"`   // 当前时段费用(元)
	TotalFee    float32   `json:"total_fee"`     // 总费用(元)
	LastBilled  time.Time `json:"last_billed"`   // 上次计费时间点
	IsInService bool      `json:"is_in_service"` // 是否在服务队列中
}

// NewBillingService 创建账单服务
func NewBillingService(scheduler *Scheduler) *BillingService {
	return &BillingService{
		roomRepo:   db.NewRoomRepository(),
		detailRepo: db.NewDetailRepository(),
		scheduler:  scheduler,
	}
}

// CalculateCurrentFee 计算实时费用
func (s *BillingService) CalculateCurrentFee(roomID int) (*CurrentBill, error) {
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 获取本次入住以来的所有详单记录
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(
		roomID,
		room.CheckinTime,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("获取详单记录失败: %v", err)
	}

	result := &CurrentBill{
		RoomID:      roomID,
		CurrentFee:  0,
		TotalFee:    0,
		LastBilled:  time.Now(),
		IsInService: false,
	}

	// 计算总费用（累计所有历史费用）
	for _, detail := range details {
		result.TotalFee += detail.Cost
	}

	// 如果空调关闭，当前费用保持为0
	if room.ACState != 1 {
		return result, nil
	}

	// 空调开启时的处理
	// 1. 从最后一次开机时间开始累计所有费用

	// 从后向前找到最后一次开机时间（间隔超过5秒的记录）
	var lastPowerOnTime time.Time
	for i := len(details) - 1; i >= 0; i-- {
		if details[i].DetailType == db.DetailTypePowerOn {
			lastPowerOnTime = details[i].StartTime
			break
		}
	}
	if lastPowerOnTime.IsZero() {
		lastPowerOnTime = room.CheckinTime
	}

	// 2. 计算当前费用（本次开机后的所有费用）
	for _, detail := range details {
		if !detail.StartTime.Before(lastPowerOnTime) {
			result.CurrentFee += detail.Cost
		}
	}

	// 3. 如果在服务队列中，加上当前服务的实时费用
	if serviceObj, exists := s.scheduler.GetServiceQueue()[roomID]; exists {
		result.IsInService = true
		duration := float32(time.Since(serviceObj.StartTime).Minutes())
		rate := speedToRate[string(serviceObj.Speed)]
		currentServiceFee := duration * rate
		result.CurrentFee += currentServiceFee
		result.TotalFee += currentServiceFee
	}

	return result, nil
}

// CreateDetail 创建详单记录
func (s *BillingService) CreateDetail(roomID int, service *ServiceObject, detailType db.DetailType) error {
	now := time.Now()
	duration := float32(now.Sub(service.StartTime).Minutes()) // 转换为分钟
	rate := speedToRate[string(service.Speed)]
	cost := duration * rate

	detail := &db.Detail{
		RoomID:      roomID,
		QueryTime:   now,
		StartTime:   service.StartTime,
		EndTime:     now,
		ServeTime:   duration,
		Speed:       string(service.Speed),
		Cost:        cost,
		Rate:        rate,
		TempChange:  service.TargetTemp - service.CurrentTemp,
		DetailType:  detailType,
		CurrentTemp: service.CurrentTemp,
	}
	return s.detailRepo.CreateDetail(detail)
}

// GetDetails 获取详单记录
func (s *BillingService) GetDetails(roomID int, startTime, endTime time.Time) ([]db.Detail, error) {
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(roomID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("获取详单记录失败: %v", err)
	}
	return details, nil
}

// GetBillingService 获取账单服务实例
func GetBillingService() *BillingService {
	return billingService
}
