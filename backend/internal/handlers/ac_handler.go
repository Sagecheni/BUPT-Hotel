// internal/handlers/ac_handler.go

package handlers

import (
	"backend/internal/ac"
	"backend/internal/db"
	"backend/internal/service"
	"backend/internal/types"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ACHandler struct {
	acService *ac.ACService
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
		acService: ac.GetACService(),
		roomRepo:  db.NewRoomRepository(),
	}
}

func (h *ACHandler) AdminPowerOn(c *gin.Context) {
	// 启动中央空调，默认制冷模式
	if err := h.acService.StartCentralAC("cooling"); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "启动中央空调失败",
			Err: err.Error(),
		})
		return
	}

	// 获取空调配置
	config := h.acService.GetConfig()

	// 获取当前模式的温度范围
	tempRange := config.TempRanges[types.ModeCooling]

	// 构建响应
	response := AdminPowerOnResponse{
		HighSpeedRate:            float64(config.Rates[types.SpeedHigh]),
		DefaultTargetTemperature: float64(config.DefaultTemp),
		LowSpeedRate:             float64(config.Rates[types.SpeedLow]),
		MaxTemperature:           int64(tempRange.Max),
		MediumSpeedRate:          float64(config.Rates[types.SpeedMedium]),
		MinTemperature:           int64(tempRange.Min),
		OperationMode:            "cooling", // 默认制冷模式
	}

	c.JSON(http.StatusOK, response)
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

// internal/handlers/ac_handler.go

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
		if bill, err := billingService.CalculateCurrentFee(room.RoomID); err == nil {
			currentFee = bill.CurrentFee
			totalFee = bill.TotalFee
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
		if bill, err := billingService.CalculateCurrentFee(room.RoomID); err == nil {
			currentFee = bill.CurrentFee
			totalFee = bill.TotalFee
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
