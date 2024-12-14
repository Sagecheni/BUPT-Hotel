// internal/handlers/auth_handler.go
package handlers

import (
	"backend/internal/db"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 用户类型和身份对应map
var userType_Router_Map = map[string]string{
	"manager":       "admin", // 管理员
	"customer":      "panel", // 客户
	"administrator": "api",   // 经理
	"reception":     "api",   // 前台
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	UserType string `json:"userType"`
	Router   string `json:"router"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required"` //使用顾客姓名作为username
	Password string `json:"password" binding:"required"`
}

type RegisterResponse struct {
	Msg      string `json:"msg"`
	UserType string `json:"userType"`
	RoomID   int    `json:"roomId,omitempty"`
}

type AuthHandler struct {
	userRepo *db.UserRepository
	roomRepo *db.RoomRepository // 添加roomRepo用于查询房间信息
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userRepo: db.NewUserRepository(),
		roomRepo: db.NewRoomRepository(),
	}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "Invalid request",
			Err: err.Error(),
		})
		return
	}

	user, err := h.userRepo.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Msg: "Invalid username or password",
		})
		return
	}

	if user.Password != req.Password {
		c.JSON(http.StatusUnauthorized, Response{
			Msg: "Invalid password",
		})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		UserType: user.Identity,
		Router:   userType_Router_Map[user.Identity],
	})
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "无效的请求格式",
			Err: err.Error(),
		})
		return
	}

	// 检查用户是否已存在
	existingUser, err := h.userRepo.GetUserByUsername(req.Username)
	if err == nil && existingUser != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "该用户名已被注册",
		})
		return
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "查询用户信息失败",
			Err: err.Error(),
		})
		return
	}

	// 获取所有入住房间
	occupiedRooms, err := h.roomRepo.GetOccupiedRooms()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "查询入住信息失败",
			Err: err.Error(),
		})
		return
	}

	// 查找是否有该顾客的入住记录
	var customerRoom *db.RoomInfo
	for _, room := range occupiedRooms {
		if room.ClientName == req.Username {
			customerRoom = &room
			break
		}
	}

	if customerRoom == nil {
		c.JSON(401, Response{
			Msg: "该顾客未入住",
		})
		return
	}

	// 创建新用户
	err = h.userRepo.CreateUser(req.Username, req.Password, "customer")
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Msg: "创建用户失败",
			Err: err.Error(),
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, RegisterResponse{
		Msg:      "注册成功",
		UserType: "customer",
		RoomID:   customerRoom.RoomID,
	})
}
