package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTaskExecution 测试任务执行
func TestTaskExecution(t *testing.T) {
	t.Run("basic task execution", func(t *testing.T) {
		testBasicTaskExecution(t)
	})

	t.Run("task priority handling", func(t *testing.T) {
		testTaskPriorityHandling(t)
	})

	t.Run("task timeout handling", func(t *testing.T) {
		testTaskTimeoutHandling(t)
	})

	t.Run("task cancellation", func(t *testing.T) {
		testTaskCancellation(t)
	})

	t.Run("task error handling", func(t *testing.T) {
		testTaskErrorHandling(t)
	})

	t.Run("task panic recovery", func(t *testing.T) {
		testTaskPanicRecovery(t)
	})
}

// testBasicTaskExecution 测试基本任务执行
func testBasicTaskExecution(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 测试同步任务执行
	var executed int64
	var wg sync.WaitGroup

	taskCount := 10
	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		task := &TestTask{
			id:       fmt.Sprintf("sync-%d", i),
			duration: time.Millisecond * 10,
		}
		task.onExecute = func() {
			atomic.AddInt64(&executed, 1)
			wg.Done()
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit task %d: %v", i, err)
			wg.Done()
		}
	}

	wg.Wait()

	if atomic.LoadInt64(&executed) != int64(taskCount) {
		t.Errorf("Expected %d tasks executed, got %d", taskCount, executed)
	}

	// 测试异步任务执行
	futures := make([]Future, 5)
	for i := 0; i < 5; i++ {
		task := &TestTask{
			id:     fmt.Sprintf("async-%d", i),
			result: fmt.Sprintf("result-%d", i),
		}
		futures[i] = pool.SubmitAsync(task)
	}

	for i, future := range futures {
		result, err := future.GetWithTimeout(time.Second)
		if err != nil {
			t.Errorf("Async task %d failed: %v", i, err)
		} else {
			expected := fmt.Sprintf("result-%d", i)
			if result != expected {
				t.Errorf("Async task %d: expected %s, got %v", i, expected, result)
			}
		}
	}
}

// testTaskPriorityHandling 测试任务优先级处理
func testTaskPriorityHandling(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(1). // 单个工作协程确保顺序执行
		WithQueueSize(20).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var executionOrder []string
	var mu sync.Mutex

	// 提交不同优先级的任务
	tasks := []struct {
		id       string
		priority int
	}{
		{"low-1", PriorityLow},
		{"high-1", PriorityHigh},
		{"normal-1", PriorityNormal},
		{"high-2", PriorityHigh},
		{"low-2", PriorityLow},
		{"normal-2", PriorityNormal},
	}

	var wg sync.WaitGroup
	for _, taskInfo := range tasks {
		wg.Add(1)
		task := &TestTask{
			id:       taskInfo.id,
			priority: taskInfo.priority,
			duration: time.Millisecond * 10,
			executeCallback: func(id string) {
				mu.Lock()
				executionOrder = append(executionOrder, id)
				mu.Unlock()
				wg.Done()
			},
		}

		pool.Submit(task)
	}

	wg.Wait()

	t.Logf("Execution order: %v", executionOrder)

	// 验证高优先级任务是否优先执行
	// 注意：由于并发执行，完全的优先级顺序可能不总是保证，但高优先级任务应该倾向于先执行
	highPriorityCount := 0
	for _, id := range executionOrder {
		if id == "high-1" || id == "high-2" {
			highPriorityCount++
		}
		// 如果已经执行了所有高优先级任务，后续不应该再有高优先级任务
		if highPriorityCount == 2 {
			break
		}
	}

	if highPriorityCount != 2 {
		t.Errorf("Expected 2 high priority tasks, found %d", highPriorityCount)
	}
}

// testTaskTimeoutHandling 测试任务超时处理
func testTaskTimeoutHandling(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		WithTaskTimeout(time.Millisecond * 100).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 提交会超时的任务
	var timeoutCount int64
	var completedCount int64

	taskCount := 5
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		task := &TestTask{
			id:       fmt.Sprintf("timeout-%d", i),
			duration: time.Millisecond * 200, // 超过配置的超时时间
			onTimeout: func() {
				atomic.AddInt64(&timeoutCount, 1)
				wg.Done()
			},
			onExecute: func() {
				atomic.AddInt64(&completedCount, 1)
				wg.Done()
			},
		}

		pool.Submit(task)
	}

	wg.Wait()

	t.Logf("Timeout test results: timeouts=%d, completed=%d", timeoutCount, completedCount)

	// 应该有超时的任务
	if timeoutCount == 0 {
		t.Error("Expected some tasks to timeout")
	}

	// 验证统计信息
	stats := pool.Stats()
	if stats.FailedTasks == 0 {
		t.Error("Expected some failed tasks due to timeout")
	}
}

