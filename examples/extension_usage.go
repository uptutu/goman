package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// CustomTask 自定义任务
type CustomTask struct {
	ID       int
	Name     string
	priority int
}

func (ct *CustomTask) Execute(ctx context.Context) (any, error) {
	// 模拟任务执行
	fmt.Printf("Executing task %d: %s\n", ct.ID, ct.Name)
	time.Sleep(100 * time.Millisecond)
	return fmt.Sprintf("Task %d completed", ct.ID), nil
}

func (ct *CustomTask) Priority() int {
	return ct.priority
}

// CustomMiddleware 自定义中间件
type CustomMiddleware struct {
	name string
}

func (cm *CustomMiddleware) Before(ctx context.Context, task pool.Task) context.Context {
	fmt.Printf("[%s] Task started: %v\n", cm.name, task)
	return context.WithValue(ctx, "start_time", time.Now())
}

func (cm *CustomMiddleware) After(ctx context.Context, task pool.Task, result any, err error) {
	startTime, _ := ctx.Value("start_time").(time.Time)
	duration := time.Since(startTime)

	if err != nil {
		fmt.Printf("[%s] Task failed after %v: %v\n", cm.name, duration, err)
	} else {
		fmt.Printf("[%s] Task completed after %v: %v\n", cm.name, duration, result)
	}
}

// CustomPlugin 自定义插件
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
	fmt.Printf("Plugin %s initialized\n", cp.name)
	return nil
}

func (cp *CustomPlugin) Start() error {
	fmt.Printf("Plugin %s started\n", cp.name)
	go cp.monitor()
	return nil
}

func (cp *CustomPlugin) Stop() error {
	fmt.Printf("Plugin %s stopping\n", cp.name)
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
				fmt.Printf("[%s] Pool stats - Active: %d, Queued: %d, Completed: %d\n",
					cp.name, stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks)
			}
		case <-cp.done:
			return
		}
	}
}

// CustomEventListener 自定义事件监听器
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
	fmt.Printf("[%s] Task completed: priority=%d, result=%v\n", cel.name, task.Priority(), result)
}

func (cel *CustomEventListener) OnWorkerPanic(workerID int, panicValue any) {
	fmt.Printf("[%s] Worker %d panicked: %v\n", cel.name, workerID, panicValue)
}

// CustomSchedulerPlugin 自定义调度器插件
type CustomSchedulerPlugin struct {
	name     string
	priority int
}

func (csp *CustomSchedulerPlugin) Name() string {
	return csp.name
}

func (csp *CustomSchedulerPlugin) CreateScheduler(config *pool.Config) (pool.Scheduler, error) {
	fmt.Printf("Creating custom scheduler with config: workers=%d, queue=%d\n",
		config.WorkerCount, config.QueueSize)
	return &CustomScheduler{}, nil
}

func (csp *CustomSchedulerPlugin) Priority() int {
	return csp.priority
}

// CustomScheduler 自定义调度器
type CustomScheduler struct{}

func (cs *CustomScheduler) Schedule(task pool.Task) error {
	fmt.Printf("Custom scheduler scheduling task with priority %d\n", task.Priority())
	return nil
}

func (cs *CustomScheduler) Start() error {
	fmt.Println("Custom scheduler started")
	return nil
}

func (cs *CustomScheduler) Stop() error {
	fmt.Println("Custom scheduler stopped")
	return nil
}

