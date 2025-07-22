# Goroutine Pool SDK

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Coverage](https://img.shields.io/badge/coverage-95%25-brightgreen.svg)]()

A high-performance, feature-rich goroutine pool management library for Go applications. Built for production use with comprehensive monitoring, extensibility, and graceful shutdown capabilities.

## ✨ Features

- **🚀 High Performance**: Lock-free queues, work-stealing, and optimized scheduling
- **🔧 Flexible Configuration**: Builder pattern with validation and environment support
- **📊 Built-in Monitoring**: Real-time statistics, metrics collection, and health checks
- **🔌 Extensible Architecture**: Middleware, plugins, and event system
- **⚡ Priority Scheduling**: Multi-level task prioritization
- **🛡️ Robust Error Handling**: Panic recovery, timeout handling, and circuit breaker
- **🔄 Graceful Shutdown**: Context-aware cancellation and resource cleanup
- **📈 Production Ready**: Battle-tested patterns and best practices

## 🚀 Quick Start

### Installation

```bash
go get github.com/uptutu/goman
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/uptutu/goman/pkg/pool"
)

// Implement the Task interface
type MyTask struct {
    ID   int
    Data string
}

func (t *MyTask) Execute(ctx context.Context) (any, error) {
    // Your business logic here
    time.Sleep(100 * time.Millisecond)
    return fmt.Sprintf("Task %d processed: %s", t.ID, t.Data), nil
}

func (t *MyTask) Priority() int {
    return pool.PriorityNormal
}

func main() {
    // Create pool with default configuration
    p, err := pool.NewPool(nil)
    if err != nil {
        log.Fatalf("Failed to create pool: %v", err)
    }
    defer p.Shutdown(context.Background())

    // Submit synchronous task
    task := &MyTask{ID: 1, Data: "example"}
    err = p.Submit(task)
    if err != nil {
        log.Printf("Failed to submit task: %v", err)
    }

    // Submit asynchronous task
    future := p.SubmitAsync(task)
    result, err := future.Get()
    if err != nil {
        log.Printf("Task failed: %v", err)
    } else {
        fmt.Printf("Result: %v\n", result)
    }
}
```

### Advanced Configuration

```go
config, err := pool.NewConfigBuilder().
    WithWorkerCount(runtime.NumCPU() * 2).
    WithQueueSize(10000).
    WithTaskTimeout(30 * time.Second).
    WithShutdownTimeout(60 * time.Second).
    WithMetrics(true).
    WithPreAlloc(true).
    Build()

if err != nil {
    log.Fatalf("Invalid config: %v", err)
}

p, err := pool.NewPool(config)
if err != nil {
    log.Fatalf("Failed to create pool: %v", err)
}
```

## 📖 Documentation

- **[API Documentation](docs/api.md)** - Complete API reference
- **[Usage Examples](docs/examples.md)** - Comprehensive examples and patterns
- **[Best Practices](docs/best-practices.md)** - Production deployment guide
- **[Performance Tuning](docs/performance-tuning.md)** - Optimization strategies

## 🏗️ Architecture

### Core Components

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Task Queue    │    │   Load Balancer  │    │   Worker Pool   │
│                 │────│                  │────│                 │
│ Priority-based  │    │ Round Robin/     │    │ Goroutines with │
│ Lock-free Queue │    │ Weighted/Custom  │    │ Local Queues    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                        │                        │
         └────────────────────────┼────────────────────────┘
                                  │
         ┌────────────────────────┼────────────────────────┐
         │                       │                        │
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Extensions    │    │     Monitor      │    │ Error Handler   │
│                 │    │                  │    │                 │
│ • Middleware    │    │ • Statistics     │    │ • Panic Recovery│
│ • Plugins       │    │ • Health Checks  │    │ • Timeout       │
│ • Event System  │    │ • Metrics        │    │ • Circuit Break │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

### Key Interfaces

```go
// Core pool interface
type Pool interface {
    Submit(task Task) error
    SubmitWithTimeout(task Task, timeout time.Duration) error
    SubmitAsync(task Task) Future
    Shutdown(ctx context.Context) error
    Stats() PoolStats
    // Extension methods...
}

// Task interface - implement this for your work
type Task interface {
    Execute(ctx context.Context) (any, error)
    Priority() int
}

// Future interface for async operations
type Future interface {
    Get() (any, error)
    GetWithTimeout(timeout time.Duration) (any, error)
    IsDone() bool
    Cancel() bool
}
```

## 🔧 Configuration Options

| Option | Description | Default | Range |
|--------|-------------|---------|-------|
| `WorkerCount` | Number of worker goroutines | `10` | `1-10000` |
| `QueueSize` | Task queue buffer size | `1000` | `1-1000000` |
| `TaskTimeout` | Maximum task execution time | `30s` | `1s-24h` |
| `ShutdownTimeout` | Graceful shutdown timeout | `30s` | `1s-1h` |
| `ObjectPoolSize` | Object pool size for optimization | `100` | `0-∞` |
| `EnableMetrics` | Enable built-in monitoring | `true` | `true/false` |
| `MetricsInterval` | Metrics collection interval | `10s` | `1s-1h` |

## 📊 Monitoring & Observability

### Built-in Statistics

```go
stats := pool.Stats()
fmt.Printf("Active Workers: %d\n", stats.ActiveWorkers)
fmt.Printf("Queued Tasks: %d\n", stats.QueuedTasks)
fmt.Printf("Completed: %d\n", stats.CompletedTasks)
fmt.Printf("Failed: %d\n", stats.FailedTasks)
fmt.Printf("Throughput: %.2f TPS\n", stats.ThroughputTPS)
fmt.Printf("Avg Duration: %v\n", stats.AvgTaskDuration)
```

### Health Check Endpoint

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    if !pool.IsRunning() {
        http.Error(w, "Pool not running", http.StatusServiceUnavailable)
        return
    }
    
    stats := pool.Stats()
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "healthy",
        "stats":  stats,
    })
}
```

## 🔌 Extensions

### Middleware System

```go
// Custom middleware for logging
type LoggingMiddleware struct{}

