// api/router.go

package api

import (
	"backend/internal/handlers"
	"backend/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(
	acHandler *handlers.ACHandler,
	// 其他handler...
) *gin.Engine {
	router := gin.Default()

	// 使用CORS中间件
	router.Use(middleware.Cors())

	// 顾客空调控制面板路由组
	panel := router.Group("/panel")
	{
		// 开机
		panel.POST("/poweron", acHandler.CustomerPowerOn)
		// 关机
		panel.POST("/poweroff", acHandler.CustomerPowerOff)
		// 调节温度
		panel.POST("/changetemp", acHandler.CustomerSetTemperature)
		// 调节风速
		panel.POST("/changespeed", acHandler.CustomerSetFanSpeed)
	}
	admin := router.Group("/admin")
	{
		admin.POST("/adminpoweron", acHandler.AdminPowerOnMainUnit)
	}

	// 其他路由组...

	return router
}
