// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/db"
	"backend/internal/scheduler"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 设置默认值请求
type SetDefaultsRequest struct {
	DefaultSpeed string  `json:"default_speed"`
	DefaultTemp  float32 `json:"default_temp"`
}

// 设置空调模式
type SetModeRequest struct {
	Mode string `json:"mode" binding:"required"` // cooling/heating
}

// 空调设置
type ACHandler struct {
	roomRepo  *db.RoomRepository
	scheduler *scheduler.Scheduler
}

// 开机请求
type PowerRequest struct {
	RoomID int `json:"room_id" binding:"required"`
}

type AirConditionRequest struct {
	RoomID     int      `json:"room_id" binding:"required"`
	TargetTemp *float32 `json:"target_temp,omitempty"` // 使用指针类型使其可选
	Speed      *string  `json:"speed,omitempty"`
}

func NewACHandler(scheduler *scheduler.Scheduler) *ACHandler {
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
	err = h.roomRepo.PowerOnAC(req.RoomID, room.Mode, defaults.DefaultTemp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "开启空调失败",
			Err:  err.Error(),
		})
		return
	}
	//设置默认风速
	inService, err := h.scheduler.HandleRequest(
		req.RoomID,
		defaults.DefaultSpeed,
		defaults.DefaultTemp,
		room.CurrentTemp, // 使用房间当前温度
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "设置默认参数失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "空调开启成功",
		Data: gin.H{
			"room_id":      req.RoomID,
			"mode":         room.Mode,
			"current_temp": room.CurrentTemp,
			"target_temp":  defaults.DefaultTemp,
			"speed":        defaults.DefaultSpeed,
			"in_service":   inService,
		},
	})
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

	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code: 404,
			Msg:  fmt.Sprintf("房间 %d 不存在", req.RoomID),
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

	// 关闭空调并清理调度队列
	err = h.roomRepo.PowerOffAC(req.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "关闭空调失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "空调关闭成功",
	})
}

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

	targetTemp := scheduler.DefaultTemp
	speed := scheduler.DefaultSpeed

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
		if speed != scheduler.SpeedLow &&
			speed != scheduler.SpeedMedium &&
			speed != scheduler.SpeedHigh {
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

	config := scheduler.DefaultConfig{
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
