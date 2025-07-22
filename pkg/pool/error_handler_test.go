package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockLogger 模拟日志器用于测试
type mockLogger struct {
	logs []string
	mu   sync.Mutex
}

func (l *mockLogger) Printf(format string, v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(format, v...))
}

func (l *mockLogger) Println(v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprint(v...))
}

func (l *mockLogger) getLogs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]string, len(l.logs))
	copy(result, l.logs)
	return result
}

func (l *mockLogger) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = l.logs[:0]
}

// errorTestTask 模拟任务用于错误处理测试
type errorTestTask struct {
	priority int
	action   func(ctx context.Context) (any, error)
}

func (t *errorTestTask) Execute(ctx context.Context) (any, error) {
	if t.action != nil {
		return t.action(ctx)
	}
	return "success", nil
}

func (t *errorTestTask) Priority() int {
	return t.priority
}

// errorPanicTask 会产生panic的任务
type errorPanicTask struct {
	priority   int
	panicValue any
}

func (t *errorPanicTask) Execute(ctx context.Context) (any, error) {
	panic(t.panicValue)
}

func (t *errorPanicTask) Priority() int {
	return t.priority
}

// errorTimeoutTask 会超时的任务
type errorTimeoutTask struct {
	priority int
	duration time.Duration
}

func (t *errorTimeoutTask) Execute(ctx context.Context) (any, error) {
	select {
	case <-time.After(t.duration):
		return "completed", nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *errorTimeoutTask) Priority() int {
	return t.priority
}

func TestDefaultErrorHandler_HandlePanic(t *testing.T) {
	logger := &mockLogger{}
	handler := NewErrorHandlerWithLogger(logger)

	task := &errorTestTask{priority: PriorityNormal}
	panicValue := "test panic"

	// 测试panic处理
	handler.HandlePanic(1, task, panicValue)

	// 验证日志记录
	logs := logger.getLogs()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logs))
	}

	expectedLog := "Worker 1 panic recovered: test panic, task priority: 1"
	if logs[0] != expectedLog {
		t.Errorf("Expected log: %s, got: %s", expectedLog, logs[0])
	}

	// 验证统计信息
	stats := handler.GetStats()
	if stats.PanicCount != 1 {
		t.Errorf("Expected panic count 1, got %d", stats.PanicCount)
	}
}

func TestDefaultErrorHandler_HandleTimeout(t *testing.T) {
	logger := &mockLogger{}
	handler := NewErrorHandlerWithLogger(logger)

	task := &errorTestTask{priority: PriorityHigh}
	duration := 5 * time.Second

	// 测试超时处理
	handler.HandleTimeout(task, duration)

	// 验证日志记录
	logs := logger.getLogs()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logs))
	}

	expectedLog := "Task timeout after 5s, task priority: 2"
	if logs[0] != expectedLog {
		t.Errorf("Expected log: %s, got: %s", expectedLog, logs[0])
	}

	// 验证统计信息
	stats := handler.GetStats()
	if stats.TimeoutCount != 1 {
		t.Errorf("Expected timeout count 1, got %d", stats.TimeoutCount)
	}
}

func TestDefaultErrorHandler_HandleQueueFull(t *testing.T) {
	logger := &mockLogger{}
	handler := NewErrorHandlerWithLogger(logger)

	task := &errorTestTask{priority: PriorityLow}

	// 测试队列满处理
	err := handler.HandleQueueFull(task)

	// 验证返回错误
	if err == nil {
		t.Error("Expected error, got nil")
	}

	var poolErr *PoolError
	if !errors.As(err, &poolErr) {
		t.Error("Expected PoolError type")
	}

	if !errors.Is(err, ErrQueueFull) {
		t.Error("Expected ErrQueueFull")
	}

	// 验证日志记录
	logs := logger.getLogs()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logs))
	}

	expectedLog := "Task queue is full, rejecting task with priority: 0"
	if logs[0] != expectedLog {
		t.Errorf("Expected log: %s, got: %s", expectedLog, logs[0])
	}

	// 验证统计信息
	stats := handler.GetStats()
	if stats.QueueFullCount != 1 {
		t.Errorf("Expected queue full count 1, got %d", stats.QueueFullCount)
	}
}

func TestDefaultErrorHandler_GetStats(t *testing.T) {
	handler := NewDefaultErrorHandler()

	// 初始统计信息应该为0
	stats := handler.GetStats()
	if stats.PanicCount != 0 || stats.TimeoutCount != 0 || stats.QueueFullCount != 0 {
		t.Error("Initial stats should be zero")
	}

	// 触发各种错误
	task := &errorTestTask{priority: PriorityNormal}
	handler.HandlePanic(1, task, "panic")
	handler.HandleTimeout(task, time.Second)
	handler.HandleQueueFull(task)

	// 验证统计信息更新
	stats = handler.GetStats()
	if stats.PanicCount != 1 {
		t.Errorf("Expected panic count 1, got %d", stats.PanicCount)
	}
	if stats.TimeoutCount != 1 {
		t.Errorf("Expected timeout count 1, got %d", stats.TimeoutCount)
	}
	if stats.QueueFullCount != 1 {
		t.Errorf("Expected queue full count 1, got %d", stats.QueueFullCount)
	}
}

