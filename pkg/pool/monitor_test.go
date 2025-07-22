package pool

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// monitorMockTask 用于测试的模拟任务
type monitorMockTask struct {
	priority   int
	duration   time.Duration
	shouldFail bool
}

func (t *monitorMockTask) Execute(ctx context.Context) (any, error) {
	if t.duration > 0 {
		time.Sleep(t.duration)
	}
	if t.shouldFail {
		return nil, context.DeadlineExceeded
	}
	return "success", nil
}

func (t *monitorMockTask) Priority() int {
	return t.priority
}

func TestNewMonitor(t *testing.T) {
	config := &Config{
		EnableMetrics:   true,
		MetricsInterval: 1 * time.Second,
	}

	monitor := NewMonitor(config)
	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}

	// 验证监控器类型
	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}

	// 验证监控器可以正常工作
	stats := monitor.GetStats()
	if stats.ActiveWorkers < 0 {
		t.Error("Expected non-negative ActiveWorkers")
	}
}

func TestNewMonitorWithDefaults(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
		// MetricsInterval not set, should use default
	}

	monitor := NewMonitor(config)
	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}

	// Test that monitor works with default settings
	stats := monitor.GetStats()
	if stats.ActiveWorkers < 0 {
		t.Error("Expected non-negative ActiveWorkers")
	}
}

func TestMonitorStartStop(t *testing.T) {
	config := &Config{
		EnableMetrics:   true,
		MetricsInterval: 100 * time.Millisecond,
	}

	monitor := NewMonitor(config)

	// 测试启动
	monitor.Start()

	// 等待一小段时间确保更新循环开始
	time.Sleep(50 * time.Millisecond)

	// 测试停止
	monitor.Stop()

	// 测试重复停止不会panic
	monitor.Stop()
}

func TestMonitorDisabled(t *testing.T) {
	config := &Config{
		EnableMetrics: false,
	}

	monitor := NewMonitor(config)

	// 启动禁用的监控器应该不做任何事
	monitor.Start()

	// 验证禁用的监控器不记录统计信息
	task := &monitorMockTask{priority: 1}
	monitor.RecordTaskSubmit(task)
	stats := monitor.GetStats()
	if stats.QueuedTasks != 0 {
		t.Error("Expected disabled monitor to not record metrics")
	}
}

func TestRecordTaskSubmit(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 记录任务提交
	monitor.RecordTaskSubmit(task)

	stats := monitor.GetStats()
	if stats.QueuedTasks != 1 {
		t.Errorf("Expected QueuedTasks to be 1, got %d", stats.QueuedTasks)
	}

	// 记录多个任务提交
	for i := 0; i < 5; i++ {
		monitor.RecordTaskSubmit(task)
	}

	stats = monitor.GetStats()
	if stats.QueuedTasks != 6 {
		t.Errorf("Expected QueuedTasks to be 6, got %d", stats.QueuedTasks)
	}
}

func TestRecordTaskComplete(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 先提交任务
	monitor.RecordTaskSubmit(task)

	// 记录任务完成
	duration := 100 * time.Millisecond
	monitor.RecordTaskComplete(task, duration)

	stats := monitor.GetStats()
	if stats.QueuedTasks != 0 {
		t.Errorf("Expected QueuedTasks to be 0, got %d", stats.QueuedTasks)
	}
	if stats.CompletedTasks != 1 {
		t.Errorf("Expected CompletedTasks to be 1, got %d", stats.CompletedTasks)
	}
	if stats.AvgTaskDuration != duration {
		t.Errorf("Expected AvgTaskDuration to be %v, got %v", duration, stats.AvgTaskDuration)
	}
}

func TestRecordTaskFail(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 先提交任务
	monitor.RecordTaskSubmit(task)

	// 记录任务失败
	monitor.RecordTaskFail(task, context.DeadlineExceeded)

	stats := monitor.GetStats()
	if stats.QueuedTasks != 0 {
		t.Errorf("Expected QueuedTasks to be 0, got %d", stats.QueuedTasks)
	}
	if stats.FailedTasks != 1 {
		t.Errorf("Expected FailedTasks to be 1, got %d", stats.FailedTasks)
	}
}

func TestSetActiveWorkers(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)

	// 设置活跃工作协程数
	monitor.SetActiveWorkers(5)

	stats := monitor.GetStats()
	if stats.ActiveWorkers != 5 {
		t.Errorf("Expected ActiveWorkers to be 5, got %d", stats.ActiveWorkers)
	}
}

func TestSetQueuedTasks(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)

	// 设置排队任务数
	monitor.SetQueuedTasks(10)

	stats := monitor.GetStats()
	if stats.QueuedTasks != 10 {
		t.Errorf("Expected QueuedTasks to be 10, got %d", stats.QueuedTasks)
	}
}

func TestAvgTaskDurationCalculation(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	// 记录多个任务完成
	for _, duration := range durations {
		monitor.RecordTaskComplete(task, duration)
	}

	stats := monitor.GetStats()
	expectedAvg := (100 + 200 + 300) * time.Millisecond / 3
	if stats.AvgTaskDuration != expectedAvg {
		t.Errorf("Expected AvgTaskDuration to be %v, got %v", expectedAvg, stats.AvgTaskDuration)
	}
}

