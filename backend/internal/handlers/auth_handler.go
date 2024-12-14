// internal/handlers/auth_handler.go
package handlers

import (
	"backend/internal/db"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	UserType string `json:"userType"`
	Router   string `json:"router"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Usertype string `json:"usertype" binding:"required"`
}

type RegisterResponse struct {
	Msg      string `json:"msg"`
	UserType string `json:"userType"`
	Router   string `json:"router"`
}

type AuthHandler struct {
	userRepo *db.UserRepository
}

// 用户类型和身份对应map
var userType_Router_Map = map[string]string{
	"manager":       "admin", // 管理员
	"customer":      "panel", // 客户
	"administrator": "api",   // 经理
	"reception":     "api",   // 前台
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userRepo: db.NewUserRepository(),
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
			Msg: "Invalid request",
			Err: err.Error(),
		})
		return
	}

	// 检查用户是否已存在
	user, err := h.userRepo.GetUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) { //如果错误是记录未找到，则表示用户不存在
			// 用户不存在，可以继续注册
			router, userType, err := h.registerUserAndGetUserType(req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Msg: "Register: 创建用户失败，是数据库问题，请稍后再试",
					Err: err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, RegisterResponse{
				Msg:      "Register: 用户注册成功",
				UserType: userType,
				Router:   router,
			})
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Msg: "Register: 查找用户时数据库错误",
				Err: err.Error(),
			})
			return
		}

	} else if user != nil {
		c.JSON(http.StatusBadRequest, Response{
			Msg: "Register: 您要注册的用户已存在",
			Err: "",
		})
		return
	}

}

// registerUserAndGetUserType 注册用户并返回用户类型
func (h *AuthHandler) registerUserAndGetUserType(req RegisterRequest) (string, string, error) {
	err := h.userRepo.CreateUser(req.Username, req.Password, req.Usertype)
	if err != nil {
		return "", "", err
	}
	return userType_Router_Map[req.Usertype], req.Usertype, nil
}
