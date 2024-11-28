package api

import (
	"backend/api/handlers"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	roomHandler := handlers.NewRoomHandler()

	api := r.Group("/api")
	{
		api.POST("/checkin", roomHandler.CheckIn)
		api.POST("/checkout", roomHandler.CheckOut)
	}
	return r
}
