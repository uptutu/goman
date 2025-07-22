# Goroutine Pool SDK 使用示例

## 概述

本文档提供了 Goroutine Pool SDK 的详细使用示例，涵盖了从基础用法到高级功能的各种场景。

## 目录

- [基础使用](#基础使用)
- [配置管理](#配置管理)
- [任务管理](#任务管理)
- [异步处理](#异步处理)
- [中间件使用](#中间件使用)
- [插件系统](#插件系统)
- [事件监听](#事件监听)
- [错误处理](#错误处理)
- [监控和统计](#监控和统计)
- [性能优化](#性能优化)
- [生产环境实践](#生产环境实践)

## 基础使用

### 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/uptutu/goman/pkg/pool"
)

// SimpleTask 简单任务示例
type SimpleTask struct {
    ID   int
    Data string
}

func (t *SimpleTask) Execute(ctx context.Context) (any, error) {
    // 模拟任务处理
    time.Sleep(100 * time.Millisecond)
    return fmt.Sprintf("Task %d processed: %s", t.ID, t.Data), nil
}

func (t *SimpleTask) Priority() int {
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
    for i := 0; i < 5; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("data-%d", i+1),
        }

        err := p.Submit(task)
        if err != nil {
            log.Printf("Failed to submit task %d: %v", i+1, err)
        }
    }

    // 等待任务完成
    time.Sleep(2 * time.Second)

    // 查看统计信息
    stats := p.Stats()
    fmt.Printf("Completed tasks: %d\n", stats.CompletedTasks)
}
```

### 带超时的任务提交

```go
func timeoutExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    task := &SimpleTask{ID: 1, Data: "timeout-test"}
    
    // 提交带超时的任务
    err := p.SubmitWithTimeout(task, 5*time.Second)
    if err != nil {
        log.Printf("Task submission failed: %v", err)
    }
}
```

## 配置管理

### 使用默认配置

```go
func defaultConfigExample() {
    // 创建默认配置
    config := pool.NewConfig()
    
    fmt.Printf("Default worker count: %d\n", config.WorkerCount)
    fmt.Printf("Default queue size: %d\n", config.QueueSize)
    
    // 使用默认配置创建协程池
    p, err := pool.NewPool(config)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    defer p.Shutdown(context.Background())
}
```

### 自定义配置

```go
func customConfigExample() {
    // 手动创建配置
    config := &pool.Config{
        WorkerCount:     20,
        QueueSize:       2000,
        TaskTimeout:     10 * time.Second,
        ShutdownTimeout: 30 * time.Second,
        EnableMetrics:   true,
        MetricsInterval: 5 * time.Second,
    }

    // 验证配置
    if err := config.Validate(); err != nil {
        log.Fatalf("Invalid config: %v", err)
    }

    p, err := pool.NewPool(config)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    defer p.Shutdown(context.Background())
}
```

### 使用配置构建器

```go
func configBuilderExample() {
    // 使用链式调用构建配置
    config, err := pool.NewConfigBuilder().
        WithWorkerCount(15).
        WithQueueSize(1500).
        WithTaskTimeout(8 * time.Second).
        WithShutdownTimeout(25 * time.Second).
        WithMetrics(true).
        WithMetricsInterval(3 * time.Second).
        WithPreAlloc(true).
        Build()

    if err != nil {
        log.Fatalf("Failed to build config: %v", err)
    }

    p, err := pool.NewPool(config)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    defer p.Shutdown(context.Background())
}
```
## 任务管理

### 不同优先级的任务

```go
// 低优先级任务
type LowPriorityTask struct {
    ID int
}

func (t *LowPriorityTask) Execute(ctx context.Context) (any, error) {
    time.Sleep(200 * time.Millisecond)
    return fmt.Sprintf("Low priority task %d completed", t.ID), nil
}

func (t *LowPriorityTask) Priority() int {
    return pool.PriorityLow
}

// 高优先级任务
type HighPriorityTask struct {
    ID int
}

func (t *HighPriorityTask) Execute(ctx context.Context) (any, error) {
    time.Sleep(50 * time.Millisecond)
    return fmt.Sprintf("High priority task %d completed", t.ID), nil
}

func (t *HighPriorityTask) Priority() int {
    return pool.PriorityHigh
}

// 关键优先级任务
type CriticalTask struct {
    ID int
}

func (t *CriticalTask) Execute(ctx context.Context) (any, error) {
    time.Sleep(30 * time.Millisecond)
    return fmt.Sprintf("Critical task %d completed", t.ID), nil
}

func (t *CriticalTask) Priority() int {
    return pool.PriorityCritical
}

func priorityExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交不同优先级的任务
    tasks := []pool.Task{
        &LowPriorityTask{ID: 1},
        &HighPriorityTask{ID: 1},
        &CriticalTask{ID: 1},
        &LowPriorityTask{ID: 2},
        &HighPriorityTask{ID: 2},
    }

    for _, task := range tasks {
        err := p.Submit(task)
        if err != nil {
            log.Printf("Failed to submit task: %v", err)
        }
    }

    time.Sleep(2 * time.Second)
}
```

### 支持上下文取消的任务

```go
type CancellableTask struct {
    ID       int
    Duration time.Duration
}

func (t *CancellableTask) Execute(ctx context.Context) (any, error) {
    // 创建定时器
    timer := time.NewTimer(t.Duration)
    defer timer.Stop()

    select {
    case <-ctx.Done():
        // 任务被取消
        return nil, ctx.Err()
    case <-timer.C:
        // 任务正常完成
        return fmt.Sprintf("Cancellable task %d completed", t.ID), nil
    }
}

func (t *CancellableTask) Priority() int {
    return pool.PriorityNormal
}

func cancellationExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交长时间运行的任务
    task := &CancellableTask{
        ID:       1,
        Duration: 5 * time.Second,
    }

    future := p.SubmitAsync(task)

    // 等待一段时间后取消任务
    time.Sleep(1 * time.Second)
    cancelled := future.Cancel()
    
    fmt.Printf("Task cancelled: %v\n", cancelled)

    // 尝试获取结果
    result, err := future.GetWithTimeout(100 * time.Millisecond)
    fmt.Printf("Result: %v, Error: %v\n", result, err)
}
```

### 批量任务处理

```go
type BatchTask struct {
    Items []string
    ID    int
}

func (t *BatchTask) Execute(ctx context.Context) (any, error) {
    results := make([]string, 0, len(t.Items))
    
    for i, item := range t.Items {
        // 检查上下文状态
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        // 处理单个项目
        processed := fmt.Sprintf("processed-%s", item)
        results = append(results, processed)

        // 模拟处理时间
        time.Sleep(10 * time.Millisecond)

        // 每处理10个项目检查一次取消状态
        if i%10 == 0 {
            select {
            case <-ctx.Done():
                return results, ctx.Err() // 返回部分结果
            default:
            }
        }
    }

    return results, nil
}

func (t *BatchTask) Priority() int {
    return pool.PriorityNormal
}

func batchProcessingExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 创建批量任务
    items := make([]string, 100)
    for i := range items {
        items[i] = fmt.Sprintf("item-%d", i)
    }

    task := &BatchTask{
        Items: items,
        ID:    1,
    }

    // 异步提交任务
    future := p.SubmitAsync(task)

    // 获取结果
    result, err := future.Get()
    if err != nil {
        log.Printf("Batch task failed: %v", err)
    } else {
        results := result.([]string)
        fmt.Printf("Processed %d items\n", len(results))
    }
}
```

## 异步处理

### 基础异步任务

```go
func basicAsyncExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交多个异步任务
    futures := make([]pool.Future, 5)
    for i := 0; i < 5; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("async-data-%d", i+1),
        }
        futures[i] = p.SubmitAsync(task)
    }

    // 等待所有任务完成
    for i, future := range futures {
        result, err := future.Get()
        if err != nil {
            log.Printf("Task %d failed: %v", i+1, err)
        } else {
            fmt.Printf("Task %d result: %v\n", i+1, result)
        }
    }
}
```

### 带超时的异步任务

```go
func asyncWithTimeoutExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交长时间运行的任务
    task := &CancellableTask{
        ID:       1,
        Duration: 3 * time.Second,
    }

    future := p.SubmitAsync(task)

    // 带超时获取结果
    result, err := future.GetWithTimeout(1 * time.Second)
    if err != nil {
        if err == pool.ErrTaskTimeout {
            fmt.Println("Task timed out")
        } else {
            fmt.Printf("Task failed: %v\n", err)
        }
    } else {
        fmt.Printf("Task result: %v\n", result)
    }
}
```

### 并发等待多个任务

```go
func concurrentWaitExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交多个任务
    futures := make([]pool.Future, 10)
    for i := 0; i < 10; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("concurrent-data-%d", i+1),
        }
        futures[i] = p.SubmitAsync(task)
    }

    // 使用 goroutine 并发等待结果
    resultChan := make(chan string, len(futures))
    errorChan := make(chan error, len(futures))

    for i, future := range futures {
        go func(index int, f pool.Future) {
            result, err := f.GetWithTimeout(5 * time.Second)
            if err != nil {
                errorChan <- fmt.Errorf("task %d failed: %w", index+1, err)
            } else {
                resultChan <- fmt.Sprintf("Task %d: %v", index+1, result)
            }
        }(i, future)
    }

    // 收集结果
    completed := 0
    for completed < len(futures) {
        select {
        case result := <-resultChan:
            fmt.Println(result)
            completed++
        case err := <-errorChan:
            log.Println(err)
            completed++
        case <-time.After(10 * time.Second):
            fmt.Println("Timeout waiting for tasks")
            return
        }
    }
}
```

## 中间件使用

### 使用内置中间件

```go
func builtinMiddlewareExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 添加日志中间件
    loggingMiddleware := pool.NewLoggingMiddleware(nil)
    p.AddMiddleware(loggingMiddleware)

    // 添加指标中间件
    metricsMiddleware := pool.NewMetricsMiddleware()
    p.AddMiddleware(metricsMiddleware)

    // 添加超时中间件
    timeoutMiddleware := pool.NewTimeoutMiddleware(2 * time.Second)
    p.AddMiddleware(timeoutMiddleware)

    // 提交任务
    for i := 0; i < 5; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("middleware-test-%d", i+1),
        }
        p.Submit(task)
    }

    time.Sleep(3 * time.Second)

    // 获取指标
    metrics := metricsMiddleware.GetMetrics()
    fmt.Printf("Metrics - Tasks: %d, Success: %d, Failure: %d, Success Rate: %.2f%%\n",
        metrics.TaskCount, metrics.SuccessCount, metrics.FailureCount, metrics.SuccessRate*100)
}
```

### 自定义中间件

```go
// 性能监控中间件
type PerformanceMiddleware struct {
    slowThreshold time.Duration
}

