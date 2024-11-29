package scheduler

import (
	"testing"
	"time"
)

// Test1 测试调度器
func Test1(t *testing.T) {
	// 测试1: 基本调度流程
	t.Run("Basic Priority Scheduling", func(t *testing.T) {
		s := NewScheduler()

		// 步骤1: 初始分配
		t.Log("Step 1: Initial Assignment")
		success1, err := s.HandleRequest(101, SpeedMedium)
		if err != nil || !success1 {
			t.Errorf("First request should succeed, got success=%v, err=%v", success1, err)
		}
		if len(s.GetServiceQueue()) != 1 {
			t.Errorf("Service queue should have 1 item, got %d", len(s.GetServiceQueue()))
		}

		// 步骤2: 填满服务队列
		t.Log("Step 2: Fill Service Queue")
		if _, err := s.HandleRequest(102, SpeedLow); err != nil {
			t.Errorf("Failed to add second request: %v", err)
		}
		if _, err := s.HandleRequest(103, SpeedLow); err != nil {
			t.Errorf("Failed to add third request: %v", err)
		}

		// 步骤3: 高优先级抢占
		t.Log("Step 3: High Priority Preemption")
		success4, err := s.HandleRequest(104, SpeedHigh)
		if err != nil {
			t.Errorf("Failed to handle high priority request: %v", err)
		}
		if !success4 {
			t.Error("High priority request should succeed through preemption")
		}

		// 验证高优先级抢占结果
		serviceQueue := s.GetServiceQueue()
		waitQueue := s.GetWaitQueue()

		found := false
		for _, service := range serviceQueue {
			if service.Speed == SpeedHigh {
				found = true
				break
			}
		}
		if !found {
			t.Error("High priority request should be in service queue")
		}

		if len(waitQueue) != 1 {
			t.Errorf("Wait queue should have 1 item, got %d", len(waitQueue))
		}
	})

	// 测试2: 时间片调度（相同优先级）
	t.Run("Equal Priority Time Slice", func(t *testing.T) {
		s := NewScheduler()

		// 步骤1: 填满服务队列（全部使用相同优先级）
		t.Log("Step 1: Fill Service Queue with Equal Priority")
		for i := 101; i <= 103; i++ {
			success, err := s.HandleRequest(i, SpeedMedium)
			if err != nil || !success {
				t.Errorf("Failed to add request %d: %v", i, err)
			}
		}

		// 步骤2: 添加相同优先级的新请求
		t.Log("Step 2: Add New Equal Priority Request")
		isScheduled, err := s.HandleRequest(104, SpeedMedium)
		if err != nil {
			t.Errorf("Failed to handle equal priority request: %v", err)
		}
		if isScheduled {
			t.Error("Equal priority request should not immediately succeed")
		}

		// 验证等待队列状态
		waitQueue := s.GetWaitQueue()
		if len(waitQueue) != 1 {
			t.Errorf("Wait queue should have 1 item, got %d", len(waitQueue))
		}

		// 步骤3: 等待时间片轮转
		t.Log("Step 3: Wait for Time Slice Rotation")
		time.Sleep(3 * time.Second)

		// 验证轮转结果
		serviceQueue := s.GetServiceQueue()
		waitQueue = s.GetWaitQueue()

		// 检查服务队列大小
		if len(serviceQueue) != MaxServices {
			t.Errorf("Service queue should maintain %d items, got %d", MaxServices, len(serviceQueue))
		}

		// 检查新请求是否已经进入服务队列
		_, inService := serviceQueue[104]
		if !inService {
			t.Error("New request should be in service queue after time slice")
		}
	})
}

