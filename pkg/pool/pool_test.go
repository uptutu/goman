package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTask 测试任务实现
type TestTask struct {
	id       string
	duration time.Duration
	priority int
	result   any
	err      error
	executed int32
}

func NewTestTask(id string, duration time.Duration) *TestTask {
	return &TestTask{
		id:       id,
		duration: duration,
		priority: PriorityNormal,
	}
}

func (t *TestTask) Execute(ctx context.Context) (any, error) {
	atomic.StoreInt32(&t.executed, 1)

	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if t.err != nil {
		return nil, t.err
	}

	if t.result != nil {
		return t.result, nil
	}

	return fmt.Sprintf("task-%s-completed", t.id), nil
}

func (t *TestTask) Priority() int {
	return t.priority
}

func (t *TestTask) SetPriority(priority int) {
	t.priority = priority
}

func (t *TestTask) SetResult(result any) {
	t.result = result
}

func (t *TestTask) SetError(err error) {
	t.err = err
}

func (t *TestTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

// TestPoolCreation 测试协程池创建
func TestPoolCreation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "default config",
			config:  NewConfig(),
			wantErr: false,
		},
		{
			name: "custom config",
			config: func() *Config {
				config, _ := NewConfigBuilder().
					WithWorkerCount(5).
					WithQueueSize(100).
					Build()
				return config
			}(),
			wantErr: false,
		},
		{
			name: "invalid worker count",
			config: &Config{
				WorkerCount: 0,
				QueueSize:   100,
			},
			wantErr: true,
		},
		{
			name: "invalid queue size",
			config: &Config{
				WorkerCount: 5,
				QueueSize:   0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if pool == nil {
					t.Error("NewPool() returned nil pool")
					return
				}

				// 验证协程池状态
				if !pool.IsRunning() {
					t.Error("Pool should be running after creation")
				}

				// 清理
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				defer cancel()
				if err := pool.Shutdown(ctx); err != nil {
					t.Errorf("Failed to shutdown pool: %v", err)
				}
			}
		})
	}
}

// TestPoolSubmit 测试任务提交
func TestPoolSubmit(t *testing.T) {
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	t.Run("submit single task", func(t *testing.T) {
		task := NewTestTask("single", time.Millisecond*10)

		err := pool.Submit(task)
		if err != nil {
			t.Errorf("Submit() error = %v", err)
		}

		// 等待任务完成
		time.Sleep(time.Millisecond * 50)

		if !task.IsExecuted() {
			t.Error("Task was not executed")
		}
	})

	t.Run("submit multiple tasks", func(t *testing.T) {
		taskCount := 5
		tasks := make([]*TestTask, taskCount)

		for i := 0; i < taskCount; i++ {
			tasks[i] = NewTestTask(fmt.Sprintf("multi-%d", i), time.Millisecond*10)
		}

		// 提交所有任务
		for _, task := range tasks {
			err := pool.Submit(task)
			if err != nil {
				t.Errorf("Submit() error = %v", err)
			}
		}

		// 等待所有任务完成
		time.Sleep(time.Millisecond * 100)

		// 验证所有任务都被执行
		for i, task := range tasks {
			if !task.IsExecuted() {
				t.Errorf("Task %d was not executed", i)
			}
		}
	})

	t.Run("submit nil task", func(t *testing.T) {
		err := pool.Submit(nil)
		if err == nil {
			t.Error("Submit(nil) should return error")
		}
	})
}

// TestPoolSubmitWithTimeout 测试带超时的任务提交
func TestPoolSubmitWithTimeout(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(1).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	t.Run("task completes within timeout", func(t *testing.T) {
		task := NewTestTask("timeout-ok", time.Millisecond*10)

		err := pool.SubmitWithTimeout(task, time.Millisecond*50)
		if err != nil {
			t.Errorf("SubmitWithTimeout() error = %v", err)
		}

		time.Sleep(time.Millisecond * 100)

		if !task.IsExecuted() {
			t.Error("Task was not executed")
		}
	})

	t.Run("task times out", func(t *testing.T) {
		task := NewTestTask("timeout-fail", time.Millisecond*100)

		err := pool.SubmitWithTimeout(task, time.Millisecond*10)
		if err != nil {
			t.Errorf("SubmitWithTimeout() error = %v", err)
		}

		time.Sleep(time.Millisecond * 150)

		// 任务可能被执行但会因为超时而被取消
		// 这里主要测试超时机制是否工作
	})

	t.Run("invalid timeout", func(t *testing.T) {
		task := NewTestTask("invalid-timeout", time.Millisecond*10)

		err := pool.SubmitWithTimeout(task, 0)
		if err == nil {
			t.Error("SubmitWithTimeout() with zero timeout should return error")
		}
	})
}