func NewPerformanceMiddleware(threshold time.Duration) *PerformanceMiddleware {
    return &PerformanceMiddleware{slowThreshold: threshold}
}

func (pm *PerformanceMiddleware) Before(ctx context.Context, task pool.Task) context.Context {
    return context.WithValue(ctx, "perf_start", time.Now())
}

func (pm *PerformanceMiddleware) After(ctx context.Context, task pool.Task, result any, err error) {
    startTime, ok := ctx.Value("perf_start").(time.Time)
    if !ok {
        return
    }

    duration := time.Since(startTime)
    if duration > pm.slowThreshold {
        log.Printf("SLOW TASK: priority=%d, duration=%v", task.Priority(), duration)
    }
}

// 重试中间件
type RetryMiddleware struct {
    maxRetries int
}

func NewRetryMiddleware(maxRetries int) *RetryMiddleware {
    return &RetryMiddleware{maxRetries: maxRetries}
}

func (rm *RetryMiddleware) Before(ctx context.Context, task pool.Task) context.Context {
    return context.WithValue(ctx, "retry_count", 0)
}

func (rm *RetryMiddleware) After(ctx context.Context, task pool.Task, result any, err error) {
    if err == nil {
        return
    }

    retryCount, ok := ctx.Value("retry_count").(int)
    if !ok || retryCount >= rm.maxRetries {
        return
    }

    // 这里可以实现重试逻辑
    log.Printf("Task failed, retry count: %d/%d", retryCount+1, rm.maxRetries)
}

func customMiddlewareExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 添加自定义中间件
    perfMiddleware := NewPerformanceMiddleware(100 * time.Millisecond)
    p.AddMiddleware(perfMiddleware)

    retryMiddleware := NewRetryMiddleware(3)
    p.AddMiddleware(retryMiddleware)

    // 提交任务
    for i := 0; i < 3; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("custom-middleware-%d", i+1),
        }
        p.Submit(task)
    }

    time.Sleep(2 * time.Second)
}
```## 
插件系统

### 使用内置插件

```go
func builtinPluginExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 注册监控插件
    monitoringPlugin := pool.NewMonitoringPlugin("PoolMonitor", 2*time.Second, nil)
    err := p.RegisterPlugin(monitoringPlugin)
    if err != nil {
        log.Printf("Failed to register monitoring plugin: %v", err)
    }

    // 提交一些任务
    for i := 0; i < 10; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("plugin-test-%d", i+1),
        }
        p.Submit(task)
        time.Sleep(100 * time.Millisecond)
    }

    time.Sleep(5 * time.Second)

    // 注销插件
    err = p.UnregisterPlugin("PoolMonitor")
    if err != nil {
        log.Printf("Failed to unregister plugin: %v", err)
    }
}
```

### 自定义插件

```go
// 健康检查插件
type HealthCheckPlugin struct {
    name           string
    pool           pool.Pool
    checkInterval  time.Duration
    ticker         *time.Ticker
    stopChan       chan struct{}
    unhealthyCount int
}

func NewHealthCheckPlugin(name string, interval time.Duration) *HealthCheckPlugin {
    return &HealthCheckPlugin{
        name:          name,
        checkInterval: interval,
        stopChan:      make(chan struct{}),
    }
}

func (hcp *HealthCheckPlugin) Name() string {
    return hcp.name
}

func (hcp *HealthCheckPlugin) Init(p pool.Pool) error {
    hcp.pool = p
    hcp.ticker = time.NewTicker(hcp.checkInterval)
    return nil
}

func (hcp *HealthCheckPlugin) Start() error {
    go hcp.healthCheck()
    return nil
}

func (hcp *HealthCheckPlugin) Stop() error {
    close(hcp.stopChan)
    if hcp.ticker != nil {
        hcp.ticker.Stop()
    }
    return nil
}

func (hcp *HealthCheckPlugin) healthCheck() {
    for {
        select {
        case <-hcp.ticker.C:
            if hcp.pool != nil {
                stats := hcp.pool.Stats()
                
                // 检查健康状态
                if stats.FailedTasks > stats.CompletedTasks/2 {
                    hcp.unhealthyCount++
                    log.Printf("HEALTH WARNING: High failure rate detected (failed: %d, completed: %d)",
                        stats.FailedTasks, stats.CompletedTasks)
                } else {
                    hcp.unhealthyCount = 0
                }

                // 检查队列积压
                if stats.QueuedTasks > 1000 {
                    log.Printf("HEALTH WARNING: High queue backlog (%d tasks)", stats.QueuedTasks)
                }

                // 连续不健康检查
                if hcp.unhealthyCount >= 3 {
                    log.Printf("HEALTH CRITICAL: Pool has been unhealthy for %d checks", hcp.unhealthyCount)
                }
            }
        case <-hcp.stopChan:
            return
        }
    }
}

// 自动扩缩容插件
type AutoScalingPlugin struct {
    name     string
    pool     pool.Pool
    ticker   *time.Ticker
    stopChan chan struct{}
}

func NewAutoScalingPlugin(name string, interval time.Duration) *AutoScalingPlugin {
    return &AutoScalingPlugin{
        name:     name,
        ticker:   time.NewTicker(interval),
        stopChan: make(chan struct{}),
    }
}

func (asp *AutoScalingPlugin) Name() string {
    return asp.name
}

func (asp *AutoScalingPlugin) Init(p pool.Pool) error {
    asp.pool = p
    return nil
}

func (asp *AutoScalingPlugin) Start() error {
    go asp.autoScale()
    return nil
}

func (asp *AutoScalingPlugin) Stop() error {
    close(asp.stopChan)
    asp.ticker.Stop()
    return nil
}

func (asp *AutoScalingPlugin) autoScale() {
    for {
        select {
        case <-asp.ticker.C:
            if asp.pool != nil {
                stats := asp.pool.Stats()
                
                // 简单的扩缩容逻辑示例
                queueRatio := float64(stats.QueuedTasks) / float64(stats.ActiveWorkers)
                
                if queueRatio > 10 {
                    log.Printf("AUTO-SCALING: High queue ratio (%.2f), consider scaling up", queueRatio)
                } else if queueRatio < 1 && stats.ActiveWorkers > 5 {
                    log.Printf("AUTO-SCALING: Low queue ratio (%.2f), consider scaling down", queueRatio)
                }
            }
        case <-asp.stopChan:
            return
        }
    }
}

func customPluginExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 注册健康检查插件
    healthPlugin := NewHealthCheckPlugin("HealthChecker", 3*time.Second)
    err := p.RegisterPlugin(healthPlugin)
    if err != nil {
        log.Printf("Failed to register health plugin: %v", err)
    }

    // 注册自动扩缩容插件
    scalingPlugin := NewAutoScalingPlugin("AutoScaler", 5*time.Second)
    err = p.RegisterPlugin(scalingPlugin)
    if err != nil {
        log.Printf("Failed to register scaling plugin: %v", err)
    }

    // 提交大量任务测试插件
    for i := 0; i < 50; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("scaling-test-%d", i+1),
        }
        p.Submit(task)
    }

    time.Sleep(10 * time.Second)

    // 检查已注册的插件
    if plugin, exists := p.GetPlugin("HealthChecker"); exists {
        fmt.Printf("Found plugin: %s\n", plugin.Name())
    }
}
```

## 事件监听

### 使用内置事件监听器

```go
func builtinEventListenerExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 添加日志事件监听器
    loggingListener := pool.NewLoggingEventListener(nil)
    p.AddEventListener(loggingListener)

    // 提交任务触发事件
    for i := 0; i < 3; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("event-test-%d", i+1),
        }
        p.Submit(task)
    }

    time.Sleep(2 * time.Second)
}
```

### 自定义事件监听器

```go
// 统计事件监听器
type StatsEventListener struct {
    name           string
    taskSubmitted  int64
    taskCompleted  int64
    workerPanics   int64
    mu             sync.RWMutex
}

