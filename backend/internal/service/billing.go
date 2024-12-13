// internal/service/billing.go
package service

import (
	"backend/internal/db"
	"fmt"
	"math"
	"time"
)

// 电费费率 (元/度)
const PowerRate = 1.0
const TimeScale = 6.0

// roundTo2Decimals 将浮点数四舍五入到2位小数
func roundTo2Decimals(value float32) float32 {
	return float32(math.Round(float64(value)*100) / 100)
}

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

// CalculateCurrentSessionFee 计算本次开机会话的费用（从开机到现在）
func (s *BillingService) CalculateCurrentSessionFee(roomID int) (float32, error) {
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return 0, fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 空调关闭时，当前费用为0
	if room.ACState != 1 {
		return 0, nil
	}

	// 获取本次开机以来的所有详单记录
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(
		roomID,
		room.LastPowerOnTime, // 使用LastPowerOnTime替代查找PowerOn详单
		time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("获取详单记录失败: %v", err)
	}
	var currentFee float32
	var lastServiceStart time.Time
	var isInService bool

	// 遍历详单记录,根据服务开始和中断事件计算费用
	for _, detail := range details {
		switch detail.DetailType {
		case db.DetailTypeServiceStart:
			lastServiceStart = detail.StartTime
			isInService = true
		case db.DetailTypeServiceInterrupt:
			if isInService {
				duration := calculateScaledDuration(lastServiceStart, detail.EndTime)
				rate := speedToRate[detail.Speed]
				currentFee += roundTo2Decimals(duration * rate)
				isInService = false
			}
		case db.DetailTypeSpeedChange:
			if isInService {
				// 计算切换前的费用
				duration := calculateScaledDuration(lastServiceStart, detail.EndTime)
				rate := speedToRate[detail.Speed]
				currentFee += roundTo2Decimals(duration * rate)
				// 更新新服务段的开始时间和费率
				lastServiceStart = detail.EndTime
			}
		}
	}

	// 如果在服务队列中，计算实时费用
	if isInService {
		if serviceObj, exists := s.scheduler.GetServiceQueue()[roomID]; exists {
			now := time.Now()
			duration := calculateScaledDuration(lastServiceStart, now)
			rate := speedToRate[string(serviceObj.Speed)]
			currentServiceFee := roundTo2Decimals(duration * rate)
			currentFee = roundTo2Decimals(currentFee + currentServiceFee)
		}
	}

	return currentFee, nil
}

// CalculateTotalFee 计算总费用
func (s *BillingService) CalculateTotalFee(roomID int) (float32, error) {
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return 0, fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 获取所有详单记录
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(
		roomID,
		room.CheckinTime,
		time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("获取详单记录失败: %v", err)
	}

	var totalFee float32
	var lastServiceStart time.Time
	var isInService bool

	// 遍历所有详单,根据服务开始和中断事件计算费用
	for _, detail := range details {
		switch detail.DetailType {
		case db.DetailTypeServiceStart:
			lastServiceStart = detail.StartTime
			isInService = true
		case db.DetailTypeServiceInterrupt:
			if isInService {
				duration := calculateScaledDuration(lastServiceStart, detail.EndTime)
				rate := speedToRate[detail.Speed]
				totalFee += roundTo2Decimals(duration * rate)
				isInService = false
			}
		case db.DetailTypeSpeedChange:
			if isInService {
				// 计算切换前的费用
				duration := calculateScaledDuration(lastServiceStart, detail.EndTime)
				rate := speedToRate[detail.Speed]
				totalFee += roundTo2Decimals(duration * rate)
				// 更新新服务段的开始时间和费率
				lastServiceStart = detail.EndTime
			}
		}

	}

	// 如果当前正在服务中,计算最后一段服务的费用
	if isInService && room.ACState == 1 {
		if serviceObj, exists := s.scheduler.GetServiceQueue()[roomID]; exists {
			now := time.Now()
			duration := calculateScaledDuration(lastServiceStart, now)
			rate := speedToRate[string(serviceObj.Speed)]
			currentServiceFee := roundTo2Decimals(duration * rate)
			totalFee = roundTo2Decimals(totalFee + currentServiceFee)
		}
	}

	return totalFee, nil
}

// calculateScaledDuration 计算缩放后的持续时间(分钟)
func calculateScaledDuration(start time.Time, end time.Time) float32 {
	realDuration := end.Sub(start).Seconds()
	// 将实际秒数转换为模拟的分钟数 (10秒=1分钟)
	return float32(realDuration) * float32(TimeScale) / 60.0
}

// CreateDetail 创建详单记录
func (s *BillingService) CreateDetail(roomID int, service *ServiceObject, detailType db.DetailType) error {
	now := time.Now()
	rate := speedToRate[string(service.Speed)]

	detail := &db.Detail{
		RoomID:      roomID,
		QueryTime:   now,
		StartTime:   service.StartTime,
		EndTime:     now,
		ServeTime:   roundTo2Decimals(calculateScaledDuration(service.StartTime, now)),
		Speed:       string(service.Speed),
		Rate:        rate,
		TempChange:  roundTo2Decimals(service.TargetTemp - service.CurrentTemp),
		DetailType:  detailType,
		TargetTemp:  service.TargetTemp,
		CurrentTemp: roundTo2Decimals(service.CurrentTemp),
	}
	// 只有服务中断和关机时才计算费用
	if detailType == db.DetailTypeServiceInterrupt {
		detail.Cost = roundTo2Decimals(detail.ServeTime * detail.Rate)
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
