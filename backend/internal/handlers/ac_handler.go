// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/ac"
	"backend/internal/billing"
	"backend/internal/logger"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ACHandler 定义空调处理器结构
type ACHandler struct {
	acService      ac.ACService
	billingService billing.BillingService // 添加计费服务
}

// NewACHandler 创建空调处理器实例
func NewACHandler(acService ac.ACService, billingService billing.BillingService) *ACHandler {
	return &ACHandler{
		acService:      acService,
		billingService: billingService,
	}
}

// ACResponse 空调开机状态响应结构
type ACResponse struct {
	OperationMode      string  `json:"operationMode"`      // 运行模式（制冷/制热）
	TargetTemperature  float32 `json:"targetTemperature"`  // 目标温度
	CurrentTemperature float32 `json:"currentTemperature"` // 当前温度
	CurrentFanSpeed    string  `json:"currentFanSpeed"`    // 当前风速
	CurrentCost        float32 `json:"currentCost"`        // 当前费用
	TotalCost          float32 `json:"totalCost"`          // 总费用
}

// PowerOffResponse 关机响应结构
type PowerOffResponse struct {
	CurrentCost float32 `json:"currentCost"`
	TotalCost   float32 `json:"totalCost"`
}

// 将空调模式转换为中文
func translateMode(mode string) string {
	switch mode {
	case "cooling":
		return "制冷"
	case "heating":
		return "制热"
	default:
		return mode
	}
}

// 将风速转换为中文
func translateSpeed(speed string) string {
	switch speed {
	case "low":
		return "低"
	case "medium":
		return "中"
	case "high":
		return "高"
	default:
		return ""
	}
}

// =============== 顾客接口 ===============