func NewStatsEventListener(name string) *StatsEventListener {
    return &StatsEventListener{name: name}
}

func (sel *StatsEventListener) OnPoolStart(p pool.Pool) {
    fmt.Printf("[%s] Pool started\n", sel.name)
}

func (sel *StatsEventListener) OnPoolShutdown(p pool.Pool) {
    sel.mu.RLock()
    defer sel.mu.RUnlock()
    fmt.Printf("[%s] Pool shutdown - Submitted: %d, Completed: %d, Panics: %d\n",
        sel.name, sel.taskSubmitted, sel.taskCompleted, sel.workerPanics)
}

func (sel *StatsEventListener) OnTaskSubmit(task pool.Task) {
    sel.mu.Lock()
    defer sel.mu.Unlock()
    sel.taskSubmitted++
    if sel.taskSubmitted%10 == 0 {
        fmt.Printf("[%s] %d tasks submitted\n", sel.name, sel.taskSubmitted)
    }
}

func (sel *StatsEventListener) OnTaskComplete(task pool.Task, result any) {
    sel.mu.Lock()
    defer sel.mu.Unlock()
    sel.taskCompleted++
}

func (sel *StatsEventListener) OnWorkerPanic(workerID int, panicValue any) {
    sel.mu.Lock()
    defer sel.mu.Unlock()
    sel.workerPanics++
    fmt.Printf("[%s] Worker %d panicked: %v (total panics: %d)\n",
        sel.name, workerID, panicValue, sel.workerPanics)
}

func (sel *StatsEventListener) GetStats() (int64, int64, int64) {
    sel.mu.RLock()
    defer sel.mu.RUnlock()
    return sel.taskSubmitted, sel.taskCompleted, sel.workerPanics
}

// 告警事件监听器
type AlertEventListener struct {
    name            string
    alertThreshold  int64
    panicThreshold  int64
    taskCount       int64
    panicCount      int64
    mu              sync.RWMutex
}

func NewAlertEventListener(name string, alertThreshold, panicThreshold int64) *AlertEventListener {
    return &AlertEventListener{
        name:           name,
        alertThreshold: alertThreshold,
        panicThreshold: panicThreshold,
    }
}

func (ael *AlertEventListener) OnPoolStart(p pool.Pool) {
    fmt.Printf("[%s] Pool monitoring started\n", ael.name)
}

func (ael *AlertEventListener) OnPoolShutdown(p pool.Pool) {
    fmt.Printf("[%s] Pool monitoring stopped\n", ael.name)
}

func (ael *AlertEventListener) OnTaskSubmit(task pool.Task) {
    ael.mu.Lock()
    defer ael.mu.Unlock()
    ael.taskCount++
    
    if ael.taskCount%ael.alertThreshold == 0 {
        fmt.Printf("[%s] ALERT: %d tasks have been submitted\n", ael.name, ael.taskCount)
    }
}

func (ael *AlertEventListener) OnTaskComplete(task pool.Task, result any) {
    // 可以在这里添加完成任务的告警逻辑
}

func (ael *AlertEventListener) OnWorkerPanic(workerID int, panicValue any) {
    ael.mu.Lock()
    defer ael.mu.Unlock()
    ael.panicCount++
    
    fmt.Printf("[%s] CRITICAL: Worker %d panicked: %v\n", ael.name, workerID, panicValue)
    
    if ael.panicCount >= ael.panicThreshold {
        fmt.Printf("[%s] EMERGENCY: Too many panics (%d), system may be unstable!\n",
            ael.name, ael.panicCount)
    }
}

func customEventListenerExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 添加统计事件监听器
    statsListener := NewStatsEventListener("StatsListener")
    p.AddEventListener(statsListener)

    // 添加告警事件监听器
    alertListener := NewAlertEventListener("AlertListener", 5, 2)
    p.AddEventListener(alertListener)

    // 提交任务
    for i := 0; i < 15; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("event-listener-test-%d", i+1),
        }
        p.Submit(task)
    }

    time.Sleep(3 * time.Second)

    // 获取统计信息
    submitted, completed, panics := statsListener.GetStats()
    fmt.Printf("Final stats - Submitted: %d, Completed: %d, Panics: %d\n",
        submitted, completed, panics)
}
```## 
错误处理

### 基础错误处理

```go
func basicErrorHandlingExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    task := &SimpleTask{ID: 1, Data: "test"}

    // 提交任务并处理错误
    err := p.Submit(task)
    if err != nil {
        switch {
        case errors.Is(err, pool.ErrQueueFull):
            fmt.Println("Queue is full, try again later")
        case errors.Is(err, pool.ErrPoolClosed):
            fmt.Println("Pool is closed")
        case errors.Is(err, pool.ErrShutdownInProgress):
            fmt.Println("Pool is shutting down")
        default:
            fmt.Printf("Unknown error: %v\n", err)
        }
    }
}
```

### 结构化错误处理

```go
func structuredErrorHandlingExample() {
    // 创建无效配置触发配置错误
    config := &pool.Config{
        WorkerCount: -1, // 无效值
        QueueSize:   0,  // 无效值
    }

    p, err := pool.NewPool(config)
    if err != nil {
        // 处理配置错误
        var configErr *pool.ConfigError
        if errors.As(err, &configErr) {
            fmt.Printf("Config error in field '%s' with value '%v': %v\n",
                configErr.Field, configErr.Value, configErr.Err)
        }

        // 处理协程池错误
        var poolErr *pool.PoolError
        if errors.As(err, &poolErr) {
            fmt.Printf("Pool operation '%s' failed: %v\n", poolErr.Op, poolErr.Err)
        }

        return
    }

    defer p.Shutdown(context.Background())
}
```

### 任务错误处理

```go
// 可能失败的任务
type FailableTask struct {
    ID          int
    ShouldFail  bool
    ShouldPanic bool
}

