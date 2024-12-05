// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/ac"
	"backend/internal/db"
	"backend/internal/service"
	"backend/internal/types"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 设置默认值请求
type SetDefaultsRequest struct {
	DefaultSpeed string  `json:"default_speed"`
	DefaultTemp  float32 `json:"default_temp"`
}

// 房间状态查询请求
type RoomStatusRequest struct {
	RoomNumber int `json:"roomNumber" binding:"required"`
}

// 房间状态响应
type RoomStatusResponse struct {
	CurrentCost        float32 `json:"currentCost"`
	TotalCost          float32 `json:"totalCost"`
	CurrentTemperature float32 `json:"currentTemperature"`
}

// PowerOnResponse 响应PowerOn请求结构
type PowerOnResponse struct {
	CurrentCost        float32 `json:"currentCost"`        // 当前费用
	CurrentFanSpeed    string  `json:"currentFanSpeed"`    // 当前风速
	CurrentTemperature float32 `json:"currentTemperature"` // 当前温度
	OperationMode      string  `json:"operationMode"`      // 工作模式
	TargetTemperature  float32 `json:"targetTemperature"`  // 目标温度
	TotalCost          float32 `json:"totalCost"`          // 总费用
}

// PowerOffResponse 关机响应结构体
type PowerOffResponse struct {
	CurrentCost float32 `json:"currentCost"` // 本次开机的费用
	TotalCost   float32 `json:"totalCost"`   // 总费用
}

// 模式映射表（英文到中文）
var modeMap = map[string]string{
	"cooling": "制冷",
	"heating": "制热",
}

// 风速映射表（英文到中文）
var fanSpeedMap = map[string]string{
	"low":    "低",
	"medium": "中",
	"high":   "高",
}

// 设置空调模式
type SetModeRequest struct {
	Mode string `json:"mode" binding:"required"` // cooling/heating
}

// 温度调节请求
type ChangeTempRequest struct {
	RoomNumber        int     `json:"roomNumber" binding:"required"`
	TargetTemperature float32 `json:"targetTemperature" binding:"required"`
}

// 风速调节请求
type ChangeSpeedRequest struct {
	RoomNumber      int    `json:"roomNumber" binding:"required"`
	CurrentFanSpeed string `json:"currentFanSpeed" binding:"required"`
}

// 空调设置
type ACHandler struct {
	roomRepo  *db.RoomRepository
	scheduler *service.Scheduler
}

// 风速映射表(中文到英文)
var speedMap = map[string]string{
	"低": "low",
	"中": "medium",
	"高": "high",
}

// 开机请求
type PowerRequest struct {
	RoomNumber int `json:"roomNumber" binding:"required"` // 房间号
}

type AirConditionRequest struct {
	RoomID     int      `json:"room_id" binding:"required"`
	TargetTemp *float32 `json:"target_temp,omitempty"` // 使用指针类型使其可选
	Speed      *string  `json:"speed,omitempty"`
}

func NewACHandler(scheduler *service.Scheduler) *ACHandler {
	return &ACHandler{
		roomRepo:  db.NewRoomRepository(),
		scheduler: scheduler,
	}
}

func (h *ACHandler) PowerOn(c *gin.Context) {
	var req PowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
		})
		return
	}

	// 检查房间是否已入住
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间未入住，无法开启空调",
		})
		return
	}

	// 检查空调是否已开启
	if room.ACState == 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "空调已开启",
		})
		return
	}

	// 使用系统默认配置
	err = h.roomRepo.PowerOnAC(req.RoomNumber, room.Mode, ac.DefaultConfig.DefaultTemp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "开启空调失败",
		})
		return
	}

	// 加入调度队列
	_, err = service.GetScheduler().HandleRequest(
		req.RoomNumber,
		ac.DefaultConfig.DefaultSpeed,
		ac.DefaultConfig.DefaultTemp,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "添加到调度队列失败",
		})
		return
	}

	// 获取账单服务
	billingService := service.GetBillingService()

	// 获取费用信息
	bill, err := billingService.GenerateBill(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取账单失败",
		})
		return
	}

	// 获取当前费用（本次开机到现在的费用）
	var currentCost float32 = 0
	details, err := billingService.GetDetails(req.RoomNumber, time.Now(), time.Now())
	if err == nil && len(details) > 0 {
		for _, detail := range details {
			currentCost += detail.Cost
		}
	}

	// 构造返回结果
	response := PowerOnResponse{
		CurrentCost:        currentCost,
		CurrentFanSpeed:    fanSpeedMap[string(ac.DefaultConfig.DefaultSpeed)], // 转换为中文
		CurrentTemperature: room.CurrentTemp,
		OperationMode:      modeMap[room.Mode], // 转换为中文
		TargetTemperature:  ac.DefaultConfig.DefaultTemp,
		TotalCost:          bill.TotalCost,
	}

	c.JSON(http.StatusOK, response)
}