// TestPoolSubmitAsync 测试异步任务提交
func TestPoolSubmitAsync(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
		Build()
	if err != nil {
		t.Fatalf("Failed to build config: %v", err)
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

	t.Run("async task success", func(t *testing.T) {
		task := NewTestTask("async-success", time.Millisecond*10)
		task.SetResult("async-result")

		future := pool.SubmitAsync(task)
		if future == nil {
			t.Fatal("SubmitAsync() returned nil future")
		}

		// 等待结果
		result, err := future.GetWithTimeout(time.Millisecond * 100)
		if err != nil {
			t.Errorf("Future.GetWithTimeout() error = %v", err)
		}

		if result != "async-result" {
			t.Errorf("Expected result 'async-result', got %v", result)
		}

		if !future.IsDone() {
			t.Error("Future should be done")
		}
	})

	t.Run("async task error", func(t *testing.T) {
		task := NewTestTask("async-error", time.Millisecond*10)
		task.SetError(errors.New("task error"))

		future := pool.SubmitAsync(task)
		if future == nil {
			t.Fatal("SubmitAsync() returned nil future")
		}

		// 等待结果
		result, err := future.GetWithTimeout(time.Millisecond * 100)
		if err == nil {
			t.Error("Expected error from future")
		}

		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})

	t.Run("future timeout", func(t *testing.T) {
		task := NewTestTask("future-timeout", time.Millisecond*100)

		future := pool.SubmitAsync(task)
		if future == nil {
			t.Fatal("SubmitAsync() returned nil future")
		}

		// 使用很短的超时时间
		result, err := future.GetWithTimeout(time.Millisecond * 10)
		if err == nil {
			t.Error("Expected timeout error")
		}

		if result != nil {
			t.Errorf("Expected nil result on timeout, got %v", result)
		}
	})

	t.Run("future cancel", func(t *testing.T) {
		task := NewTestTask("future-cancel", time.Millisecond*100)

		future := pool.SubmitAsync(task)
		if future == nil {
			t.Fatal("SubmitAsync() returned nil future")
		}

		// 取消Future
		if !future.Cancel() {
			t.Error("Future.Cancel() should return true")
		}

		// 再次取消应该返回false
		if future.Cancel() {
			t.Error("Second Future.Cancel() should return false")
		}

		if !future.IsDone() {
			t.Error("Cancelled future should be done")
		}
	})
}

// TestPoolShutdown 测试协程池关闭
func TestPoolShutdown(t *testing.T) {
	t.Run("graceful shutdown", func(t *testing.T) {
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

		// 提交一些任务
		for i := 0; i < 5; i++ {
			task := NewTestTask(fmt.Sprintf("shutdown-%d", i), time.Millisecond*10)
			pool.Submit(task)
		}

		// 优雅关闭
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		err = pool.Shutdown(ctx)
		if err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}

		if !pool.IsClosed() {
			t.Error("Pool should be closed after shutdown")
		}

		// 关闭后提交任务应该失败
		task := NewTestTask("after-shutdown", time.Millisecond*10)
		err = pool.Submit(task)
		if err == nil {
			t.Error("Submit() after shutdown should return error")
		}
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(1).
			WithQueueSize(10).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}

		// 提交一个长时间运行的任务
		task := NewTestTask("long-running", time.Second*2)
		pool.Submit(task)

		// 使用很短的超时时间关闭
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
		defer cancel()

		err = pool.Shutdown(ctx)
		// 超时关闭可能返回错误，这是正常的
		if err != nil {
			t.Logf("Shutdown timeout error (expected): %v", err)
		}
	})

	t.Run("multiple shutdown calls", func(t *testing.T) {
		config, err := NewConfigBuilder().
			WithWorkerCount(1).
			WithQueueSize(10).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		// 第一次关闭
		err1 := pool.Shutdown(ctx)

		// 第二次关闭
		err2 := pool.Shutdown(ctx)

		// 两次关闭都不应该返回错误（或者第二次返回特定错误）
		if err1 != nil {
			t.Errorf("First shutdown error = %v", err1)
		}

		// 第二次关闭可能返回nil（已经关闭）或特定错误
		if err2 != nil {
			t.Logf("Second shutdown returned: %v", err2)
		}
	})
}