func (t *FailableTask) Execute(ctx context.Context) (any, error) {
    if t.ShouldPanic {
        panic(fmt.Sprintf("Task %d panicked intentionally", t.ID))
    }

    if t.ShouldFail {
        return nil, fmt.Errorf("task %d failed intentionally", t.ID)
    }

    return fmt.Sprintf("Task %d completed successfully", t.ID), nil
}

func (t *FailableTask) Priority() int {
    return pool.PriorityNormal
}

func taskErrorHandlingExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 提交不同类型的任务
    tasks := []*FailableTask{
        {ID: 1, ShouldFail: false, ShouldPanic: false}, // 成功
        {ID: 2, ShouldFail: true, ShouldPanic: false},  // 失败
        {ID: 3, ShouldFail: false, ShouldPanic: true},  // panic
        {ID: 4, ShouldFail: false, ShouldPanic: false}, // 成功
    }

    futures := make([]pool.Future, len(tasks))
    for i, task := range tasks {
        futures[i] = p.SubmitAsync(task)
    }

    // 处理结果和错误
    for i, future := range futures {
        result, err := future.Get()
        if err != nil {
            // 检查不同类型的错误
            var panicErr *pool.PanicError
            var timeoutErr *pool.TimeoutError
            var taskErr *pool.TaskError

            switch {
            case errors.As(err, &panicErr):
                fmt.Printf("Task %d panicked: %v\n", i+1, panicErr.Value)
            case errors.As(err, &timeoutErr):
                fmt.Printf("Task %d timed out after %v\n", i+1, timeoutErr.Duration)
            case errors.As(err, &taskErr):
                fmt.Printf("Task %d failed: %v\n", i+1, taskErr.Err)
            case errors.Is(err, pool.ErrTaskCancelled):
                fmt.Printf("Task %d was cancelled\n", i+1)
            default:
                fmt.Printf("Task %d failed with unknown error: %v\n", i+1, err)
            }
        } else {
            fmt.Printf("Task %d result: %v\n", i+1, result)
        }
    }
}
```

### 自定义错误处理器

```go
// 自定义错误处理器
type CustomErrorHandler struct {
    panicCount     int64
    timeoutCount   int64
    queueFullCount int64
    mu             sync.RWMutex
}

func NewCustomErrorHandler() *CustomErrorHandler {
    return &CustomErrorHandler{}
}

func (ceh *CustomErrorHandler) HandlePanic(workerID int, task pool.Task, panicValue any) {
    ceh.mu.Lock()
    defer ceh.mu.Unlock()
    ceh.panicCount++
    
    log.Printf("PANIC HANDLER: Worker %d panicked while executing task (priority: %d): %v",
        workerID, task.Priority(), panicValue)
    
    // 可以在这里添加告警、重启逻辑等
    if ceh.panicCount > 10 {
        log.Printf("WARNING: Too many panics (%d), system may be unstable", ceh.panicCount)
    }
}

func (ceh *CustomErrorHandler) HandleTimeout(task pool.Task, duration time.Duration) {
    ceh.mu.Lock()
    defer ceh.mu.Unlock()
    ceh.timeoutCount++
    
    log.Printf("TIMEOUT HANDLER: Task timed out after %v (priority: %d)",
        duration, task.Priority())
}

func (ceh *CustomErrorHandler) HandleQueueFull(task pool.Task) error {
    ceh.mu.Lock()
    defer ceh.mu.Unlock()
    ceh.queueFullCount++
    
    log.Printf("QUEUE FULL HANDLER: Queue full when submitting task (priority: %d)",
        task.Priority())
    
    // 可以实现重试逻辑或降级策略
    if task.Priority() >= pool.PriorityHigh {
        // 高优先级任务等待重试
        time.Sleep(100 * time.Millisecond)
        return nil // 返回nil表示重试
    }
    
    return pool.ErrQueueFull // 返回错误表示拒绝
}

func (ceh *CustomErrorHandler) GetStats() pool.ErrorStats {
    ceh.mu.RLock()
    defer ceh.mu.RUnlock()
    
    return pool.ErrorStats{
        PanicCount:     ceh.panicCount,
        TimeoutCount:   ceh.timeoutCount,
        QueueFullCount: ceh.queueFullCount,
    }
}

func customErrorHandlerExample() {
    // 创建带自定义错误处理器的配置
    errorHandler := NewCustomErrorHandler()
    
    config := pool.NewConfigBuilder().
        WithWorkerCount(2).
        WithQueueSize(5). // 小队列容易触发队列满
        WithTaskTimeout(1 * time.Second).
        Build()

    p, _ := pool.NewPool(config)
    defer p.Shutdown(context.Background())

    // 提交大量任务测试队列满处理
    for i := 0; i < 20; i++ {
        task := &FailableTask{
            ID:          i + 1,
            ShouldFail:  i%5 == 0,  // 每5个任务失败一个
            ShouldPanic: i%7 == 0,  // 每7个任务panic一个
        }

        err := p.Submit(task)
        if err != nil {
            fmt.Printf("Failed to submit task %d: %v\n", i+1, err)
        }
    }

    time.Sleep(5 * time.Second)

    // 获取错误统计
    stats := errorHandler.GetStats()
    fmt.Printf("Error stats - Panics: %d, Timeouts: %d, Queue Full: %d\n",
        stats.PanicCount, stats.TimeoutCount, stats.QueueFullCount)
}
```

## 监控和统计

### 基础监控

```go
func basicMonitoringExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 启动监控协程
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                stats := p.Stats()
                fmt.Printf("Stats - Active: %d, Queued: %d, Completed: %d, Failed: %d, TPS: %.2f\n",
                    stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks,
                    stats.FailedTasks, stats.ThroughputTPS)
            }
        }
    }()

    // 提交任务
    for i := 0; i < 20; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("monitoring-test-%d", i+1),
        }
        p.Submit(task)
        time.Sleep(100 * time.Millisecond)
    }

    time.Sleep(5 * time.Second)
}
```

### 详细监控

```go
// 详细监控器
type DetailedMonitor struct {
    pool           pool.Pool
    ticker         *time.Ticker
    stopChan       chan struct{}
    taskHistory    []TaskRecord
    mu             sync.RWMutex
}

type TaskRecord struct {
    SubmitTime    time.Time
    CompleteTime  time.Time
    Duration      time.Duration
    Priority      int
    Success       bool
    Error         error
}

