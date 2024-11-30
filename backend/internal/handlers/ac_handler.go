// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/db"
	"backend/internal/service"
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
	OperationMode     string  `json:"operationMode"`     // 工作模式 "制冷"/"制热"
	TargetTemperature float32 `json:"targetTemperature"` // 目标温度
	CurrentCost       float32 `json:"currentCost"`       // 当前费用
	TotalCost         float32 `json:"totalCost"`         // 总费用
	CurrentFanSpeed   string  `json:"currentFanSpeed"`   // 当前风速
}

// PowerOffResponse 响应Poweroff请求结构
type PowerOffResponse struct {
	CurrentCost float32 `json:"currentCost"`
	TotalCost   float32 `json:"totalCost"`
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
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err:  err.Error(),
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "房间未入住，无法开启空调",
		})
		return
	}

	if room.ACState == 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "空调已开启",
		})
		return
	}
	// 使用默认配置
	defaults := h.scheduler.GetDefaultConfig()
	err = h.roomRepo.PowerOnAC(req.RoomNumber, room.Mode, defaults.DefaultTemp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "开启空调失败",
			Err:  err.Error(),
		})
		return
	}
	//设置默认风速
	_, err = h.scheduler.HandleRequest(
		req.RoomNumber,
		defaults.DefaultSpeed,
		defaults.DefaultTemp,
		room.CurrentTemp, // 使用房间已有的温度
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "添加到调度队列失败",
			Err:  err.Error(),
		})
		return
	}
	// 获取账单服务实例
	billingService := service.NewBillingService()
	// 获取费用信息
	bill, err := billingService.GenerateBill(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "获取账单失败",
			Err:  err.Error(),
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

	// 构造响应
	response := PowerOnResponse{
		OperationMode:     modeMap[room.Mode],                 // 工作模式转中文
		TargetTemperature: defaults.DefaultTemp,               // 使用默认目标温度
		CurrentCost:       currentCost,                        // 当前费用
		TotalCost:         bill.TotalCost,                     // 总费用
		CurrentFanSpeed:   fanSpeedMap[defaults.DefaultSpeed], // 风速转中文
	}

	c.JSON(http.StatusOK, response)
}

func (h *ACHandler) PowerOff(c *gin.Context) {
	var req PowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err:  err.Error(),
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "空调未开启",
		})
		return
	}

	// 获取账单服务实例
	billingService := service.NewBillingService()

	// 获取本次开机的费用和总费用
	bill, err := billingService.GenerateBill(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "获取账单失败",
			Err:  err.Error(),
		})
		return
	}

	// 关闭空调并清理调度队列
	h.scheduler.RemoveRoom(req.RoomNumber)
	err = h.roomRepo.PowerOffAC(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "关闭空调失败",
			Err:  err.Error(),
		})
		return
	}

	// 计算当前消费（从最后一次开机到现在的消费）
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

	c.JSON(http.StatusOK, PowerOffResponse{
		CurrentCost: currentCost,
		TotalCost:   bill.TotalCost,
	})
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

// 设置空调温度和风速
func (h *ACHandler) SetAirCondition(c *gin.Context) {
	var req AirConditionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}
	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomID),
			Err:  err.Error(),
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "房间未入住",
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "请先开启空调",
		})
		return
	}

	targetTemp := service.DefaultTemp
	speed := service.DefaultSpeed

	// 如果请求中包含了温度，使用请求的温度
	if req.TargetTemp != nil {
		targetTemp = *req.TargetTemp
		// 验证温度是否在合法范围内
		if err := h.validateTemp(room.Mode, targetTemp); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Code: 400,
				Msg:  err.Error(),
			})
			return
		}
	} else {
		// 如果没有指定温度，使用房间当前的目标温度
		targetTemp = room.TargetTemp
	}

	// 如果请求中包含了风速，使用请求的风速
	if req.Speed != nil {
		speed = *req.Speed
		// 验证风速值
		if speed != service.SpeedLow &&
			speed != service.SpeedMedium &&
			speed != service.SpeedHigh {
			c.JSON(http.StatusBadRequest, Response{
				Code: 400,
				Msg:  "无效的风速值",
			})
			return
		}
	} else {
		// 如果没有指定风速，使用房间当前的风速
		if room.CurrentSpeed != "" {
			speed = room.CurrentSpeed
		}
	}

	inService, err := h.scheduler.HandleRequest(
		req.RoomID,
		speed,
		targetTemp,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "处理空调请求失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "空调设置成功",
		Data: gin.H{
			"room_id":     req.RoomID,
			"target_temp": req.TargetTemp,
			"speed":       req.Speed,
			"in_service":  inService,
		},
	})
}

func (h *ACHandler) validateTemp(mode string, temp float32) error {
	switch mode {
	case "cooling":
		if temp < 18 || temp > 28 {
			return fmt.Errorf("制冷模式温度范围为16-24度")
		}
	case "heating":
		if temp < 22 || temp > 28 {
			return fmt.Errorf("制热模式温度范围为22-28度")
		}
	default:
		return fmt.Errorf("无效的工作模式")
	}
	return nil
}

