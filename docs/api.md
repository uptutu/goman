# Goroutine Pool SDK API 文档

## 概述

Goroutine Pool SDK 是一个高性能的 Go 协程池管理库，提供了完整的协程池管理、任务调度、监控和扩展功能。本文档详细介绍了 SDK 的所有公开 API 接口。

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/uptutu/goman/pkg/pool"
)

// 实现 Task 接口
type MyTask struct {
    ID   int
    Data string
}

func (t *MyTask) Execute(ctx context.Context) (any, error) {
    // 执行业务逻辑
    time.Sleep(100 * time.Millisecond)
    return fmt.Sprintf("Task %d processed: %s", t.ID, t.Data), nil
}

func (t *MyTask) Priority() int {
    return pool.PriorityNormal
}

func main() {
    // 创建协程池
    p, err := pool.NewPool(nil) // 使用默认配置
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    defer p.Shutdown(context.Background())

    // 提交任务
    task := &MyTask{ID: 1, Data: "example"}
    err = p.Submit(task)
    if err != nil {
        log.Printf("Failed to submit task: %v", err)
    }

    // 异步提交任务
    future := p.SubmitAsync(task)
    result, err := future.Get()
    if err != nil {
        log.Printf("Task failed: %v", err)
    } else {
        fmt.Printf("Result: %v\n", result)
    }
}
```

## 目录

- [核心接口](#核心接口)
- [配置管理](#配置管理)
- [任务管理](#任务管理)
- [监控和统计](#监控和统计)
- [扩展机制](#扩展机制)
- [错误处理](#错误处理)
- [常量定义](#常量定义)

## 核心接口

### Pool 接口

`Pool` 是协程池的主要接口，提供了协程池的核心功能。

```go
type Pool interface {
    // 任务提交方法
    Submit(task Task) error
    SubmitWithTimeout(task Task, timeout time.Duration) error
    SubmitAsync(task Task) Future
    
    // 生命周期管理
    Shutdown(ctx context.Context) error
    
    // 状态查询
    IsRunning() bool
    IsShutdown() bool
    IsClosed() bool
    
    // 统计信息
    Stats() PoolStats
    
    // 扩展管理
    AddMiddleware(middleware Middleware)
    RemoveMiddleware(middleware Middleware)
    RegisterPlugin(plugin Plugin) error
    UnregisterPlugin(name string) error
    GetPlugin(name string) (Plugin, bool)
    AddEventListener(listener EventListener)
    RemoveEventListener(listener EventListener)
    SetSchedulerPlugin(plugin SchedulerPlugin)
    GetSchedulerPlugin() SchedulerPlugin
}
```

#### 方法说明

##### Submit(task Task) error
提交任务到协程池进行同步执行。

**参数:**
- `task`: 实现了 `Task` 接口的任务对象

**返回值:**
- `error`: 提交失败时返回错误，成功时返回 `nil`

**可能的错误:**
- `ErrPoolClosed`: 协程池已关闭
- `ErrShutdownInProgress`: 协程池正在关闭
- `ErrQueueFull`: 任务队列已满

**示例:**
```go
task := &MyTask{data: "example"}
err := pool.Submit(task)
if err != nil {
    log.Printf("Failed to submit task: %v", err)
}
```

##### SubmitWithTimeout(task Task, timeout time.Duration) error
提交任务到协程池，并设置执行超时时间。

**参数:**
- `task`: 实现了 `Task` 接口的任务对象
- `timeout`: 任务执行超时时间

**返回值:**
- `error`: 提交失败时返回错误，成功时返回 `nil`

**示例:**
```go
task := &MyTask{data: "example"}
err := pool.SubmitWithTimeout(task, 5*time.Second)
if err != nil {
    log.Printf("Failed to submit task with timeout: %v", err)
}
```

##### SubmitAsync(task Task) Future
异步提交任务到协程池，立即返回 `Future` 对象用于获取结果。

**参数:**
- `task`: 实现了 `Task` 接口的任务对象

**返回值:**
- `Future`: 用于获取异步执行结果的 Future 对象

**示例:**
```go
task := &MyTask{data: "example"}
future := pool.SubmitAsync(task)

