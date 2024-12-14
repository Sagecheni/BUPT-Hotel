// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/service"
	"backend/internal/types"
	"fmt"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ACHandler struct {
	acService *service.ACService
	roomRepo  *db.RoomRepository
}

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

// 设置空调模式
type SetModeRequest struct {
	Mode string `json:"mode" binding:"required"` // cooling/heating
}

// 温度调节请求
type ChangeTempRequest struct {
	RoomNumber        int     `json:"roomNumber" binding:"required"`
	TargetTemperature float32 `json:"targetTemperature" binding:"required"`
}

// 开机请求
type PowerRequest struct {
	RoomNumber int `json:"roomNumber" binding:"required"` // 房间号
}

type AdminPowerOnResponse struct {
	HighSpeedRate            float64 `json:"highSpeedRate"`
	DefaultTargetTemperature float64 `json:"defaultTargetTemperature"`
	LowSpeedRate             float64 `json:"lowSpeedRate"`
	MaxTemperature           int64   `json:"maxTemperature"`
	MediumSpeedRate          float64 `json:"mediumSpeedRate"`
	MinTemperature           int64   `json:"minTemperature"`
	OperationMode            string  `json:"operationMode"`
}

func NewACHandler() *ACHandler {
	return &ACHandler{
		acService: service.GetACService(),
		roomRepo:  db.NewRoomRepository(),
	}
}

// AdminPowerOnRequest 管理员开启中央空调的请求结构
type AdminPowerOnRequest struct {
	OperationMode            string  `json:"operationMode" binding:"required"`
	MinTemperature           float32 `json:"minTemperature" binding:"required"`
	MaxTemperature           float32 `json:"maxTemperature" binding:"required"`
	LowSpeedRate             float32 `json:"lowSpeedRate" binding:"required"`
	MediumSpeedRate          float32 `json:"mediumSpeedRate" binding:"required"`
	HighSpeedRate            float32 `json:"highSpeedRate" binding:"required"`
	DefaultTargetTemperature float32 `json:"defaultTargetTemperature" binding:"required"`
}

// AdminPowerOn 处理管理员开启中央空调的请求
func (h *ACHandler) AdminPowerOn(c *gin.Context) {
	var req AdminPowerOnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 验证模式
	var mode types.Mode
	switch req.OperationMode {
	case "制冷":
		mode = types.ModeCooling
	case "制热":
		mode = types.ModeHeating
	default:
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的运行模式，只能是 'cooling' 或 'heating'",
		})
		return
	}

	// 验证温度范围
	if req.MinTemperature >= req.MaxTemperature {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "最低温度必须小于最高温度",
		})
		return
	}

	// 验证默认温度是否在范围内
	if req.DefaultTargetTemperature < req.MinTemperature ||
		req.DefaultTargetTemperature > req.MaxTemperature {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "默认目标温度必须在温度范围内",
		})
		return
	}

	// 验证费率
	if req.LowSpeedRate <= 0 || req.MediumSpeedRate <= 0 || req.HighSpeedRate <= 0 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "费率必须大于0",
		})
		return
	}

	if !(req.LowSpeedRate <= req.MediumSpeedRate && req.MediumSpeedRate <= req.HighSpeedRate) {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "费率必须满足低速≤中速≤高速",
		})
		return
	}

	// 准备新的配置
	config := types.Config{
		DefaultTemp:  req.DefaultTargetTemperature,
		DefaultSpeed: types.SpeedMedium,
		TempRanges: map[types.Mode]types.TempRange{
			mode: {
				Min: req.MinTemperature,
				Max: req.MaxTemperature,
			},
		},
		Rates: map[types.Speed]float32{
			types.SpeedLow:    req.LowSpeedRate,
			types.SpeedMedium: req.MediumSpeedRate,
			types.SpeedHigh:   req.HighSpeedRate,
		},
	}

	// 设置配置
	if err := h.acService.SetConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置空调配置失败",
			Err: err.Error(),
		})
		return
	}

	// 启动中央空调
	if err := h.acService.StartCentralAC(mode); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "启动中央空调失败",
			Err: err.Error(),
		})
		return
	}

	// 返回成功状态码
	c.JSON(http.StatusOK, Response{
		Msg: "中央空调启动成功",
	})
}

