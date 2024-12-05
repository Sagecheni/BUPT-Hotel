// server/server.go

package server

import (
	"backend/internal/handlers"
	"backend/internal/logger"
	"backend/internal/service"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Server struct {
	router *gin.Engine
	srv    *http.Server
}

func NewServer() *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// 配置CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 使用自定义logger中间件
	router.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		logger.Info("[%s] %s %s %v", c.Request.Method, path, c.ClientIP(), latency)
	})

	return &Server{
		router: router,
	}
}

func (s *Server) Start(host string, port int) error {
	scheduler := service.GetScheduler()

	// 初始化handlers
	acHandler := handlers.NewACHandler(scheduler)
	roomHandler := handlers.NewRoomHandler()

	// 注册路由
	api := s.router.Group("/api")
	{
		api.POST("/power-on", acHandler.PowerOn)
		api.POST("/power-off", acHandler.PowerOff)
		api.POST("/set-mode", acHandler.SetMode)
		api.POST("/check-in", roomHandler.CheckIn)
		api.POST("/check-out", roomHandler.CheckOut)

	}

	addr := fmt.Sprintf("%s:%d", host, port)
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	logger.Info("Server starting on %s", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}
