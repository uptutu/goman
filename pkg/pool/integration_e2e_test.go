package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEndToEndScenarios 端到端场景测试
func TestEndToEndScenarios(t *testing.T) {
	t.Run("complete workflow", func(t *testing.T) {
		testCompleteWorkflow(t)
	})

	t.Run("high concurrency scenario", func(t *testing.T) {
		testHighConcurrencyScenario(t)
	})

	t.Run("mixed workload scenario", func(t *testing.T) {
		testMixedWorkloadScenario(t)
	})

	t.Run("resource exhaustion scenario", func(t *testing.T) {
		testResourceExhaustionScenario(t)
	})

	t.Run("graceful degradation scenario", func(t *testing.T) {
		testGracefulDegradationScenario(t)
	})
}

// testCompleteWorkflow 测试完整的工作流程
func testCompleteWorkflow(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
		WithTaskTimeout(time.Second * 2).
		WithShutdownTimeout(time.Second * 5).
		WithMetrics(true).
		WithMetricsInterval(time.Millisecond * 100).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// 阶段1: 验证初始状态
	if !pool.IsRunning() {
		t.Error("Pool should be running after creation")
	}

	initialStats := pool.Stats()
	if initialStats.ActiveWorkers != 0 {
		t.Errorf("Expected 0 active workers initially, got %d", initialStats.ActiveWorkers)
	}

	// 阶段2: 提交不同类型的任务
	taskCount := 50
	var completedCount int64
	var errorCount int64
	var wg sync.WaitGroup

	// 快速任务
	for i := 0; i < taskCount/2; i++ {
		wg.Add(1)
		task := &E2ETask{
			id:       fmt.Sprintf("fast-%d", i),
			duration: time.Millisecond * 10,
			onComplete: func() {
				atomic.AddInt64(&completedCount, 1)
				wg.Done()
			},
		}
		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit fast task %d: %v", i, err)
			wg.Done()
		}
	}

	// 慢速任务
	for i := 0; i < taskCount/2; i++ {
		wg.Add(1)
		task := &E2ETask{
			id:       fmt.Sprintf("slow-%d", i),
			duration: time.Millisecond * 100,
			onComplete: func() {
				atomic.AddInt64(&completedCount, 1)
				wg.Done()
			},
		}
		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit slow task %d: %v", i, err)
			wg.Done()
		}
	}

	// 阶段3: 等待任务完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 任务完成
	case <-time.After(time.Second * 10):
		t.Fatal("Timeout waiting for tasks to complete")
	}

	// 阶段4: 验证执行结果
	if atomic.LoadInt64(&completedCount) != int64(taskCount) {
		t.Errorf("Expected %d completed tasks, got %d", taskCount, completedCount)
	}

	// 等待统计信息更新
	time.Sleep(time.Millisecond * 200)

	finalStats := pool.Stats()
	if finalStats.CompletedTasks < int64(taskCount) {
		t.Errorf("Expected at least %d completed tasks in stats, got %d", taskCount, finalStats.CompletedTasks)
	}

	if finalStats.FailedTasks != errorCount {
		t.Errorf("Expected %d failed tasks, got %d", errorCount, finalStats.FailedTasks)
	}

	// 阶段5: 优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown pool: %v", err)
	}

	if !pool.IsClosed() {
		t.Error("Pool should be closed after shutdown")
	}

	// 阶段6: 验证关闭后状态
	task := &E2ETask{id: "after-shutdown"}
	if err := pool.Submit(task); err == nil {
		t.Error("Submit should fail after shutdown")
	}
}

// testHighConcurrencyScenario 测试高并发场景
func testHighConcurrencyScenario(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU() * 2).
		WithQueueSize(1000).
		WithTaskTimeout(time.Second * 5).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 高并发提交任务
	goroutineCount := 50
	tasksPerGoroutine := 100
	totalTasks := goroutineCount * tasksPerGoroutine

	var completedCount int64
	var errorCount int64
	var submitErrors int64
	var wg sync.WaitGroup

	startTime := time.Now()

	for g := 0; g < goroutineCount; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < tasksPerGoroutine; i++ {
				task := &E2ETask{
					id:       fmt.Sprintf("g%d-t%d", goroutineID, i),
					duration: time.Millisecond * time.Duration(10+i%50), // 变化的执行时间
					onComplete: func() {
						atomic.AddInt64(&completedCount, 1)
					},
					onError: func() {
						atomic.AddInt64(&errorCount, 1)
					},
				}

				if err := pool.Submit(task); err != nil {
					atomic.AddInt64(&submitErrors, 1)
				}
			}
		}(g)
	}

	wg.Wait()
	submitDuration := time.Since(startTime)

	// 等待所有任务完成
	timeout := time.After(time.Second * 30)
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for tasks to complete. Completed: %d, Expected: %d",
				atomic.LoadInt64(&completedCount), totalTasks)
		case <-ticker.C:
			completed := atomic.LoadInt64(&completedCount)
			errors := atomic.LoadInt64(&errorCount)
			if completed+errors >= int64(totalTasks-int(submitErrors)) {
				goto completed
			}
		}
	}