// AdminPowerOff 管理员关闭中央空调
func (h *ACHandler) AdminPowerOff(c *gin.Context) {
	// 关闭中央空调
	if err := h.acService.StopCentralAC(); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "关闭中央空调失败",
			Err: err.Error(),
		})
		return
	}

	// 返回成功状态码
	c.JSON(http.StatusOK, Response{
		Msg: "中央空调关闭成功",
	})
}

// PanelResponse 面板通用响应结构
type PanelPowerOnResponse struct {
	CurrentCost        float64 `json:"currentCost"`
	CurrentFanSpeed    string  `json:"currentFanSpeed"`
	CurrentTemperature float64 `json:"currentTemperature"`
	OperationMode      string  `json:"operationMode"`
	TargetTemperature  int64   `json:"targetTemperature"`
	TotalCost          float64 `json:"totalCost"`
}

type PanelPowerOffResponse struct {
	CurrentCost float64 `json:"currentCost"`
	TotalCost   float64 `json:"totalCost"`
}

// PanelPowerOn 处理面板开机请求
func (h *ACHandler) PanelPowerOn(c *gin.Context) {
	var req PowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 开启空调
	if err := h.acService.PowerOn(req.RoomNumber); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "开启空调失败",
			Err: err.Error(),
		})
		return
	}

	// 获取空调状态
	status, err := h.acService.GetACStatus(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "获取空调状态失败",
			Err: err.Error(),
		})
		return
	}

	billingService := service.GetBillingService()
	var currentFee, totalFee float32 = 0, 0
	if billingService != nil {
		// 使用新的独立方法获取费用
		currentFee, err = billingService.CalculateCurrentSessionFee(room.RoomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		totalFee, err = billingService.CalculateTotalFee(room.RoomID)
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	// 构建响应
	response := PanelPowerOnResponse{
		CurrentCost:        float64(currentFee),
		CurrentFanSpeed:    string(status.CurrentSpeed),
		CurrentTemperature: float64(status.CurrentTemp),
		OperationMode:      string(status.Mode),
		TargetTemperature:  int64(status.TargetTemp),
		TotalCost:          float64(totalFee),
	}

	c.JSON(http.StatusOK, response)
}

// PanelPowerOff 处理面板关机请求
func (h *ACHandler) PanelPowerOff(c *gin.Context) {
	var req PowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	billingService := service.GetBillingService()
	var currentFee, totalFee float32 = 0, 0
	if billingService != nil {
		// 在关机前获取最终费用
		currentFee, err = billingService.CalculateCurrentSessionFee(room.RoomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		totalFee, err = billingService.CalculateTotalFee(room.RoomID)
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	// 关闭空调
	if err := h.acService.PowerOff(req.RoomNumber); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "关闭空调失败",
			Err: err.Error(),
		})
		return
	}

	// 构建响应
	response := PanelPowerOffResponse{
		CurrentCost: float64(currentFee),
		TotalCost:   float64(totalFee),
	}

	c.JSON(http.StatusOK, response)
}

// 风速调节请求
type ChangeSpeedRequest struct {
	RoomNumber      int    `json:"roomNumber" binding:"required"`
	CurrentFanSpeed string `json:"currentFanSpeed" binding:"required"`
}

// PanelChangeTemp 处理面板温度调节请求
func (h *ACHandler) PanelChangeTemp(c *gin.Context) {
	var req ChangeTempRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 检查房间状态
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住",
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "空调未开启",
		})
		return
	}

	// 设置温度
	if err := h.acService.SetTemperature(req.RoomNumber, req.TargetTemperature); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置温度失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: "温度设置成功",
	})
}

// PanelChangeSpeed 处理面板风速调节请求
func (h *ACHandler) PanelChangeSpeed(c *gin.Context) {
	var req ChangeSpeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 检查房间状态
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住",
		})
		return
	}

	if room.ACState != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "空调未开启",
		})
		return
	}

	// 将风速字符串转换为types.Speed类型
	var speed types.Speed
	switch req.CurrentFanSpeed {
	case "低":
		speed = types.SpeedLow
	case "中":
		speed = types.SpeedMedium
	case "高":
		speed = types.SpeedHigh
	default:
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的风速设置",
		})
		return
	}

	// 设置风速
	if err := h.acService.SetFanSpeed(req.RoomNumber, speed); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置风速失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: "风速设置成功",
	})
}

