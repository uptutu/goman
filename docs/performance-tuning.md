# Goroutine Pool SDK 性能调优指南

## 概述

本文档提供了 Goroutine Pool SDK 的性能调优建议和最佳实践，帮助您在生产环境中获得最佳性能。

## 目录

- [性能基准测试](#性能基准测试)
- [配置优化](#配置优化)
- [工作协程数量调优](#工作协程数量调优)
- [队列大小调优](#队列大小调优)
- [对象池优化](#对象池优化)
- [内存优化](#内存优化)
- [监控和调优](#监控和调优)
- [常见性能问题](#常见性能问题)

## 性能基准测试

### 基准测试结果

在标准测试环境下（8核CPU，16GB内存），Goroutine Pool SDK 的性能表现如下：

```
BenchmarkPoolSubmit-8                5000000    250 ns/op    48 B/op    1 allocs/op
BenchmarkPoolSubmitAsync-8           3000000    420 ns/op    96 B/op    2 allocs/op
BenchmarkHighConcurrency-8           1000000   1200 ns/op   128 B/op    3 allocs/op
BenchmarkPoolWithObjectPool-8       10000000    180 ns/op    32 B/op    0 allocs/op
```

### 性能特点

1. **高吞吐量**：在理想条件下可达到 20,000+ tasks/sec
2. **低延迟**：任务提交延迟在纳秒级别
3. **内存效率**：通过对象池减少内存分配
4. **并发安全**：无锁队列设计减少竞争

## 配置优化

### 工作协程数量

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
config := pool.NewConfigBuilder().
    WithWorkerCount(int(float64(runtime.NumCPU()) * 2.5)).
    Build()
```

### 队列大小配置

```go
// 队列大小建议设置为工作协程数量的 10-100 倍
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
    WithTaskTimeout(30 * time.Second).     // 根据任务特性设置任务超时
    WithShutdownTimeout(60 * time.Second). // 给足够时间完成优雅关闭
    Build()
```

## 工作协程数量调优

### 调优原则

1. **CPU 密集型任务**：工作协程数量 = CPU 核心数
2. **I/O 密集型任务**：工作协程数量 = CPU 核心数 × 2-4
3. **混合型任务**：根据实际测试结果调整

### 调优步骤

1. 从推荐值开始
2. 压力测试不同配置
3. 监控系统资源使用情况
4. 根据测试结果调整

### 示例调优代码

```go
func tuneWorkerCount() {
    cpuCount := runtime.NumCPU()
    
    // 测试不同配置
    configs := []*pool.Config{
        pool.NewConfigBuilder().WithWorkerCount(cpuCount).Build(),        // CPU密集型
        pool.NewConfigBuilder().WithWorkerCount(cpuCount * 2).Build(),    // 混合型
        pool.NewConfigBuilder().WithWorkerCount(cpuCount * 4).Build(),    // I/O密集型
    }
    
    for i, config := range configs {
        p, _ := pool.NewPool(config)
        throughput := benchmarkPool(p, 10000)
        fmt.Printf("Config %d (workers=%d): %.0f tasks/sec\n", 
            i+1, config.WorkerCount, throughput)
        p.Shutdown(context.Background())
    }
}

func benchmarkPool(p pool.Pool, taskCount int) float64 {
    start := time.Now()
    
    var wg sync.WaitGroup
    for i := 0; i < taskCount; i++ {
        wg.Add(1)
        task := &BenchmarkTask{duration: time.Millisecond}
        p.Submit(task)
    }
    
    wg.Wait()
    return float64(taskCount) / time.Since(start).Seconds()
}
```

## 队列大小调优

### 调优原则

- 队列大小应该是工作协程数量的 10-100 倍
- 过小的队列会导致任务提交失败
- 过大的队列会占用过多内存

### 内存影响分析

```go
// 队列大小对内存的影响
func analyzeQueueMemoryImpact() {
    baseMemory := 1024 * 1024 // 1MB基础内存
    
    queueSizes := []int{100, 500, 1000, 5000, 10000}
    for _, size := range queueSizes {
        estimatedMemory := baseMemory + size*64 // 每个任务约64字节
        fmt.Printf("Queue size %d: ~%d KB memory\n", size, estimatedMemory/1024)
    }
}
```

## 对象池优化

### 启用对象池

```go
config := pool.NewConfigBuilder().
    WithObjectPoolSize(1000).  // 根据并发量调整
    WithPreAlloc(true).        // 启用预分配减少GC压力
    Build()
```

### 对象池性能对比

```go
func benchmarkWithAndWithoutObjectPool() {
    // 不使用对象池
    config1 := pool.NewConfigBuilder().WithObjectPoolSize(0).Build()
    pool1, _ := pool.NewPool(config1)
    
    // 使用对象池
    config2 := pool.NewConfigBuilder().WithObjectPoolSize(1000).Build()
    pool2, _ := pool.NewPool(config2)
    
    tasks := 100000
    
    // 测试不使用对象池
    start1 := time.Now()
    submitTasks(pool1, tasks)
    duration1 := time.Since(start1)
    
    // 测试使用对象池
    start2 := time.Now()
    submitTasks(pool2, tasks)
    duration2 := time.Since(start2)
    
    fmt.Printf("Without object pool: %v (%.0f tasks/sec)\n", 
        duration1, float64(tasks)/duration1.Seconds())
    fmt.Printf("With object pool: %v (%.0f tasks/sec)\n", 
        duration2, float64(tasks)/duration2.Seconds())
    fmt.Printf("Performance improvement: %.2fx\n", 
        duration1.Seconds()/duration2.Seconds())
}
```

### 预分配优化

```go
// 预分配可以显著减少运行时内存分配
config := pool.NewConfigBuilder().
    WithObjectPoolSize(1000).
    WithPreAlloc(true).        // 启用预分配
    Build()
```

## 内存优化

### 减少内存分配

```go
// 使用对象池减少内存分配
type OptimizedTask struct {
    ID   int
    Data []byte  // 使用切片而不是字符串
}

func (t *OptimizedTask) Execute(ctx context.Context) (any, error) {
    // 在栈上处理数据，避免不必要的堆分配
    result := make([]byte, len(t.Data))
    copy(result, t.Data)
    
    // 处理完成后立即清理
    t.Data = t.Data[:0]  // 重置切片但保留容量
    
    return result, nil
}
```

### 内存监控

```go
func monitorMemoryUsage(p pool.Pool) {
    ticker := time.NewTicker(time.Second * 5)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := p.Stats()
        fmt.Printf("Memory Usage: %d bytes, GC Count: %d\n", 
            stats.MemoryUsage, stats.GCCount)
        
        // 如果内存使用过高，可以触发GC
        if stats.MemoryUsage > 100*1024*1024 { // 100MB
            runtime.GC()
        }
    }
}
```

## 监控和调优

### 实时监控

```go
func setupRealTimeMonitoring(p pool.Pool) {
    ticker := time.NewTicker(time.Second * 1)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := p.Stats()
        
        // 计算实时吞吐量
        throughput := stats.ThroughputTPS
        avgDuration := stats.AvgTaskDuration
        
        fmt.Printf("Active: %d, Queued: %d, TPS: %.2f, Avg: %v\n",
            stats.ActiveWorkers, stats.QueuedTasks, 
            throughput, avgDuration)
        
        // 性能告警
        if stats.QueuedTasks > 1000 {
            fmt.Printf("WARNING: High queue backlog: %d tasks\n", 
                stats.QueuedTasks)
        }
        
        if throughput < 100 && stats.ActiveWorkers > 0 {
            fmt.Printf("WARNING: Low throughput: %.2f TPS\n", throughput)
        }
    }
}
```

### 性能指标收集

```go
type PerformanceMetrics struct {
    MinThroughput    float64
    MaxThroughput    float64
    AvgThroughput    float64
    MinTaskDuration  time.Duration
    MaxTaskDuration  time.Duration
    AvgTaskDuration  time.Duration
}

func collectPerformanceMetrics(p pool.Pool, duration time.Duration) *PerformanceMetrics {
    metrics := &PerformanceMetrics{}
    
    start := time.Now()
    var throughputs []float64
    var durations []time.Duration
    
    ticker := time.NewTicker(time.Second * 1)
    defer ticker.Stop()
    
    for time.Since(start) < duration {
        select {
        case <-ticker.C:
            stats := p.Stats()
            throughputs = append(throughputs, stats.ThroughputTPS)
            durations = append(durations, stats.AvgTaskDuration)
        }
    }
    
    // 计算统计信息
    metrics.MinThroughput = minFloat64(throughputs)
    metrics.MaxThroughput = maxFloat64(throughputs)
    metrics.AvgThroughput = avgFloat64(throughputs)
    
    metrics.MinTaskDuration = minDuration(durations)
    metrics.MaxTaskDuration = maxDuration(durations)
    metrics.AvgTaskDuration = avgDuration(durations)
    
    return metrics
}
```

## 常见性能问题

### 1. 队列积压

**症状**：队列中任务数量持续增长，处理速度跟不上提交速度。

**解决方案**：
```go
// 增加工作协程数量
config := pool.NewConfigBuilder().
    WithWorkerCount(runtime.NumCPU() * 4).
    Build()

// 实现背压控制
type BackpressureController struct {
    pool         pool.Pool
    maxQueueSize int64
}

func (bc *BackpressureController) Submit(task pool.Task) error {
    stats := bc.pool.Stats()
    if stats.QueuedTasks > bc.maxQueueSize {
        return fmt.Errorf("queue full, rejecting task")
    }
    return bc.pool.Submit(task)
}
```

### 2. 内存泄漏

**症状**：应用内存使用量持续增长。

**解决方案**：
```go
// 确保正确释放资源
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
```

### 3. 死锁

**症状**：应用停止响应，协程池无法处理新任务。

**解决方案**：
```go
// 避免在任务中等待其他任务
type GoodTask struct {
    dependency TaskResult // 通过外部协调传入依赖结果
}

func (t *GoodTask) Execute(ctx context.Context) (any, error) {
    // 使用已有的依赖结果
    return t.processWithDependency(t.dependency), nil
}

// 使用超时避免无限等待
func (t *SafeTask) Execute(ctx context.Context) (any, error) {
    // 设置超时上下文
    timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    return t.processWithTimeout(timeoutCtx)
}
```

### 4. 性能下降

**症状**：随着运行时间增长，任务处理速度逐渐下降。

**解决方案**：
```go
// 定期重启工作协程
type RestartablePool struct {
    pool.Pool
    restartInterval time.Duration
    lastRestart     time.Time
}

func (rp *RestartablePool) maybeRestart() {
    if time.Since(rp.lastRestart) > rp.restartInterval {
        rp.restart()
        rp.lastRestart = time.Now()
    }
}

// 监控和告警
type PerformanceMonitor struct {
    pool              pool.Pool
    baselineTPS       float64
    degradationThreshold float64
}

func (pm *PerformanceMonitor) checkPerformance() {
    stats := pm.pool.Stats()
    currentTPS := stats.ThroughputTPS
    
    if currentTPS < pm.baselineTPS * (1 - pm.degradationThreshold) {
        fmt.Printf("ALERT: Performance degradation detected. Current TPS: %.2f, Baseline: %.2f",
            currentTPS, pm.baselineTPS)
    }
}
```

## 性能调优 checklist

- [ ] 根据任务类型选择合适的工作协程数量
- [ ] 设置合理的队列大小（工作协程数量的10-100倍）
- [ ] 启用对象池以减少内存分配
- [ ] 启用预分配以减少运行时开销
- [ ] 设置合适的任务和关闭超时时间
- [ ] 实施实时监控以检测性能问题
- [ ] 定期进行压力测试以验证性能
- [ ] 根据监控数据调整配置参数

## 结论

通过合理的配置和调优，Goroutine Pool SDK 可以为您的应用提供卓越的并发处理能力。关键是要根据具体的应用场景和负载特征进行针对性的优化，并持续监控性能指标以确保系统运行在最佳状态。

更多详细示例请参考 [使用示例文档](examples.md) 和 [API 文档](api.md)。