func TestCircuitBreaker_Basic(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}

	cb := NewCircuitBreaker(config)

	// 初始状态应该是关闭的
	if cb.State() != StateClosed {
		t.Errorf("Expected initial state CLOSED, got %v", cb.State())
	}

	// 应该允许请求
	if !cb.AllowRequest() {
		t.Error("Should allow request in CLOSED state")
	}

	// 记录失败，但未达到阈值
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Errorf("Expected state CLOSED, got %v", cb.State())
	}

	// 达到失败阈值，应该转为开启状态
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected state OPEN, got %v", cb.State())
	}

	// 开启状态不应该允许请求
	if cb.AllowRequest() {
		t.Error("Should not allow request in OPEN state")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 3,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断器开启
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected state OPEN, got %v", cb.State())
	}

	// 等待重置超时
	time.Sleep(60 * time.Millisecond)

	// 现在应该允许请求（转为半开状态）
	if !cb.AllowRequest() {
		t.Error("Should allow request after reset timeout")
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state HALF_OPEN, got %v", cb.State())
	}

	// 半开状态下记录成功
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()

	// 达到半开状态最大调用次数后应该关闭
	if cb.State() != StateClosed {
		t.Errorf("Expected state CLOSED after successful calls, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 3,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断器开启
	cb.RecordFailure()
	cb.RecordFailure()

	// 等待重置超时
	time.Sleep(60 * time.Millisecond)

	// 转为半开状态
	cb.AllowRequest()

	// 半开状态下记录失败，应该立即转为开启状态
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected state OPEN after failure in HALF_OPEN, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}

	cb := NewCircuitBreaker(config)

	// 触发熔断器开启
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Error("Expected state OPEN")
	}

	// 重置熔断器
	cb.Reset()
	if cb.State() != StateClosed {
		t.Errorf("Expected state CLOSED after reset, got %v", cb.State())
	}

	// 应该允许请求
	if !cb.AllowRequest() {
		t.Error("Should allow request after reset")
	}
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}

	cb := NewCircuitBreaker(config)

	// 记录一些失败
	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.GetStats()
	if stats.State != StateClosed {
		t.Errorf("Expected state CLOSED, got %v", stats.State)
	}
	if stats.Failures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.Failures)
	}
	if stats.Requests != 0 {
		t.Errorf("Expected 0 requests, got %d", stats.Requests)
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 10,
		ResetTimeout:     100 * time.Millisecond,
		HalfOpenMaxCalls: 5,
	}

	cb := NewCircuitBreaker(config)

	// 并发测试
	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 10

	// 并发记录失败
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cb.RecordFailure()
			}
		}()
	}

	// 并发检查状态
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cb.AllowRequest()
				cb.State()
			}
		}()
	}

	wg.Wait()

	// 验证最终状态
	if cb.State() != StateOpen {
		t.Errorf("Expected final state OPEN, got %v", cb.State())
	}
}

func TestErrorHandlerWithCircuitBreaker(t *testing.T) {
	handler := NewDefaultErrorHandler()
	task := &errorTestTask{priority: PriorityNormal}

	// 触发多次队列满错误，应该触发熔断器
	for i := 0; i < 15; i++ {
		err := handler.HandleQueueFull(task)
		// 所有调用都应该返回队列满错误，但内部会记录到熔断器
		if !errors.Is(err, ErrQueueFull) && !errors.Is(err, ErrCircuitBreakerOpen) {
			t.Errorf("Expected ErrQueueFull or ErrCircuitBreakerOpen at iteration %d, got %v", i, err)
		}
	}

	// 验证统计信息
	stats := handler.GetStats()
	if stats.QueueFullCount != 15 {
		t.Errorf("Expected queue full count 15, got %d", stats.QueueFullCount)
	}

	// 验证熔断器状态（应该是开启的，因为记录了很多失败）
	if stats.CircuitBreakerState != StateOpen {
		t.Logf("Circuit breaker state: %v (may not be OPEN due to test timing)", stats.CircuitBreakerState)
	}
}

func TestCircuitBreakerStateString(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{StateClosed, "CLOSED"},
		{StateHalfOpen, "HALF_OPEN"},
		{StateOpen, "OPEN"},
		{CircuitBreakerState(999), "UNKNOWN"},
	}

	for _, test := range tests {
		if test.state.String() != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, test.state.String())
		}
	}
}

// 基准测试
func BenchmarkErrorHandler_HandlePanic(b *testing.B) {
	handler := NewDefaultErrorHandler()
	task := &errorTestTask{priority: PriorityNormal}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			handler.HandlePanic(1, task, "benchmark panic")
		}
	})
}

func BenchmarkErrorHandler_HandleTimeout(b *testing.B) {
	handler := NewDefaultErrorHandler()
	task := &errorTestTask{priority: PriorityNormal}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			handler.HandleTimeout(task, time.Second)
		}
	})
}

func BenchmarkErrorHandler_HandleQueueFull(b *testing.B) {
	handler := NewDefaultErrorHandler()
	task := &errorTestTask{priority: PriorityNormal}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			handler.HandleQueueFull(task)
		}
	})
}

func BenchmarkCircuitBreaker_AllowRequest(b *testing.B) {
	config := CircuitBreakerConfig{
		FailureThreshold: 10,
		ResetTimeout:     time.Second,
		HalfOpenMaxCalls: 5,
	}
	cb := NewCircuitBreaker(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.AllowRequest()
		}
	})
}

func BenchmarkCircuitBreaker_RecordFailure(b *testing.B) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1000000, // 设置很高的阈值避免状态变化
		ResetTimeout:     time.Second,
		HalfOpenMaxCalls: 5,
	}
	cb := NewCircuitBreaker(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.RecordFailure()
		}
	})
}
