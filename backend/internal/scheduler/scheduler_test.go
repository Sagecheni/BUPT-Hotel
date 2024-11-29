package scheduler

import (
	"backend/internal/logger"
	"os"
	"testing"
	"time"
)

func init() {
	// 设置日志级别为Debug，记录详细信息
	logger.SetLevel(logger.DebugLevel)
	// 创建日志文件
	logFile, err := os.OpenFile("scheduler_test.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	logger.SetOutput(logFile)
}

// 测试直接分配服务对象
func TestDirectAssignment(t *testing.T) {
	logger.Info("=== Starting TestDirectAssignment ===")
	s := NewScheduler()
	defer s.Stop()

	tests := []struct {
		roomID     int
		speed      string
		targetTemp float32
		wantServed bool
	}{
		{101, SpeedLow, 25.0, true},    // 第一个请求
		{102, SpeedHigh, 23.0, true},   // 第二个请求
		{103, SpeedMedium, 24.0, true}, // 第三个请求
		{104, SpeedLow, 26.0, false},   // 第四个请求应该进入等待队列
	}

	for i, tt := range tests {
		logger.Debug("Testing room %d with speed %s", tt.roomID, tt.speed)
		served, err := s.HandleRequest(tt.roomID, tt.speed, tt.targetTemp)
		if err != nil {
			t.Errorf("test %d: HandleRequest failed: %v", i, err)
		}
		if served != tt.wantServed {
			t.Errorf("test %d: got served = %v, want %v", i, served, tt.wantServed)
		}

		// 验证服务队列和等待队列状态
		serviceQueue := s.GetServiceQueue()
		waitQueue := s.GetWaitQueue()
		if tt.wantServed && serviceQueue[tt.roomID] == nil {
			t.Errorf("test %d: room %d should be in service queue", i, tt.roomID)
		}
		if !tt.wantServed && waitQueue[tt.roomID] == nil {
			t.Errorf("test %d: room %d should be in wait queue", i, tt.roomID)
		}
	}
}

// 测试优先级抢占
func TestPriorityPreemption(t *testing.T) {
	logger.Info("=== Starting TestPriorityPreemption ===")
	s := NewScheduler()
	defer s.Stop()

	// 先添加3个低优先级请求
	lowPriorityRooms := []struct {
		roomID int
		speed  string
	}{
		{201, SpeedLow},
		{202, SpeedLow},
		{203, SpeedLow},
	}

	for _, room := range lowPriorityRooms {
		served, err := s.HandleRequest(room.roomID, room.speed, 25.0)
		if err != nil {
			t.Fatalf("Failed to add low priority room %d: %v", room.roomID, err)
		}
		if !served {
			t.Errorf("Room %d should be served immediately", room.roomID)
		}
	}

	// 添加高优先级请求
	logger.Debug("Adding high priority request")
	served, err := s.HandleRequest(204, SpeedHigh, 23.0)
	if err != nil {
		t.Fatal(err)
	}
	if !served {
		t.Error("High priority request should preempt a low priority service")
	}

	// 验证是否有一个低优先级请求被移到等待队列
	waitQueue := s.GetWaitQueue()
	if len(waitQueue) != 1 {
		t.Errorf("Wait queue should have exactly 1 room, got %d", len(waitQueue))
	}
}

// 测试时间片轮转
func TestTimeSliceRotation(t *testing.T) {
	logger.Info("=== Starting TestTimeSliceRotation ===")
	s := NewScheduler()
	defer s.Stop()

	// 添加3个相同优先级的请求
	for i := 0; i < 3; i++ {
		roomID := 301 + i
		served, err := s.HandleRequest(roomID, SpeedMedium, 24.0)
		if err != nil {
			t.Fatalf("Failed to add room %d: %v", roomID, err)
		}
		if !served {
			t.Errorf("Room %d should be served immediately", roomID)
		}
	}

	// 添加第4个相同优先级的请求
	served, err := s.HandleRequest(304, SpeedMedium, 24.0)
	if err != nil {
		t.Fatal(err)
	}
	if served {
		t.Error("Fourth request should wait")
	}

	// 等待时间片到期
	logger.Debug("Waiting for time slice expiration")
	time.Sleep(3 * time.Second)

	// 验证队列状态
	serviceQueue := s.GetServiceQueue()
	waitQueue := s.GetWaitQueue()
	if len(serviceQueue) != 3 {
		t.Errorf("Service queue should have 3 rooms, got %d", len(serviceQueue))
	}
	if len(waitQueue) != 1 {
		t.Errorf("Wait queue should have 1 room, got %d", len(waitQueue))
	}
}

// 测试等待队列管理
func TestWaitQueueManagement(t *testing.T) {
	logger.Info("=== Starting TestWaitQueueManagement ===")
	s := NewScheduler()
	defer s.Stop()

	// 填满服务队列
	for i := 0; i < MaxServices; i++ {
		roomID := 401 + i
		s.HandleRequest(roomID, SpeedMedium, 24.0)
	}

	// 添加多个等待请求
	waitRooms := []struct {
		roomID int
		speed  string
	}{
		{404, SpeedMedium},
		{405, SpeedMedium},
		{406, SpeedMedium},
	}

	for _, room := range waitRooms {
		served, err := s.HandleRequest(room.roomID, room.speed, 24.0)
		if err != nil {
			t.Fatalf("Failed to add waiting room %d: %v", room.roomID, err)
		}
		if served {
			t.Errorf("Room %d should not be served immediately", room.roomID)
		}
	}

	// 验证等待时长分配
	waitQueue := s.GetWaitQueue()
	for roomID, wait := range waitQueue {
		if wait.WaitDuration <= 0 {
			t.Errorf("Room %d should have positive wait duration", roomID)
		}
		logger.Debug("Room %d assigned wait duration: %.2f", roomID, wait.WaitDuration)
	}
}

// 测试服务状态监控
func TestServiceMonitoring(t *testing.T) {
	logger.Info("=== Starting TestServiceMonitoring ===")
	s := NewScheduler()
	defer s.Stop()

	// 添加服务请求
	s.HandleRequest(501, SpeedMedium, 24.0)

	// 等待一段时间让服务时长累积
	time.Sleep(2 * time.Second)

	// 检查服务时长是否正确更新
	serviceQueue := s.GetServiceQueue()
	service := serviceQueue[501]
	if service.Duration <= 0 {
		t.Error("Service duration should be greater than 0")
	}
	logger.Debug("Service duration for room 501: %.2f seconds", service.Duration)
}

// 测试异常情况
func TestEdgeCases(t *testing.T) {
	logger.Info("=== Starting TestEdgeCases ===")
	s := NewScheduler()
	defer s.Stop()

	// 测试重复请求
	logger.Debug("Testing duplicate request")
	s.HandleRequest(601, SpeedLow, 25.0)
	served, err := s.HandleRequest(601, SpeedHigh, 23.0)
	if err != nil {
		t.Fatal(err)
	}
	if !served {
		t.Error("Duplicate request should be marked as served")
	}

	// 测试无效风速
	logger.Debug("Testing invalid speed")
	_, err = s.HandleRequest(602, "invalid_speed", 25.0)
	if err == nil {
		t.Error("Should return error for invalid speed")
	}
}
