// internal/app/app.go

package app

import (
	"backend/api"
	"backend/internal/ac"
	"backend/internal/billing"
	"backend/internal/db"
	"backend/internal/events"
	"backend/internal/handlers"
	"backend/internal/logger"
	"backend/internal/monitor"
	"backend/internal/scheduler"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type App struct {
	eventBus    *events.EventBus
	scheduler   *scheduler.Scheduler
	acService   ac.ACService
	billService billing.BillingService
	monitor     *monitor.Monitor
	server      *http.Server
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

func NewApp() *App {
	return &App{
		stopChan: make(chan struct{}),
	}
}

func (a *App) Initialize() error {
	db.Init_DB()
	a.eventBus = events.NewEventBus()

	roomRepo := db.NewRoomRepository()
	serviceRepo := db.NewServiceRepository(db.DB)
	acConfigRepo := db.NewACConfigRepository(db.DB)

	schedulerConfig := &scheduler.Config{
		MaxServices:    3,
		BaseWaitTime:   20,
		DefaultSpeed:   "medium",
		DefaultTemp:    25.0,
		TempThreshold:  0.1,
		ServiceTimeout: 300,
	}

	a.scheduler = scheduler.NewScheduler(a.eventBus, roomRepo, schedulerConfig, serviceRepo)
	a.acService = ac.NewACService(roomRepo, a.eventBus, serviceRepo, acConfigRepo)
	a.billService = billing.NewBillingService(serviceRepo)
	a.monitor = monitor.NewMonitor(a.eventBus, roomRepo, serviceRepo, acConfigRepo, 5*time.Second)

	return nil
}

func (a *App) Start(port int) error {
	a.monitor.Start()
	logger.Info("Monitor started")

	// 创建处理器
	acHandler := handlers.NewACHandler(a.acService, a.billService)

	// 设置路由
	router := api.SetupRouter(acHandler)

	a.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
		}
	}()

	logger.Info("Server started on port %d", port)
	return nil
}

func (a *App) Stop(ctx context.Context) error {
	// 发送停止信号
	close(a.stopChan)

	// 停止监控器
	a.monitor.Stop()

	// 等待所有goroutine完成
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	// 等待优雅关闭或超时
	select {
	case <-done:
		logger.Info("Application stopped gracefully")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout")
	}
}