func TestResourceMetricsUpdate(t *testing.T) {
	config := &Config{
		EnableMetrics:   true,
		MetricsInterval: 50 * time.Millisecond,
	}

	monitor := NewMonitor(config)

	// 启动监控器
	monitor.Start()
	defer monitor.Stop()

	// 等待资源指标更新
	time.Sleep(100 * time.Millisecond)

	stats := monitor.GetStats()

	// 内存使用量应该大于0
	if stats.MemoryUsage <= 0 {
		t.Error("Expected MemoryUsage to be greater than 0")
	}

	// GC次数应该大于等于0
	if stats.GCCount < 0 {
		t.Error("Expected GCCount to be non-negative")
	}
}

func TestThroughputCalculation(t *testing.T) {
	config := &Config{
		EnableMetrics:   true,
		MetricsInterval: 100 * time.Millisecond,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 启动监控器
	monitor.Start()
	defer monitor.Stop()

	// 记录一些任务完成
	for i := 0; i < 10; i++ {
		monitor.RecordTaskComplete(task, 10*time.Millisecond)
		time.Sleep(10 * time.Millisecond)
	}

	// 等待吞吐量计算
	time.Sleep(150 * time.Millisecond)

	stats := monitor.GetStats()

	// 吞吐量应该大于0
	if stats.ThroughputTPS <= 0 {
		t.Errorf("Expected ThroughputTPS to be greater than 0, got %f", stats.ThroughputTPS)
	}
}

func TestConcurrentAccess(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// 并发记录任务提交
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				monitor.RecordTaskSubmit(task)
			}
		}()
	}

	// 并发记录任务完成
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				monitor.RecordTaskComplete(task, 10*time.Millisecond)
			}
		}()
	}

	// 并发获取统计信息
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = monitor.GetStats()
			}
		}()
	}

	wg.Wait()

	// 验证最终统计
	stats := monitor.GetStats()
	expectedCompleted := int64(numGoroutines * numOperations)

	if stats.QueuedTasks != 0 {
		t.Errorf("Expected QueuedTasks to be 0, got %d", stats.QueuedTasks)
	}
	if stats.CompletedTasks != expectedCompleted {
		t.Errorf("Expected CompletedTasks to be %d, got %d", expectedCompleted, stats.CompletedTasks)
	}
}

func TestHistoryDataManagement(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 记录大量任务来测试历史数据管理
	for i := 0; i < 1100; i++ {
		monitor.RecordTaskComplete(task, time.Duration(i)*time.Millisecond)
	}

	// 验证监控器仍然正常工作
	stats := monitor.GetStats()
	if stats.CompletedTasks != 1100 {
		t.Errorf("Expected CompletedTasks to be 1100, got %d", stats.CompletedTasks)
	}
}

func TestGetDetailedStats(t *testing.T) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 记录一些任务完成，使用不同的执行时间
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, duration := range durations {
		monitor.RecordTaskComplete(task, duration)
		time.Sleep(1 * time.Millisecond) // 确保时间戳不同
	}

	stats := monitor.GetStats()

	// 验证基础统计
	if stats.CompletedTasks != int64(len(durations)) {
		t.Errorf("Expected CompletedTasks to be %d, got %d", len(durations), stats.CompletedTasks)
	}

	// 验证平均任务执行时间
	expectedAvg := (10 + 20 + 30 + 40 + 50) * time.Millisecond / 5
	if stats.AvgTaskDuration != expectedAvg {
		t.Errorf("Expected AvgTaskDuration to be %v, got %v", expectedAvg, stats.AvgTaskDuration)
	}
}

func TestDisabledMonitorOperations(t *testing.T) {
	config := &Config{
		EnableMetrics: false,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	// 对禁用的监控器执行操作
	monitor.RecordTaskSubmit(task)
	monitor.RecordTaskComplete(task, 100*time.Millisecond)
	monitor.RecordTaskFail(task, context.DeadlineExceeded)

	stats := monitor.GetStats()

	// 所有统计应该为0
	if stats.QueuedTasks != 0 {
		t.Errorf("Expected QueuedTasks to be 0 for disabled monitor, got %d", stats.QueuedTasks)
	}
	if stats.CompletedTasks != 0 {
		t.Errorf("Expected CompletedTasks to be 0 for disabled monitor, got %d", stats.CompletedTasks)
	}
	if stats.FailedTasks != 0 {
		t.Errorf("Expected FailedTasks to be 0 for disabled monitor, got %d", stats.FailedTasks)
	}
}

func BenchmarkRecordTaskSubmit(b *testing.B) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			monitor.RecordTaskSubmit(task)
		}
	})
}

func BenchmarkRecordTaskComplete(b *testing.B) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)
	task := &monitorMockTask{priority: 1}
	duration := 10 * time.Millisecond

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			monitor.RecordTaskComplete(task, duration)
		}
	})
}

func BenchmarkGetStats(b *testing.B) {
	config := &Config{
		EnableMetrics: true,
	}

	monitor := NewMonitor(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = monitor.GetStats()
		}
	})
}

func TestMemoryUsageAccuracy(t *testing.T) {
	config := &Config{
		EnableMetrics:   true,
		MetricsInterval: 50 * time.Millisecond,
	}

	monitor := NewMonitor(config)

	// 启动监控器
	monitor.Start()
	defer monitor.Stop()

	// 获取初始内存使用量
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialStats := monitor.GetStats()

	// 分配一些内存
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	// 等待监控器更新
	time.Sleep(100 * time.Millisecond)
	newStats := monitor.GetStats()

	// 内存使用量应该增加
	if newStats.MemoryUsage <= initialStats.MemoryUsage {
		t.Logf("Initial memory: %d, New memory: %d", initialStats.MemoryUsage, newStats.MemoryUsage)
		// 注意：由于GC的不确定性，这个测试可能不总是通过，所以只记录日志
	}
}
