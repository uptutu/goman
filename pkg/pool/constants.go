package pool

import "time"

// 协程池状态常量
const (
	// PoolStateRunning 运行状态
	PoolStateRunning int32 = iota
	// PoolStateShutdown 关闭中状态
	PoolStateShutdown
	// PoolStateClosed 已关闭状态
	PoolStateClosed
)

// 默认配置常量
const (
	// DefaultWorkerCount 默认工作协程数量
	DefaultWorkerCount = 10

	// DefaultQueueSize 默认任务队列大小
	DefaultQueueSize = 1000

	// DefaultTaskTimeout 默认任务执行超时时间
	DefaultTaskTimeout = 30 * time.Second

	// DefaultShutdownTimeout 默认关闭超时时间
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultObjectPoolSize 默认对象池大小
	DefaultObjectPoolSize = 100

	// DefaultMetricsInterval 默认监控间隔
	DefaultMetricsInterval = 10 * time.Second
)

// 任务优先级常量
const (
	// PriorityLow 低优先级
	PriorityLow = iota
	// PriorityNormal 普通优先级
	PriorityNormal
	// PriorityHigh 高优先级
	PriorityHigh
	// PriorityCritical 关键优先级
	PriorityCritical
)

// 性能相关常量
const (
	// MaxQueueSize 最大队列大小
	MaxQueueSize = 1000000

	// MaxWorkerCount 最大工作协程数量
	MaxWorkerCount = 10000

	// MinWorkerCount 最小工作协程数量
	MinWorkerCount = 1

	// LocalQueueSize 本地队列大小
	LocalQueueSize = 256
)

// 熔断器相关常量
const (
	// CircuitBreakerClosed 熔断器关闭状态
	CircuitBreakerClosed int32 = iota
	// CircuitBreakerOpen 熔断器开启状态
	CircuitBreakerOpen
	// CircuitBreakerHalfOpen 熔断器半开状态
	CircuitBreakerHalfOpen
)

// 监控相关常量
const (
	// MetricsBufferSize 监控指标缓冲区大小
	MetricsBufferSize = 1000

	// HealthCheckInterval 健康检查间隔
	HealthCheckInterval = 5 * time.Second

	// StatsUpdateInterval 统计信息更新间隔
	StatsUpdateInterval = 1 * time.Second
)

// 队列相关常量
const (
	// QueueTypeLockFree 无锁队列类型
	QueueTypeLockFree = "lockfree"

	// QueueTypeChannel 通道队列类型
	QueueTypeChannel = "channel"

	// DefaultQueueType 默认队列类型
	DefaultQueueType = QueueTypeLockFree
)
