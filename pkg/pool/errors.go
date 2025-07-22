package pool

import (
	"errors"
	"fmt"
	"time"
)

// 基础错误类型
var (
	// ErrPoolClosed 协程池已关闭错误
	ErrPoolClosed = errors.New("pool is closed")

	// ErrQueueFull 任务队列已满错误
	ErrQueueFull = errors.New("task queue is full")

	// ErrInvalidConfig 无效配置错误
	ErrInvalidConfig = errors.New("invalid pool configuration")

	// ErrTaskTimeout 任务执行超时错误
	ErrTaskTimeout = errors.New("task execution timeout")

	// ErrTaskCancelled 任务被取消错误
	ErrTaskCancelled = errors.New("task was cancelled")

	// ErrInvalidWorkerCount 无效工作协程数量错误
	ErrInvalidWorkerCount = errors.New("worker count must be greater than 0")

	// ErrInvalidQueueSize 无效队列大小错误
	ErrInvalidQueueSize = errors.New("queue size must be greater than 0")

	// ErrShutdownTimeout 关闭超时错误
	ErrShutdownTimeout = errors.New("shutdown timeout exceeded")

	// ErrShutdownInProgress 关闭进行中错误
	ErrShutdownInProgress = errors.New("pool shutdown is in progress")

	// ErrForceShutdown 强制关闭错误
	ErrForceShutdown = errors.New("pool was force shutdown due to timeout")

	// ErrCircuitBreakerOpen 熔断器开启错误
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

	// ErrPoolAlreadyRunning 协程池已在运行错误
	ErrPoolAlreadyRunning = errors.New("pool is already running")

	// ErrPoolNotRunning 协程池未运行错误
	ErrPoolNotRunning = errors.New("pool is not running")
)

// PoolError 协程池错误类型
type PoolError struct {
	Op  string // 操作名称
	Err error  // 原始错误
}

func (e *PoolError) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return e.Op + ": " + e.Err.Error()
}

func (e *PoolError) Unwrap() error {
	return e.Err
}

// NewPoolError 创建协程池错误
func NewPoolError(op string, err error) *PoolError {
	return &PoolError{
		Op:  op,
		Err: err,
	}
}

// TaskError 任务执行错误类型
type TaskError struct {
	TaskID string // 任务ID
	Op     string // 操作名称
	Err    error  // 原始错误
}

func (e *TaskError) Error() string {
	if e.TaskID == "" {
		return e.Op + ": " + e.Err.Error()
	}
	return "task " + e.TaskID + " " + e.Op + ": " + e.Err.Error()
}

func (e *TaskError) Unwrap() error {
	return e.Err
}

// NewTaskError 创建任务错误
func NewTaskError(taskID, op string, err error) *TaskError {
	return &TaskError{
		TaskID: taskID,
		Op:     op,
		Err:    err,
	}
}

// ConfigError 配置错误类型
type ConfigError struct {
	Field string // 配置字段名
	Value any    // 配置值
	Err   error  // 原始错误
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config field '%s' with value '%v': %v", e.Field, e.Value, e.Err)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

// NewConfigError 创建配置错误
func NewConfigError(field string, value any, err error) *ConfigError {
	return &ConfigError{
		Field: field,
		Value: value,
		Err:   err,
	}
}

// PanicError panic错误类型
type PanicError struct {
	Value any  // panic值
	Task  Task // 发生panic的任务
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("task panic: %v", e.Value)
}

// NewPanicError 创建panic错误
func NewPanicError(value any, task Task) *PanicError {
	return &PanicError{
		Value: value,
		Task:  task,
	}
}

// TimeoutError 超时错误类型
type TimeoutError struct {
	Duration time.Duration // 超时时长
	Task     Task          // 超时的任务
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("task timeout after %v", e.Duration)
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(duration time.Duration, task Task) *TimeoutError {
	return &TimeoutError{
		Duration: duration,
		Task:     task,
	}
}