func (h *ACHandler) GetSchedulerStatus(c *gin.Context) {
	// 获取服务队列状态
	serviceQueue := h.scheduler.GetServiceQueue()
	serviceStatus := make([]map[string]interface{}, 0)

	for roomID, service := range serviceQueue {
		serviceStatus = append(serviceStatus, map[string]interface{}{
			"room_id":      roomID,
			"current_temp": service.CurrentTemp,
			"target_temp":  service.TargetTemp,
			"speed":        service.Speed,
			"duration":     service.Duration,
			"is_completed": service.IsCompleted,
			"start_time":   service.StartTime.Format("15:04:05"),
		})
	}

	// 获取等待队列状态
	waitQueue := h.scheduler.GetWaitQueue()
	waitStatus := make([]map[string]interface{}, 0)

	for _, wait := range waitQueue {
		waitStatus = append(waitStatus, map[string]interface{}{
			"room_id":       wait.RoomID,
			"current_temp":  wait.CurrentTemp,
			"target_temp":   wait.TargetTemp,
			"speed":         wait.Speed,
			"wait_duration": wait.WaitDuration,
			"request_time":  wait.RequestTime.Format("15:04:05"),
		})
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "获取调度状态成功",
		Data: gin.H{
			"service_queue_size": len(serviceQueue),
			"wait_queue_size":    len(waitQueue),
			"service_queue":      serviceStatus,
			"wait_queue":         waitStatus,
		},
	})
}

func (h *ACHandler) SetDefaults(c *gin.Context) {
	var req SetDefaultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	config := service.DefaultConfig{
		DefaultSpeed: req.DefaultSpeed,
		DefaultTemp:  req.DefaultTemp,
	}

	if err := h.scheduler.SetDefaultConfig(config); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "设置默认参数失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "设置默认参数成功",
		Data: config,
	})
}

func (h *ACHandler) GetDefaults(c *gin.Context) {
	config := h.scheduler.GetDefaultConfig()

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "获取默认参数成功",
		Data: config,
	})
}

// 调节温度
func (h *ACHandler) ChangeTemperature(c *gin.Context) {
	var req ChangeTempRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err:  err.Error(),
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "房间未入住",
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "空调未开启",
		})
		return
	}

	// 验证温度范围
	if err := h.validateTemp(room.Mode, req.TargetTemperature); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  err.Error(),
		})
		return
	}

	// 使用当前风速（如果没有风速，使用默认值）
	currentSpeed := room.CurrentSpeed
	if currentSpeed == "" {
		currentSpeed = service.DefaultSpeed
	}

	// 请求温度变化
	inService, err := h.scheduler.HandleRequest(
		req.RoomNumber,
		currentSpeed,
		req.TargetTemperature,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "处理温度调节请求失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "温度调节请求已接受",
		Data: gin.H{
			"room_id":      req.RoomNumber,
			"target_temp":  req.TargetTemperature,
			"current_temp": room.CurrentTemp,
			"speed":        currentSpeed,
			"in_service":   inService,
		},
	})
}

// 调节风速
func (h *ACHandler) ChangeSpeed(c *gin.Context) {
	var req ChangeSpeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	// 转换中文风速为英文
	speed, ok := speedMap[req.CurrentFanSpeed]
	if !ok {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的风速值",
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err:  err.Error(),
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "房间未入住",
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "空调未开启",
		})
		return
	}

	// 请求风速变化
	inService, err := h.scheduler.HandleRequest(
		req.RoomNumber,
		speed,
		room.TargetTemp,
		room.CurrentTemp,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "处理风速调节请求失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "风速调节请求已接受",
		Data: gin.H{
			"room_id":      req.RoomNumber,
			"current_temp": room.CurrentTemp,
			"target_temp":  room.TargetTemp,
			"speed":        speed,
			"in_service":   inService,
		},
	})
}

// RoomStatus 获取房间当前状态
func (h *ACHandler) RoomStatus(c *gin.Context) {
	var req RoomStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "Invalid request",
			Err:  err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err:  err.Error(),
		})
		return
	}

	// 获取账单服务实例
	billingService := service.NewBillingService()

	// 获取总费用信息
	bill, err := billingService.GenerateBill(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "获取账单失败",
			Err:  err.Error(),
		})
		return
	}

	// 获取当前费用（本次开机到现在的费用）
	var currentCost float32 = 0
	if room.ACState == 1 { // 只有空调开启时才计算当前费用
		details, err := billingService.GetDetails(req.RoomNumber, room.CheckinTime, time.Now())
		if err == nil && len(details) > 0 {
			// 找到最后一次开机时间
			var lastPowerOnTime time.Time
			for _, detail := range details {
				if detail.StartTime.After(lastPowerOnTime) {
					lastPowerOnTime = detail.StartTime
				}
			}
			// 计算从最后一次开机到现在的费用
			for _, detail := range details {
				if detail.StartTime.After(lastPowerOnTime) || detail.StartTime.Equal(lastPowerOnTime) {
					currentCost += detail.Cost
				}
			}
		}
	}

	response := RoomStatusResponse{
		CurrentCost:        currentCost,
		TotalCost:          bill.TotalCost,
		CurrentTemperature: room.CurrentTemp,
	}

	c.JSON(http.StatusOK, response)
}