func (h *ACHandler) PowerOff(c *gin.Context) {
	var req PowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
		})
		return
	}

	// 检查空调是否已开启
	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "空调未开启",
		})
		return
	}

	// 获取账单服务
	billingService := service.GetBillingService()

	// 获取账单信息
	bill, err := billingService.GenerateBill(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取账单失败",
		})
		return
	}

	// 计算本次开机的费用
	var currentCost float32 = 0
	if len(bill.Details) > 0 {
		lastPowerOnTime := room.CheckinTime // 默认使用入住时间
		// 查找最后一次开机的时间
		for _, detail := range bill.Details {
			if detail.StartTime.After(lastPowerOnTime) {
				lastPowerOnTime = detail.StartTime
			}
		}
		// 获取这段时间内的消费
		details, err := billingService.GetDetails(req.RoomNumber, lastPowerOnTime, time.Now())
		if err == nil {
			for _, detail := range details {
				currentCost += detail.Cost
			}
		}
	}

	// 从调度队列中移除房间
	service.GetScheduler().RemoveRoom(req.RoomNumber)

	// 关闭房间空调
	err = h.roomRepo.PowerOffAC(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "关闭空调失败",
		})
		return
	}

	// 构造返回结果
	response := PowerOffResponse{
		CurrentCost: currentCost,
		TotalCost:   bill.TotalCost,
	}

	c.JSON(http.StatusOK, response)
}

// 设置空调模式
func (h *ACHandler) SetMode(c *gin.Context) {
	var req SetModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	// 验证
	if req.Mode != "cooling" && req.Mode != "heating" {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的工作模式，必须是 cooling 或 heating",
		})
		return
	}

	// 更新所有房间的工作模式
	if err := h.roomRepo.SetACMode(req.Mode); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "设置工作模式失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "设置工作模式成功",
		Data: gin.H{
			"mode": req.Mode,
		},
	})
}

// ChangeTemperature 修改房间空调温度
func (h *ACHandler) ChangeTemperature(c *gin.Context) {
	var req ChangeTempRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
		})
		return
	}

	// 检查房间是否已入住
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间未入住",
		})
		return
	}

	// 检查空调是否开启
	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "空调未开启",
		})
		return
	}

	// 验证温度范围
	if mode := types.Mode(room.Mode); mode == types.ModeCooling {
		if req.TargetTemperature < ac.DefaultConfig.TempRanges[types.ModeCooling].Min ||
			req.TargetTemperature > ac.DefaultConfig.TempRanges[types.ModeCooling].Max {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("制冷模式温度范围为 %.1f-%.1f 度",
					ac.DefaultConfig.TempRanges[types.ModeCooling].Min,
					ac.DefaultConfig.TempRanges[types.ModeCooling].Max),
			})
			return
		}
	} else if mode == types.ModeHeating {
		if req.TargetTemperature < ac.DefaultConfig.TempRanges[types.ModeHeating].Min ||
			req.TargetTemperature > ac.DefaultConfig.TempRanges[types.ModeHeating].Max {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("制热模式温度范围为 %.1f-%.1f 度",
					ac.DefaultConfig.TempRanges[types.ModeHeating].Min,
					ac.DefaultConfig.TempRanges[types.ModeHeating].Max),
			})
			return
		}
	}

	// 获取当前风速，如果没有则使用默认风速
	currentSpeed := room.CurrentSpeed
	if currentSpeed == "" {
		currentSpeed = string(ac.DefaultConfig.DefaultSpeed)
	}

	// 提交温度调节请求
	_, err = service.GetScheduler().HandleRequest(
		req.RoomNumber,
		types.Speed(currentSpeed),
		req.TargetTemperature,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "处理温度调节请求失败",
		})
		return
	}

	c.Status(http.StatusOK)
}

// ChangeSpeed 修改房间空调风速
func (h *ACHandler) ChangeSpeed(c *gin.Context) {
	var req ChangeSpeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	// 转换中文风速为英文
	speed, ok := speedMap[req.CurrentFanSpeed]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的风速值，只能为低、中、高",
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
		})
		return
	}

	// 检查房间是否已入住
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "房间未入住",
		})
		return
	}

	// 检查空调是否开启
	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "空调未开启",
		})
		return
	}

	// 提交风速调节请求
	_, err = service.GetScheduler().HandleRequest(
		req.RoomNumber,
		types.Speed(speed),
		room.TargetTemp,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "处理风速调节请求失败",
		})
		return
	}

	c.Status(http.StatusOK)
}