func main() {
	fmt.Println("=== Goroutine Pool Extension Usage Example ===")

	// 创建协程池配置
	config := &pool.Config{
		WorkerCount:     3,
		QueueSize:       10,
		EnableMetrics:   true,
		MetricsInterval: 1 * time.Second,
	}

	// 创建协程池
	p, err := pool.New(config)
	if err != nil {
		log.Fatalf("Failed to create pool: %v", err)
	}

	fmt.Println("\n1. Adding middleware...")

	// 添加自定义中间件
	customMiddleware := &CustomMiddleware{name: "CustomMiddleware"}
	p.AddMiddleware(customMiddleware)

	// 添加内置日志中间件
	loggingMiddleware := pool.NewLoggingMiddleware(nil)
	p.AddMiddleware(loggingMiddleware)

	// 添加内置指标中间件
	metricsMiddleware := pool.NewMetricsMiddleware()
	p.AddMiddleware(metricsMiddleware)

	fmt.Println("\n2. Registering plugins...")

	// 注册自定义插件
	customPlugin := NewCustomPlugin("CustomMonitor", 2*time.Second)
	err = p.RegisterPlugin(customPlugin)
	if err != nil {
		log.Fatalf("Failed to register custom plugin: %v", err)
	}

	// 注册内置监控插件
	monitoringPlugin := pool.NewMonitoringPlugin("BuiltinMonitor", 3*time.Second, nil)
	err = p.RegisterPlugin(monitoringPlugin)
	if err != nil {
		log.Fatalf("Failed to register monitoring plugin: %v", err)
	}

	fmt.Println("\n3. Adding event listeners...")

	// 添加自定义事件监听器
	customListener := &CustomEventListener{name: "CustomListener"}
	p.AddEventListener(customListener)

	// 添加内置日志事件监听器
	loggingListener := pool.NewLoggingEventListener(nil)
	p.AddEventListener(loggingListener)

	fmt.Println("\n4. Setting scheduler plugin...")

	// 设置自定义调度器插件
	schedulerPlugin := &CustomSchedulerPlugin{
		name:     "CustomScheduler",
		priority: 10,
	}
	p.SetSchedulerPlugin(schedulerPlugin)

	fmt.Println("\n5. Submitting tasks...")

	// 提交一些任务
	for i := 0; i < 5; i++ {
		task := &CustomTask{
			ID:       i + 1,
			Name:     fmt.Sprintf("Task-%d", i+1),
			priority: i % 3,
		}

		err := p.Submit(task)
		if err != nil {
			fmt.Printf("Failed to submit task %d: %v\n", i+1, err)
		}

		// 稍微延迟以便观察输出
		time.Sleep(50 * time.Millisecond)
	}

	fmt.Println("\n6. Waiting for tasks to complete...")
	time.Sleep(3 * time.Second)

	fmt.Println("\n7. Getting metrics...")

	// 获取指标中间件的统计信息
	metrics := metricsMiddleware.GetMetrics()
	fmt.Printf("Metrics - Tasks: %d, Success: %d, Failure: %d, Avg Duration: %v, Success Rate: %.2f%%\n",
		metrics.TaskCount, metrics.SuccessCount, metrics.FailureCount,
		metrics.AvgDuration, metrics.SuccessRate*100)

	// 获取协程池统计信息
	stats := p.Stats()
	fmt.Printf("Pool Stats - Active: %d, Queued: %d, Completed: %d, Failed: %d, TPS: %.2f\n",
		stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks, stats.FailedTasks, stats.ThroughputTPS)

	fmt.Println("\n8. Testing plugin management...")

	// 获取插件
	plugin, exists := p.GetPlugin("CustomMonitor")
	if exists {
		fmt.Printf("Found plugin: %s\n", plugin.Name())
	}

	// 注销插件
	err = p.UnregisterPlugin("CustomMonitor")
	if err != nil {
		fmt.Printf("Failed to unregister plugin: %v\n", err)
	} else {
		fmt.Println("Plugin unregistered successfully")
	}

	fmt.Println("\n9. Testing middleware removal...")

	// 移除中间件
	p.RemoveMiddleware(customMiddleware)
	fmt.Println("Custom middleware removed")

	// 提交一个任务验证中间件已移除
	task := &CustomTask{
		ID:       99,
		Name:     "Final Task",
		priority: 1,
	}
	err = p.Submit(task)
	if err != nil {
		fmt.Printf("Failed to submit final task: %v\n", err)
	}

	time.Sleep(1 * time.Second)

	fmt.Println("\n10. Shutting down...")

	// 优雅关闭协程池
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = p.Shutdown(ctx)
	if err != nil {
		fmt.Printf("Failed to shutdown pool: %v\n", err)
	} else {
		fmt.Println("Pool shutdown successfully")
	}

	fmt.Println("\n=== Extension Usage Example Completed ===")
}
