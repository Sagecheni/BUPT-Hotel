package billing

import (
	"backend/internal/db"
	"time"
)

// Speed Constants
const (
	SpeedLow    = "low"
	SpeedMedium = "medium"
	SpeedHigh   = "high"
)

// Rate Constants (元/°C)
var SpeedRateMap = map[string]float32{
	SpeedLow:    0.5,
	SpeedMedium: 1.0,
	SpeedHigh:   2.0,
}

// 费率常量
const (
	RateLow    = 0.5 // 低速风费率
	RateMedium = 1.0 // 中速风费率
	RateHigh   = 2.0 // 高速风费率
)

type BillingService interface {
	// CalculateCurrentFee 计算当前服务的实时费用
	CalculateCurrentFee(roomID int) (float32, error)

	// CreateServiceDetail 创建新的服务详情
	CreateServiceDetail(roomID int, speed string, initialTemp float32) error

	// UpdateServiceDetail 更新服务详情
	UpdateServiceDetail(roomID int, currentTemp float32) error

	// CompleteServiceDetail 完成服务详情
	CompleteServiceDetail(roomID int, finalTemp float32) error

	// GetBillDetails 获取账单详情
	GetBillDetails(roomID int, startTime, endTime time.Time) ([]*db.ServiceDetail, error)

	// CalculateTotalFee 计算总费用
	CalculateTotalFee(roomID int, startTime, endTime time.Time) (float32, error)
}

type billingService struct {
	serviceRepo db.ServiceRepositoryInterface
}

func NewBillingService(serviceRepo db.ServiceRepositoryInterface) BillingService {
	return &billingService{
		serviceRepo: serviceRepo,
	}
}
func (s *billingService) CalculateCurrentFee(roomID int) (float32, error) {
	detail, err := s.serviceRepo.GetActiveServiceDetail(roomID)
	if err != nil {
		return 0, err
	}
	if detail == nil {
		return 0, nil
	}

	// 计算当前费用
	tempDiff := detail.InitialTemp - detail.FinalTemp
	if tempDiff < 0 {
		tempDiff = -tempDiff
	}
	rate := getSpeedRate(detail.Speed)
	currentFee := tempDiff * rate

	return currentFee, nil
}

// 获取风速对应的费率
func getSpeedRate(speed string) float32 {
	switch speed {
	case "high":
		return RateHigh
	case "medium":
		return RateMedium
	case "low":
		return RateLow
	default:
		return 0
	}
}

func (s *billingService) CreateServiceDetail(roomID int, speed string, initialTemp float32) error {
	detail := &db.ServiceDetail{
		RoomID:       roomID,
		StartTime:    time.Now(),
		InitialTemp:  initialTemp,
		Speed:        speed,
		ServiceState: "active",
	}
	return s.serviceRepo.CreateServiceDetail(detail)
}

func (s *billingService) UpdateServiceDetail(roomID int, currentTemp float32) error {
	detail, err := s.serviceRepo.GetActiveServiceDetail(roomID)
	if err != nil {
		return err
	}
	if detail == nil {
		return nil
	}

	// 计算当前费用
	duration := float32(time.Since(detail.StartTime).Seconds())
	tempDiff := detail.InitialTemp - currentTemp
	if tempDiff < 0 {
		tempDiff = -tempDiff
	}
	rate := getSpeedRate(detail.Speed)
	cost := tempDiff * rate

	detail.ServiceDuration = duration
	detail.FinalTemp = currentTemp
	detail.Cost = cost
	return s.serviceRepo.UpdateServiceDetail(detail)
}

func (s *billingService) CompleteServiceDetail(roomID int, finalTemp float32) error {
	detail, err := s.serviceRepo.GetActiveServiceDetail(roomID)
	if err != nil {
		return err
	}
	if detail == nil {
		return nil
	}

	now := time.Now()
	duration := float32(now.Sub(detail.StartTime).Seconds())
	tempDiff := detail.InitialTemp - finalTemp
	if tempDiff < 0 {
		tempDiff = -tempDiff
	}
	rate := getSpeedRate(detail.Speed)
	cost := tempDiff * rate

	detail.EndTime = now
	detail.ServiceDuration = duration
	detail.FinalTemp = finalTemp
	detail.ServiceState = "completed"
	detail.Cost = cost
	detail.TotalFee = cost
	return s.serviceRepo.UpdateServiceDetail(detail)
}

func (s *billingService) GetBillDetails(roomID int, startTime, endTime time.Time) ([]*db.ServiceDetail, error) {
	return s.serviceRepo.GetServiceHistory(roomID, startTime, endTime)
}

func (s *billingService) CalculateTotalFee(roomID int, startTime, endTime time.Time) (float32, error) {
	details, err := s.serviceRepo.GetServiceHistory(roomID, startTime, endTime)
	if err != nil {
		return 0, err
	}

	var totalFee float32
	for _, detail := range details {
		if detail.ServiceState == "completed" {
			totalFee += detail.TotalFee
		}
	}
	return totalFee, nil
}
