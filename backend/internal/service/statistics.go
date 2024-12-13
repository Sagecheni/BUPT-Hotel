// internal/service/statistics.go
package service

import (
	"backend/internal/db"
	"backend/internal/logger"
	"time"
)

type StatisticRecord struct {
	Room                   int     `json:"room"`                   // 房间号
	SwitchCount            int     `json:"switchCount"`            // 开关次数
	DispatchCount          int     `json:"dispatchCount"`          // 调度次数
	DetailCount            int     `json:"detailCount"`            // 详单条数
	TemperatureChangeCount int     `json:"temperatureChangeCount"` // 调温次数
	FanSpeedChangeCount    int     `json:"fanSpeedChangeCount"`    // 调风次数
	Duration               float32 `json:"duration"`               // 使用时长(分钟)
	TotalCost              float32 `json:"totalCost"`              // 总费用
}

type StatisticsService struct {
	detailRepo *db.DetailRepository
	roomRepo   *db.RoomRepository
}

func NewStatisticsService() *StatisticsService {
	return &StatisticsService{
		detailRepo: db.NewDetailRepository(),
		roomRepo:   db.NewRoomRepository(),
	}
}

// GetDailyReport 获取日报数据
func (s *StatisticsService) GetDailyReport(date time.Time) ([]StatisticRecord, error) {
	startTime := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endTime := startTime.Add(24 * time.Hour).Add(-time.Second)
	return s.getReport(startTime, endTime)
}

// GetWeeklyReport 获取周报数据
func (s *StatisticsService) GetWeeklyReport(date time.Time) ([]StatisticRecord, error) {
	offset := int(date.Weekday())
	if offset == 0 {
		offset = 7
	}
	monday := date.AddDate(0, 0, -offset+1)
	startTime := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, date.Location())
	endTime := startTime.Add(7 * 24 * time.Hour).Add(-time.Second)
	return s.getReport(startTime, endTime)
}

// ServicePeriod 表示一个服务时间段
type ServicePeriod struct {
	StartTime time.Time
	EndTime   time.Time
}

func (s *StatisticsService) getReport(startTime, endTime time.Time) ([]StatisticRecord, error) {
	rooms, err := s.roomRepo.GetAllRooms()
	if err != nil {
		return nil, err
	}

	statistics := make([]StatisticRecord, 0)

	for _, room := range rooms {
		details, err := s.detailRepo.GetDetailsByRoomAndTimeRange(room.RoomID, startTime, endTime)
		if err != nil {
			logger.Error("获取房间 %d 详单失败: %v", room.RoomID, err)
			continue
		}

		if len(details) == 0 {
			continue
		}

		var (
			dispatchCount          int
			temperatureChangeCount int
			fanSpeedChangeCount    int
			totalCost              float32
			servicePeriods         []ServicePeriod
			currentPeriod          *ServicePeriod
		)

		for _, detail := range details {
			totalCost += detail.Cost

			switch detail.DetailType {
			case db.DetailTypeSpeedChange:
				fanSpeedChangeCount++

			case db.DetailTypeServiceInterrupt:
				dispatchCount++
				if currentPeriod != nil {
					currentPeriod.EndTime = detail.EndTime
					servicePeriods = append(servicePeriods, *currentPeriod)
					currentPeriod = nil
				}

			case db.DetailTypeServiceStart:
				currentPeriod = &ServicePeriod{
					StartTime: detail.StartTime,
				}

			case db.DetailTypeTemp:
				temperatureChangeCount++
			}
		}

		// 获取该时间段内的开关次数
		var count int64
		if err := s.roomRepo.GetDB().Model(&db.RoomInfo{}).
			Where("room_id = ? AND last_power_on_time BETWEEN ? AND ?", room.RoomID, startTime, endTime).
			Count(&count).Error; err != nil {
			logger.Error("获取房间 %d 开关次数失败: %v", room.RoomID, err)
			continue
		}

		// 计算总服务时长
		var totalDuration float32
		for _, period := range servicePeriods {
			duration := period.EndTime.Sub(period.StartTime).Minutes()
			totalDuration += float32(duration)
		}
		switchCount := int(count)
		stat := StatisticRecord{
			Room:                   room.RoomID,
			SwitchCount:            switchCount,
			DispatchCount:          dispatchCount,
			DetailCount:            len(details),
			TemperatureChangeCount: temperatureChangeCount,
			FanSpeedChangeCount:    fanSpeedChangeCount,
			Duration:               totalDuration,
			TotalCost:              totalCost,
		}

		statistics = append(statistics, stat)
	}

	return statistics, nil
}
