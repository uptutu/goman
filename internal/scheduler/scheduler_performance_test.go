package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptutu/goman/internal/queue"
	"github.com/uptutu/goman/internal/worker"
	"github.com/uptutu/goman/pkg/pool"
)

// PerformanceTask 性能测试任务
type PerformanceTask struct {
	id       int
	priority int
	workload time.Duration
	executed int32
}

func NewPerformanceTask(id int, priority int, workload time.Duration) *PerformanceTask {
	return &PerformanceTask{
		id:       id,
		priority: priority,
		workload: workload,
	}
}

func (t *PerformanceTask) Execute(ctx context.Context) (interface{}, error) {
	atomic.StoreInt32(&t.executed, 1)
	if t.workload > 0 {
		time.Sleep(t.workload)
	}
	return t.id, nil
}

func (t *PerformanceTask) Priority() int {
	return t.priority
}

func (t *PerformanceTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

// 创建性能测试用的调度器
func createPerformanceScheduler(workerCount int, queueSize int) (*Scheduler, []pool.Worker, *MockMonitor, *MockErrorHandler) {
	config := pool.NewConfig()
	config.WorkerCount = workerCount
	config.QueueSize = queueSize

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

// BenchmarkSchedulerThroughput 测试调度器吞吐量
func BenchmarkSchedulerThroughput(b *testing.B) {
	scheduler, workers, _, _ := createPerformanceScheduler(8, 1000)
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
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		taskID := 0
		for pb.Next() {
			task := NewPerformanceTask(taskID, pool.PriorityNormal, 0)
			scheduler.Schedule(task)
			taskID++
		}
	})
}

// BenchmarkSchedulerWithWorkload 测试带工作负载的调度器性能
func BenchmarkSchedulerWithWorkload(b *testing.B) {
	scheduler, workers, _, _ := createPerformanceScheduler(4, 500)
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
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := NewPerformanceTask(i, pool.PriorityNormal, time.Microsecond*10)
		scheduler.Schedule(task)
	}
}

// BenchmarkSchedulerPriorityMix 测试混合优先级任务的性能
func BenchmarkSchedulerPriorityMix(b *testing.B) {
	scheduler, workers, _, _ := createPerformanceScheduler(6, 800)
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
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	priorities := []int{pool.PriorityLow, pool.PriorityNormal, pool.PriorityHigh, pool.PriorityCritical}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		priority := priorities[i%len(priorities)]
		task := NewPerformanceTask(i, priority, 0)
		scheduler.Schedule(task)
	}
}

// BenchmarkLoadBalancerComparison 比较不同负载均衡器的性能
func BenchmarkLoadBalancerComparison(b *testing.B) {
	testCases := []struct {
		name     string
		balancer pool.LoadBalancer
	}{
		{"RoundRobin", NewRoundRobinBalancer()},
		{"LeastLoad", NewLeastLoadBalancer()},
		{"Random", NewRandomBalancer()},
		{"WeightedRoundRobin", NewWeightedRoundRobinBalancer([]int{1, 2, 3, 4})},
		{"Adaptive", NewAdaptiveBalancer()},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			scheduler, workers, _, _ := createPerformanceScheduler(4, 400)
			scheduler.SetLoadBalancer(tc.balancer)

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
				b.Fatalf("Failed to start scheduler: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				task := NewPerformanceTask(i, pool.PriorityNormal, 0)
				scheduler.Schedule(task)
			}
		})
	}
}

// BenchmarkWorkStealing 测试工作窃取性能
func BenchmarkWorkStealing(b *testing.B) {
	scheduler, workers, _, _ := createPerformanceScheduler(8, 1000)
	defer func() {
		scheduler.Stop()
		for _, worker := range workers {
			worker.Stop()
		}
	}()

	// 启用工作窃取
	scheduler.EnableWorkStealing(2, time.Millisecond*10)

	// 启动工作协程
	var wg sync.WaitGroup
	for _, worker := range workers {
		worker.Start(&wg)
	}

	err := scheduler.Start()
	if err != nil {
		b.Fatalf("Failed to start scheduler: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		taskID := 0
		for pb.Next() {
			// 创建不同工作负载的任务来触发工作窃取
			workload := time.Duration(taskID%10) * time.Microsecond
			task := NewPerformanceTask(taskID, pool.PriorityNormal, workload)
			scheduler.Schedule(task)
			taskID++
		}
	})
}