func NewDetailedMonitor(p pool.Pool, interval time.Duration) *DetailedMonitor {
    return &DetailedMonitor{
        pool:        p,
        ticker:      time.NewTicker(interval),
        stopChan:    make(chan struct{}),
        taskHistory: make([]TaskRecord, 0),
    }
}

func (dm *DetailedMonitor) Start() {
    go dm.monitor()
}

func (dm *DetailedMonitor) Stop() {
    close(dm.stopChan)
    dm.ticker.Stop()
}

func (dm *DetailedMonitor) RecordTask(submitTime, completeTime time.Time, priority int, success bool, err error) {
    dm.mu.Lock()
    defer dm.mu.Unlock()

    record := TaskRecord{
        SubmitTime:   submitTime,
        CompleteTime: completeTime,
        Duration:     completeTime.Sub(submitTime),
        Priority:     priority,
        Success:      success,
        Error:        err,
    }

    dm.taskHistory = append(dm.taskHistory, record)

    // 保持历史记录在合理范围内
    if len(dm.taskHistory) > 1000 {
        dm.taskHistory = dm.taskHistory[100:]
    }
}

func (dm *DetailedMonitor) monitor() {
    for {
        select {
        case <-dm.ticker.C:
            dm.printDetailedStats()
        case <-dm.stopChan:
            return
        }
    }
}

func (dm *DetailedMonitor) printDetailedStats() {
    stats := dm.pool.Stats()
    
    dm.mu.RLock()
    historyLen := len(dm.taskHistory)
    
    if historyLen == 0 {
        dm.mu.RUnlock()
        return
    }

    // 计算最近任务的统计信息
    recentTasks := dm.taskHistory
    if historyLen > 100 {
        recentTasks = dm.taskHistory[historyLen-100:]
    }

    var totalDuration time.Duration
    var successCount, failCount int
    priorityCount := make(map[int]int)

    for _, record := range recentTasks {
        totalDuration += record.Duration
        if record.Success {
            successCount++
        } else {
            failCount++
        }
        priorityCount[record.Priority]++
    }

    avgDuration := totalDuration / time.Duration(len(recentTasks))
    successRate := float64(successCount) / float64(len(recentTasks)) * 100

    dm.mu.RUnlock()

    fmt.Printf("=== Detailed Pool Stats ===\n")
    fmt.Printf("Pool Status - Active: %d, Queued: %d, TPS: %.2f\n",
        stats.ActiveWorkers, stats.QueuedTasks, stats.ThroughputTPS)
    fmt.Printf("Recent Tasks (%d) - Success Rate: %.1f%%, Avg Duration: %v\n",
        len(recentTasks), successRate, avgDuration)
    fmt.Printf("Priority Distribution - Low: %d, Normal: %d, High: %d, Critical: %d\n",
        priorityCount[pool.PriorityLow], priorityCount[pool.PriorityNormal],
        priorityCount[pool.PriorityHigh], priorityCount[pool.PriorityCritical])
    fmt.Printf("Memory Usage: %d bytes, GC Count: %d\n",
        stats.MemoryUsage, stats.GCCount)
    fmt.Println("===========================")
}

func detailedMonitoringExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 启动详细监控
    monitor := NewDetailedMonitor(p, 3*time.Second)
    monitor.Start()
    defer monitor.Stop()

    // 模拟任务记录（在实际使用中，这应该通过中间件或事件监听器来实现）
    go func() {
        for i := 0; i < 30; i++ {
            submitTime := time.Now()
            
            task := &SimpleTask{
                ID:   i + 1,
                Data: fmt.Sprintf("detailed-monitoring-%d", i+1),
            }
            
            future := p.SubmitAsync(task)
            
            go func(st time.Time, priority int) {
                result, err := future.Get()
                completeTime := time.Now()
                success := err == nil && result != nil
                monitor.RecordTask(st, completeTime, priority, success, err)
            }(submitTime, task.Priority())
            
            time.Sleep(200 * time.Millisecond)
        }
    }()

    time.Sleep(15 * time.Second)
}
```## 性能优
化

### 对象池优化

```go
func objectPoolOptimizationExample() {
    // 配置对象池以减少内存分配
    config := pool.NewConfigBuilder().
        WithWorkerCount(10).
        WithQueueSize(1000).
        WithObjectPoolSize(500).  // 增大对象池
        WithPreAlloc(true).       // 启用预分配
        Build()

    p, _ := pool.NewPool(config)
    defer p.Shutdown(context.Background())

    // 提交大量任务测试对象池效果
    start := time.Now()
    
    for i := 0; i < 1000; i++ {
        task := &SimpleTask{
            ID:   i + 1,
            Data: fmt.Sprintf("object-pool-test-%d", i+1),
        }
        p.Submit(task)
    }

    // 等待所有任务完成
    for {
        stats := p.Stats()
        if stats.CompletedTasks >= 1000 {
            break
        }
        time.Sleep(10 * time.Millisecond)
    }

    duration := time.Since(start)
    fmt.Printf("Processed 1000 tasks in %v\n", duration)
}
```

### 批量处理优化

```go
// 批量处理任务
type BatchProcessingTask struct {
    Items    []string
    BatchID  int
    Callback func([]string) error
}

func (t *BatchProcessingTask) Execute(ctx context.Context) (any, error) {
    // 批量处理所有项目
    results := make([]string, 0, len(t.Items))
    
    for _, item := range t.Items {
        // 检查上下文取消
        select {
        case <-ctx.Done():
            return results, ctx.Err()
        default:
        }
        
        // 处理单个项目
        processed := fmt.Sprintf("processed-%s", item)
        results = append(results, processed)
    }

    // 执行回调
    if t.Callback != nil {
        if err := t.Callback(results); err != nil {
            return results, err
        }
    }

    return results, nil
}

func (t *BatchProcessingTask) Priority() int {
    return pool.PriorityNormal
}

