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
    details, err := s.getDetailsSinceLastPowerOn(roomID)
    if err != nil {
        return 0, fmt.Errorf("获取详单记录失败: %v", err)
    }

    // 计算已产生的详单费用
    var currentFee float32
    for _, detail := range details {
        if detail.DetailType != db.DetailTypePowerOn {
            currentFee += detail.Cost
        }
    }

    // 如果在服务队列中，计算实时费用
    if serviceObj, exists := s.scheduler.GetServiceQueue()[roomID]; exists {
        duration := float32(time.Since(serviceObj.StartTime).Minutes())
        rate := speedToRate[string(serviceObj.Speed)]
        currentServiceFee := roundTo2Decimals(duration * rate)
        currentFee = roundTo2Decimals(currentFee + currentServiceFee)
    }

    return currentFee, nil
}

// getDetailsSinceLastPowerOn 获取最近一次开机以来的所有详单
func (s *BillingService) getDetailsSinceLastPowerOn(roomID int) ([]db.Detail, error) {
    room, err := s.roomRepo.GetRoomByID(roomID)
    if err != nil {
        return nil, err
    }

    details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(
        roomID,
        room.CheckinTime,
        time.Now(),
    )
    if err != nil {
        return nil, err
    }

    // 找到最后一次开机时间
    var lastPowerOnTime time.Time
    for i := len(details) - 1; i >= 0; i-- {
        if details[i].DetailType == db.DetailTypePowerOn {
            lastPowerOnTime = details[i].StartTime
            break
        }
    }
    if lastPowerOnTime.IsZero() {
        return nil, nil
    }

    // 过滤出最后一次开机之后的详单
    currentDetails := make([]db.Detail, 0)
    for _, detail := range details {
        if !detail.StartTime.Before(lastPowerOnTime) {
            currentDetails = append(currentDetails, detail)
        }
    }

    return currentDetails, nil
}

// CalculateTotalFee 计算总费用（避免重复计算）
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

    // 按开关机周期分组计算
    var cycles [][]db.Detail
    var currentCycle []db.Detail

    for _, detail := range details {
        if detail.DetailType == db.DetailTypePowerOn {
            // 开始新的周期
            if len(currentCycle) > 0 {
                cycles = append(cycles, currentCycle)
            }
            currentCycle = []db.Detail{detail}
        } else {
            if len(currentCycle) > 0 {
                currentCycle = append(currentCycle, detail)
                if detail.DetailType == db.DetailTypePowerOff {
                    cycles = append(cycles, currentCycle)
                    currentCycle = nil
                }
            }
        }
    }

    // 如果最后一个周期未结束，也加入统计
    if len(currentCycle) > 0 {
        cycles = append(cycles, currentCycle)
    }

    // 计算每个周期的费用
    for _, cycle := range cycles {
        if len(cycle) > 0 {
            lastDetail := cycle[len(cycle)-1]
            if lastDetail.DetailType == db.DetailTypePowerOff {
                // 已结束的周期使用关机详单的费用
                totalFee += lastDetail.Cost
            } else if room.ACState == 1 {
                // 当前正在进行的周期
                if currentFee, err := s.CalculateCurrentSessionFee(roomID); err == nil {
                    totalFee += currentFee
                }
            }
        }
    }

    return roundTo2Decimals(totalFee), nil
}

// CreateDetail 创建详单记录
func (s *BillingService) CreateDetail(roomID int, service *ServiceObject, detailType db.DetailType) error {
	now := time.Now()
	duration := float32(now.Sub(service.StartTime).Minutes())
	rate := speedToRate[string(service.Speed)]
	cost := roundTo2Decimals(duration * rate)

	detail := &db.Detail{
		RoomID:      roomID,
		QueryTime:   now,
		StartTime:   service.StartTime,
		EndTime:     now,
		ServeTime:   roundTo2Decimals(duration),
		Speed:       string(service.Speed),
		Cost:        cost,
		Rate:        rate,
		TempChange:  roundTo2Decimals(service.TargetTemp - service.CurrentTemp),
		DetailType:  detailType,
		CurrentTemp: roundTo2Decimals(service.CurrentTemp),
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