// PowerOn 顾客开启房间空调
func (h *ACHandler) CustomerPowerOn(c *gin.Context) {
	var req struct {
		RoomNumber int `json:"roomNumber" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	// 开启空调
	if err := h.acService.PowerOn(req.RoomNumber); err != nil {
		logger.Error("Failed to power on AC: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 获取空调状态
	state, err := h.acService.GetACState(req.RoomNumber)
	if err != nil {
		logger.Error("Failed to get AC state: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 获取费用信息
	currentCost, err := h.billingService.CalculateCurrentFee(req.RoomNumber)
	if err != nil {
		logger.Error("Failed to calculate current fee: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	totalCost, err := h.billingService.CalculateTotalFee(req.RoomNumber, time.Now().AddDate(0, 0, -1), time.Now())
	if err != nil {
		logger.Error("Failed to calculate total fee: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}
	// 构造响应
	response := ACResponse{
		OperationMode:      translateMode(state.Mode),
		TargetTemperature:  state.TargetTemp,
		CurrentTemperature: state.CurrentTemp,
		CurrentFanSpeed:    translateSpeed(state.Speed),
		CurrentCost:        currentCost,
		TotalCost:          totalCost,
	}

	c.JSON(http.StatusOK, response)
}

// PowerOff 顾客关闭房间空调
func (h *ACHandler) CustomerPowerOff(c *gin.Context) {
	var req struct {
		RoomNumber int `json:"roomNumber" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	// 在关机前先获取当前费用
	currentCost, err := h.billingService.CalculateCurrentFee(req.RoomNumber)
	if err != nil {
		logger.Error("Failed to calculate current fee: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 计算总费用（这里假设查询最近24小时的总费用）
	totalCost, err := h.billingService.CalculateTotalFee(
		req.RoomNumber,
		time.Now().AddDate(0, 0, -1),
		time.Now(),
	)
	if err != nil {
		logger.Error("Failed to calculate total fee: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 关闭空调
	if err := h.acService.PowerOff(req.RoomNumber); err != nil {
		logger.Error("Failed to power off AC: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 返回费用信息
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: PowerOffResponse{
			CurrentCost: currentCost,
			TotalCost:   totalCost,
		},
	})
}

// SetTemperature 顾客调节温度
func (h *ACHandler) CustomerSetTemperature(c *gin.Context) {
	var req struct {
		RoomNumber        int     `json:"roomNumber" binding:"required"`
		TargetTemperature float32 `json:"targetTemperature" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	if err := h.acService.SetTemperature(req.RoomNumber, req.TargetTemperature); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
	})
}

// SetFanSpeed 顾客调节风速
func (h *ACHandler) CustomerSetFanSpeed(c *gin.Context) {
	var req struct {
		RoomNumber      int    `json:"roomNumber" binding:"required"`
		CurrentFanSpeed string `json:"currentFanSpeed" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	speed := ""
	switch req.CurrentFanSpeed {
	case "低":
		speed = "low"
	case "中":
		speed = "medium"
	case "高":
		speed = "high"
	default:
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid fan speed",
		})
		return
	}

	if err := h.acService.SetFanSpeed(req.RoomNumber, speed); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
	})
}

// GetACState 顾客查询空调状态
func (h *ACHandler) CustomerGetACState(c *gin.Context) {
	roomID := c.GetInt("roomID")

	state, err := h.acService.GetACState(roomID)
	if err != nil {
		logger.Error("Failed to get AC state: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: state,
	})
}

// =============== 管理员接口 ===============

// AdminPowerOnResponse 中央空调开机响应结构
type AdminPowerOnResponse struct {
	OperationMode            string  `json:"operationMode"`
	MinTemperature           float32 `json:"minTemperature"`
	MaxTemperature           float32 `json:"maxTemperature"`
	LowSpeedRate             float32 `json:"lowSpeedRate"`
	MediumSpeedRate          float32 `json:"mediumSpeedRate"`
	HighSpeedRate            float32 `json:"highSpeedRate"`
	DefaultTargetTemperature float32 `json:"defaultTargetTemperature"`
}

func (h *ACHandler) AdminPowerOnMainUnit(c *gin.Context) {
	if err := h.acService.PowerOnMainUnit(); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	// 获取当前温度范围配置
	config, err := h.acService.GetTemperatureRange("cooling")
	if err != nil {
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	response := AdminPowerOnResponse{
		OperationMode:            "制冷",
		MinTemperature:           config.MinTemp,
		MaxTemperature:           config.MaxTemp,
		LowSpeedRate:             0.5, // 低速风费率
		MediumSpeedRate:          1.0, // 中速风费率
		HighSpeedRate:            2.0, // 高速风费率
		DefaultTargetTemperature: config.DefaultTemp,
	}

	c.JSON(http.StatusOK, response)
}

// PowerOffMainUnit 管理员关闭中央空调
func (h *ACHandler) AdminPowerOffMainUnit(c *gin.Context) {
	if err := h.acService.PowerOffMainUnit(); err != nil {
		logger.Error("Failed to power off main AC unit: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
	})
}

// GetMainUnitState 获取中央空调状态
func (h *ACHandler) AdminGetMainUnitState(c *gin.Context) {
	state, err := h.acService.GetMainUnitState()
	if err != nil {
		logger.Error("Failed to get main unit state: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: map[string]interface{}{
			"main_unit_on": state,
		},
	})
}

// SetMode 管理员设置工作模式
func (h *ACHandler) AdminSetMode(c *gin.Context) {
	var req struct {
		Mode string `json:"mode" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	if err := h.acService.SetMode(req.Mode); err != nil {
		logger.Error("Failed to set AC mode: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
	})
}

// SetTemperatureRange 管理员设置温度范围
func (h *ACHandler) AdminSetTemperatureRange(c *gin.Context) {
	var req struct {
		Mode        string  `json:"mode" binding:"required"`
		MinTemp     float32 `json:"min_temp" binding:"required"`
		MaxTemp     float32 `json:"max_temp" binding:"required"`
		DefaultTemp float32 `json:"default_temp" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "invalid request parameters",
		})
		return
	}

	if err := h.acService.SetTemperatureRange(req.Mode, req.MinTemp, req.MaxTemp, req.DefaultTemp); err != nil {
		logger.Error("Failed to set temperature range: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
	})
}

// GetTemperatureRange 获取温度范围配置
func (h *ACHandler) AdminGetTemperatureRange(c *gin.Context) {
	mode := c.Query("mode")
	if mode == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code: -1,
			Msg:  "mode parameter is required",
		})
		return
	}

	config, err := h.acService.GetTemperatureRange(mode)
	if err != nil {
		logger.Error("Failed to get temperature range: %v", err)
		c.JSON(http.StatusOK, Response{
			Code: -1,
			Msg:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "success",
		Data: config,
	})
}