completed:
	executionDuration := time.Since(startTime)

	// 验证性能指标
	stats := pool.Stats()
	t.Logf("High concurrency test results:")
	t.Logf("  Total tasks: %d", totalTasks)
	t.Logf("  Submit errors: %d", submitErrors)
	t.Logf("  Completed tasks: %d", completedCount)
	t.Logf("  Failed tasks: %d", errorCount)
	t.Logf("  Submit duration: %v", submitDuration)
	t.Logf("  Execution duration: %v", executionDuration)
	t.Logf("  Throughput: %.2f tasks/sec", float64(completedCount)/executionDuration.Seconds())
	t.Logf("  Stats: %+v", stats)

	// 基本验证
	if completedCount == 0 {
		t.Error("No tasks were completed")
	}

	if stats.CompletedTasks < completedCount {
		t.Errorf("Stats completed tasks (%d) less than actual completed (%d)", stats.CompletedTasks, completedCount)
	}
}

// testMixedWorkloadScenario 测试混合工作负载场景
func testMixedWorkloadScenario(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(6).
		WithQueueSize(200).
		WithTaskTimeout(time.Second * 3).
		WithMetrics(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var results struct {
		sync.Mutex
		fastCompleted  int64
		slowCompleted  int64
		errorCompleted int64
		panicCompleted int64
		asyncCompleted int64
	}

	// 快速任务 (50个)
	for i := 0; i < 50; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("fast-%d", i),
			duration: time.Millisecond * 5,
			onComplete: func() {
				atomic.AddInt64(&results.fastCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 慢速任务 (20个)
	for i := 0; i < 20; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("slow-%d", i),
			duration: time.Millisecond * 200,
			onComplete: func() {
				atomic.AddInt64(&results.slowCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 错误任务 (10个)
	for i := 0; i < 10; i++ {
		task := &E2ETask{
			id:    fmt.Sprintf("error-%d", i),
			error: fmt.Errorf("test error %d", i),
			onError: func() {
				atomic.AddInt64(&results.errorCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// Panic任务 (5个)
	for i := 0; i < 5; i++ {
		task := &E2ETask{
			id:          fmt.Sprintf("panic-%d", i),
			shouldPanic: true,
			onError: func() {
				atomic.AddInt64(&results.panicCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 异步任务 (15个)
	futures := make([]Future, 15)
	for i := 0; i < 15; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("async-%d", i),
			duration: time.Millisecond * 50,
			result:   fmt.Sprintf("async-result-%d", i),
		}
		futures[i] = pool.SubmitAsync(task)
	}

	// 等待异步任务完成
	for i, future := range futures {
		result, err := future.GetWithTimeout(time.Second * 5)
		if err != nil {
			t.Errorf("Async task %d failed: %v", i, err)
		} else {
			expected := fmt.Sprintf("async-result-%d", i)
			if result != expected {
				t.Errorf("Async task %d: expected %s, got %v", i, expected, result)
			} else {
				atomic.AddInt64(&results.asyncCompleted, 1)
			}
		}
	}

	// 等待所有任务完成
	timeout := time.After(time.Second * 10)
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	expectedTotal := int64(50 + 20 + 10 + 5 + 15)
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for mixed workload to complete")
		case <-ticker.C:
			total := atomic.LoadInt64(&results.fastCompleted) +
				atomic.LoadInt64(&results.slowCompleted) +
				atomic.LoadInt64(&results.errorCompleted) +
				atomic.LoadInt64(&results.panicCompleted) +
				atomic.LoadInt64(&results.asyncCompleted)

			if total >= expectedTotal {
				goto completed
			}
		}
	}

completed:
	// 验证结果
	t.Logf("Mixed workload results:")
	t.Logf("  Fast tasks: %d/50", results.fastCompleted)
	t.Logf("  Slow tasks: %d/20", results.slowCompleted)
	t.Logf("  Error tasks: %d/10", results.errorCompleted)
	t.Logf("  Panic tasks: %d/5", results.panicCompleted)
	t.Logf("  Async tasks: %d/15", results.asyncCompleted)

	stats := pool.Stats()
	t.Logf("  Pool stats: %+v", stats)

	// 基本验证
	if results.fastCompleted != 50 {
		t.Errorf("Expected 50 fast tasks completed, got %d", results.fastCompleted)
	}
	if results.slowCompleted != 20 {
		t.Errorf("Expected 20 slow tasks completed, got %d", results.slowCompleted)
	}
	if results.asyncCompleted != 15 {
		t.Errorf("Expected 15 async tasks completed, got %d", results.asyncCompleted)
	}
}

// testResourceExhaustionScenario 测试资源耗尽场景
func testResourceExhaustionScenario(t *testing.T) {
	// 创建小容量的协程池
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		WithTaskTimeout(time.Millisecond * 500).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 提交大量长时间运行的任务，耗尽队列
	longRunningTasks := 5
	var longTasksStarted int64
	var longTasksCompleted int64

	for i := 0; i < longRunningTasks; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("long-%d", i),
			duration: time.Millisecond * 300,
			onStart: func() {
				atomic.AddInt64(&longTasksStarted, 1)
			},
			onComplete: func() {
				atomic.AddInt64(&longTasksCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 等待长任务开始执行
	time.Sleep(time.Millisecond * 50)

	// 尝试提交更多任务，应该会因为队列满而失败
	var queueFullErrors int64
	var successfulSubmits int64

	for i := 0; i < 20; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("overflow-%d", i),
			duration: time.Millisecond * 10,
		}

		if err := pool.Submit(task); err != nil {
			if IsQueueFullError(err) {
				atomic.AddInt64(&queueFullErrors, 1)
			}
		} else {
			atomic.AddInt64(&successfulSubmits, 1)
		}
	}

	// 等待任务完成
	time.Sleep(time.Second * 2)

	t.Logf("Resource exhaustion results:")
	t.Logf("  Long tasks started: %d", longTasksStarted)
	t.Logf("  Long tasks completed: %d", longTasksCompleted)
	t.Logf("  Queue full errors: %d", queueFullErrors)
	t.Logf("  Successful submits: %d", successfulSubmits)

	// 验证队列满错误
	if queueFullErrors == 0 {
		t.Error("Expected some queue full errors in resource exhaustion scenario")
	}

	stats := pool.Stats()
	if stats.FailedTasks == 0 {
		t.Error("Expected some failed tasks in resource exhaustion scenario")
	}
}

// testGracefulDegradationScenario 测试优雅降级场景
func testGracefulDegradationScenario(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(50).
		WithTaskTimeout(time.Millisecond * 200).
		WithShutdownTimeout(time.Second * 3).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// 提交一些正常任务
	var normalCompleted int64
	for i := 0; i < 10; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("normal-%d", i),
			duration: time.Millisecond * 50,
			onComplete: func() {
				atomic.AddInt64(&normalCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 提交一些会超时的任务
	var timeoutErrors int64
	for i := 0; i < 5; i++ {
		task := &E2ETask{
			id:       fmt.Sprintf("timeout-%d", i),
			duration: time.Millisecond * 500, // 超过配置的超时时间
			onError: func() {
				atomic.AddInt64(&timeoutErrors, 1)
			},
		}
		pool.Submit(task)
	}

	// 等待一段时间让任务开始执行
	time.Sleep(time.Millisecond * 100)

	// 开始优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	shutdownStart := time.Now()
	err = pool.Shutdown(shutdownCtx)
	shutdownDuration := time.Since(shutdownStart)

	if err != nil {
		t.Logf("Shutdown completed with error (expected): %v", err)
	}

	t.Logf("Graceful degradation results:")
	t.Logf("  Normal tasks completed: %d/10", normalCompleted)
	t.Logf("  Timeout errors: %d", timeoutErrors)
	t.Logf("  Shutdown duration: %v", shutdownDuration)
	t.Logf("  Pool closed: %v", pool.IsClosed())

	// 验证优雅关闭
	if !pool.IsClosed() {
		t.Error("Pool should be closed after shutdown")
	}

	// 验证关闭后的行为
	task := &E2ETask{id: "after-shutdown"}
	if err := pool.Submit(task); err == nil {
		t.Error("Submit should fail after shutdown")
	}
}

// E2ETask 端到端测试任务
type E2ETask struct {
	id          string
	duration    time.Duration
	result      any
	error       error
	shouldPanic bool
	onStart     func()
	onComplete  func()
	onError     func()
}

func (t *E2ETask) Execute(ctx context.Context) (any, error) {
	if t.onStart != nil {
		t.onStart()
	}

	if t.shouldPanic {
		panic(fmt.Sprintf("test panic from task %s", t.id))
	}

	if t.error != nil {
		if t.onError != nil {
			t.onError()
		}
		return nil, t.error
	}

	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			if t.onError != nil {
				t.onError()
			}
			return nil, ctx.Err()
		}
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	if t.result != nil {
		return t.result, nil
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *E2ETask) Priority() int {
	return PriorityNormal
}

// IsQueueFullError 检查是否是队列满错误
func IsQueueFullError(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrQueueFull ||
		(err.Error() != "" && (err.Error() == "queue is full" ||
			err.Error() == "task queue is full"))
}
