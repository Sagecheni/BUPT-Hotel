package handlers

import (
	"backend/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type BillingHandler struct {
	billingService *service.BillingService
}

func NewBillingHandler() *BillingHandler {
	return &BillingHandler{billingService: service.NewBillingService()}
}

// GetBill 获取房间账单(含总费用)
func (h *BillingHandler) GetBill(c *gin.Context) {
	roomID, err := strconv.Atoi(c.Param("roomId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的房间号",
			Err:  err.Error(),
		})
		return
	}

	bill, err := h.billingService.GenerateBill(roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "生成账单失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "获取账单成功",
		Data: bill,
	})
}

// QueryDetails 查询详单记录
// 其实可以考虑不用时间，直接返回所有详单记录
type QueryDetailsRequest struct {
	StartTime string `form:"start_time" binding:"required"` // 格式: 2024-11-30 00:00:00
	EndTime   string `form:"end_time" binding:"required"`   // 格式: 2024-11-30 23:59:59
}

// GetDetails 获取房间详单
func (h *BillingHandler) GetDetails(c *gin.Context) {
	roomID, err := strconv.Atoi(c.Param("roomId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的房间号",
			Err:  err.Error(),
		})
		return
	}

	var query QueryDetailsRequest
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的查询参数",
			Err:  err.Error(),
		})
		return
	}

	// 解析时间字符串
	startTime, err := time.Parse("2006-01-02 15:04:05", query.StartTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的开始时间格式",
			Err:  err.Error(),
		})
		return
	}

	endTime, err := time.Parse("2006-01-02 15:04:05", query.EndTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "无效的结束时间格式",
			Err:  err.Error(),
		})
		return
	}

	details, err := h.billingService.GetDetails(roomID, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "获取详单失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 200,
		Msg:  "获取详单成功",
		Data: details,
	})
}
