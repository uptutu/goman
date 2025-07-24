package pool

import (
	"sync"
	"sync/atomic"
	"time"
)

// defaultErrorHandler 默认错误处理器实现
type defaultErrorHandler struct {
	logger         Logger
	panicCount     int64
	timeoutCount   int64
	queueFullCount int64
	mu             sync.RWMutex
	circuitBreaker *CircuitBreaker
}

// Logger 日志接口
type Logger interface {
	Printf(format string, v ...any)
	Println(v ...any)
}

// NewDefaultErrorHandler 创建默认错误处理器
func NewDefaultErrorHandler() ErrorHandler {
	return &defaultErrorHandler{
		logger: &defaultLogger{},
		circuitBreaker: NewCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold: 10,
			ResetTimeout:     30 * time.Second,
			HalfOpenMaxCalls: 5,
		}),
	}
}

// NewErrorHandlerWithLogger 创建带自定义日志的错误处理器
func NewErrorHandlerWithLogger(logger Logger) ErrorHandler {
	return &defaultErrorHandler{
		logger: logger,
		circuitBreaker: NewCircuitBreaker(CircuitBreakerConfig{
			FailureThreshold: 10,
			ResetTimeout:     30 * time.Second,
			HalfOpenMaxCalls: 5,
		}),
	}
}

// HandlePanic 处理panic异常
func (h *defaultErrorHandler) HandlePanic(workerID int, task Task, panicValue any) {
	atomic.AddInt64(&h.panicCount, 1)

	// 记录panic到熔断器
	h.circuitBreaker.RecordFailure()

	h.logger.Printf("Worker %d panic recovered: %v, task priority: %d",
		workerID, panicValue, task.Priority())

	// 可以在这里添加更多的panic处理逻辑，比如发送告警等
}

// HandleTimeout 处理任务超时
func (h *defaultErrorHandler) HandleTimeout(task Task, duration time.Duration) {
	atomic.AddInt64(&h.timeoutCount, 1)

	// 记录超时到熔断器
	h.circuitBreaker.RecordFailure()

	h.logger.Printf("Task timeout after %v, task priority: %d",
		duration, task.Priority())
}

// HandleQueueFull 处理队列满的情况
func (h *defaultErrorHandler) HandleQueueFull(task Task) error {
	atomic.AddInt64(&h.queueFullCount, 1)

	// 检查熔断器状态
	if !h.circuitBreaker.AllowRequest() {
		return NewPoolError("submit", ErrCircuitBreakerOpen)
	}

	h.logger.Printf("Task queue is full, rejecting task with priority: %d",
		task.Priority())

	// 记录队列满失败到熔断器
	h.circuitBreaker.RecordFailure()

	// 返回队列满错误，让调用者决定如何处理
	return NewPoolError("submit", ErrQueueFull)
}

// GetStats 获取错误处理统计信息
func (h *defaultErrorHandler) GetStats() ErrorStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return ErrorStats{
		PanicCount:          atomic.LoadInt64(&h.panicCount),
		TimeoutCount:        atomic.LoadInt64(&h.timeoutCount),
		QueueFullCount:      atomic.LoadInt64(&h.queueFullCount),
		CircuitBreakerState: h.circuitBreaker.State(),
	}
}

// CircuitBreaker 熔断器实现
type CircuitBreaker struct {
	config       CircuitBreakerConfig
	state        CircuitBreakerState
	failures     int64
	requests     int64
	lastFailTime time.Time
	mu           sync.RWMutex
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold int           // 失败阈值
	ResetTimeout     time.Duration // 重置超时时间
	HalfOpenMaxCalls int           // 半开状态最大调用次数
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// AllowRequest 检查是否允许请求
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// 检查是否可以转换到半开状态
		if now.Sub(cb.lastFailTime) > cb.config.ResetTimeout {
			cb.state = StateHalfOpen
			cb.requests = 0
			return true
		}
		return false
	case StateHalfOpen:
		// 半开状态下限制请求数量
		return cb.requests < int64(cb.config.HalfOpenMaxCalls)
	default:
		return false
	}
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.requests++

	if cb.state == StateHalfOpen {
		// 半开状态下的成功可能导致关闭熔断器
		if cb.requests >= int64(cb.config.HalfOpenMaxCalls) {
			cb.state = StateClosed
			cb.failures = 0
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= int64(cb.config.FailureThreshold) {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		// 半开状态下的失败立即转为开启状态
		cb.state = StateOpen
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.requests = 0
}

// GetStats 获取熔断器统计信息
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:        cb.state,
		Failures:     cb.failures,
		Requests:     cb.requests,
		LastFailTime: cb.lastFailTime,
	}
}

// CircuitBreakerStats 熔断器统计信息
type CircuitBreakerStats struct {
	State        CircuitBreakerState
	Failures     int64
	Requests     int64
	LastFailTime time.Time
}