// PanelRequestStatus 处理面板状态查询请求
func (h *ACHandler) PanelRequestStatus(c *gin.Context) {
	var req RoomStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 获取账单服务
	billingService := service.GetBillingService()
	var currentFee, totalFee float32 = 0, 0
	if billingService != nil && room.ACState == 1 {
		// 获取当前费用
		currentFee, err = billingService.CalculateCurrentSessionFee(room.RoomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		// 获取总费用
		totalFee, err = billingService.CalculateTotalFee(room.RoomID)
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	response := RoomStatusResponse{
		CurrentCost:        float32(currentFee),
		CurrentTemperature: room.CurrentTemp,
		TotalCost:          float32(totalFee),
	}

	c.JSON(http.StatusOK, response)
}

// 在ACHandler struct下方添加新的请求和响应结构体

// AllStateRequest 查询所有状态的请求结构
type AllStateRequest struct {
	RoomNumber int `json:"roomNumber" binding:"required"`
}

// AllStateResponse 所有状态的响应结构
type AllStateResponse struct {
	ACState            bool    `json:"acState"`
	CurrentCost        float64 `json:"currentCost"`
	CurrentFanSpeed    string  `json:"currentFanSpeed"`
	CurrentTemperature float64 `json:"currentTemperature"`
	OperationMode      string  `json:"operationMode"`
	TargetTemperature  float64 `json:"targetTemperature"`
	TotalCost          float64 `json:"totalCost"`
}

// PanelRequestAllState 处理查询所有状态的请求
func (h *ACHandler) PanelRequestAllState(c *gin.Context) {
	var req AllStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 获取账单服务
	billingService := service.GetBillingService()
	var currentFee, totalFee float32 = 0, 0
	if billingService != nil && room.ACState == 1 {
		// 获取当前费用
		currentFee, err = billingService.CalculateCurrentSessionFee(room.RoomID)
		if err != nil {
			logger.Error("计算当前费用失败: %v", err)
		}

		// 获取总费用
		totalFee, err = billingService.CalculateTotalFee(room.RoomID)
		if err != nil {
			logger.Error("计算总费用失败: %v", err)
		}
	}

	response := AllStateResponse{
		ACState:            room.ACState == 1,
		CurrentCost:        float64(currentFee),
		CurrentFanSpeed:    room.CurrentSpeed,
		CurrentTemperature: float64(room.CurrentTemp),
		OperationMode:      room.Mode,
		TargetTemperature:  float64(room.TargetTemp),
		TotalCost:          float64(totalFee),
	}

	c.JSON(http.StatusOK, response)
}

// AdminChangeModeRequest 修改中央空调模式的请求结构
type AdminChangeModeRequest struct {
	OperationMode string `json:"operationMode" binding:"required"`
}

// AdminChangeMode 处理管理员更改中央空调模式的请求
func (h *ACHandler) AdminChangeMode(c *gin.Context) {
	var req AdminChangeModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 验证模式是否合法
	var mode types.Mode
	switch req.OperationMode {
	case "cooling":
		mode = types.ModeCooling
	case "heating":
		mode = types.ModeHeating
	default:
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的运行模式，只能是 'cooling' 或 'heating'",
		})
		return
	}

	// 设置中央空调模式
	if err := h.acService.SetCentralACMode(mode); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "更改中央空调模式失败",
			Err: err.Error(),
		})
		return
	}

	// 返回成功状态码
	c.JSON(http.StatusOK, Response{
		Msg: fmt.Sprintf("中央空调模式已更改为 %s", mode),
	})
}

// AdminChangeTempRangeRequest 修改温度范围的请求结构
type AdminChangeTempRangeRequest struct {
	MinTemperature float32 `json:"minTemperature" binding:"required"`
	MaxTemperature float32 `json:"maxTemperature" binding:"required"`
}

