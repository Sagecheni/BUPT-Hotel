package main

import (
	"backend/internal/app"
	"backend/internal/logger"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Println("Hello, World!")
	// 初始化日志
	// 创建应用实例
	app := app.NewApp()
	if err := app.Initialize(); err != nil {
		logger.Error("Init error: %v", err)
		os.Exit(1)
	}

	if err := app.Start(8080); err != nil {
		logger.Error("Start error: %v", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Stop(ctx); err != nil {
		logger.Error("Stop error: %v", err)
	}
}
