package main

import (
	"backend/api"
	"backend/internal/db"
	"backend/internal/logger"
	"backend/internal/service"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	fmt.Println("Hello, World!")
	// 初始化日志
	logger.SetLevel(logger.InfoLevel)
	defer logger.Close() // 添加这行确保日志文件正确关闭

	// 初始化数据库连接
	db.Init_DB()
	defer db.SQLDB.Close()

	// 初始化服务
	service.InitServices()
	defer service.StopServices()

	// 设置路由
	r := api.SetupRouter()
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// 优雅关闭
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen: %s\n", err)
			os.Exit(1)
		}
	}()

	logger.Info("Server is running on port 8080")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exiting")
}
