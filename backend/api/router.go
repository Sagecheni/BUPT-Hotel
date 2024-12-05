package api

import (
	"backend/internal/handlers"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	// 初始化所有服务
	service.InitServices()

	router := gin.Default()
	// 创建处理器实例
	acHandler := handlers.NewACHandler(service.GetScheduler())
	roomHandler := handlers.NewRoomHandler()

	// 空调控制面板相关路由组
	panel := router.Group("/panel")
	{
		// 开关机
		panel.POST("/poweron", acHandler.PowerOn)
		panel.POST("/poweroff", acHandler.PowerOff)

		// 温度和风速调节
		panel.POST("/changetemp", acHandler.ChangeTemperature)
		panel.POST("/changespeed", acHandler.ChangeSpeed)
	}

	// 房间管理相关路由组
	room := router.Group("/api")
	{
		room.POST("/checkin", roomHandler.CheckIn)
		room.POST("/checkout", roomHandler.CheckOut)
	}

	return router
}
