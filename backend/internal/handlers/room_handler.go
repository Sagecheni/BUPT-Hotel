package handlers

import (
	"backend/internal/db"
	"backend/internal/service"
	"backend/internal/utils"
	"bytes"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type CheckInRequest struct {
	RoomID     int     `json:"room_id" binding:"required"`
	ClientID   string  `json:"client_id" binding:"required"`
	ClientName string  `json:"client_name" binding:"required"`
	Deposit    float32 `json:"deposit" binding:"required"` // 添加押金字段
}

// PrintDetailRequest 打印详单请求结构
type PrintDetailRequest struct {
	RoomID int `json:"room_id" binding:"required"`
}

type RoomHandler struct {
	roomRepo *db.RoomRepository
}

func NewRoomHandler() *RoomHandler {
	return &RoomHandler{
		roomRepo: db.NewRoomRepository(),
	}
}

func (h *RoomHandler) CheckIn(c *gin.Context) {
	var req CheckInRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "Invalid request",
			Err: err.Error(),
		})
		return
	}
	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomID),
			Err: err.Error(),
		})
		return
	}

	if room.State != 0 {
		c.JSON(http.StatusBadRequest, Response{

			Msg: "房间已被占用",
		})
		return
	}

	err = h.roomRepo.CheckIn(req.RoomID, req.ClientID, req.ClientName, req.Deposit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{

			Msg: "入住失败",
			Err: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg":    "入住成功",
		"RoomID": req.RoomID,
	})

}

type CheckOutRequest struct {
	RoomID int `json:"room_id" binding:"required"`
}

type CheckOutResponse struct {
	AirConFare   float64 `json:"airConFare"`   // 空调费用
	CheckinTime  string  `json:"CheckinTime"`  // 入住时间
	CheckoutTime string  `json:"CheckoutTime"` // 退房时间
	ClientID     string  `json:"client_ID"`    // 用户身份证号
	ClientName   string  `json:"client_name"`  // 用户名字
	Cost         float64 `json:"Cost"`         // 房费
	Msg          string  `json:"msg"`          // 消息
}

func (h *RoomHandler) CheckOut(c *gin.Context) {
	var req CheckOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "Invalid request",
			Err: err.Error(),
		})
		return
	}

	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomID),
			Err: err.Error(),
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住，无法退房",
		})
		return
	}

	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住，无法退房",
		})
		return
	}

	// 获取空调费用
	var airConFare float64
	if room.ACState == 1 {
		// 如果空调还在运行，先关闭空调
		scheduler := service.GetScheduler()
		if scheduler != nil {
			scheduler.RemoveRoom(req.RoomID)
		}
	}

	// 计算总空调费用
	billingService := service.GetBillingService()
	if billingService != nil {
		totalFee, err := billingService.CalculateTotalFee(req.RoomID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Msg: "计算空调费用失败",
				Err: err.Error(),
			})
			return
		}
		airConFare = float64(totalFee)
	}
	// 计算房费 (按天计算，每天100元)
	checkoutTime := time.Now()
	days := int(checkoutTime.Sub(room.CheckinTime).Hours()/24) + 1 // 向上取整
	roomCost := float64(days * 100)
	// 处理退房
	err = h.roomRepo.CheckOut(req.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "退房失败",
			Err: err.Error(),
		})
		return
	}
	// 构造响应
	response := CheckOutResponse{
		AirConFare:   airConFare,
		CheckinTime:  room.CheckinTime.Format("2006-01-02 15:04:05"),
		CheckoutTime: checkoutTime.Format("2006-01-02 15:04:05"),
		ClientID:     room.ClientID,
		ClientName:   room.ClientName,
		Cost:         roomCost,
		Msg:          "退房成功",
	}
	c.JSON(http.StatusOK, response)
}