func batchOptimizationExample() {
    p, _ := pool.NewPool(nil)
    defer p.Shutdown(context.Background())

    // 准备大量数据
    allItems := make([]string, 10000)
    for i := range allItems {
        allItems[i] = fmt.Sprintf("item-%d", i)
    }

    // 分批处理
    batchSize := 100
    start := time.Now()

    var wg sync.WaitGroup
    for i := 0; i < len(allItems); i += batchSize {
        end := i + batchSize
        if end > len(allItems) {
            end = len(allItems)
        }

        batch := allItems[i:end]
        
        task := &BatchProcessingTask{
            Items:   batch,
            BatchID: i / batchSize,
            Callback: func(results []string) error {
                // 处理批量结果
                fmt.Printf("Batch processed: %d items\n", len(results))
                return nil
            },
        }

        wg.Add(1)
        future := p.SubmitAsync(task)
        
        go func() {
            defer wg.Done()
            _, err := future.Get()
            if err != nil {
                log.Printf("Batch processing failed: %v", err)
            }
        }()
    }

    wg.Wait()
    duration := time.Since(start)
    fmt.Printf("Batch processed %d items in %v\n", len(allItems), duration)
}
```

### 内存优化

```go
// 内存友好的任务
type MemoryEfficientTask struct {
    ID   int
    data []byte // 使用字节切片而不是字符串
}

func (t *MemoryEfficientTask) Execute(ctx context.Context) (any, error) {
    // 在栈上处理数据，避免不必要的堆分配
    var result [64]byte
    copy(result[:], t.data)
    
    // 处理完成后立即清理
    t.data = nil
    
    return result[:len(t.data)], nil
}

func (t *MemoryEfficientTask) Priority() int {
    return pool.PriorityNormal
}

func memoryOptimizationExample() {
    // 配置较小的对象池以观察内存使用
    config := pool.NewConfigBuilder().
        WithWorkerCount(5).
        WithQueueSize(100).
        WithObjectPoolSize(50).
        WithMetrics(true).
        WithMetricsInterval(1 * time.Second).
        Build()

    p, _ := pool.NewPool(config)
    defer p.Shutdown(context.Background())

    // 监控内存使用
    go func() {
        ticker := time.NewTicker(2 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                stats := p.Stats()
                fmt.Printf("Memory usage: %d bytes, GC count: %d\n",
                    stats.MemoryUsage, stats.GCCount)
                
                // 手动触发GC以观察效果
                runtime.GC()
            }
        }
    }()

    // 提交内存友好的任务
    for i := 0; i < 100; i++ {
        data := make([]byte, 32)
        for j := range data {
            data[j] = byte(i + j)
        }

        task := &MemoryEfficientTask{
            ID:   i + 1,
            data: data,
        }

        p.Submit(task)
        time.Sleep(50 * time.Millisecond)
    }

    time.Sleep(5 * time.Second)
}
```

## 生产环境实践

### 完整的生产环境示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "runtime"
    "sync"
    "syscall"
    "time"

    "github.com/uptutu/goman/pkg/pool"
)

// ProductionTask 生产环境任务示例
type ProductionTask struct {
    ID       string
    Type     string
    Payload  map[string]interface{}
    Retries  int
    MaxRetries int
}

func (t *ProductionTask) Execute(ctx context.Context) (any, error) {
    // 检查上下文取消
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // 模拟不同类型的任务处理
    switch t.Type {
    case "email":
        return t.processEmail(ctx)
    case "notification":
        return t.processNotification(ctx)
    case "data_processing":
        return t.processData(ctx)
    default:
        return nil, fmt.Errorf("unknown task type: %s", t.Type)
    }
}

func (t *ProductionTask) processEmail(ctx context.Context) (any, error) {
    // 模拟邮件发送
    time.Sleep(100 * time.Millisecond)
    
    // 模拟偶尔失败
    if t.ID == "email-fail" {
        return nil, fmt.Errorf("failed to send email")
    }
    
    return fmt.Sprintf("Email sent: %s", t.ID), nil
}

func (t *ProductionTask) processNotification(ctx context.Context) (any, error) {
    // 模拟推送通知
    time.Sleep(50 * time.Millisecond)
    return fmt.Sprintf("Notification sent: %s", t.ID), nil
}

func (t *ProductionTask) processData(ctx context.Context) (any, error) {
    // 模拟数据处理
    time.Sleep(200 * time.Millisecond)
    
    // 检查上下文取消
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    return fmt.Sprintf("Data processed: %s", t.ID), nil
}

func (t *ProductionTask) Priority() int {
    switch t.Type {
    case "email":
        return pool.PriorityHigh
    case "notification":
        return pool.PriorityCritical
    case "data_processing":
        return pool.PriorityNormal
    default:
        return pool.PriorityLow
    }
}

// ProductionApp 生产环境应用
type ProductionApp struct {
    pool     pool.Pool
    monitor  *ProductionMonitor
    shutdown chan os.Signal
    wg       sync.WaitGroup
}

// ProductionMonitor 生产环境监控
type ProductionMonitor struct {
    pool         pool.Pool
    ticker       *time.Ticker
    stopChan     chan struct{}
    alertChannel chan string
}

func NewProductionMonitor(p pool.Pool) *ProductionMonitor {
    return &ProductionMonitor{
        pool:         p,
        ticker:       time.NewTicker(10 * time.Second),
        stopChan:     make(chan struct{}),
        alertChannel: make(chan string, 100),
    }
}

func (pm *ProductionMonitor) Start() {
    go pm.monitor()
    go pm.handleAlerts()
}

func (pm *ProductionMonitor) Stop() {
    close(pm.stopChan)
    pm.ticker.Stop()
    close(pm.alertChannel)
}

func (pm *ProductionMonitor) monitor() {
    for {
        select {
        case <-pm.ticker.C:
            stats := pm.pool.Stats()
            
            // 记录关键指标
            log.Printf("Pool Stats - Active: %d, Queued: %d, Completed: %d, Failed: %d, TPS: %.2f, Memory: %d",
                stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks,
                stats.FailedTasks, stats.ThroughputTPS, stats.MemoryUsage)
            
            // 检查告警条件
            if stats.QueuedTasks > 1000 {
                pm.alertChannel <- fmt.Sprintf("High queue backlog: %d tasks", stats.QueuedTasks)
            }
            
            if stats.FailedTasks > 0 && stats.CompletedTasks > 0 {
                failureRate := float64(stats.FailedTasks) / float64(stats.CompletedTasks+stats.FailedTasks)
                if failureRate > 0.1 { // 10% 失败率
                    pm.alertChannel <- fmt.Sprintf("High failure rate: %.2f%%", failureRate*100)
                }
            }
            
            if stats.ThroughputTPS < 1.0 && stats.QueuedTasks > 0 {
                pm.alertChannel <- "Low throughput detected"
            }
            
        case <-pm.stopChan:
            return
        }
    }
}

func (pm *ProductionMonitor) handleAlerts() {
    for alert := range pm.alertChannel {
        log.Printf("ALERT: %s", alert)
        // 在实际生产环境中，这里可以发送到告警系统
    }
}

func NewProductionApp() *ProductionApp {
    // 根据系统资源配置协程池
    workerCount := runtime.NumCPU() * 2
    queueSize := workerCount * 100

    config := pool.NewConfigBuilder().
        WithWorkerCount(workerCount).
        WithQueueSize(queueSize).
        WithTaskTimeout(30 * time.Second).
        WithShutdownTimeout(60 * time.Second).
        WithMetrics(true).
        WithMetricsInterval(5 * time.Second).
        WithObjectPoolSize(queueSize / 10).
        WithPreAlloc(true).
        Build()

    p, err := pool.NewPool(config)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }

    // 添加生产环境中间件
    loggingMiddleware := pool.NewLoggingMiddleware(nil)
    p.AddMiddleware(loggingMiddleware)

    metricsMiddleware := pool.NewMetricsMiddleware()
    p.AddMiddleware(metricsMiddleware)

    // 添加监控插件
    monitoringPlugin := pool.NewMonitoringPlugin("ProductionMonitor", 30*time.Second, nil)
    p.RegisterPlugin(monitoringPlugin)

    app := &ProductionApp{
        pool:     p,
        monitor:  NewProductionMonitor(p),
        shutdown: make(chan os.Signal, 1),
    }

    // 监听系统信号
    signal.Notify(app.shutdown, syscall.SIGINT, syscall.SIGTERM)

    return app
}

func (app *ProductionApp) Start() {
    log.Println("Starting production application...")
    
    // 启动监控
    app.monitor.Start()
    
    // 启动任务处理器
    app.wg.Add(1)
    go app.taskProcessor()
    
    // 等待关闭信号
    <-app.shutdown
    log.Println("Received shutdown signal, starting graceful shutdown...")
    
    app.Stop()
}

func (app *ProductionApp) Stop() {
    // 停止监控
    app.monitor.Stop()
    
    // 优雅关闭协程池
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()
    
    if err := app.pool.Shutdown(ctx); err != nil {
        log.Printf("Failed to shutdown pool gracefully: %v", err)
    } else {
        log.Println("Pool shutdown successfully")
    }
    
    // 等待所有协程结束
    app.wg.Wait()
    log.Println("Application stopped")
}

func (app *ProductionApp) taskProcessor() {
    defer app.wg.Done()
    
    // 模拟任务生成
    taskTypes := []string{"email", "notification", "data_processing"}
    
    for i := 0; i < 1000; i++ {
        select {
        case <-app.shutdown:
            return
        default:
        }
        
        taskType := taskTypes[i%len(taskTypes)]
        task := &ProductionTask{
            ID:         fmt.Sprintf("%s-%d", taskType, i),
            Type:       taskType,
            Payload:    map[string]interface{}{"data": fmt.Sprintf("payload-%d", i)},
            MaxRetries: 3,
        }
        
        // 提交任务，处理队列满的情况
        err := app.pool.Submit(task)
        if err != nil {
            if errors.Is(err, pool.ErrQueueFull) {
                // 队列满时等待一段时间再重试
                time.Sleep(100 * time.Millisecond)
                continue
            }
            log.Printf("Failed to submit task %s: %v", task.ID, err)
        }
        
        // 控制任务提交速率
        time.Sleep(10 * time.Millisecond)
    }
}

func productionExample() {
    app := NewProductionApp()
    app.Start()
}

func main() {
    // 设置日志格式
    log.SetFlags(log.LstdFlags | log.Lshortfile)
    
    // 运行生产环境示例
    productionExample()
}
```

