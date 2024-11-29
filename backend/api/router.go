package api

import (
	"backend/internal/handlers"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	acHandler := handlers.NewACHandler(service.GetScheduler())
	roomHandler := handlers.NewRoomHandler()
	api := r.Group("/api")
	{
		api.POST("/checkin", roomHandler.CheckIn)
		api.POST("/checkout", roomHandler.CheckOut)
		api.POST("/control", acHandler.SetAirCondition)
		api.POST("/poweron", acHandler.PowerOn)
		api.POST("/poweroff", acHandler.PowerOff)
		api.POST("/setmode", acHandler.SetMode)
		api.GET("/status", acHandler.GetSchedulerStatus)
	}
	return r
}
