package scheduler

import (
	"backend/internal/logger"
	"testing"
	"time"
)

func init() {
	// 设置日志级别为 Debug，显示详细日志
	logger.SetLevel(logger.DebugLevel)
}

// 测试直接分配服务对象的场景
func TestDirectAssignment(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始测试直接分配服务对象 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	room := 101
	targetTemp := float32(25.0)
	currentTemp := float32(28.0)

	logger.Info("添加房间%d的请求: 目标温度=%.1f, 当前温度=%.1f", room, targetTemp, currentTemp)
	success, _ := scheduler.HandleRequest(room, SpeedMedium, targetTemp, currentTemp)

	if !success {
		t.Error("第一个请求应该被直接分配服务对象")
	}

	serviceQueue := scheduler.GetServiceQueue()
	waitQueue := scheduler.GetWaitQueue()

	logger.Info("验证调度结果:")
	logger.Info("- 服务队列长度: %d", len(serviceQueue))
	logger.Info("- 等待队列长度: %d", len(waitQueue))

	if service, exists := serviceQueue[room]; exists {
		logger.Info("房间%d的服务状态:", room)
		logger.Info("- 风速: %s", service.Speed)
		logger.Info("- 目标温度: %.1f", service.TargetTemp)
		logger.Info("- 当前温度: %.1f", service.CurrentTemp)
	}
}

// 测试优先级调度
func TestPriorityScheduling(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始测试优先级调度 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	// 首先填满服务队列
	logger.Info("步骤1: 添加3个低优先级请求填满服务队列")
	for _, room := range []int{101, 102, 103} {
		success, _ := scheduler.HandleRequest(room, SpeedLow, 25.0, 28.0)
		logger.Info("添加房间%d: 成功=%v", room, success)
	}

	// 打印当前队列状态
	logger.Info("\n当前服务队列状态:")
	for room, service := range scheduler.GetServiceQueue() {
		logger.Info("房间%d: 速度=%s, 目标温度=%.1f", room, service.Speed, service.TargetTemp)
	}

	// 添加高优先级请求
	logger.Info("\n步骤2: 添加高优先级请求")
	highPriorityRoom := 104
	success, _ := scheduler.HandleRequest(highPriorityRoom, SpeedHigh, 25.0, 28.0)

	if !success {
		t.Error("高优先级请求应该抢占低优先级服务")
	}

	// 验证结果
	serviceQueue := scheduler.GetServiceQueue()
	waitQueue := scheduler.GetWaitQueue()

	logger.Info("\n验证调度结果:")
	logger.Info("- 服务队列长度: %d", len(serviceQueue))
	logger.Info("- 等待队列长度: %d", len(waitQueue))

	if _, exists := serviceQueue[highPriorityRoom]; exists {
		logger.Info("- 高优先级房间%d成功进入服务队列", highPriorityRoom)
	}
}

// 测试时间片轮转
func TestTimeSliceRotation(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始测试时间片轮转 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	// 填满服务队列
	logger.Info("步骤1: 添加3个中速请求填满服务队列")
	for room := 101; room <= 103; room++ {
		success, _ := scheduler.HandleRequest(room, SpeedMedium, 25.0, 28.0)
		logger.Info("添加房间%d: 成功=%v", room, success)
	}

	// 添加第4个相同优先级请求
	newRoom := 104
	logger.Info("\n步骤2: 添加第4个中速请求(房间%d)", newRoom)
	success, _ := scheduler.HandleRequest(newRoom, SpeedMedium, 25.0, 28.0)

	logger.Info("新请求进入等待队列: 成功=%v", !success)

	waitQueue := scheduler.GetWaitQueue()
	logger.Info("等待队列状态:")
	for room, wait := range waitQueue {
		logger.Info("- 房间%d: 等待时长=%.1f秒", room, wait.WaitDuration)
	}

	// 等待时间片到期
	logger.Info("\n步骤3: 等待时间片到期(%.1f秒)...", WaitTime+1)
	time.Sleep(time.Duration(WaitTime+1) * time.Second)

	// 验证轮转结果
	serviceQueue := scheduler.GetServiceQueue()
	logger.Info("\n轮转后的服务队列:")
	for room, service := range serviceQueue {
		logger.Info("- 房间%d: 速度=%s, 服务时长=%.1f秒",
			room, service.Speed, service.Duration)
	}
	logger.Info("\n等待队列状态:")
	for room, wait := range waitQueue {
		logger.Info("- 房间%d: 等待时长=%.1f秒", room, wait.WaitDuration)
	}
}

