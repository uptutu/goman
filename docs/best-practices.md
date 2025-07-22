# Goroutine Pool SDK 最佳实践指南

## 概述

本文档提供了使用 Goroutine Pool SDK 的最佳实践建议，帮助您在生产环境中高效、安全地使用协程池。

## 目录

- [配置优化](#配置优化)
- [任务设计](#任务设计)
- [错误处理](#错误处理)
- [性能调优](#性能调优)
- [监控和观测](#监控和观测)
- [生产环境部署](#生产环境部署)
- [常见问题和解决方案](#常见问题和解决方案)

## 配置优化

### 工作协程数量配置

#### CPU 密集型任务
```go
// 对于 CPU 密集型任务，工作协程数量应该等于 CPU 核心数
config := pool.NewConfigBuilder().
    WithWorkerCount(runtime.NumCPU()).
    Build()
```

#### I/O 密集型任务
```go
// 对于 I/O 密集型任务，可以设置为 CPU 核心数的 2-4 倍
config := pool.NewConfigBuilder().
    WithWorkerCount(runtime.NumCPU() * 3).
    Build()
```

#### 混合型任务
```go
// 对于混合型任务，建议通过压力测试确定最佳值
// 一般可以从 CPU 核心数的 1.5-2 倍开始调试
config := pool.NewConfigBuilder().
    WithWorkerCount(int(float64(runtime.NumCPU()) * 1.5)).
    Build()
```

### 队列大小配置

```go
// 队列大小建议设置为工作协程数量的 10-100 倍
// 具体取决于任务的执行时间和提交频率
workerCount := runtime.NumCPU() * 2
queueSize := workerCount * 50 // 50倍是一个较好的起始值

config := pool.NewConfigBuilder().
    WithWorkerCount(workerCount).
    WithQueueSize(queueSize).
    Build()
```

### 超时配置

```go
config := pool.NewConfigBuilder().
    WithTaskTimeout(30 * time.Second).     // 根据业务需求设置任务超时
    WithShutdownTimeout(60 * time.Second). // 给足够时间完成优雅关闭
    Build()
```

### 对象池优化

```go
config := pool.NewConfigBuilder().
    WithObjectPoolSize(1000).  // 根据并发量调整
    WithPreAlloc(true).        // 启用预分配减少GC压力
    Build()
```

## 任务设计

### 良好的任务设计原则

#### 1. 支持上下文取消

```go
type GoodTask struct {
    data string
}

func (t *GoodTask) Execute(ctx context.Context) (any, error) {
    // 在长时间运行的操作前检查上下文
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // 执行业务逻辑
    result := t.processData()
    
    // 在返回前再次检查上下文
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    return result, nil
}

func (t *GoodTask) processData() string {
    // 在循环中定期检查上下文
    for i := 0; i < 1000; i++ {
        // 每100次迭代检查一次
        if i%100 == 0 {
            select {
            case <-ctx.Done():
                return ""
            default:
            }
        }
        // 处理逻辑
    }
    return "processed"
}
```

#### 2. 合理设置任务优先级

```go
type BusinessTask struct {
    UserType string
    TaskType string
}

func (t *BusinessTask) Priority() int {
    // 根据业务重要性设置优先级
    switch {
    case t.UserType == "vip" && t.TaskType == "payment":
        return pool.PriorityCritical
    case t.UserType == "vip":
        return pool.PriorityHigh
    case t.TaskType == "payment":
        return pool.PriorityHigh
    default:
        return pool.PriorityNormal
    }
}
```

#### 3. 避免任务间依赖

```go
// 不好的设计：任务间有依赖关系
type BadTask struct {
    dependsOn *BadTask
}

// 好的设计：任务独立，通过外部协调
type GoodTask struct {
    id   string
    data TaskData
}

// 使用外部协调器管理任务依赖
type TaskCoordinator struct {
    pool pool.Pool
}

func (tc *TaskCoordinator) ExecuteWorkflow(tasks []GoodTask) error {
    // 按依赖关系顺序执行任务
    for _, task := range tasks {
        future := tc.pool.SubmitAsync(&task)
        _, err := future.Get()
        if err != nil {
            return err
        }
    }
    return nil
}
```

### 任务大小控制

```go
// 避免过大的任务
type LargeTask struct {
    items []Item // 可能包含数万个项目
}

// 更好的方式：将大任务拆分为小任务
type SmallTask struct {
    items []Item // 限制在100-1000个项目
    batch int
}

func splitLargeTask(items []Item, batchSize int) []*SmallTask {
    var tasks []*SmallTask
    for i := 0; i < len(items); i += batchSize {
        end := i + batchSize
        if end > len(items) {
            end = len(items)
        }
        
        tasks = append(tasks, &SmallTask{
            items: items[i:end],
            batch: i / batchSize,
        })
    }
    return tasks
}
```

## 错误处理

### 统一错误处理策略

```go
// 定义业务错误类型
type BusinessError struct {
    Code    string
    Message string
    Cause   error
}

func (e *BusinessError) Error() string {
    return fmt.Sprintf("business error [%s]: %s", e.Code, e.Message)
}

// 任务中的错误处理
func (t *BusinessTask) Execute(ctx context.Context) (any, error) {
    result, err := t.doBusinessLogic(ctx)
    if err != nil {
        // 包装业务错误
        return nil, &BusinessError{
            Code:    "BUSINESS_LOGIC_ERROR",
            Message: "Failed to process business logic",
            Cause:   err,
        }
    }
    return result, nil
}

// 统一的错误处理函数
func handleTaskError(err error) {
    var businessErr *BusinessError
    var poolErr *pool.PoolError
    var timeoutErr *pool.TimeoutError
    
    switch {
    case errors.As(err, &businessErr):
        log.Printf("Business error [%s]: %s, cause: %v", 
            businessErr.Code, businessErr.Message, businessErr.Cause)
        // 发送告警或记录指标
    case errors.As(err, &poolErr):
        log.Printf("Pool error in operation %s: %v", poolErr.Op, poolErr.Err)
        // 可能需要重启或降级
    case errors.As(err, &timeoutErr):
        log.Printf("Task timeout after %v", timeoutErr.Duration)
        // 记录超时指标
    default:
        log.Printf("Unknown error: %v", err)
    }
}
```

### 重试机制

```go
type RetryableTask struct {
    originalTask pool.Task
    maxRetries   int
    retryDelay   time.Duration
}

func (rt *RetryableTask) Execute(ctx context.Context) (any, error) {
    var lastErr error
    
    for attempt := 0; attempt <= rt.maxRetries; attempt++ {
        if attempt > 0 {
            // 等待重试延迟
            select {
            case <-time.After(rt.retryDelay):
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
        
        result, err := rt.originalTask.Execute(ctx)
        if err == nil {
            return result, nil
        }
        
        lastErr = err
        
        // 检查是否应该重试
        if !rt.shouldRetry(err) {
            break
        }
        
        log.Printf("Task failed (attempt %d/%d): %v", 
            attempt+1, rt.maxRetries+1, err)
    }
    
    return nil, fmt.Errorf("task failed after %d attempts: %w", 
        rt.maxRetries+1, lastErr)
}

func (rt *RetryableTask) shouldRetry(err error) bool {
    // 定义哪些错误可以重试
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return false // 超时不重试
    case errors.Is(err, context.Canceled):
        return false // 取消不重试
    default:
        return true // 其他错误可以重试
    }
}

func (rt *RetryableTask) Priority() int {
    return rt.originalTask.Priority()
}
```

## 性能调优

### 内存优化

```go
// 使用对象池减少内存分配
type TaskData struct {
    buffer []byte
    items  []Item
}

var taskDataPool = sync.Pool{
    New: func() interface{} {
        return &TaskData{
            buffer: make([]byte, 0, 1024),
            items:  make([]Item, 0, 100),
        }
    },
}

type OptimizedTask struct {
    id string
}

func (t *OptimizedTask) Execute(ctx context.Context) (any, error) {
    // 从对象池获取数据结构
    data := taskDataPool.Get().(*TaskData)
    defer func() {
        // 清理并归还对象池
        data.buffer = data.buffer[:0]
        data.items = data.items[:0]
        taskDataPool.Put(data)
    }()
    
    // 使用 data 进行处理
    return t.processWithData(data), nil
}
```

### 批量处理优化

```go
type BatchProcessor struct {
    pool       pool.Pool
    batchSize  int
    flushTimer *time.Timer
    buffer     []Task
    mu         sync.Mutex
}

func NewBatchProcessor(p pool.Pool, batchSize int, flushInterval time.Duration) *BatchProcessor {
    bp := &BatchProcessor{
        pool:      p,
        batchSize: batchSize,
        buffer:    make([]Task, 0, batchSize),
    }
    
    // 定期刷新缓冲区
    bp.flushTimer = time.AfterFunc(flushInterval, bp.flush)
    
    return bp
}

func (bp *BatchProcessor) Submit(task Task) error {
    bp.mu.Lock()
    defer bp.mu.Unlock()
    
    bp.buffer = append(bp.buffer, task)
    
    if len(bp.buffer) >= bp.batchSize {
        return bp.flushLocked()
    }
    
    return nil
}

func (bp *BatchProcessor) flush() {
    bp.mu.Lock()
    defer bp.mu.Unlock()
    bp.flushLocked()
}

func (bp *BatchProcessor) flushLocked() error {
    if len(bp.buffer) == 0 {
        return nil
    }
    
    // 创建批量任务
    batchTask := &BatchTask{
        tasks: make([]Task, len(bp.buffer)),
    }
    copy(batchTask.tasks, bp.buffer)
    
    // 清空缓冲区
    bp.buffer = bp.buffer[:0]
    
    // 提交批量任务
    return bp.pool.Submit(batchTask)
}
```

### 负载均衡优化

```go
// 自定义负载均衡器
type WeightedRoundRobinBalancer struct {
    workers []WorkerInfo
    current int
    mu      sync.RWMutex
}

type WorkerInfo struct {
    worker pool.Worker
    weight int
    currentWeight int
}

func (wrr *WeightedRoundRobinBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
    wrr.mu.Lock()
    defer wrr.mu.Unlock()
    
    if len(wrr.workers) != len(workers) {
        wrr.initWorkers(workers)
    }
    
    // 加权轮询算法
    totalWeight := 0
    selected := -1
    
    for i := range wrr.workers {
        wrr.workers[i].currentWeight += wrr.workers[i].weight
        totalWeight += wrr.workers[i].weight
        
        if selected == -1 || wrr.workers[i].currentWeight > wrr.workers[selected].currentWeight {
            selected = i
        }
    }
    
    if selected >= 0 {
        wrr.workers[selected].currentWeight -= totalWeight
        return wrr.workers[selected].worker
    }
    
    return workers[0] // 回退到第一个工作协程
}

func (wrr *WeightedRoundRobinBalancer) initWorkers(workers []pool.Worker) {
    wrr.workers = make([]WorkerInfo, len(workers))
    for i, worker := range workers {
        // 根据工作协程的历史性能设置权重
        weight := wrr.calculateWeight(worker)
        wrr.workers[i] = WorkerInfo{
            worker: worker,
            weight: weight,
            currentWeight: 0,
        }
    }
}

func (wrr *WeightedRoundRobinBalancer) calculateWeight(worker pool.Worker) int {
    // 根据工作协程的任务完成数量和成功率计算权重
    taskCount := worker.TaskCount()
    if taskCount == 0 {
        return 1
    }
    
    // 简单的权重计算：任务数量越多权重越高
    weight := int(taskCount / 100)
    if weight < 1 {
        weight = 1
    }
    if weight > 10 {
        weight = 10
    }
    
    return weight
}
```

## 监控和观测

### 指标收集

```go
type MetricsCollector struct {
    pool           pool.Pool
    taskSubmitted  prometheus.Counter
    taskCompleted  prometheus.Counter
    taskFailed     prometheus.Counter
    taskDuration   prometheus.Histogram
    queueSize      prometheus.Gauge
    activeWorkers  prometheus.Gauge
}

func NewMetricsCollector(p pool.Pool) *MetricsCollector {
    mc := &MetricsCollector{
        pool: p,
        taskSubmitted: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "goroutine_pool_tasks_submitted_total",
            Help: "Total number of tasks submitted to the pool",
        }),
        taskCompleted: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "goroutine_pool_tasks_completed_total",
            Help: "Total number of tasks completed by the pool",
        }),
        taskFailed: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "goroutine_pool_tasks_failed_total",
            Help: "Total number of tasks failed in the pool",
        }),
        taskDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "goroutine_pool_task_duration_seconds",
            Help: "Task execution duration in seconds",
            Buckets: prometheus.DefBuckets,
        }),
        queueSize: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "goroutine_pool_queue_size",
            Help: "Current number of tasks in the queue",
        }),
        activeWorkers: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "goroutine_pool_active_workers",
            Help: "Current number of active workers",
        }),
    }
    
    // 注册指标
    prometheus.MustRegister(
        mc.taskSubmitted,
        mc.taskCompleted,
        mc.taskFailed,
        mc.taskDuration,
        mc.queueSize,
        mc.activeWorkers,
    )
    
    // 启动指标收集
    go mc.collectMetrics()
    
    return mc
}

func (mc *MetricsCollector) collectMetrics() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := mc.pool.Stats()
        
        mc.queueSize.Set(float64(stats.QueuedTasks))
        mc.activeWorkers.Set(float64(stats.ActiveWorkers))
        
        // 更新累计指标
        mc.taskCompleted.Add(float64(stats.CompletedTasks))
        mc.taskFailed.Add(float64(stats.FailedTasks))
    }
}
```

### 健康检查

```go
type HealthChecker struct {
    pool           pool.Pool
    maxQueueSize   int64
    maxFailureRate float64
}

func NewHealthChecker(p pool.Pool) *HealthChecker {
    return &HealthChecker{
        pool:           p,
        maxQueueSize:   10000,
        maxFailureRate: 0.1, // 10%
    }
}

func (hc *HealthChecker) CheckHealth() error {
    if !hc.pool.IsRunning() {
        return fmt.Errorf("pool is not running")
    }
    
    stats := hc.pool.Stats()
    
    // 检查队列积压
    if stats.QueuedTasks > hc.maxQueueSize {
        return fmt.Errorf("queue size too large: %d > %d", 
            stats.QueuedTasks, hc.maxQueueSize)
    }
    
    // 检查失败率
    totalTasks := stats.CompletedTasks + stats.FailedTasks
    if totalTasks > 0 {
        failureRate := float64(stats.FailedTasks) / float64(totalTasks)
        if failureRate > hc.maxFailureRate {
            return fmt.Errorf("failure rate too high: %.2f%% > %.2f%%", 
                failureRate*100, hc.maxFailureRate*100)
        }
    }
    
    return nil
}

// HTTP 健康检查端点
func (hc *HealthChecker) HealthHandler(w http.ResponseWriter, r *http.Request) {
    err := hc.CheckHealth()
    if err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  err.Error(),
        })
        return
    }
    
    stats := hc.pool.Stats()
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "healthy",
        "stats":  stats,
    })
}
```

## 生产环境部署

### 优雅启动和关闭

```go
type Application struct {
    pool   pool.Pool
    server *http.Server
    config *AppConfig
}

type AppConfig struct {
    PoolConfig      *pool.Config
    ServerAddr      string
    ShutdownTimeout time.Duration
}

func NewApplication(config *AppConfig) *Application {
    p, err := pool.NewPool(config.PoolConfig)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    
    app := &Application{
        pool:   p,
        config: config,
        server: &http.Server{
            Addr:         config.ServerAddr,
            ReadTimeout:  10 * time.Second,
            WriteTimeout: 10 * time.Second,
        },
    }
    
    // 设置路由
    app.setupRoutes()
    
    return app
}

func (app *Application) Start() error {
    log.Printf("Starting application on %s", app.server.Addr)
    return app.server.ListenAndServe()
}

func (app *Application) Shutdown(ctx context.Context) error {
    log.Println("Shutting down application...")
    
    // 1. 停止接收新的 HTTP 请求
    shutdownCtx, cancel := context.WithTimeout(ctx, app.config.ShutdownTimeout)
    defer cancel()
    
    if err := app.server.Shutdown(shutdownCtx); err != nil {
        log.Printf("HTTP server shutdown error: %v", err)
    }
    
    // 2. 关闭协程池
    if err := app.pool.Shutdown(shutdownCtx); err != nil {
        log.Printf("Pool shutdown error: %v", err)
        return err
    }
    
    log.Println("Application shutdown completed")
    return nil
}

func main() {
    // 配置
    config := &AppConfig{
        PoolConfig: pool.NewConfigBuilder().
            WithWorkerCount(runtime.NumCPU() * 2).
            WithQueueSize(10000).
            WithTaskTimeout(30 * time.Second).
            WithShutdownTimeout(60 * time.Second).
            Build(),
        ServerAddr:      ":8080",
        ShutdownTimeout: 30 * time.Second,
    }
    
    app := NewApplication(config)
    
    // 启动应用
    go func() {
        if err := app.Start(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server failed to start: %v", err)
        }
    }()
    
    // 等待中断信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    // 优雅关闭
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()
    
    if err := app.Shutdown(ctx); err != nil {
        log.Printf("Application shutdown failed: %v", err)
        os.Exit(1)
    }
}
```

### 配置管理

```go
// 使用环境变量和配置文件
type PoolConfigFromEnv struct {
    WorkerCount     int           `env:"POOL_WORKER_COUNT" envDefault:"10"`
    QueueSize       int           `env:"POOL_QUEUE_SIZE" envDefault:"1000"`
    TaskTimeout     time.Duration `env:"POOL_TASK_TIMEOUT" envDefault:"30s"`
    ShutdownTimeout time.Duration `env:"POOL_SHUTDOWN_TIMEOUT" envDefault:"60s"`
    EnableMetrics   bool          `env:"POOL_ENABLE_METRICS" envDefault:"true"`
    MetricsInterval time.Duration `env:"POOL_METRICS_INTERVAL" envDefault:"10s"`
}

func LoadConfigFromEnv() (*pool.Config, error) {
    var envConfig PoolConfigFromEnv
    if err := env.Parse(&envConfig); err != nil {
        return nil, err
    }
    
    return pool.NewConfigBuilder().
        WithWorkerCount(envConfig.WorkerCount).
        WithQueueSize(envConfig.QueueSize).
        WithTaskTimeout(envConfig.TaskTimeout).
        WithShutdownTimeout(envConfig.ShutdownTimeout).
        WithMetrics(envConfig.EnableMetrics).
        WithMetricsInterval(envConfig.MetricsInterval).
        Build()
}
```

## 常见问题和解决方案

### 问题1：任务堆积

**症状**：队列中任务数量持续增长，处理速度跟不上提交速度。

**解决方案**：
```go
// 1. 增加工作协程数量
config := pool.NewConfigBuilder().
    WithWorkerCount(runtime.NumCPU() * 4). // 增加协程数
    Build()

// 2. 实现背压控制
type BackpressureController struct {
    pool          pool.Pool
    maxQueueSize  int64
    checkInterval time.Duration
}

func (bc *BackpressureController) Submit(task pool.Task) error {
    stats := bc.pool.Stats()
    if stats.QueuedTasks > bc.maxQueueSize {
        return fmt.Errorf("queue full, rejecting task")
    }
    return bc.pool.Submit(task)
}

// 3. 实现任务优先级丢弃
func (bc *BackpressureController) SubmitWithDrop(task pool.Task) error {
    err := bc.pool.Submit(task)
    if errors.Is(err, pool.ErrQueueFull) {
        // 丢弃低优先级任务
        if task.Priority() <= pool.PriorityLow {
            log.Printf("Dropping low priority task due to queue full")
            return nil
        }
    }
    return err
}
```

### 问题2：内存泄漏

**症状**：应用内存使用量持续增长。

**解决方案**：
```go
// 1. 确保正确释放资源
type ResourceTask struct {
    file *os.File
    conn net.Conn
}

func (t *ResourceTask) Execute(ctx context.Context) (any, error) {
    // 确保资源被释放
    defer func() {
        if t.file != nil {
            t.file.Close()
        }
        if t.conn != nil {
            t.conn.Close()
        }
    }()
    
    // 执行业务逻辑
    return t.process()
}

// 2. 使用对象池
var taskPool = sync.Pool{
    New: func() interface{} {
        return &ResourceTask{}
    },
}

func NewResourceTask() *ResourceTask {
    return taskPool.Get().(*ResourceTask)
}

func (t *ResourceTask) Release() {
    // 清理状态
    t.file = nil
    t.conn = nil
    taskPool.Put(t)
}
```

### 问题3：死锁

**症状**：应用停止响应，协程池无法处理新任务。

**解决方案**：
```go
// 1. 避免在任务中等待其他任务
// 错误的做法
type BadTask struct {
    pool pool.Pool
}

func (t *BadTask) Execute(ctx context.Context) (any, error) {
    // 这会导致死锁！
    future := t.pool.SubmitAsync(&AnotherTask{})
    return future.Get() // 等待另一个任务完成
}

// 正确的做法
type GoodTask struct {
    dependency TaskResult // 通过外部协调传入依赖结果
}

func (t *GoodTask) Execute(ctx context.Context) (any, error) {
    // 使用已有的依赖结果
    return t.processWithDependency(t.dependency), nil
}

// 2. 使用超时避免无限等待
func (t *SafeTask) Execute(ctx context.Context) (any, error) {
    // 设置超时上下文
    timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    return t.processWithTimeout(timeoutCtx)
}
```

### 问题4：性能下降

**症状**：随着运行时间增长，任务处理速度逐渐下降。

**解决方案**：
```go
// 1. 定期重启工作协程
type RestartablePool struct {
    pool.Pool
    restartInterval time.Duration
    lastRestart     time.Time
}

func (rp *RestartablePool) maybeRestart() {
    if time.Since(rp.lastRestart) > rp.restartInterval {
        // 实现协程池重启逻辑
        rp.restart()
        rp.lastRestart = time.Now()
    }
}

// 2. 监控和告警
type PerformanceMonitor struct {
    pool              pool.Pool
    baselineTPS       float64
    degradationThreshold float64
}

func (pm *PerformanceMonitor) checkPerformance() {
    stats := pm.pool.Stats()
    currentTPS := stats.ThroughputTPS
    
    if currentTPS < pm.baselineTPS * (1 - pm.degradationThreshold) {
        log.Printf("ALERT: Performance degradation detected. Current TPS: %.2f, Baseline: %.2f",
            currentTPS, pm.baselineTPS)
        // 发送告警
    }
}
```

---

通过遵循这些最佳实践，您可以在生产环境中安全、高效地使用 Goroutine Pool SDK。记住要根据具体的业务场景和性能要求调整配置和实现细节。