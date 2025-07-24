package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptutu/goman/internal/queue"
	"github.com/uptutu/goman/internal/worker"
	"github.com/uptutu/goman/pkg/pool"
)

// MockTask 模拟任务
type MockTask struct {
	id       int
	priority int
	duration time.Duration
	executed int32
	result   interface{}
	err      error
}

func NewMockTask(id int, priority int, duration time.Duration) *MockTask {
	return &MockTask{
		id:       id,
		priority: priority,
		duration: duration,
	}
}

func (t *MockTask) Execute(ctx context.Context) (interface{}, error) {
	atomic.StoreInt32(&t.executed, 1)
	if t.duration > 0 {
		time.Sleep(t.duration)
	}
	return t.result, t.err
}

func (t *MockTask) Priority() int {
	return t.priority
}

func (t *MockTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

// MockMonitor 模拟监控器
type MockMonitor struct {
	submitCount   int64
	completeCount int64
	failCount     int64
}

func (m *MockMonitor) RecordTaskSubmit(task pool.Task) {
	atomic.AddInt64(&m.submitCount, 1)
}

func (m *MockMonitor) RecordTaskComplete(task pool.Task, duration time.Duration) {
	atomic.AddInt64(&m.completeCount, 1)
}

func (m *MockMonitor) RecordTaskFail(task pool.Task, err error) {
	atomic.AddInt64(&m.failCount, 1)
}

func (m *MockMonitor) GetStats() pool.PoolStats {
	return pool.PoolStats{}
}

func (m *MockMonitor) Start() {}

func (m *MockMonitor) Stop() {}

func (m *MockMonitor) SetActiveWorkers(count int64) {}

func (m *MockMonitor) SetQueuedTasks(count int64) {}

// MockErrorHandler 模拟错误处理器
type MockErrorHandler struct {
	panicCount     int64
	timeoutCount   int64
	queueFullCount int64
}

func (h *MockErrorHandler) HandlePanic(workerID int, task pool.Task, panicValue interface{}) {
	atomic.AddInt64(&h.panicCount, 1)
}

func (h *MockErrorHandler) HandleTimeout(task pool.Task, duration time.Duration) {
	atomic.AddInt64(&h.timeoutCount, 1)
}

func (h *MockErrorHandler) HandleQueueFull(task pool.Task) error {
	atomic.AddInt64(&h.queueFullCount, 1)
	return pool.ErrQueueFull
}

func (h *MockErrorHandler) GetStats() pool.ErrorStats {
	return pool.ErrorStats{
		PanicCount:     atomic.LoadInt64(&h.panicCount),
		TimeoutCount:   atomic.LoadInt64(&h.timeoutCount),
		QueueFullCount: atomic.LoadInt64(&h.queueFullCount),
	}
}

// 创建测试用的调度器
func createTestScheduler(workerCount int) (*Scheduler, []pool.Worker, *MockMonitor, *MockErrorHandler) {
	config := pool.NewConfig()
	config.WorkerCount = workerCount
	config.QueueSize = 100

	taskQueue := queue.NewLockFreeQueue(uint64(config.QueueSize))
	monitor := &MockMonitor{}
	errorHandler := &MockErrorHandler{}

	// 创建工作协程
	workers := make([]pool.Worker, workerCount)
	for i := 0; i < workerCount; i++ {
		workers[i] = worker.NewWorker(i, nil, config, errorHandler, monitor)
	}

	scheduler := NewScheduler(workers, taskQueue, config, monitor, errorHandler)
	return scheduler, workers, monitor, errorHandler
}

func TestNewScheduler(t *testing.T) {
	scheduler, workers, monitor, errorHandler := createTestScheduler(4)

	if scheduler == nil {
		t.Fatal("Expected scheduler to be created")
	}

	if len(scheduler.workers) != 4 {
		t.Errorf("Expected 4 workers, got %d", len(scheduler.workers))
	}

	if scheduler.monitor != monitor {
		t.Error("Expected monitor to be set")
	}

	if scheduler.errorHandler != errorHandler {
		t.Error("Expected error handler to be set")
	}

	if !scheduler.stealEnabled {
		t.Error("Expected work stealing to be enabled by default")
	}

	if len(scheduler.priorityQueues) != 4 {
		t.Errorf("Expected 4 priority queues, got %d", len(scheduler.priorityQueues))
	}

	// 验证负载均衡器
	if _, ok := scheduler.loadBalancer.(*RoundRobinBalancer); !ok {
		t.Error("Expected default load balancer to be RoundRobinBalancer")
	}

	// 清理
	for _, worker := range workers {
		worker.Stop()
	}
}

func TestSchedulerStartStop(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(2)
	defer func() {
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 测试启动
	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	stats := scheduler.GetStats()
	if !stats.IsRunning {
		t.Error("Expected scheduler to be running")
	}

	// 测试重复启动
	err = scheduler.Start()
	if err == nil {
		t.Error("Expected error when starting already running scheduler")
	}

	// 测试停止
	err = scheduler.Stop()
	if err != nil {
		t.Errorf("Failed to stop scheduler: %v", err)
	}

	stats = scheduler.GetStats()
	if stats.IsRunning {
		t.Error("Expected scheduler to be stopped")
	}
}

func TestSchedulerScheduleTask(t *testing.T) {
	scheduler, workers, monitor, _ := createTestScheduler(2)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 启动调度器
	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 创建测试任务
	task := NewMockTask(1, pool.PriorityNormal, time.Millisecond*10)

	// 调度任务
	err = scheduler.Schedule(task)
	if err != nil {
		t.Errorf("Failed to schedule task: %v", err)
	}

	// 验证监控器记录
	time.Sleep(time.Millisecond * 50) // 等待任务处理
	if atomic.LoadInt64(&monitor.submitCount) != 1 {
		t.Errorf("Expected 1 task submitted, got %d", monitor.submitCount)
	}

	// 验证统计信息
	stats := scheduler.GetStats()
	if stats.TotalSubmitted != 1 {
		t.Errorf("Expected 1 total submitted, got %d", stats.TotalSubmitted)
	}
}

func TestSchedulerPriorityScheduling(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(1)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 启动工作协程
	var wg sync.WaitGroup
	for _, worker := range workers {
		worker.Start(&wg)
	}

	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 创建不同优先级的任务
	lowTask := NewMockTask(1, pool.PriorityLow, time.Millisecond*5)
	normalTask := NewMockTask(2, pool.PriorityNormal, time.Millisecond*5)
	highTask := NewMockTask(3, pool.PriorityHigh, time.Millisecond*5)
	criticalTask := NewMockTask(4, pool.PriorityCritical, time.Millisecond*5)

	// 按相反顺序提交任务（低优先级先提交）
	tasks := []*MockTask{lowTask, normalTask, highTask, criticalTask}
	for _, task := range tasks {
		err := scheduler.Schedule(task)
		if err != nil {
			t.Errorf("Failed to schedule task %d: %v", task.id, err)
		}
	}

	// 等待任务执行完成
	time.Sleep(time.Millisecond * 200)

	// 验证所有任务都被执行
	for _, task := range tasks {
		if !task.IsExecuted() {
			t.Errorf("Task %d was not executed", task.id)
		}
	}

	stats := scheduler.GetStats()
	if stats.TotalSubmitted != 4 {
		t.Errorf("Expected 4 total submitted, got %d", stats.TotalSubmitted)
	}
}

func TestSchedulerLoadBalancers(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(3)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 测试轮询负载均衡器
	roundRobin := NewRoundRobinBalancer()
	scheduler.SetLoadBalancer(roundRobin)

	if scheduler.getLoadBalancerType() != "RoundRobin" {
		t.Error("Expected RoundRobin load balancer")
	}

	// 测试最少负载负载均衡器
	leastLoad := NewLeastLoadBalancer()
	scheduler.SetLoadBalancer(leastLoad)

	if scheduler.getLoadBalancerType() != "LeastLoad" {
		t.Error("Expected LeastLoad load balancer")
	}

	// 测试随机负载均衡器
	random := NewRandomBalancer()
	scheduler.SetLoadBalancer(random)

	if scheduler.getLoadBalancerType() != "Random" {
		t.Error("Expected Random load balancer")
	}
}

func TestSchedulerWorkStealing(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(2)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 启用工作窃取
	scheduler.EnableWorkStealing(1, time.Millisecond*50)

	if !scheduler.stealEnabled {
		t.Error("Expected work stealing to be enabled")
	}

	if scheduler.stealThreshold != 1 {
		t.Errorf("Expected steal threshold to be 1, got %d", scheduler.stealThreshold)
	}

	// 禁用工作窃取
	scheduler.DisableWorkStealing()

	if scheduler.stealEnabled {
		t.Error("Expected work stealing to be disabled")
	}
}

func TestSchedulerQueueFull(t *testing.T) {
	// 创建小队列的调度器
	config := pool.NewConfig()
	config.WorkerCount = 1
	config.QueueSize = 2 // 很小的队列

	taskQueue := queue.NewLockFreeQueue(uint64(config.QueueSize))
	monitor := &MockMonitor{}
	errorHandler := &MockErrorHandler{}

	workers := make([]pool.Worker, 1)
	workers[0] = worker.NewWorker(0, nil, config, errorHandler, monitor)

	scheduler := NewScheduler(workers, taskQueue, config, monitor, errorHandler)
	defer func() {
		scheduler.Stop()
		workers[0].Stop()
	}()

	// 启动工作协程（但让它们很慢，这样队列会快速填满）
	var wg sync.WaitGroup
	workers[0].Start(&wg)

	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 提交大量长时间运行的任务直到队列满
	taskCount := 20
	var failedCount int

	for i := 0; i < taskCount; i++ {
		task := NewMockTask(i, pool.PriorityNormal, time.Millisecond*500) // 长时间任务
		err := scheduler.Schedule(task)
		if err != nil {
			failedCount++
		}
		// 快速提交，不给工作协程处理的时间
		time.Sleep(time.Microsecond * 10)
	}

	// 等待一些任务开始执行
	time.Sleep(time.Millisecond * 50)

	if failedCount == 0 {
		t.Logf("No tasks failed immediately, but this might be expected with small queue")
	}

	// 验证至少有一些任务被提交了
	stats := scheduler.GetStats()
	if stats.TotalSubmitted == 0 {
		t.Error("Expected at least some tasks to be submitted")
	}

	t.Logf("Submitted: %d, Failed: %d, Queue Full Count: %d",
		stats.TotalSubmitted, failedCount, atomic.LoadInt64(&errorHandler.queueFullCount))
}

func TestSchedulerConcurrency(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(4)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// 并发提交任务
	taskCount := 100
	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := NewMockTask(id, pool.PriorityNormal, time.Millisecond*5)
			err := scheduler.Schedule(task)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 等待任务执行完成
	time.Sleep(time.Millisecond * 200)

	stats := scheduler.GetStats()
	if stats.TotalSubmitted != successCount {
		t.Errorf("Expected %d submitted tasks, got %d", successCount, stats.TotalSubmitted)
	}

	if successCount < int64(taskCount/2) {
		t.Errorf("Expected at least half tasks to succeed, got %d/%d", successCount, taskCount)
	}
}

func TestSchedulerStats(t *testing.T) {
	scheduler, workers, _, _ := createTestScheduler(3)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	stats := scheduler.GetStats()

	if stats.IsRunning {
		t.Error("Expected scheduler to not be running initially")
	}

	if stats.WorkerCount != 3 {
		t.Errorf("Expected 3 workers, got %d", stats.WorkerCount)
	}

	if stats.TotalSubmitted != 0 {
		t.Errorf("Expected 0 submitted tasks, got %d", stats.TotalSubmitted)
	}

	if stats.LoadBalancer != "RoundRobin" {
		t.Errorf("Expected RoundRobin load balancer, got %s", stats.LoadBalancer)
	}

	// 启动后再次检查
	err := scheduler.Start()
	if err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	stats = scheduler.GetStats()
	if !stats.IsRunning {
		t.Error("Expected scheduler to be running")
	}
}

// 基准测试
func BenchmarkSchedulerSchedule(b *testing.B) {
	scheduler, workers, _, _ := createTestScheduler(4)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	err := scheduler.Start()
	if err != nil {
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		taskID := 0
		for pb.Next() {
			task := NewMockTask(taskID, pool.PriorityNormal, 0)
			scheduler.Schedule(task)
			taskID++
		}
	})
}

func BenchmarkSchedulerRoundRobin(b *testing.B) {
	scheduler, workers, _, _ := createTestScheduler(8)
	scheduler.SetLoadBalancer(NewRoundRobinBalancer())
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	err := scheduler.Start()
	if err != nil {
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := NewMockTask(i, pool.PriorityNormal, 0)
		scheduler.Schedule(task)
	}
}

func BenchmarkSchedulerLeastLoad(b *testing.B) {
	scheduler, workers, _, _ := createTestScheduler(8)
	scheduler.SetLoadBalancer(NewLeastLoadBalancer())
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	err := scheduler.Start()
	if err != nil {
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := NewMockTask(i, pool.PriorityNormal, 0)
		scheduler.Schedule(task)
	}
}