// 测试温度变化
func TestTemperatureChange(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始测试温度变化 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	room := 101
	targetTemp := float32(25.0)
	currentTemp := float32(28.0)

	logger.Info("添加降温请求:")
	logger.Info("- 房间: %d", room)
	logger.Info("- 当前温度: %.1f", currentTemp)
	logger.Info("- 目标温度: %.1f", targetTemp)
	logger.Info("- 速度: %s (变化率: %.1f/秒)", SpeedMedium, TempChangeRateMedium)

	scheduler.HandleRequest(room, SpeedMedium, targetTemp, currentTemp)

	// 等待温度变化
	waitTime := 2
	logger.Info("\n等待%d秒观察温度变化...", waitTime)
	time.Sleep(time.Duration(waitTime) * time.Second)

	// 验证温度变化
	serviceQueue := scheduler.GetServiceQueue()
	if service, exists := serviceQueue[room]; exists {
		expectedTemp := currentTemp - float32(float64(waitTime)*float64(TempChangeRateMedium))
		logger.Info("\n温度变化结果:")
		logger.Info("- 预期温度: %.1f", expectedTemp)
		logger.Info("- 实际温度: %.1f", service.CurrentTemp)
		logger.Info("- 温度变化: %.1f", currentTemp-service.CurrentTemp)

		if service.CurrentTemp > currentTemp || service.CurrentTemp < expectedTemp-0.1 {
			t.Errorf("温度变化异常，预期: %.1f, 实际: %.1f",
				expectedTemp, service.CurrentTemp)
		}
	}
}

// 复杂场景测试
func TestComplexScenario(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始复杂场景测试 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	// 第一阶段：初始化3个房间的请求
	logger.Info("\n第一阶段: 初始化3个房间的请求")
	requests := []struct {
		roomID      int
		speed       string
		targetTemp  float32
		currentTemp float32
	}{
		{101, SpeedLow, 25.0, 28.0},    // 低速降温
		{102, SpeedMedium, 26.0, 23.0}, // 中速升温
		{103, SpeedHigh, 24.0, 29.0},   // 高速降温
	}

	for _, req := range requests {
		success, _ := scheduler.HandleRequest(
			req.roomID, req.speed, req.targetTemp, req.currentTemp)
		logger.Info("添加房间%d请求: 速度=%s, 目标温度=%.1f, 当前温度=%.1f, 成功=%v",
			req.roomID, req.speed, req.targetTemp, req.currentTemp, success)
	}

	// 打印初始状态
	logQueueStatus(scheduler, "初始状态")

	// 第二阶段：等待一段时间让温度变化，然后添加新的高优先级请求
	logger.Info("\n第二阶段: 等待温度变化(2秒)并添加新请求...")
	time.Sleep(2 * time.Second)
	logQueueStatus(scheduler, "温度变化后")

	// 添加新的高优先级请求
	logger.Info("\n添加新的高优先级请求:")
	success, _ := scheduler.HandleRequest(104, SpeedHigh, 23.0, 28.0)
	logger.Info("房间104(高优先级): 成功=%v", success)
	logQueueStatus(scheduler, "添加高优先级请求后")

	// 第三阶段：模拟某个房间达到目标温度
	logger.Info("\n第三阶段: 模拟房间102接近目标温度")
	// 添加一个接近目标温度的请求
	scheduler.HandleRequest(105, SpeedMedium, 25.0, 25.1)
	logger.Info("添加房间105(接近目标温度)...")
	time.Sleep(2 * time.Second)
	logQueueStatus(scheduler, "房间温度接近目标值后")

	// 第四阶段：在等待队列中的请求改变风速
	logger.Info("\n第四阶段: 改变等待队列中请求的风速")
	waitQueue := scheduler.GetWaitQueue()
	var waitingRoom int
	for roomID := range waitQueue {
		waitingRoom = roomID
		break
	}
	if waitingRoom != 0 {
		logger.Info("将房间%d的风速从%s改为%s",
			waitingRoom,
			scheduler.GetWaitQueue()[waitingRoom].Speed,
			SpeedHigh)
		success, _ := scheduler.HandleRequest(waitingRoom, SpeedHigh, 25.0, 28.0)
		logger.Info("改变风速: 成功=%v", success)
	}
	logQueueStatus(scheduler, "改变等待队列风速后")

	// 第五阶段：模拟服务对象完成任务后的切换
	logger.Info("\n第五阶段: 等待服务完成和队列切换(3秒)...")
	time.Sleep(3 * time.Second)
	logQueueStatus(scheduler, "最终状态")

	// 验证最终状态
	serviceQueue := scheduler.GetServiceQueue()
	waitQueue = scheduler.GetWaitQueue()

	logger.Info("\n验证最终状态:")
	logger.Info("- 服务队列长度: %d", len(serviceQueue))
	logger.Info("- 等待队列长度: %d", len(waitQueue))

	// 检查服务对象的状态
	logger.Info("\n服务对象状态:")
	for roomID, service := range serviceQueue {
		logger.Info("房间%d:", roomID)
		logger.Info("- 速度: %s", service.Speed)
		logger.Info("- 当前温度: %.1f", service.CurrentTemp)
		logger.Info("- 目标温度: %.1f", service.TargetTemp)
		logger.Info("- 服务时长: %.1f秒", service.Duration)
		logger.Info("- 是否完成: %v", service.IsCompleted)
	}
}

