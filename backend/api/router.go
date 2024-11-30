package api

import (
	"backend/internal/handlers"
	"backend/internal/service"
	"backend/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()
	r.Use(middleware.Cors())
	acHandler := handlers.NewACHandler(service.GetScheduler())
	roomHandler := handlers.NewRoomHandler()
	billingHandler := handlers.NewBillingHandler()
	api := r.Group("/api")
	{
		api.POST("/checkin", roomHandler.CheckIn)
		api.POST("/checkout", roomHandler.CheckOut)
		api.POST("/control", acHandler.SetAirCondition) // 设置空调温度/风速
		api.POST("/poweron", acHandler.PowerOn)         // 打开空调
		api.POST("/poweroff", acHandler.PowerOff)       // 关闭空调
		api.POST("/setmode", acHandler.SetMode)         // 设置空调模式

		api.GET("/status", acHandler.GetSchedulerStatus)       // 获取调度器状态
		api.GET("/:roomId/bill", billingHandler.GetBill)       // 获取账单
		api.GET("/:roomId/details", billingHandler.GetDetails) // 获取详单
		api.POST("/changetemp", acHandler.ChangeTemperature)   // 修改温度
		api.POST("/changespeed", acHandler.ChangeSpeed)        // 修改风速
		api.POST("/requeststate", acHandler.RoomStatus)        // 查询房间状态
	}
	return r
}
