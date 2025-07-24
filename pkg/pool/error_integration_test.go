package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPoolErrorHandling_PanicRecovery 测试协程池的panic恢复机制
func TestPoolErrorHandling_PanicRecovery(t *testing.T) {
	config := &Config{
		WorkerCount:     2,
		QueueSize:       10,
		TaskTimeout:     time.Second,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	// 创建会panic的任务
	panicTask := &errorPanicTask{
		priority:   PriorityNormal,
		panicValue: "test panic for recovery",
	}

	// 提交panic任务
	err = pool.Submit(panicTask)
	if err != nil {
		t.Fatalf("Failed to submit panic task: %v", err)
	}

	// 等待任务执行完成
	time.Sleep(100 * time.Millisecond)

	// 提交正常任务验证协程池仍然工作
	normalTask := &errorTestTask{
		priority: PriorityNormal,
		action: func(ctx context.Context) (any, error) {
			return "normal task completed", nil
		},
	}

	future := pool.SubmitAsync(normalTask)
	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Errorf("Normal task failed after panic recovery: %v", err)
	}

	if result != "normal task completed" {
		t.Errorf("Expected 'normal task completed', got %v", result)
	}

	// 验证统计信息 - panic被恢复，任务可能被标记为失败或完成
	stats := pool.Stats()
	totalProcessed := stats.CompletedTasks + stats.FailedTasks
	if totalProcessed == 0 {
		t.Error("Expected some tasks to be processed (completed or failed)")
	}

	t.Logf("Pool stats: Completed=%d, Failed=%d", stats.CompletedTasks, stats.FailedTasks)
}

// TestPoolErrorHandling_TaskTimeout 测试任务超时处理
func TestPoolErrorHandling_TaskTimeout(t *testing.T) {
	config := &Config{
		WorkerCount:     1,
		QueueSize:       10,
		TaskTimeout:     100 * time.Millisecond, // 短超时时间
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	// 创建会超时的任务
	timeoutTask := &errorTimeoutTask{
		priority: PriorityNormal,
		duration: 200 * time.Millisecond, // 超过超时时间
	}

	// 提交超时任务
	future := pool.SubmitAsync(timeoutTask)
	result, err := future.GetWithTimeout(time.Second)

	// 应该返回超时错误
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Errorf("Expected TimeoutError, got %T: %v", err, err)
	}

	if result != nil {
		t.Errorf("Expected nil result for timeout task, got %v", result)
	}

	// 验证统计信息
	stats := pool.Stats()
	if stats.FailedTasks == 0 {
		t.Error("Expected failed tasks count > 0 due to timeout")
	}
}

