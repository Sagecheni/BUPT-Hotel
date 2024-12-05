package handlers

import (
	"backend/internal/db"
	"backend/internal/service"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CheckInRequest struct {
	RoomID     int    `json:"room_id" binding:"required"`
	ClientID   string `json:"client_id" binding:"required"`
	ClientName string `json:"client_name" binding:"required"`
}

type CheckOutRequest struct {
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

	if room.State != 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code: 400,
			Msg:  "房间已被占用",
		})
		return
	}

	err = h.roomRepo.CheckIn(req.RoomID, req.ClientID, req.ClientName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "入住失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg":    "入住成功",
		"RoomID": req.RoomID,
	})

}

func (h *RoomHandler) CheckOut(c *gin.Context) {
	var req CheckOutRequest
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
			Msg:  "房间未入住，无法退房",
		})
		return
	}

	// 如果空调开着，先处理空调关闭
	if room.ACState == 1 {
		// 从调度队列中移除
		scheduler := service.GetScheduler()
		if scheduler != nil {
			scheduler.RemoveRoom(req.RoomID)
		}
	}

	// 处理退房
	err = h.roomRepo.CheckOut(req.RoomID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code: 500,
			Msg:  "退房失败",
			Err:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg": "退房成功",
	})
}