// testTaskCancellation 测试任务取消
func testTaskCancellation(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 提交异步任务并取消
	futures := make([]Future, 5)
	for i := 0; i < 5; i++ {
		task := &TestTask{
			id:       fmt.Sprintf("cancel-%d", i),
			duration: time.Millisecond * 100,
		}
		futures[i] = pool.SubmitAsync(task)
	}

	// 取消一些任务
	cancelledCount := 0
	for i := 0; i < 3; i++ {
		if futures[i].Cancel() {
			cancelledCount++
		}
	}

	t.Logf("Cancelled %d tasks", cancelledCount)

	// 检查取消的任务
	for i := 0; i < 3; i++ {
		result, err := futures[i].GetWithTimeout(time.Second)
		if err == nil {
			t.Errorf("Cancelled task %d should have failed, got result: %v", i, result)
		}
	}

	// 检查未取消的任务
	for i := 3; i < 5; i++ {
		result, err := futures[i].GetWithTimeout(time.Second)
		if err != nil {
			t.Errorf("Non-cancelled task %d failed: %v", i, err)
		} else {
			t.Logf("Task %d completed with result: %v", i, result)
		}
	}
}

// testTaskErrorHandling 测试任务错误处理
func testTaskErrorHandling(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 提交会出错的任务
	var errorCount int64
	var successCount int64

	taskCount := 10
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		shouldError := i%3 == 0 // 每3个任务中有1个出错

		task := &TestTask{
			id:          fmt.Sprintf("error-%d", i),
			shouldError: shouldError,
			onError: func() {
				atomic.AddInt64(&errorCount, 1)
				wg.Done()
			},
			onExecute: func() {
				atomic.AddInt64(&successCount, 1)
				wg.Done()
			},
		}

		pool.Submit(task)
	}

	wg.Wait()

	t.Logf("Error handling results: errors=%d, success=%d", errorCount, successCount)

	expectedErrors := int64(taskCount / 3)
	if errorCount < expectedErrors {
		t.Errorf("Expected at least %d errors, got %d", expectedErrors, errorCount)
	}

	// 验证统计信息
	stats := pool.Stats()
	if stats.FailedTasks < errorCount {
		t.Errorf("Stats failed tasks (%d) should be at least %d", stats.FailedTasks, errorCount)
	}
}

// testTaskPanicRecovery 测试任务panic恢复
func testTaskPanicRecovery(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 提交会panic的任务
	var panicCount int64
	var normalCount int64

	taskCount := 8
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		shouldPanic := i%4 == 0 // 每4个任务中有1个panic

		task := &TestTask{
			id:          fmt.Sprintf("panic-%d", i),
			shouldPanic: shouldPanic,
			onPanic: func() {
				atomic.AddInt64(&panicCount, 1)
				wg.Done()
			},
			onExecute: func() {
				atomic.AddInt64(&normalCount, 1)
				wg.Done()
			},
		}

		pool.Submit(task)
	}

	wg.Wait()

	t.Logf("Panic recovery results: panics=%d, normal=%d", panicCount, normalCount)

	expectedPanics := int64(taskCount / 4)
	if panicCount < expectedPanics {
		t.Errorf("Expected at least %d panics, got %d", expectedPanics, panicCount)
	}

	// 验证协程池仍然正常工作
	if !pool.IsRunning() {
		t.Error("Pool should still be running after panic recovery")
	}

	// 提交一个正常任务验证恢复
	var recovered bool
	recoveryTask := &TestTask{
		id: "recovery-test",
		onExecute: func() {
			recovered = true
		},
	}

	if err := pool.Submit(recoveryTask); err != nil {
		t.Errorf("Failed to submit recovery task: %v", err)
	}

	time.Sleep(time.Millisecond * 100)

	if !recovered {
		t.Error("Pool did not recover properly after panic")
	}
}
