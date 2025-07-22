package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPoolIntegration 测试协程池核心功能集成
func TestPoolIntegration(t *testing.T) {
	t.Run("basic pool operations", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(4).
			WithQueueSize(100).
			WithTaskTimeout(time.Second * 5).
			WithShutdownTimeout(time.Second * 3).
			WithMetrics(true).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}

		// 验证初始状态
		if !pool.IsRunning() {
			t.Error("Pool should be running after creation")
		}

		// 提交一些任务
		taskCount := 10
		var completedCount int64
		var wg sync.WaitGroup

		for i := 0; i < taskCount; i++ {
			wg.Add(1)
			task := &IntegrationTask{
				id:       i,
				duration: time.Millisecond * 50,
				onComplete: func() {
					atomic.AddInt64(&completedCount, 1)
					wg.Done()
				},
			}

			err := pool.Submit(task)
			if err != nil {
				t.Errorf("Failed to submit task %d: %v", i, err)
				wg.Done()
			}
		}

		// 等待所有任务完成
		wg.Wait()

		// 验证任务完成情况
		if atomic.LoadInt64(&completedCount) != int64(taskCount) {
			t.Errorf("Expected %d completed tasks, got %d", taskCount, completedCount)
		}

		// 获取统计信息
		stats := pool.Stats()
		if stats.CompletedTasks < int64(taskCount) {
			t.Errorf("Expected at least %d completed tasks in stats, got %d", taskCount, stats.CompletedTasks)
		}

		// 关闭协程池
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		err = pool.Shutdown(ctx)
		if err != nil {
			t.Errorf("Failed to shutdown pool: %v", err)
		}

		if !pool.IsClosed() {
			t.Error("Pool should be closed after shutdown")
		}
	})

	t.Run("async task execution", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(2).
			WithQueueSize(50).
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

		// 提交异步任务
		futures := make([]Future, 5)
		for i := 0; i < 5; i++ {
			task := &IntegrationTask{
				id:       i,
				duration: time.Millisecond * 100,
				result:   fmt.Sprintf("result-%d", i),
			}

			futures[i] = pool.SubmitAsync(task)
		}

		// 获取所有结果
		for i, future := range futures {
			result, err := future.GetWithTimeout(time.Second * 2)
			if err != nil {
				t.Errorf("Failed to get result for task %d: %v", i, err)
				continue
			}

			expected := fmt.Sprintf("result-%d", i)
			if result != expected {
				t.Errorf("Expected result %s, got %v", expected, result)
			}
		}
	})

	t.Run("timeout handling", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(1).
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

		// 提交一个会超时的任务
		task := &IntegrationTask{
			id:       1,
			duration: time.Millisecond * 500, // 超过配置的超时时间
		}

		future := pool.SubmitAsync(task)
		result, err := future.GetWithTimeout(time.Second)

		// 应该收到超时错误
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}

		if result != nil {
			t.Errorf("Expected nil result on timeout, got %v", result)
		}
	})

	t.Run("concurrent submissions", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(8).
			WithQueueSize(200).
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

		// 并发提交任务
		goroutineCount := 10
		tasksPerGoroutine := 20
		totalTasks := goroutineCount * tasksPerGoroutine

		var completedCount int64
		var wg sync.WaitGroup

		for g := 0; g < goroutineCount; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for i := 0; i < tasksPerGoroutine; i++ {
					task := &IntegrationTask{
						id:       goroutineID*tasksPerGoroutine + i,
						duration: time.Millisecond * 10,
						onComplete: func() {
							atomic.AddInt64(&completedCount, 1)
						},
					}

					err := pool.Submit(task)
					if err != nil {
						t.Errorf("Failed to submit task: %v", err)
					}
				}
			}(g)
		}

		wg.Wait()

		// 等待所有任务完成
		timeout := time.After(time.Second * 10)
		ticker := time.NewTicker(time.Millisecond * 100)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				t.Fatalf("Timeout waiting for tasks to complete. Completed: %d, Expected: %d",
					atomic.LoadInt64(&completedCount), totalTasks)
			case <-ticker.C:
				if atomic.LoadInt64(&completedCount) >= int64(totalTasks) {
					goto completed
				}
			}
		}

	completed:
		// 验证统计信息
		stats := pool.Stats()
		if stats.CompletedTasks < int64(totalTasks) {
			t.Errorf("Expected at least %d completed tasks, got %d", totalTasks, stats.CompletedTasks)
		}
	})

	t.Run("error handling", func(t *testing.T) {
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
		errorTask := &IntegrationTask{
			id:    1,
			error: fmt.Errorf("test error"),
		}

		future := pool.SubmitAsync(errorTask)
		result, err := future.GetWithTimeout(time.Second)

		if err == nil {
			t.Error("Expected error, got nil")
		}

		if result != nil {
			t.Errorf("Expected nil result on error, got %v", result)
		}

		// 提交会panic的任务
		panicTask := &IntegrationTask{
			id:          2,
			shouldPanic: true,
		}

		future2 := pool.SubmitAsync(panicTask)
		result2, err2 := future2.GetWithTimeout(time.Second)

		if err2 == nil {
			t.Error("Expected panic error, got nil")
		}

		if result2 != nil {
			t.Errorf("Expected nil result on panic, got %v", result2)
		}
	})
}

