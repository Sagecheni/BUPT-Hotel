// internal/handlers/report_handler.go
package handlers

import (
	"backend/internal/logger"
	"backend/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ReportRequest struct {
	Period string `json:"period" binding:"required"`
}

type ReportResponse struct {
	DetailCount            string   `json:"detailCount"`            // 详单条数
	DispatchCount          string   `json:"dispatchCount"`          // 调度次数
	Duration               string   `json:"duration"`               // 请求时长
	FanSpeedChangeCount    string   `json:"fanSpeedChangeCount"`    // 调风次数
	Room                   *float64 `json:"room,omitempty"`         // 房间号
	SwitchCount            float64  `json:"switchCount"`            // 开关次数
	TemperatureChangeCount string   `json:"temperatureChangeCount"` // 调温次数
	TotalCost              string   `json:"totalCost"`              // 总费用
}

type ReportHandler struct {
	statsService *service.StatisticsService
}

func NewReportHandler() *ReportHandler {
	return &ReportHandler{
		statsService: service.NewStatisticsService(),
	}
}

func (h *ReportHandler) GetReport(c *gin.Context) {
	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 处理时间范围
	var stats []service.StatisticRecord
	var err error

	switch req.Period {
	case "daily":
		stats, err = h.statsService.GetDailyReport(time.Now())
	case "weekly":
		stats, err = h.statsService.GetWeeklyReport(time.Now())
	default:
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的时间周期，必须是 'daily' 或 'weekly'",
		})
		return
	}

	if err != nil {
		logger.Error("获取报表失败: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "获取报表失败",
			Err: err.Error(),
		})
		return
	}

	// 转换为响应格式
	responses := make([]ReportResponse, 0, len(stats))
	for _, stat := range stats {
		// 转换房间号
		roomFloat := float64(stat.Room)

		response := ReportResponse{
			DetailCount:            strconv.Itoa(stat.DetailCount),
			DispatchCount:          strconv.Itoa(stat.DispatchCount),
			Duration:               strconv.FormatFloat(float64(stat.Duration), 'f', 2, 32),
			FanSpeedChangeCount:    strconv.Itoa(stat.FanSpeedChangeCount),
			Room:                   &roomFloat,
			SwitchCount:            float64(stat.SwitchCount),
			TemperatureChangeCount: strconv.Itoa(stat.TemperatureChangeCount),
			TotalCost:              strconv.FormatFloat(float64(stat.TotalCost), 'f', 2, 32),
		}
		responses = append(responses, response)
	}

	c.JSON(http.StatusOK, Response{
		Msg:  "获取报表成功",
		Data: responses,
	})
}