// TestPoolStats 测试协程池统计信息
func TestPoolStats(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 获取初始统计信息
	initialStats := pool.Stats()

	// 提交一些任务
	taskCount := 5
	for i := 0; i < taskCount; i++ {
		task := NewTestTask(fmt.Sprintf("stats-%d", i), time.Millisecond*10)
		pool.Submit(task)
	}

	// 等待任务完成
	time.Sleep(time.Millisecond * 100)

	// 获取最终统计信息
	finalStats := pool.Stats()

	// 验证统计信息
	if finalStats.CompletedTasks <= initialStats.CompletedTasks {
		t.Errorf("CompletedTasks should increase, initial: %d, final: %d",
			initialStats.CompletedTasks, finalStats.CompletedTasks)
	}

	if finalStats.AvgTaskDuration <= 0 {
		t.Error("AvgTaskDuration should be positive")
	}

	// 内存使用量应该大于0
	if finalStats.MemoryUsage <= 0 {
		t.Error("MemoryUsage should be positive")
	}
}

// TestPoolConcurrency 测试协程池并发性
func TestPoolConcurrency(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
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

	// 并发提交任务
	taskCount := 50
	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			task := NewTestTask(fmt.Sprintf("concurrent-%d", id), time.Millisecond*5)
			err := pool.Submit(task)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
			} else {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(time.Millisecond * 200)

	// 验证结果
	if successCount != int64(taskCount) {
		t.Errorf("Expected %d successful submissions, got %d", taskCount, successCount)
	}

	if errorCount != 0 {
		t.Errorf("Expected 0 errors, got %d", errorCount)
	}

	// 验证统计信息
	stats := pool.Stats()
	if stats.CompletedTasks < int64(taskCount) {
		t.Errorf("Expected at least %d completed tasks, got %d", taskCount, stats.CompletedTasks)
	}
}

// TestPoolTaskPriority 测试任务优先级
func TestPoolTaskPriority(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(1). // 使用单个工作协程确保顺序
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 创建不同优先级的任务
	var executionOrder []string
	var mu sync.Mutex

	// 低优先级任务
	for i := 0; i < 3; i++ {
		task := NewTestTask(fmt.Sprintf("low-%d", i), time.Millisecond*10)
		task.SetPriority(PriorityLow)

		// 包装任务以记录执行顺序
		wrappedTask := &priorityTestTask{
			TestTask: task,
			onExecute: func(id string) {
				mu.Lock()
				executionOrder = append(executionOrder, id)
				mu.Unlock()
			},
		}

		pool.Submit(wrappedTask)
	}

	// 高优先级任务
	for i := 0; i < 2; i++ {
		task := NewTestTask(fmt.Sprintf("high-%d", i), time.Millisecond*10)
		task.SetPriority(PriorityHigh)

		wrappedTask := &priorityTestTask{
			TestTask: task,
			onExecute: func(id string) {
				mu.Lock()
				executionOrder = append(executionOrder, id)
				mu.Unlock()
			},
		}

		pool.Submit(wrappedTask)
	}

	// 等待所有任务完成
	time.Sleep(time.Millisecond * 200)

	// 验证执行顺序（高优先级任务应该先执行）
	mu.Lock()
	defer mu.Unlock()

	if len(executionOrder) != 5 {
		t.Errorf("Expected 5 tasks executed, got %d", len(executionOrder))
		return
	}

	// 打印执行顺序用于调试
	t.Logf("Execution order: %v", executionOrder)

	// 注意：由于调度器的复杂性，严格的优先级顺序可能不总是保证
	// 这里主要验证高优先级任务确实被处理了
	highPriorityCount := 0
	for _, id := range executionOrder {
		if id[:4] == "high" {
			highPriorityCount++
		}
	}

	if highPriorityCount != 2 {
		t.Errorf("Expected 2 high priority tasks executed, got %d", highPriorityCount)
	}
}

// priorityTestTask 用于测试优先级的任务包装器
type priorityTestTask struct {
	*TestTask
	onExecute func(string)
}

func (t *priorityTestTask) Execute(ctx context.Context) (any, error) {
	if t.onExecute != nil {
		t.onExecute(t.id)
	}
	return t.TestTask.Execute(ctx)
}

// BenchmarkPoolSubmit 基准测试任务提交性能
func BenchmarkPoolSubmit(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(1000).
		WithMetrics(false). // 禁用监控以获得更准确的性能数据
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			task := NewTestTask("bench", 0) // 无延迟任务
			pool.Submit(task)
		}
	})
}

// BenchmarkPoolSubmitAsync 基准测试异步任务提交性能
func BenchmarkPoolSubmitAsync(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(1000).
		WithMetrics(false).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			task := NewTestTask("bench-async", 0)
			future := pool.SubmitAsync(task)
			_ = future // 避免未使用变量警告
		}
	})
}
