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
			switchCount            int
			dispatchCount          int
			temperatureChangeCount int
			fanSpeedChangeCount    int
			totalCost              float32
			isInSession            bool
			servicePeriods         []ServicePeriod
			currentPeriod          *ServicePeriod
		)

		for _, detail := range details {
			totalCost += detail.Cost

			switch detail.DetailType {
			case db.DetailTypePowerOn:
				switchCount++
				isInSession = true

			case db.DetailTypePowerOff:
				isInSession = false

			case db.DetailTypeSpeedChange:
				if isInSession {
					fanSpeedChangeCount++
				}

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
				if isInSession {
					temperatureChangeCount++
				}
			}
		}

		// 计算总服务时长
		var totalDuration float32
		for _, period := range servicePeriods {
			duration := period.EndTime.Sub(period.StartTime).Minutes()
			totalDuration += float32(duration)
		}

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