// PrintDetail 处理打印详单请求
func (h *RoomHandler) PrintDetail(c *gin.Context) {
	var req PrintDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomID),
			Err: err.Error(),
		})
		return
	}

	// 检查房间是否已入住
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住，无法打印详单",
		})
		return
	}

	// 获取详单信息
	billingService := service.GetBillingService()
	details, err := billingService.GetDetails(req.RoomID, room.CheckinTime, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "获取详单失败",
			Err: err.Error(),
		})
		return
	}

	// 如果没有详单记录
	if len(details) == 0 {
		c.JSON(http.StatusNotFound, Response{
			Msg: "该房间没有空调使用记录",
		})
		return
	}

	// 计算总费用
	totalCost, err := billingService.CalculateTotalFee(req.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "计算总费用失败",
			Err: err.Error(),
		})
		return
	}

	// 准备账单数据
	bill := utils.DetailBill{
		RoomID:       req.RoomID,
		ClientName:   room.ClientName,
		ClientID:     room.ClientID,
		CheckInTime:  room.CheckinTime,
		CheckOutTime: time.Now(),
		TotalCost:    totalCost,
		Details:      details,
	}

	// 生成PDF
	pdf, err := utils.GenerateDetailPDF(bill)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "生成PDF失败",
			Err: err.Error(),
		})
		return
	}

	// 创建一个buffer来存储PDF数据
	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "生成PDF文件失败",
			Err: err.Error(),
		})
		return
	}
	// 获取PDF字节数组
	pdfBytes := buf.Bytes()
	// 设置响应头，告诉前端这是一个PDF文件
	fileName := fmt.Sprintf("空调详单_房间%d_%s.pdf",
		req.RoomID,
		time.Now().Format("20060102150405"))

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))

	// 直接写入响应
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// PrintBillRequest 打印账单请求结构
type PrintBillRequest struct {
	RoomID int `json:"room_id" binding:"required"`
}

// PrintBill 处理打印账单请求
func (h *RoomHandler) PrintBill(c *gin.Context) {
	var req PrintBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 获取房间信息
	room, err := h.roomRepo.GetRoomByID(req.RoomID)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Msg: fmt.Sprintf("房间 %d 不存在", req.RoomID),
			Err: err.Error(),
		})
		return
	}

	// 检查房间是否已入住
	if room.State != 1 {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "房间未入住，无法打印账单",
		})
		return
	}

	// 获取账单服务实例
	billingService := service.GetBillingService()

	// 计算空调费用总额
	acCost, err := billingService.CalculateTotalFee(req.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "计算空调费用失败",
			Err: err.Error(),
		})
		return
	}

	// 计算入住天数（向上取整，不足一天按一天计算）
	daysStayed := int(math.Ceil(time.Since(room.CheckinTime).Hours() / 24))
	if daysStayed < 1 {
		daysStayed = 1
	}

	// 每日房费
	roomCost := float32(daysStayed) * room.DailyRate

	// 准备账单数据
	bill := utils.Bill{
		RoomID:       req.RoomID,
		ClientName:   room.ClientName,
		ClientID:     room.ClientID,
		CheckInTime:  room.CheckinTime,
		CheckOutTime: time.Now(),
		DaysStayed:   daysStayed,
		RoomRate:     room.DailyRate,
		TotalRoom:    roomCost,
		TotalAC:      acCost,
		Deposit:      room.Deposit,
		FinalTotal:   roomCost + acCost - room.Deposit,
	}

	// 生成PDF
	pdf, err := utils.GenerateBillPDF(bill)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "生成账单PDF失败",
			Err: err.Error(),
		})
		return
	}

	// 创建buffer来存储PDF数据
	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "生成PDF文件失败",
			Err: err.Error(),
		})
		return
	}

	// 设置文件名
	fileName := fmt.Sprintf("账单_房间%d_%s.pdf",
		req.RoomID,
		time.Now().Format("20060102150405"))

	// 设置响应头
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Length", fmt.Sprintf("%d", len(buf.Bytes())))

	// 发送PDF文件
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}


