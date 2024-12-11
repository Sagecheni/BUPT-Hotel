package api

import (
	"backend/internal/handlers"
	"backend/internal/service"
	"backend/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	// 初始化所有服务
	service.InitServices()

	router := gin.Default()
	router.Use(middleware.CORSMiddleware())
	// 创建处理器实例
	acHandler := handlers.NewACHandler()
	roomHandler := handlers.NewRoomHandler()

	// 空调控制面板相关路由组
	panel := router.Group("/panel")
	{
		// 开关机
		panel.POST("/poweron", acHandler.PanelPowerOn)
		panel.POST("/poweroff", acHandler.PanelPowerOff)
		router.POST("/panel/changetemp", acHandler.PanelChangeTemp)
		router.POST("/panel/changespeed", acHandler.PanelChangeSpeed)
		router.POST("/panel/requeststatus", acHandler.PanelRequestStatus)

	}

	// 房间管理相关路由组
	room := router.Group("/api")
	{
		room.POST("/checkin", roomHandler.CheckIn)
		room.POST("/checkout", roomHandler.CheckOut)
	}
	admin := router.Group("/admin")
	{
		admin.POST("/adminpoweron", acHandler.AdminPowerOn)
		admin.POST("/adminpoweroff", acHandler.AdminPowerOff)
		admin.POST("/changemode", acHandler.AdminChangeMode)
		admin.POST("/changetemprange", acHandler.AdminChangeTempRange)
		admin.POST("/changerate", acHandler.AdminChangeRate)
		admin.POST("/requestallstate", acHandler.AdminRequestAllState)
		admin.POST("/changedefaulttemp", acHandler.AdminChangeDefaultTemp)
	}
	monitor := router.Group("/monitor")
	{
		monitor.POST("/monitorpoweron", acHandler.MonitorPowerOn)
		monitor.POST("/monitorpoweroff", acHandler.MonitorPowerOff)
	}
	return router
}