// 获取结果
result, err := future.Get()
if err != nil {
    log.Printf("Task failed: %v", err)
} else {
    log.Printf("Task result: %v", result)
}
```

##### Shutdown(ctx context.Context) error
优雅关闭协程池。等待正在执行的任务完成，然后关闭协程池。

**参数:**
- `ctx`: 用于控制关闭超时的上下文

**返回值:**
- `error`: 关闭失败时返回错误，成功时返回 `nil`

**示例:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := pool.Shutdown(ctx)
if err != nil {
    log.Printf("Failed to shutdown pool: %v", err)
}
```

### Task 接口

`Task` 接口定义了任务的基本行为。

```go
type Task interface {
    Execute(ctx context.Context) (any, error)
    Priority() int
}
```

#### 方法说明

##### Execute(ctx context.Context) (any, error)
执行任务的具体逻辑。

**参数:**
- `ctx`: 任务执行的上下文，可用于取消和超时控制

**返回值:**
- `any`: 任务执行结果
- `error`: 执行错误

##### Priority() int
返回任务的优先级。

**返回值:**
- `int`: 任务优先级，数值越大优先级越高

**优先级常量:**
- `PriorityLow = 0`: 低优先级
- `PriorityNormal = 1`: 普通优先级
- `PriorityHigh = 2`: 高优先级
- `PriorityCritical = 3`: 关键优先级

### Future 接口

`Future` 接口用于获取异步任务的执行结果。

```go
type Future interface {
    Get() (any, error)
    GetWithTimeout(timeout time.Duration) (any, error)
    IsDone() bool
    Cancel() bool
}
```

#### 方法说明

##### Get() (any, error)
阻塞等待任务完成并获取结果。

**返回值:**
- `any`: 任务执行结果
- `error`: 执行错误

##### GetWithTimeout(timeout time.Duration) (any, error)
带超时的结果获取。

**参数:**
- `timeout`: 等待超时时间

**返回值:**
- `any`: 任务执行结果
- `error`: 执行错误或超时错误

##### IsDone() bool
检查任务是否已完成。

**返回值:**
- `bool`: 任务是否已完成

##### Cancel() bool
尝试取消任务。

**返回值:**
- `bool`: 是否成功取消

## 配置管理

### Config 结构体

`Config` 结构体包含了协程池的所有配置选项。

```go
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
```

### 配置创建和验证

#### NewConfig() *Config
创建默认配置。

**返回值:**
- `*Config`: 包含默认值的配置对象

**示例:**
```go
config := pool.NewConfig()
```

#### (*Config) Validate() error
验证配置的有效性。

**返回值:**
- `error`: 配置无效时返回错误，有效时返回 `nil`

**示例:**
```go
config := &pool.Config{
    WorkerCount: 10,
    QueueSize:   1000,
}

if err := config.Validate(); err != nil {
    log.Printf("Invalid config: %v", err)
}
```

### ConfigBuilder

`ConfigBuilder` 提供了链式调用的配置构建方式。

```go
type ConfigBuilder struct {
    config *Config
}
```

#### 创建和使用

```go
config, err := pool.NewConfigBuilder().
    WithWorkerCount(20).
    WithQueueSize(2000).
    WithTaskTimeout(10 * time.Second).
    WithMetrics(true).
    Build()

if err != nil {
    log.Printf("Failed to build config: %v", err)
}
```

#### 方法列表

