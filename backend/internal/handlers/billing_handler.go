package handlers

import (
	"backend/internal/billing"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type BillingHandler struct {
	billingService billing.BillingService
}

func NewBillingHandler(service billing.BillingService) *BillingHandler {
	return &BillingHandler{
		billingService: service,
	}
}

// GetCurrentFee 获取当前费用
func (h *BillingHandler) GetCurrentFee(c *gin.Context) {
	roomID := c.GetInt("roomID")

	fee, err := h.billingService.CalculateCurrentFee(roomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: -1,
			Msg:  "Failed to get current fee",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "Success",
		Data: fee,
	})
}

// GetDetails 获取详单
func (h *BillingHandler) GetDetails(c *gin.Context) {
	roomID := c.GetInt("roomID")
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")

	start, _ := time.Parse(time.RFC3339, startTime)
	end, _ := time.Parse(time.RFC3339, endTime)

	details, err := h.billingService.GetDetails(roomID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: -1,
			Msg:  "Failed to get details",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Msg:  "Success",
		Data: details,
	})
}