// 辅助函数：打印队列状态
func logQueueStatus(s *Scheduler, stage string) {
	logger.Info("\n=== %s ===", stage)

	logger.Info("服务队列:")
	for roomID, service := range s.GetServiceQueue() {
		logger.Info("房间%d: 速度=%s, 当前温度=%.1f, 目标温度=%.1f, 服务时长=%.1f秒, 完成=%v",
			roomID, service.Speed, service.CurrentTemp, service.TargetTemp,
			service.Duration, service.IsCompleted)
	}

	logger.Info("等待队列:")
	for roomID, wait := range s.GetWaitQueue() {
		logger.Info("房间%d: 速度=%s, 当前温度=%.1f, 目标温度=%.1f, 等待时长=%.1f秒",
			roomID, wait.Speed, wait.CurrentTemp, wait.TargetTemp, wait.WaitDuration)
	}
	logger.Info("------------------------")
}

func TestHandleDuplicateRequests(t *testing.T) {
	if testing.Verbose() {
		logger.SetLevel(logger.DebugLevel) // 使用 -v 运行测试时显示详细日志
	} else {
		logger.SetLevel(logger.InfoLevel)
	}
	logger.Info("=== 开始测试重复请求处理 ===")
	scheduler := NewScheduler()
	defer scheduler.Stop()

	// 第一阶段：初始化请求
	logger.Info("\n第一阶段: 添加初始请求")
	scheduler.HandleRequest(101, SpeedLow, 25.0, 28.0)    // 直接进入服务队列
	scheduler.HandleRequest(102, SpeedMedium, 26.0, 23.0) // 直接进入服务队列
	scheduler.HandleRequest(103, SpeedHigh, 24.0, 29.0)   // 直接进入服务队列
	scheduler.HandleRequest(104, SpeedMedium, 25.0, 27.0) // 进入等待队列

	logQueueStatus(scheduler, "初始状态")

	// 第二阶段：对服务队列中的房间发出新请求
	logger.Info("\n第二阶段: 修改服务队列中房间101的参数")
	scheduler.HandleRequest(101, SpeedMedium, 24.0, 28.0) // 应该只更新参数

	logQueueStatus(scheduler, "修改服务队列参数后")

	// 验证服务队列状态
	serviceQueue := scheduler.GetServiceQueue()
	if service, exists := serviceQueue[101]; exists {
		if service.Speed != SpeedMedium {
			t.Errorf("Speed should be updated to medium, got %s", service.Speed)
		}
		if service.TargetTemp != 24.0 {
			t.Errorf("Target temperature should be updated to 24.0, got %f", service.TargetTemp)
		}
	}

	// 第三阶段：对等待队列中的房间发出新请求
	logger.Info("\n第三阶段: 修改等待队列中房间104的参数")
	// 先用相同优先级
	scheduler.HandleRequest(104, SpeedMedium, 23.0, 27.0) // 应该只更新参数
	logQueueStatus(scheduler, "修改等待队列参数后(相同优先级)")

	// 再用更高优先级
	logger.Info("\n第四阶段: 提高等待队列中房间104的优先级")
	scheduler.HandleRequest(104, SpeedHigh, 23.0, 27.0) // 应该触发重新调度
	logQueueStatus(scheduler, "修改等待队列参数后(提高优先级)")

	// 验证是否触发了重新调度
	serviceQueue = scheduler.GetServiceQueue()
	waitQueue := scheduler.GetWaitQueue()

	if _, exists := serviceQueue[104]; !exists {
		t.Error("Room 104 should be promoted to service queue after priority increase")
	}
	if len(waitQueue) == 0 {
		t.Error("Wait queue should not be empty after promotion")
	}
}