- `WithWorkerCount(count int) *ConfigBuilder`: 设置工作协程数量
- `WithQueueSize(size int) *ConfigBuilder`: 设置队列大小
- `WithObjectPoolSize(size int) *ConfigBuilder`: 设置对象池大小
- `WithPreAlloc(preAlloc bool) *ConfigBuilder`: 设置是否预分配内存
- `WithTaskTimeout(timeout time.Duration) *ConfigBuilder`: 设置任务超时时间
- `WithShutdownTimeout(timeout time.Duration) *ConfigBuilder`: 设置关闭超时时间
- `WithMetrics(enable bool) *ConfigBuilder`: 设置是否启用监控
- `WithMetricsInterval(interval time.Duration) *ConfigBuilder`: 设置监控间隔
- `WithPanicHandler(handler func(any)) *ConfigBuilder`: 设置panic处理器
- `WithTaskInterceptor(interceptor TaskInterceptor) *ConfigBuilder`: 设置任务拦截器
- `Build() (*Config, error)`: 构建并验证配置
- `BuildUnsafe() *Config`: 构建配置但不验证

## 任务管理

### 创建协程池

#### NewPool(config *Config) (Pool, error)
创建新的协程池实例。

**参数:**
- `config`: 协程池配置，如果为 `nil` 则使用默认配置

**返回值:**
- `Pool`: 协程池实例
- `error`: 创建失败时返回错误

**示例:**
```go
config := pool.NewConfig()
config.WorkerCount = 20
config.QueueSize = 2000

p, err := pool.NewPool(config)
if err != nil {
    log.Fatalf("Failed to create pool: %v", err)
}
defer p.Shutdown(context.Background())
```

### 任务实现示例

```go
// 简单任务示例
type SimpleTask struct {
    ID   int
    Data string
}

func (t *SimpleTask) Execute(ctx context.Context) (any, error) {
    // 检查上下文是否被取消
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // 执行具体业务逻辑
    time.Sleep(100 * time.Millisecond) // 模拟工作
    
    return fmt.Sprintf("Processed task %d: %s", t.ID, t.Data), nil
}

func (t *SimpleTask) Priority() int {
    return pool.PriorityNormal
}

// 高优先级任务示例
type UrgentTask struct {
    Message string
}

func (t *UrgentTask) Execute(ctx context.Context) (any, error) {
    // 紧急任务处理逻辑
    return "Urgent: " + t.Message, nil
}

func (t *UrgentTask) Priority() int {
    return pool.PriorityHigh
}
```

## 监控和统计

### PoolStats 结构体

`PoolStats` 包含了协程池的运行统计信息。

```go
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
```

### 获取统计信息

```go
stats := pool.Stats()
fmt.Printf("Active Workers: %d\n", stats.ActiveWorkers)
fmt.Printf("Queued Tasks: %d\n", stats.QueuedTasks)
fmt.Printf("Completed Tasks: %d\n", stats.CompletedTasks)
fmt.Printf("Failed Tasks: %d\n", stats.FailedTasks)
fmt.Printf("Average Duration: %v\n", stats.AvgTaskDuration)
fmt.Printf("Throughput: %.2f TPS\n", stats.ThroughputTPS)
fmt.Printf("Memory Usage: %d bytes\n", stats.MemoryUsage)
fmt.Printf("GC Count: %d\n", stats.GCCount)
```

### Monitor 接口

`Monitor` 接口用于自定义监控实现。

```go
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
```

## 扩展机制

### 中间件系统

#### Middleware 接口

```go
type Middleware interface {
    Before(ctx context.Context, task Task) context.Context
    After(ctx context.Context, task Task, result any, err error)
}
```

#### 内置中间件

##### LoggingMiddleware
日志记录中间件，记录任务的执行过程。

```go
// 创建日志中间件
loggingMiddleware := pool.NewLoggingMiddleware(nil) // 使用默认日志器
pool.AddMiddleware(loggingMiddleware)
```

##### MetricsMiddleware
指标收集中间件，收集任务执行的统计信息。

```go
// 创建指标中间件
metricsMiddleware := pool.NewMetricsMiddleware()
pool.AddMiddleware(metricsMiddleware)

// 获取指标
metrics := metricsMiddleware.GetMetrics()
fmt.Printf("Success Rate: %.2f%%\n", metrics.SuccessRate*100)
```

##### TimeoutMiddleware
超时控制中间件，为任务添加超时控制。

```go
// 创建超时中间件
timeoutMiddleware := pool.NewTimeoutMiddleware(5 * time.Second)
pool.AddMiddleware(timeoutMiddleware)
```

