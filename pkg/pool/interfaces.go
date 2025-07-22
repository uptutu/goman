package pool

import (
	"context"
	"sync"
	"time"
)

// Pool 协程池主接口
type Pool interface {
	// Submit 提交任务到协程池
	Submit(task Task) error
	// SubmitWithTimeout 带超时的任务提交
	SubmitWithTimeout(task Task, timeout time.Duration) error
	// SubmitAsync 异步提交任务，返回Future
	SubmitAsync(task Task) Future
	// Shutdown 优雅关闭协程池
	Shutdown(ctx context.Context) error
	// Stats 获取协程池统计信息
	Stats() PoolStats
	// IsRunning 检查协程池是否正在运行
	IsRunning() bool
	// IsShutdown 检查协程池是否正在关闭
	IsShutdown() bool
	// IsClosed 检查协程池是否已关闭
	IsClosed() bool

	// Extension management methods
	// AddMiddleware 添加中间件
	AddMiddleware(middleware Middleware)
	// RemoveMiddleware 移除中间件
	RemoveMiddleware(middleware Middleware)
	// RegisterPlugin 注册插件
	RegisterPlugin(plugin Plugin) error
	// UnregisterPlugin 注销插件
	UnregisterPlugin(name string) error
	// GetPlugin 获取插件
	GetPlugin(name string) (Plugin, bool)
	// AddEventListener 添加事件监听器
	AddEventListener(listener EventListener)
	// RemoveEventListener 移除事件监听器
	RemoveEventListener(listener EventListener)
	// SetSchedulerPlugin 设置调度器插件
	SetSchedulerPlugin(plugin SchedulerPlugin)
	// GetSchedulerPlugin 获取调度器插件
	GetSchedulerPlugin() SchedulerPlugin
}

// Task 任务接口
type Task interface {
	Execute(ctx context.Context) (any, error)
	Priority() int // 任务优先级
}

// Future 异步结果接口
type Future interface {
	Get() (any, error)
	GetWithTimeout(timeout time.Duration) (any, error)
	IsDone() bool
	Cancel() bool
}

// PoolStats 协程池统计信息
type PoolStats struct {
	// 基础指标
	ActiveWorkers  int64 // 活跃工作协程数
	QueuedTasks    int64 // 排队任务数
	CompletedTasks int64 // 已完成任务数
	FailedTasks    int64 // 失败任务数

	// 性能指标
	AvgTaskDuration time.Duration // 平均任务执行时间
	ThroughputTPS   float64       // 每秒处理任务数

	// 资源指标
	MemoryUsage int64 // 内存使用量
	GCCount     int64 // GC次数
}

// DetailedStats 详细统计信息
type DetailedStats struct {
	PoolStats
	TaskDurationStats DurationStats
	ThroughputTrend   []ThroughputPoint
}

// DurationStats 任务执行时间统计
type DurationStats struct {
	Min    time.Duration
	Max    time.Duration
	Median time.Duration
	P95    time.Duration
	P99    time.Duration
}

// ThroughputPoint 吞吐量数据点
type ThroughputPoint struct {
	Time       time.Time
	Throughput float64
}

// Config 协程池配置
type Config struct {
	// 核心配置
	WorkerCount int // 工作协程数量
	QueueSize   int // 任务队列大小

	// 性能配置
	ObjectPoolSize int  // 对象池大小
	PreAlloc       bool // 是否预分配内存

	// 超时配置
	TaskTimeout     time.Duration // 任务执行超时
	ShutdownTimeout time.Duration // 关闭超时

	// 监控配置
	EnableMetrics   bool          // 是否启用监控
	MetricsInterval time.Duration // 监控间隔

	// 扩展配置
	PanicHandler    func(any)       // panic处理器
	TaskInterceptor TaskInterceptor // 任务拦截器
}

// TaskInterceptor 任务拦截器接口
type TaskInterceptor interface {
	Before(ctx context.Context, task Task) context.Context
	After(ctx context.Context, task Task, result any, err error)
}

// ErrorHandler 错误处理接口
type ErrorHandler interface {
	HandlePanic(workerID int, task Task, panicValue any)
	HandleTimeout(task Task, duration time.Duration)
	HandleQueueFull(task Task) error
	GetStats() ErrorStats
}

// ErrorStats 错误统计信息
type ErrorStats struct {
	PanicCount          int64
	TimeoutCount        int64
	QueueFullCount      int64
	CircuitBreakerState CircuitBreakerState
}

// CircuitBreakerState 熔断器状态
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateHalfOpen
	StateOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF_OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

// Monitor 监控接口
type Monitor interface {
	RecordTaskSubmit(task Task)
	RecordTaskComplete(task Task, duration time.Duration)
	RecordTaskFail(task Task, err error)
	GetStats() PoolStats
	Start()
	Stop()
	SetActiveWorkers(count int64)
	SetQueuedTasks(count int64)
}

// DetailedMonitor 详细监控接口，提供更多统计信息
type DetailedMonitor interface {
	Monitor
	GetDetailedStats() DetailedStats
}

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	SelectWorker(workers []Worker) Worker
}

// Worker 工作协程接口
type Worker interface {
	ID() int
	IsActive() bool
	IsRunning() bool
	TaskCount() int64
	LastActiveTime() time.Time
	RestartCount() int32
	LocalQueueSize() int
	Start(wg *sync.WaitGroup)
	Stop()
}

// Scheduler 任务调度器接口
type Scheduler interface {
	Schedule(task Task) error
	Start() error
	Stop() error
}

// Middleware 中间件接口
type Middleware interface {
	Before(ctx context.Context, task Task) context.Context
	After(ctx context.Context, task Task, result any, err error)
}

// Plugin 插件接口
type Plugin interface {
	Name() string
	Init(pool Pool) error
	Start() error
	Stop() error
}

// EventListener 事件监听器接口
type EventListener interface {
	OnPoolStart(pool Pool)
	OnPoolShutdown(pool Pool)
	OnTaskSubmit(task Task)
	OnTaskComplete(task Task, result any)
	OnWorkerPanic(workerID int, panicValue any)
}

// SchedulerPlugin 可插拔调度器接口
type SchedulerPlugin interface {
	Name() string
	CreateScheduler(config *Config) (Scheduler, error)
	Priority() int // 优先级，数值越大优先级越高
}