// Test2 测试调度器
func Test2(t *testing.T) {
	// 测试1: 未达到服务上限时的直接分配
	t.Run("Direct Assignment", func(t *testing.T) {
		s := NewScheduler()
		success, err := s.HandleRequest(101, SpeedMedium)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !success {
			t.Error("Should assign directly when under limit")
		}
		if len(s.serviceQueue) != 1 {
			t.Errorf("Service queue should have 1 item, got %d", len(s.serviceQueue))
		}
	})

	// 测试2: 优先级抢占
	t.Run("Priority Preemption", func(t *testing.T) {
		s := NewScheduler()

		// 填满服务队列
		if _, err := s.HandleRequest(102, SpeedLow); err != nil {
			t.Errorf("Failed to add first low priority request: %v", err)
		}
		if _, err := s.HandleRequest(103, SpeedLow); err != nil {
			t.Errorf("Failed to add second low priority request: %v", err)
		}
		if _, err := s.HandleRequest(104, SpeedLow); err != nil {
			t.Errorf("Failed to add third low priority request: %v", err)
		}

		// 高优先级请求
		isScheduled, err := s.HandleRequest(105, SpeedHigh)
		if err != nil {
			t.Errorf("Failed to handle high priority request: %v", err)
		}
		if !isScheduled {
			t.Error("High priority request should preempt low priority")
		}

		if len(s.waitQueue) != 1 {
			t.Errorf("Wait queue should have 1 item, got %d", len(s.waitQueue))
		}
	})

	// 测试3: 时间片轮转
	t.Run("Time Slice Rotation", func(t *testing.T) {
		s := NewScheduler()

		// 填满相同优先级的请求
		for i := 101; i <= 103; i++ {
			if _, err := s.HandleRequest(i, SpeedMedium); err != nil {
				t.Errorf("Failed to add request %d: %v", i, err)
			}
		}

		// 新的相同优先级请求
		isScheduled, err := s.HandleRequest(104, SpeedMedium)
		if err != nil {
			t.Errorf("Failed to handle request: %v", err)
		}
		if isScheduled {
			t.Error("Should not immediately succeed with equal priority")
		}

		// 等待时间片
		time.Sleep(3 * time.Second)

		// 检查是否轮转成功
		if _, exists := s.serviceQueue[104]; !exists {
			t.Error("Request should be promoted after time slice")
		}
	})

	// 测试4: 多个低优先级时选择最低优先级
	t.Run("Multiple Low Priority Selection", func(t *testing.T) {
		s := NewScheduler()

		// 添加混合优先级的请求
		if _, err := s.HandleRequest(101, SpeedLow); err != nil {
			t.Errorf("Failed to add first request: %v", err)
		}
		if _, err := s.HandleRequest(102, SpeedMedium); err != nil {
			t.Errorf("Failed to add second request: %v", err)
		}
		if _, err := s.HandleRequest(103, SpeedLow); err != nil {
			t.Errorf("Failed to add third request: %v", err)
		}

		if _, err := s.HandleRequest(104, SpeedHigh); err != nil {
			t.Errorf("Failed to add high priority request: %v", err)
		}

		// 验证是否选择了一个低速服务
		lowSpeedCount := 0
		for _, service := range s.serviceQueue {
			if service.Speed == SpeedLow {
				lowSpeedCount++
			}
		}

		if lowSpeedCount != 1 {
			t.Errorf("Should have only one low speed service, got %d", lowSpeedCount)
		}
	})

	// 测试5: 服务时间最长优先被替换
	t.Run("Longest Service Time Replacement", func(t *testing.T) {
		s := NewScheduler()

		// 创建具有不同服务时间的服务
		if _, err := s.HandleRequest(101, SpeedMedium); err != nil {
			t.Errorf("Failed to add first request: %v", err)
		}
		time.Sleep(2 * time.Second)

		if _, err := s.HandleRequest(102, SpeedMedium); err != nil {
			t.Errorf("Failed to add second request: %v", err)
		}
		time.Sleep(1 * time.Second)

		if _, err := s.HandleRequest(103, SpeedMedium); err != nil {
			t.Errorf("Failed to add third request: %v", err)
		}

		if _, err := s.HandleRequest(104, SpeedHigh); err != nil {
			t.Errorf("Failed to add high priority request: %v", err)
		}

		// 验证最长服务时间的被替换
		if _, exists := s.serviceQueue[101]; exists {
			t.Error("Longest serving request should be replaced")
		}
	})
}

// 模拟负载测试
func BenchmarkScheduler(b *testing.B) {
	s := NewScheduler()

	b.RunParallel(func(pb *testing.PB) {
		roomID := 100
		for pb.Next() {
			_, err := s.HandleRequest(roomID, SpeedMedium)
			if err != nil {
				b.Errorf("Failed to handle request: %v", err)
			}
			roomID++
		}
	})
}