#### 自定义中间件示例

```go
type CustomMiddleware struct {
    name string
}

func (cm *CustomMiddleware) Before(ctx context.Context, task Task) context.Context {
    fmt.Printf("[%s] Task starting: priority=%d\n", cm.name, task.Priority())
    return context.WithValue(ctx, "start_time", time.Now())
}

func (cm *CustomMiddleware) After(ctx context.Context, task Task, result any, err error) {
    startTime, _ := ctx.Value("start_time").(time.Time)
    duration := time.Since(startTime)
    
    if err != nil {
        fmt.Printf("[%s] Task failed after %v: %v\n", cm.name, duration, err)
    } else {
        fmt.Printf("[%s] Task completed after %v\n", cm.name, duration)
    }
}

// 使用自定义中间件
customMiddleware := &CustomMiddleware{name: "MyMiddleware"}
pool.AddMiddleware(customMiddleware)
```

### 插件系统

#### Plugin 接口

```go
type Plugin interface {
    Name() string
    Init(pool Pool) error
    Start() error
    Stop() error
}
```

#### 内置插件

##### MonitoringPlugin
监控插件，定期输出协程池状态信息。

```go
// 创建监控插件
monitoringPlugin := pool.NewMonitoringPlugin("Monitor", 5*time.Second, nil)
err := pool.RegisterPlugin(monitoringPlugin)
if err != nil {
    log.Printf("Failed to register plugin: %v", err)
}
```

#### 自定义插件示例

```go
type CustomPlugin struct {
    name   string
    pool   pool.Pool
    ticker *time.Ticker
    done   chan struct{}
}

func NewCustomPlugin(name string, interval time.Duration) *CustomPlugin {
    return &CustomPlugin{
        name:   name,
        ticker: time.NewTicker(interval),
        done:   make(chan struct{}),
    }
}

func (cp *CustomPlugin) Name() string {
    return cp.name
}

func (cp *CustomPlugin) Init(p pool.Pool) error {
    cp.pool = p
    return nil
}

func (cp *CustomPlugin) Start() error {
    go cp.monitor()
    return nil
}

func (cp *CustomPlugin) Stop() error {
    close(cp.done)
    cp.ticker.Stop()
    return nil
}

func (cp *CustomPlugin) monitor() {
    for {
        select {
        case <-cp.ticker.C:
            if cp.pool != nil {
                stats := cp.pool.Stats()
                fmt.Printf("Plugin %s - Active: %d, Completed: %d\n", 
                    cp.name, stats.ActiveWorkers, stats.CompletedTasks)
            }
        case <-cp.done:
            return
        }
    }
}

// 使用自定义插件
customPlugin := NewCustomPlugin("MyPlugin", 3*time.Second)
err := pool.RegisterPlugin(customPlugin)
if err != nil {
    log.Printf("Failed to register plugin: %v", err)
}
```

### 事件系统

#### EventListener 接口

```go
type EventListener interface {
    OnPoolStart(pool Pool)
    OnPoolShutdown(pool Pool)
    OnTaskSubmit(task Task)
    OnTaskComplete(task Task, result any)
    OnWorkerPanic(workerID int, panicValue any)
}
```

#### 内置事件监听器

##### LoggingEventListener
日志事件监听器，记录协程池的各种事件。

```go
// 创建日志事件监听器
loggingListener := pool.NewLoggingEventListener(nil)
pool.AddEventListener(loggingListener)
```

#### 自定义事件监听器示例