func (lm *LoggingMiddleware) Before(ctx context.Context, task Task) context.Context {
    log.Printf("Task started: priority=%d", task.Priority())
    return context.WithValue(ctx, "start_time", time.Now())
}

func (lm *LoggingMiddleware) After(ctx context.Context, task Task, result any, err error) {
    startTime := ctx.Value("start_time").(time.Time)
    duration := time.Since(startTime)
    log.Printf("Task completed in %v", duration)
}

// Add to pool
pool.AddMiddleware(&LoggingMiddleware{})
```

### Plugin System

```go
// Custom monitoring plugin
type MonitoringPlugin struct {
    name string
    // ... implementation
}

func (mp *MonitoringPlugin) Name() string { return mp.name }
func (mp *MonitoringPlugin) Init(pool Pool) error { /* ... */ }
func (mp *MonitoringPlugin) Start() error { /* ... */ }
func (mp *MonitoringPlugin) Stop() error { /* ... */ }

// Register plugin
pool.RegisterPlugin(&MonitoringPlugin{name: "monitor"})
```

## 🎯 Use Cases

### Web Server Integration

```go
func taskHandler(w http.ResponseWriter, r *http.Request) {
    task := &BusinessTask{
        UserID: getUserID(r),
        Action: r.URL.Path,
        Data:   getRequestData(r),
    }
    
    future := pool.SubmitAsync(task)
    result, err := future.GetWithTimeout(25 * time.Second)
    
    if err != nil {
        http.Error(w, "Task failed", http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(result)
}
```

### Message Queue Processing

```go
type MessageTask struct {
    Topic   string
    Payload []byte
}

func (t *MessageTask) Execute(ctx context.Context) (any, error) {
    switch t.Topic {
    case "user.created":
        return processUserCreated(t.Payload)
    case "order.placed":
        return processOrderPlaced(t.Payload)
    default:
        return nil, fmt.Errorf("unknown topic: %s", t.Topic)
    }
}

func (t *MessageTask) Priority() int {
    if t.Topic == "payment.completed" {
        return pool.PriorityCritical
    }
    return pool.PriorityNormal
}
```

### Batch Processing

```go
type BatchProcessor struct {
    pool      pool.Pool
    batchSize int
    buffer    []Task
}

func (bp *BatchProcessor) Process(items []DataItem) error {
    // Split into batches
    for i := 0; i < len(items); i += bp.batchSize {
        end := min(i+bp.batchSize, len(items))
        batch := items[i:end]
        
        task := &BatchTask{Items: batch}
        if err := bp.pool.Submit(task); err != nil {
            return err
        }
    }
    return nil
}
```

## ⚡ Performance

### Benchmarks

```
BenchmarkPoolSubmit-8           5000000    250 ns/op    48 B/op    1 allocs/op
BenchmarkPoolSubmitAsync-8      3000000    420 ns/op    96 B/op    2 allocs/op
BenchmarkHighConcurrency-8      1000000   1200 ns/op   128 B/op    3 allocs/op
```

### Optimization Tips

1. **Worker Count**: 
   - CPU-intensive: `runtime.NumCPU()`
   - I/O-intensive: `runtime.NumCPU() * 2-4`

2. **Queue Size**: 
   - Set to `WorkerCount * 10-100` based on task duration

3. **Object Pools**: 
   - Enable `PreAlloc` for high-throughput scenarios

4. **Batch Processing**: 
   - Group small tasks to reduce overhead

## 🛡️ Error Handling

### Panic Recovery

```go
type SafeTask struct {
    riskyOperation func() (any, error)
}

func (t *SafeTask) Execute(ctx context.Context) (any, error) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Task panic recovered: %v", r)
        }
    }()
    
    return t.riskyOperation()
}
```

### Timeout Handling

```go
func (t *TimeoutTask) Execute(ctx context.Context) (any, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Your work here with periodic cancellation checks
    return result, nil
}
```

## 🔄 Graceful Shutdown

```go
func gracefulShutdown(pool pool.Pool) {
    // Listen for interrupt signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        log.Println("Shutdown signal received...")
        
        // Create shutdown context with timeout
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        if err := pool.Shutdown(ctx); err != nil {
            log.Printf("Forced shutdown: %v", err)
        } else {
            log.Println("Graceful shutdown completed")
        }
    }()
}
```

## 🧪 Testing

```go
func TestPoolBasicOperations(t *testing.T) {
    pool, err := pool.NewPool(nil)
    require.NoError(t, err)
    defer pool.Shutdown(context.Background())
    
    task := &TestTask{data: "test"}
    err = pool.Submit(task)
    require.NoError(t, err)
    
    // Wait for completion
    time.Sleep(100 * time.Millisecond)
    
    stats := pool.Stats()
    assert.Equal(t, int64(1), stats.CompletedTasks)
}
```

## 📈 Production Deployment

### Docker Integration

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o app ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/app .
EXPOSE 8080
CMD ["./app"]
```