// BenchmarkSchedulerScalability 测试调度器可扩展性
func BenchmarkSchedulerScalability(b *testing.B) {
	workerCounts := []int{1, 2, 4, 8, 16}

	for _, workerCount := range workerCounts {
		b.Run(fmt.Sprintf("Workers%d", workerCount), func(b *testing.B) {
			scheduler, workers, _, _ := createPerformanceScheduler(workerCount, workerCount*100)
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
				b.Fatalf("Failed to start scheduler: %v", err)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				taskID := 0
				for pb.Next() {
					task := NewPerformanceTask(taskID, pool.PriorityNormal, 0)
					scheduler.Schedule(task)
					taskID++
				}
			})
		})
	}
}

// TestSchedulerStressTest 压力测试
func TestSchedulerStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	scheduler, workers, monitor, _ := createPerformanceScheduler(8, 2000)
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

	// 压力测试参数
	const (
		numGoroutines     = 10
		tasksPerGoroutine = 100
		totalTasks        = numGoroutines * tasksPerGoroutine
	)

	var submitWg sync.WaitGroup
	var successCount int64

	startTime := time.Now()

	// 启动多个goroutine并发提交任务
	for i := 0; i < numGoroutines; i++ {
		submitWg.Add(1)
		go func(goroutineID int) {
			defer submitWg.Done()

			for j := 0; j < tasksPerGoroutine; j++ {
				taskID := goroutineID*tasksPerGoroutine + j
				priority := []int{pool.PriorityLow, pool.PriorityNormal, pool.PriorityHigh, pool.PriorityCritical}[taskID%4]
				task := NewPerformanceTask(taskID, priority, time.Microsecond*time.Duration(taskID%100))

				err := scheduler.Schedule(task)
				if err == nil {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	submitWg.Wait()
	submitTime := time.Since(startTime)

	// 等待所有任务执行完成
	time.Sleep(time.Second * 2)

	stats := scheduler.GetStats()

	t.Logf("Stress Test Results:")
	t.Logf("  Total tasks: %d", totalTasks)
	t.Logf("  Successfully submitted: %d", successCount)
	t.Logf("  Submit time: %v", submitTime)
	t.Logf("  Submit rate: %.2f tasks/sec", float64(successCount)/submitTime.Seconds())
	t.Logf("  Scheduler stats: %+v", stats)
	t.Logf("  Monitor submitted: %d", atomic.LoadInt64(&monitor.submitCount))
	t.Logf("  Monitor completed: %d", atomic.LoadInt64(&monitor.completeCount))

	// 验证基本指标
	if successCount < int64(totalTasks*0.8) {
		t.Errorf("Expected at least 80%% tasks to be submitted successfully, got %.2f%%",
			float64(successCount)/float64(totalTasks)*100)
	}

	if stats.TotalSubmitted != successCount {
		t.Errorf("Expected scheduler submitted count %d to match success count %d",
			stats.TotalSubmitted, successCount)
	}
}

// TestSchedulerLongRunning 长时间运行测试
func TestSchedulerLongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long running test in short mode")
	}

	scheduler, workers, monitor, _ := createPerformanceScheduler(4, 1000)
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

	// 长时间运行测试（30秒）
	duration := time.Second * 30
	endTime := time.Now().Add(duration)

	var taskID int64
	var submitCount int64

	t.Logf("Starting long running test for %v", duration)

	for time.Now().Before(endTime) {
		id := atomic.AddInt64(&taskID, 1)
		task := NewPerformanceTask(int(id), pool.PriorityNormal, time.Microsecond*10)

		err := scheduler.Schedule(task)
		if err == nil {
			atomic.AddInt64(&submitCount, 1)
		}

		// 控制提交速率
		time.Sleep(time.Microsecond * 100)
	}

	// 等待剩余任务完成
	time.Sleep(time.Second * 2)

	finalStats := scheduler.GetStats()

	t.Logf("Long Running Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Tasks submitted: %d", submitCount)
	t.Logf("  Average rate: %.2f tasks/sec", float64(submitCount)/duration.Seconds())
	t.Logf("  Final stats: %+v", finalStats)
	t.Logf("  Monitor completed: %d", atomic.LoadInt64(&monitor.completeCount))

	// 验证系统稳定性
	if finalStats.TotalSubmitted != submitCount {
		t.Errorf("Expected scheduler submitted count %d to match actual %d",
			finalStats.TotalSubmitted, submitCount)
	}

	// 验证没有内存泄漏（简单检查）
	if finalStats.QueueSize > 100 {
		t.Errorf("Queue size seems too large after test: %d", finalStats.QueueSize)
	}
}