```go
type CustomEventListener struct {
    name string
}

func (cel *CustomEventListener) OnPoolStart(p pool.Pool) {
    fmt.Printf("[%s] Pool started\n", cel.name)
}

func (cel *CustomEventListener) OnPoolShutdown(p pool.Pool) {
    fmt.Printf("[%s] Pool shutdown\n", cel.name)
}

func (cel *CustomEventListener) OnTaskSubmit(task pool.Task) {
    fmt.Printf("[%s] Task submitted: priority=%d\n", cel.name, task.Priority())
}

func (cel *CustomEventListener) OnTaskComplete(task pool.Task, result any) {
    fmt.Printf("[%s] Task completed: priority=%d\n", cel.name, task.Priority())
}

func (cel *CustomEventListener) OnWorkerPanic(workerID int, panicValue any) {
    fmt.Printf("[%s] Worker %d panicked: %v\n", cel.name, workerID, panicValue)
}

// 使用自定义事件监听器
customListener := &CustomEventListener{name: "MyListener"}
pool.AddEventListener(customListener)
```

## 错误处理

### 错误类型

SDK 定义了多种错误类型来处理不同的异常情况：

#### 基础错误

```go
var (
    ErrPoolClosed         = errors.New("pool is closed")
    ErrQueueFull          = errors.New("task queue is full")
    ErrInvalidConfig      = errors.New("invalid pool configuration")
    ErrTaskTimeout        = errors.New("task execution timeout")
    ErrTaskCancelled      = errors.New("task was cancelled")
    ErrShutdownTimeout    = errors.New("shutdown timeout exceeded")
    ErrShutdownInProgress = errors.New("pool shutdown is in progress")
    ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)
```

#### 结构化错误类型

##### PoolError
协程池操作错误。

```go
type PoolError struct {
    Op  string // 操作名称
    Err error  // 原始错误
}
```

##### TaskError
任务执行错误。

```go
type TaskError struct {
    TaskID string // 任务ID
    Op     string // 操作名称
    Err    error  // 原始错误
}
```

##### ConfigError
配置错误。

```go
type ConfigError struct {
    Field string // 配置字段名
    Value any    // 配置值
    Err   error  // 原始错误
}
```

##### PanicError
任务执行时发生的 panic 错误。

```go
type PanicError struct {
    Value any  // panic值
    Task  Task // 发生panic的任务
}
```

##### TimeoutError
任务执行超时错误。

```go
type TimeoutError struct {
    Duration time.Duration // 超时时长
    Task     Task          // 超时的任务
}
```

### ErrorHandler 接口

`ErrorHandler` 接口用于自定义错误处理逻辑。

```go
type ErrorHandler interface {
    HandlePanic(workerID int, task Task, panicValue any)
    HandleTimeout(task Task, duration time.Duration)
    HandleQueueFull(task Task) error
    GetStats() ErrorStats
}
```

### 错误处理示例

```go
// 检查特定错误类型
err := pool.Submit(task)
if err != nil {
    switch {
    case errors.Is(err, pool.ErrQueueFull):
        fmt.Println("Queue is full, try again later")
    case errors.Is(err, pool.ErrPoolClosed):
        fmt.Println("Pool is closed")
    default:
        fmt.Printf("Unknown error: %v\n", err)
    }
}

// 处理结构化错误
var poolErr *pool.PoolError
if errors.As(err, &poolErr) {
    fmt.Printf("Pool operation '%s' failed: %v\n", poolErr.Op, poolErr.Err)
}

var configErr *pool.ConfigError
if errors.As(err, &configErr) {
    fmt.Printf("Config field '%s' with value '%v' is invalid: %v\n", 
        configErr.Field, configErr.Value, configErr.Err)
}
```

## 常量定义

### 协程池状态

```go
const (
    PoolStateRunning  int32 = iota // 运行状态
    PoolStateShutdown              // 关闭中状态
    PoolStateClosed                // 已关闭状态
)
```

### 默认配置

```go
const (
    DefaultWorkerCount     = 10                // 默认工作协程数量
    DefaultQueueSize       = 1000              // 默认任务队列大小
    DefaultTaskTimeout     = 30 * time.Second  // 默认任务执行超时时间
    DefaultShutdownTimeout = 30 * time.Second  // 默认关闭超时时间
    DefaultObjectPoolSize  = 100               // 默认对象池大小
    DefaultMetricsInterval = 10 * time.Second  // 默认监控间隔
)
```

### 任务优先级

