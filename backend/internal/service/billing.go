// internal/service/billing.go
package service

import (
	"backend/internal/db"
	"backend/internal/logger"
	"fmt"
	"math"
	"time"
)

// 费率定义
const (
	RateLow    = 0.5 // 低速费率 (元/度)
	RateMedium = 1.0 // 中速费率
	RateHigh   = 2.0 // 高速费率
)

// 费率映射表
var speedToRate = map[string]float32{
	SpeedLow:    RateLow,
	SpeedMedium: RateMedium,
	SpeedHigh:   RateHigh,
}

// 账单服务
type BillingService struct {
	roomRepo   *db.RoomRepository
	detailRepo *db.DetailRepository
}

// 账单响应
type BillResponse struct {
	RoomID        int       `json:"room_id"`
	CheckInTime   time.Time `json:"check_in_time"`
	CheckOutTime  time.Time `json:"check_out_time"`
	TotalDuration float32   `json:"total_duration"` // 总使用时长(秒)
	TotalCost     float32   `json:"total_cost"`     // 总费用
	Details       []Detail  `json:"details"`        // 详单列表
}

// 详单记录
type Detail struct {
	RoomID    int       `json:"room_id"`    // 房间号
	QueryTime time.Time `json:"query_time"` // 请求时间
	StartTime time.Time `json:"start_time"` // 服务开始时间
	EndTime   time.Time `json:"end_time"`   // 服务结束时间
	Duration  float32   `json:"duration"`   // 服务时长(秒)
	Speed     string    `json:"speed"`      // 风速
	Cost      float32   `json:"cost"`       // 当前费用
	Rate      float32   `json:"rate"`       // 费率
	// 额外的有用信息
	TempChange  float32 `json:"temp_change"`  // 温度变化
	CurrentTemp float32 `json:"current_temp"` // 当前温度
	TargetTemp  float32 `json:"target_temp"`  // 目标温度
}

// NewBillingService 创建账单服务
func NewBillingService() *BillingService {
	return &BillingService{
		roomRepo:   db.NewRoomRepository(),
		detailRepo: db.NewDetailRepository(),
	}
}

// CreateDetail 创建详单记录
func (s *BillingService) CreateDetail(roomID int, service *ServiceObject) error {
	now := time.Now()
	duration := float32(now.Sub(service.StartTime).Seconds())
	rate := speedToRate[service.Speed]

	// 计算温度变化和费用
	tempChange := float32(math.Abs(float64(service.CurrentTemp - service.TargetTemp)))
	cost := rate * tempChange

	detail := &db.Detail{
		RoomID:     roomID,
		QueryTime:  now,
		StartTime:  service.StartTime,
		EndTime:    now,
		ServeTime:  duration,
		Speed:      service.Speed,
		Cost:       cost,
		Rate:       rate,
		TempChange: tempChange,
	}

	return s.detailRepo.CreateDetail(detail)
}

// GenerateBill 生成账单和详单
func (s *BillingService) GenerateBill(roomID int) (*BillResponse, error) {
	room, err := s.roomRepo.GetRoomByID(roomID)
	if err != nil {
		return nil, fmt.Errorf("获取房间信息失败: %v", err)
	}

	// 获取详单记录
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(
		roomID,
		room.CheckinTime,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("获取详单记录失败: %v", err)
	}

	response := &BillResponse{
		RoomID:       roomID,
		CheckInTime:  room.CheckinTime,
		CheckOutTime: time.Now(),
	}

	for _, d := range details {
		detail := Detail{
			StartTime:  d.StartTime,
			EndTime:    d.EndTime,
			Duration:   d.ServeTime,
			Speed:      d.Speed,
			Rate:       d.Rate,
			Cost:       d.Cost,
			TempChange: d.TempChange,
		}
		response.Details = append(response.Details, detail)
		response.TotalDuration += detail.Duration
		response.TotalCost += detail.Cost
	}

	return response, nil
}

// AddDetail 添加详单记录
func (s *BillingService) AddDetail(roomID int, startTime time.Time, endTime time.Time,
	speed string, currentTemp float32, targetTemp float32) error {

	duration := float32(endTime.Sub(startTime).Seconds())
	rate := speedToRate[speed]
	tempChange := float32(math.Abs(float64(currentTemp - targetTemp)))
	cost := rate * tempChange

	detail := &db.Detail{
		RoomID:    roomID,
		QueryTime: time.Now(),
		StartTime: startTime,
		EndTime:   endTime,
		ServeTime: duration,
		Speed:     speed,
		Cost:      cost,
		Rate:      rate,
	}

	err := s.detailRepo.CreateDetail(detail)
	if err != nil {
		logger.Error("创建详单记录失败: %v", err)
		return err
	}

	return nil
}

// GetDetails 获取详单记录
func (s *BillingService) GetDetails(roomID int, startTime, endTime time.Time) ([]db.Detail, error) {
	details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(roomID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("获取详单记录失败: %v", err)
	}
	return details, nil
}