// IntegrationTask 集成测试任务
type IntegrationTask struct {
	id          int
	duration    time.Duration
	result      any
	error       error
	shouldPanic bool
	onComplete  func()
}

func (t *IntegrationTask) Execute(ctx context.Context) (any, error) {
	if t.shouldPanic {
		panic("test panic")
	}

	if t.error != nil {
		return nil, t.error
	}

	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	if t.result != nil {
		return t.result, nil
	}

	return fmt.Sprintf("task-%d-completed", t.id), nil
}

func (t *IntegrationTask) Priority() int {
	return PriorityNormal
}

// TestPoolLifecycle 测试协程池生命周期管理
func TestPoolLifecycle(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		WithShutdownTimeout(time.Second * 2).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// 验证初始状态
	if !pool.IsRunning() {
		t.Error("Pool should be running")
	}
	if pool.IsShutdown() {
		t.Error("Pool should not be shutdown")
	}
	if pool.IsClosed() {
		t.Error("Pool should not be closed")
	}

	// 提交一些任务
	for i := 0; i < 5; i++ {
		task := &IntegrationTask{
			id:       i,
			duration: time.Millisecond * 100,
		}
		pool.Submit(task)
	}

	// 开始关闭
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err = pool.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// 验证关闭后状态
	if pool.IsRunning() {
		t.Error("Pool should not be running after shutdown")
	}
	if !pool.IsClosed() {
		t.Error("Pool should be closed after shutdown")
	}

	// 关闭后提交任务应该失败
	task := &IntegrationTask{id: 999}
	err = pool.Submit(task)
	if err == nil {
		t.Error("Submit should fail after shutdown")
	}
}

// TestPoolStatsIntegration 测试统计信息功能
func TestPoolStatsIntegration(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(3).
		WithQueueSize(20).
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 获取初始统计信息
	initialStats := pool.Stats()

	// 提交一些任务
	taskCount := 10
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		task := &IntegrationTask{
			id:       i,
			duration: time.Millisecond * 50,
			onComplete: func() {
				wg.Done()
			},
		}

		pool.Submit(task)
	}

	// 等待任务完成
	wg.Wait()

	// 等待统计信息更新
	time.Sleep(time.Millisecond * 200)

	// 获取最终统计信息
	finalStats := pool.Stats()

	// 验证统计信息
	if finalStats.CompletedTasks <= initialStats.CompletedTasks {
		t.Errorf("CompletedTasks should increase, initial: %d, final: %d",
			initialStats.CompletedTasks, finalStats.CompletedTasks)
	}

	if finalStats.CompletedTasks < int64(taskCount) {
		t.Errorf("Expected at least %d completed tasks, got %d", taskCount, finalStats.CompletedTasks)
	}

	// 内存使用量应该大于0
	if finalStats.MemoryUsage <= 0 {
		t.Error("MemoryUsage should be positive")
	}

	// 平均任务执行时间应该大于0（如果有任务完成）
	if finalStats.CompletedTasks > 0 && finalStats.AvgTaskDuration <= 0 {
		t.Error("AvgTaskDuration should be positive when tasks are completed")
	}
}