// AdminChangeTempRange 处理管理员更改温度范围的请求
func (h *ACHandler) AdminChangeTempRange(c *gin.Context) {
	var req AdminChangeTempRangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 检查温度范围是否有效
	if req.MinTemperature >= req.MaxTemperature {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "最低温度必须小于最高温度",
		})
		return
	}

	// 获取当前的空调配置
	config := h.acService.GetConfig()

	// 获取当前空调状态
	isOn, mode := h.acService.GetCentralACState()
	if !isOn {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "中央空调未开启",
		})
		return
	}

	// 更新当前模式的温度范围
	config.TempRanges[mode] = types.TempRange{
		Min: req.MinTemperature,
		Max: req.MaxTemperature,
	}

	// 设置新的配置
	if err := h.acService.SetConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置温度范围失败",
			Err: err.Error(),
		})
		return
	}

	// 返回成功状态码
	c.JSON(http.StatusOK, Response{
		Msg: fmt.Sprintf("温度范围已更新为 %.1f°C - %.1f°C", req.MinTemperature, req.MaxTemperature),
	})
}

// AdminChangeRateRequest 修改费率的请求结构
type AdminChangeRateRequest struct {
	LowSpeedRate    float32 `json:"lowSpeedRate" binding:"required"`
	MediumSpeedRate float32 `json:"mediumSpeedRate" binding:"required"`
	HighSpeedRate   float32 `json:"highSpeedRate" binding:"required"`
}

// AdminChangeRate 处理管理员更改费率的请求
func (h *ACHandler) AdminChangeRate(c *gin.Context) {
	var req AdminChangeRateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 验证费率是否合法（必须为正数）
	if req.LowSpeedRate <= 0 || req.MediumSpeedRate <= 0 || req.HighSpeedRate <= 0 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "费率必须大于0",
		})
		return
	}

	// 验证费率递增关系
	if !(req.LowSpeedRate <= req.MediumSpeedRate && req.MediumSpeedRate <= req.HighSpeedRate) {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "费率必须满足低速≤中速≤高速",
		})
		return
	}

	// 获取当前配置
	config := h.acService.GetConfig()

	// 更新费率
	config.Rates = map[types.Speed]float32{
		types.SpeedLow:    req.LowSpeedRate,
		types.SpeedMedium: req.MediumSpeedRate,
		types.SpeedHigh:   req.HighSpeedRate,
	}

	// 设置新的配置
	if err := h.acService.SetConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置费率失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: fmt.Sprintf("费率已更新 - 低速: %.2f 元/度, 中速: %.2f 元/度, 高速: %.2f 元/度",
			req.LowSpeedRate, req.MediumSpeedRate, req.HighSpeedRate),
	})
}

// AdminAllStateResponse 管理员获取所有状态的响应结构
type AdminAllStateResponse struct {
	ACState                  bool    `json:"acState"`
	DefaultTargetTemperature float64 `json:"defaultTargetTemperature"`
	HighSpeedRate            float64 `json:"highSpeedRate"`
	LowSpeedRate             float64 `json:"lowSpeedRate"`
	MaxTemperature           int64   `json:"maxTemperature"`
	MediumSpeedRate          float64 `json:"mediumSpeedRate"`
	MinTemperature           int64   `json:"minTemperature"`
	OperationMode            string  `json:"operationMode"`
}

// AdminRequestAllState 处理管理员获取所有状态的请求
func (h *ACHandler) AdminRequestAllState(c *gin.Context) {

	// 获取中央空调状态和模式
	isOn, mode := h.acService.GetCentralACState()

	// 获取当前配置
	config := h.acService.GetConfig()

	// 获取当前模式的温度范围
	tempRange := config.TempRanges[mode]

	// 构建响应
	response := AdminAllStateResponse{
		ACState:                  isOn,
		DefaultTargetTemperature: float64(config.DefaultTemp),
		HighSpeedRate:            float64(config.Rates[types.SpeedHigh]),
		LowSpeedRate:             float64(config.Rates[types.SpeedLow]),
		MaxTemperature:           int64(tempRange.Max),
		MediumSpeedRate:          float64(config.Rates[types.SpeedMedium]),
		MinTemperature:           int64(tempRange.Min),
		OperationMode:            string(mode),
	}

	c.JSON(http.StatusOK, response)
}

// AdminChangeDefaultTempRequest 修改默认温度的请求结构
type AdminChangeDefaultTempRequest struct {
	DefaultTargetTemperature int64 `json:"defaultTargetTemperature" binding:"required"`
}

