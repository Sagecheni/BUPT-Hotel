// internal/handlers/auth_handler.go
package handlers

import (
	"backend/internal/db"
	"net/http"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	UserType string `json:"userType"`
}

type AuthHandler struct {
	userRepo *db.UserRepository
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
			Msg: "Invalid username or password",
		})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		UserType: user.Identity,
	})
}
