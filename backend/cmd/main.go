package main

import (
	"backend/api"
	"backend/internal/db"
	"backend/internal/logger"
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
	defer logger.Close() // 确保日志文件正确关闭

	// 初始化数据库连接
	db.Init_DB()
	defer db.SQLDB.Close()

	// 设置路由
	r := api.SetupRouter()
	srv := &http.Server{
		// 修改这里的地址来监听所有网络接口
		Addr:    "0.0.0.0:8080",
		Handler: r,
	}

	// 优雅关闭
	go func() {
		logger.Info("Server is starting on 0.0.0.0:8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen: %s\n", err)
			os.Exit(1)
		}
	}()

	logger.Info("Server is running, you can access it via http://localhost:8080 or http://<your-local-ip>:8080")

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exiting")
}