// TestPoolErrorHandling_QueueFull 测试队列满的处理
func TestPoolErrorHandling_QueueFull(t *testing.T) {
	config := &Config{
		WorkerCount:     1,
		QueueSize:       2, // 很小的队列
		TaskTimeout:     time.Second,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		// Use a short timeout for shutdown to avoid test timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 创建阻塞任务占用工作协程
	blockingTask := &errorTestTask{
		priority: PriorityNormal,
		action: func(ctx context.Context) (any, error) {
			time.Sleep(500 * time.Millisecond) // 足够长的阻塞时间
			return "blocking task completed", nil
		},
	}

	// 提交足够多的任务填满队列（考虑本地队列大小）
	var submitErrors []error
	// 需要提交超过 1(worker) + 256(local queue) + 1(task channel) + 2(global queue) = 260 个任务
	for i := 0; i < 300; i++ {
		err := pool.Submit(blockingTask)
		if err != nil {
			submitErrors = append(submitErrors, err)
		}
		// 如果已经有足够的错误，就停止提交
		if len(submitErrors) >= 10 {
			break
		}
	}

	// 应该有一些提交失败（队列满）
	if len(submitErrors) == 0 {
		t.Error("Expected some submit errors due to queue full")
	}

	// 验证错误类型
	for _, err := range submitErrors {
		if !errors.Is(err, ErrQueueFull) && !errors.Is(err, ErrCircuitBreakerOpen) {
			t.Errorf("Expected ErrQueueFull or ErrCircuitBreakerOpen, got %v", err)
		}
	}
}

// TestPoolErrorHandling_CircuitBreaker 测试熔断器机制
func TestPoolErrorHandling_CircuitBreaker(t *testing.T) {
	config := &Config{
		WorkerCount:     1,
		QueueSize:       2, // 很小的队列，容易触发熔断器
		TaskTimeout:     time.Second,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		// Use a short timeout for shutdown to avoid test timeout
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 创建阻塞任务
	blockingTask := &errorTestTask{
		priority: PriorityNormal,
		action: func(ctx context.Context) (any, error) {
			time.Sleep(500 * time.Millisecond) // 足够长的阻塞时间
			return "completed", nil
		},
	}

	// 快速提交大量任务触发熔断器
	var circuitBreakerErrors int
	var queueFullErrors int

	// 需要提交足够多的任务来触发队列满，然后触发熔断器
	for i := 0; i < 300; i++ {
		err := pool.Submit(blockingTask)
		if err != nil {
			if errors.Is(err, ErrCircuitBreakerOpen) {
				circuitBreakerErrors++
			} else if errors.Is(err, ErrQueueFull) {
				queueFullErrors++
			}
		}
		// 如果已经有熔断器错误，就停止提交
		if circuitBreakerErrors >= 5 {
			break
		}
	}

	// 应该有熔断器错误
	if circuitBreakerErrors == 0 {
		t.Error("Expected circuit breaker errors")
	}

	t.Logf("Queue full errors: %d, Circuit breaker errors: %d",
		queueFullErrors, circuitBreakerErrors)
}

// TestPoolErrorHandling_GracefulShutdown 测试优雅关闭时的错误处理
func TestPoolErrorHandling_GracefulShutdown(t *testing.T) {
	config := &Config{
		WorkerCount:     2,
		QueueSize:       10,
		TaskTimeout:     time.Second,
		ShutdownTimeout: 200 * time.Millisecond, // 短关闭超时
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// 提交长时间运行的任务
	longRunningTask := &errorTestTask{
		priority: PriorityNormal,
		action: func(ctx context.Context) (any, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return "completed", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	// 提交多个长时间运行的任务
	for i := 0; i < 5; i++ {
		pool.Submit(longRunningTask)
	}

	// 立即关闭，应该触发强制关闭
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = pool.Shutdown(ctx)
	if err == nil {
		t.Error("Expected shutdown error due to timeout")
	}

	// 验证错误类型
	var poolErr *PoolError
	if !errors.As(err, &poolErr) {
		t.Errorf("Expected PoolError, got %T: %v", err, err)
	}
}

// TestPoolErrorHandling_ConcurrentErrors 测试并发错误处理
func TestPoolErrorHandling_ConcurrentErrors(t *testing.T) {
	config := &Config{
		WorkerCount:     4,
		QueueSize:       20,
		TaskTimeout:     100 * time.Millisecond,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	var wg sync.WaitGroup
	var panicCount int64
	var timeoutCount int64
	var successCount int64

	// 并发提交不同类型的任务
	numGoroutines := 10
	tasksPerGoroutine := 20

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < tasksPerGoroutine; j++ {
				var task Task

				switch j % 3 {
				case 0:
					// 正常任务
					task = &errorTestTask{
						priority: PriorityNormal,
						action: func(ctx context.Context) (any, error) {
							time.Sleep(10 * time.Millisecond)
							return "success", nil
						},
					}
				case 1:
					// panic任务
					task = &errorPanicTask{
						priority:   PriorityNormal,
						panicValue: "concurrent panic",
					}
				case 2:
					// 超时任务
					task = &errorTimeoutTask{
						priority: PriorityNormal,
						duration: 200 * time.Millisecond,
					}
				}

				future := pool.SubmitAsync(task)
				result, err := future.GetWithTimeout(300 * time.Millisecond)

				if err != nil {
					var panicErr *PanicError
					var timeoutErr *TimeoutError
					if errors.As(err, &panicErr) {
						atomic.AddInt64(&panicCount, 1)
					} else if errors.As(err, &timeoutErr) {
						atomic.AddInt64(&timeoutCount, 1)
					}
				} else if result == "success" {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证结果
	totalTasks := int64(numGoroutines * tasksPerGoroutine)
	processedTasks := atomic.LoadInt64(&panicCount) +
		atomic.LoadInt64(&timeoutCount) +
		atomic.LoadInt64(&successCount)

	t.Logf("Total tasks: %d, Processed: %d, Success: %d, Panic: %d, Timeout: %d",
		totalTasks, processedTasks, successCount, panicCount, timeoutCount)

	if processedTasks == 0 {
		t.Error("No tasks were processed")
	}

	if atomic.LoadInt64(&successCount) == 0 {
		t.Error("No successful tasks")
	}

	// 验证统计信息
	stats := pool.Stats()
	if stats.CompletedTasks == 0 && stats.FailedTasks == 0 {
		t.Error("No tasks recorded in pool stats")
	}
}

// TestPoolErrorHandling_ErrorRecovery 测试错误恢复机制
func TestPoolErrorHandling_ErrorRecovery(t *testing.T) {
	config := &Config{
		WorkerCount:     2,
		QueueSize:       10,
		TaskTimeout:     time.Second,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	// 第一阶段：提交会导致panic的任务
	for i := 0; i < 5; i++ {
		panicTask := &errorPanicTask{
			priority:   PriorityNormal,
			panicValue: "recovery test panic",
		}
		pool.Submit(panicTask)
	}

	// 等待panic任务执行完成
	time.Sleep(200 * time.Millisecond)

	// 第二阶段：验证协程池仍然可以处理正常任务
	var successCount int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			normalTask := &errorTestTask{
				priority: PriorityNormal,
				action: func(ctx context.Context) (any, error) {
					return "recovered", nil
				},
			}

			future := pool.SubmitAsync(normalTask)
			result, err := future.GetWithTimeout(time.Second)

			if err == nil && result == "recovered" {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// 验证恢复效果
	if atomic.LoadInt64(&successCount) != 10 {
		t.Errorf("Expected 10 successful tasks after recovery, got %d",
			atomic.LoadInt64(&successCount))
	}

	// 验证协程池状态
	if !pool.IsRunning() {
		t.Error("Pool should still be running after error recovery")
	}
}

// TestPoolErrorHandling_CustomErrorHandler 测试错误处理器功能
func TestPoolErrorHandling_CustomErrorHandler(t *testing.T) {
	config := &Config{
		WorkerCount:     1,
		QueueSize:       5,
		TaskTimeout:     100 * time.Millisecond,
		ShutdownTimeout: 5 * time.Second,
		EnableMetrics:   true,
		MetricsInterval: time.Second,
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Shutdown(context.Background())

	// 提交各种类型的错误任务
	panicTask := &errorPanicTask{
		priority:   PriorityNormal,
		panicValue: "custom handler panic",
	}

	timeoutTask := &errorTimeoutTask{
		priority: PriorityNormal,
		duration: 200 * time.Millisecond,
	}

	pool.Submit(panicTask)
	pool.Submit(timeoutTask)

	// 等待任务执行
	time.Sleep(300 * time.Millisecond)

	// 验证协程池仍然正常工作（错误处理器正常工作）
	normalTask := &errorTestTask{
		priority: PriorityNormal,
		action: func(ctx context.Context) (any, error) {
			return "normal task completed", nil
		},
	}

	err = pool.Submit(normalTask)
	if err != nil {
		t.Errorf("Pool should still work after error handling: %v", err)
	}

	// 验证协程池状态
	if !pool.IsRunning() {
		t.Error("Pool should still be running after error handling")
	}
}