### 性能基准测试

```go
func benchmarkExample() {
    // 不同配置的性能测试
    configs := []*pool.Config{
        // 配置1: 少量工作协程
        pool.NewConfigBuilder().WithWorkerCount(2).WithQueueSize(100).Build(),
        // 配置2: 中等工作协程
        pool.NewConfigBuilder().WithWorkerCount(10).WithQueueSize(1000).Build(),
        // 配置3: 大量工作协程
        pool.NewConfigBuilder().WithWorkerCount(50).WithQueueSize(5000).Build(),
    }

    taskCount := 10000

    for i, config := range configs {
        fmt.Printf("\n=== Benchmark %d: %d workers, %d queue size ===\n", 
            i+1, config.WorkerCount, config.QueueSize)
        
        p, _ := pool.NewPool(config)
        
        start := time.Now()
        
        // 提交任务
        for j := 0; j < taskCount; j++ {
            task := &SimpleTask{
                ID:   j + 1,
                Data: fmt.Sprintf("benchmark-task-%d", j+1),
            }
            p.Submit(task)
        }
        
        // 等待完成
        for {
            stats := p.Stats()
            if stats.CompletedTasks >= int64(taskCount) {
                break
            }
            time.Sleep(10 * time.Millisecond)
        }
        
        duration := time.Since(start)
        tps := float64(taskCount) / duration.Seconds()
        
        fmt.Printf("Duration: %v\n", duration)
        fmt.Printf("TPS: %.2f\n", tps)
        
        finalStats := p.Stats()
        fmt.Printf("Final stats - Completed: %d, Failed: %d\n", 
            finalStats.CompletedTasks, finalStats.FailedTasks)
        
        p.Shutdown(context.Background())
    }
}
```

---

本文档提供了 Goroutine Pool SDK 的全面使用示例，涵盖了从基础用法到生产环境部署的各种场景。这些示例可以帮助开发者快速上手并在实际项目中有效使用该 SDK。

更多高级用法和最佳实践，请参考 [API 文档](api.md) 和 [性能调优指南](performance-tuning.md)。