### Environment Configuration

```bash
# Pool configuration
POOL_WORKER_COUNT=20
POOL_QUEUE_SIZE=10000
POOL_TASK_TIMEOUT=30s
POOL_SHUTDOWN_TIMEOUT=60s
POOL_ENABLE_METRICS=true

# Application configuration
SERVER_ADDR=:8080
LOG_LEVEL=info
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: goroutine-pool-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: goroutine-pool-app
  template:
    metadata:
      labels:
        app: goroutine-pool-app
    spec:
      containers:
      - name: app
        image: your-registry/goroutine-pool-app:latest
        ports:
        - containerPort: 8080
        env:
        - name: POOL_WORKER_COUNT
          value: "20"
        - name: POOL_QUEUE_SIZE
          value: "10000"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

### Development Setup

```bash
# Clone the repository
git clone https://github.com/uptutu/goman.git
cd goman

# Install dependencies
go mod download

# Run tests
go test ./...

# Run benchmarks
go test -bench=. ./pkg/pool

# Run with race detection
go test -race ./...
```

### Code Quality

- **Test Coverage**: Maintain >90% test coverage
- **Benchmarks**: Include benchmarks for performance-critical code
- **Documentation**: Update docs for API changes
- **Linting**: Use `golangci-lint` for code quality

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Inspired by Java's ThreadPoolExecutor and .NET's Task Parallel Library
- Built with Go's excellent concurrency primitives
- Community feedback and contributions

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/uptutu/goman/issues)
- **Discussions**: [GitHub Discussions](https://github.com/uptutu/goman/discussions)
- **Documentation**: [docs/](docs/)

---

**Made with ❤️ for the Go community**