// AdminChangeDefaultTemp 处理管理员更改默认温度的请求
func (h *ACHandler) AdminChangeDefaultTemp(c *gin.Context) {
	var req AdminChangeDefaultTempRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取当前空调状态和配置
	isOn, mode := h.acService.GetCentralACState()
	if !isOn {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "中央空调未开启",
		})
		return
	}

	config := h.acService.GetConfig()
	tempRange := config.TempRanges[mode]

	// 检查温度是否在当前模式的范围内
	if float32(req.DefaultTargetTemperature) < tempRange.Min ||
		float32(req.DefaultTargetTemperature) > tempRange.Max {
		c.JSON(http.StatusBadRequest, Response{
			Msg: fmt.Sprintf("默认温度必须在 %.1f°C - %.1f°C 范围内", tempRange.Min, tempRange.Max),
		})
		return
	}

	// 更新配置中的默认温度
	config.DefaultTemp = float32(req.DefaultTargetTemperature)
	if err := h.acService.SetConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "设置默认温度失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: fmt.Sprintf("默认温度已设置为 %d°C", req.DefaultTargetTemperature),
	})
}

// MonitorPowerRequest 监控面板开机请求结构
type MonitorPowerRequest struct {
	RoomNumber int `json:"roomNumber" binding:"required"`
}

// MonitorPowerOn 处理监控面板开机请求
func (h *ACHandler) MonitorPowerOn(c *gin.Context) {
	var req MonitorPowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	_, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 开启空调
	if err := h.acService.PowerOn(req.RoomNumber); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "开启空调失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: "空调开启成功",
	})
}

// MonitorPowerOff 处理监控面板关机请求
func (h *ACHandler) MonitorPowerOff(c *gin.Context) {
	var req MonitorPowerRequest // 可以复用开机的请求结构
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	_, err := h.roomRepo.GetRoomByID(req.RoomNumber)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomNumber),
			Err: err.Error(),
		})
		return
	}

	// 关闭空调
	if err := h.acService.PowerOff(req.RoomNumber); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "关闭空调失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Msg: "空调关闭成功",
	})
}

// MonitorRequestStatesRequest 监控面板状态查询请求
type MonitorRequestStatesRequest struct {
	Index int `json:"index" binding:"required"`
}

// MonitorStateResponse 监控面板状态响应
type MonitorStateResponse struct {
	ACState            bool    `json:"acState"`
	CurrentFanSpeed    string  `json:"currentFanSpeed"`
	CurrentTemperature float64 `json:"currentTemperature"`
	OperationMode      string  `json:"operationMode"`
	RoomNumber         int64   `json:"roomNumber"`
	ScheduleStatus     bool    `json:"scheduleStatus"`
	TargetTemperature  float64 `json:"targetTemperature"`
	TotalCost          float64 `json:"totalCost"`
	Valid              bool    `json:"valid"`
}

// MonitorRequestStates 处理监控面板状态查询请求
func (h *ACHandler) MonitorRequestStates(c *gin.Context) {
	var req MonitorRequestStatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取所有已入住房间
	rooms, err := h.roomRepo.GetOccupiedRooms()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "获取房间信息失败",
			Err: err.Error(),
		})
		return
	}

	// 如果请求的索引超出有效房间数量，返回无效面板
	if req.Index >= len(rooms) {
		c.JSON(http.StatusOK, MonitorStateResponse{
			Valid: false,
		})
		return
	}

	// 获取当前索引对应的房间
	room := rooms[req.Index]

	// 获取空调状态
	acStatus, err := h.acService.GetACStatus(room.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "获取空调状态失败",
			Err: err.Error(),
		})
		return
	}

	// 获取调度状态
	serviceQueue := h.acService.GetScheduler().GetServiceQueue()
	_, isInService := serviceQueue[room.RoomID]

	response := MonitorStateResponse{
		ACState:            acStatus.PowerState,
		CurrentFanSpeed:    string(acStatus.CurrentSpeed),
		CurrentTemperature: math.Round(float64(acStatus.CurrentTemp)*100) / 100,
		OperationMode:      string(acStatus.Mode),
		RoomNumber:         int64(room.RoomID),
		ScheduleStatus:     isInService,
		TargetTemperature:  math.Round(float64(acStatus.TargetTemp)*100) / 100, // 保留2位小数
		TotalCost:          float64(acStatus.TotalFee),
		Valid:              true,
	}

	c.JSON(http.StatusOK, response)
}