```go
const (
    PriorityLow      = 0 // 低优先级
    PriorityNormal   = 1 // 普通优先级
    PriorityHigh     = 2 // 高优先级
    PriorityCritical = 3 // 关键优先级
)
```

### 性能限制

```go
const (
    MaxQueueSize    = 1000000 // 最大队列大小
    MaxWorkerCount  = 10000   // 最大工作协程数量
    MinWorkerCount  = 1       // 最小工作协程数量
    LocalQueueSize  = 256     // 本地队列大小
)
```

### 熔断器状态

```go
const (
    CircuitBreakerClosed   int32 = iota // 熔断器关闭状态
    CircuitBreakerOpen                  // 熔断器开启状态
    CircuitBreakerHalfOpen              // 熔断器半开状态
)
```

## 最佳实践

### 1. 协程池大小配置

```go
// 根据 CPU 核心数配置工作协程数量
workerCount := runtime.NumCPU() * 2

// 根据预期并发量配置队列大小
queueSize := workerCount * 100

config := pool.NewConfigBuilder().
    WithWorkerCount(workerCount).
    WithQueueSize(queueSize).
    Build()
```

### 2. 任务设计

```go
// 好的任务设计：支持上下文取消
type GoodTask struct {
    data string
}

func (t *GoodTask) Execute(ctx context.Context) (any, error) {
    // 定期检查上下文状态
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // 执行业务逻辑
    result := processData(t.data)
    
    // 再次检查上下文状态
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    return result, nil
}
```

### 3. 错误处理

```go
// 统一的错误处理函数
func handleSubmitError(err error) {
    switch {
    case errors.Is(err, pool.ErrQueueFull):
        // 队列满时的处理策略
        time.Sleep(time.Millisecond * 10)
        // 可以实现重试逻辑
    case errors.Is(err, pool.ErrPoolClosed):
        // 协程池关闭时的处理
        log.Println("Pool is closed, stopping task submission")
        return
    default:
        log.Printf("Unexpected error: %v", err)
    }
}
```

### 4. 优雅关闭

```go
func gracefulShutdown(pool pool.Pool) {
    // 创建带超时的上下文
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // 监听系统信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("Received shutdown signal, closing pool...")
        
        if err := pool.Shutdown(ctx); err != nil {
            log.Printf("Failed to shutdown pool gracefully: %v", err)
        } else {
            log.Println("Pool shutdown successfully")
        }
    }()
}
```

### 5. 监控和调试

```go
// 定期输出统计信息
func monitorPool(pool pool.Pool) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            stats := pool.Stats()
            log.Printf("Pool Stats - Active: %d, Queued: %d, Completed: %d, Failed: %d, TPS: %.2f",
                stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks, 
                stats.FailedTasks, stats.ThroughputTPS)
        }
    }
}
```

## 性能调优建议

### 1. 工作协程数量调优

- **CPU 密集型任务**: 工作协程数量 = CPU 核心数
- **I/O 密集型任务**: 工作协程数量 = CPU 核心数 × 2-4
- **混合型任务**: 根据实际测试结果调整

### 2. 队列大小调优

- 队列大小应该是工作协程数量的 10-100 倍
- 过小的队列会导致任务提交失败
- 过大的队列会占用过多内存

### 3. 对象池优化

```go
config := pool.NewConfigBuilder().
    WithObjectPoolSize(1000).  // 根据并发量调整
    WithPreAlloc(true).        // 启用预分配
    Build()
```

### 4. 监控间隔调优

```go
config := pool.NewConfigBuilder().
    WithMetricsInterval(5 * time.Second).  // 根据需要调整监控频率
    Build()
```

### 5. 超时配置

```go
config := pool.NewConfigBuilder().
    WithTaskTimeout(10 * time.Second).     // 根据任务特性设置
    WithShutdownTimeout(30 * time.Second). // 给足够时间完成关闭
    Build()
```

---

本文档涵盖了 Goroutine Pool SDK 的所有主要 API 接口和使用方法。更多详细示例请参考 [使用示例文档](examples.md) 和 [最佳实践指南](best-practices.md)。