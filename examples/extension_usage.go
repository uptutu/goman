package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// ExtensionTask 扩展示例任务
type ExtensionTask struct {
	ID       int
	Name     string
	TaskType string
	priority int
	workFunc func(ctx context.Context) (any, error)
}

func NewExtensionTask(id int, name, taskType string, priority int, workFunc func(ctx context.Context) (any, error)) *ExtensionTask {
	return &ExtensionTask{
		ID:       id,
		Name:     name,
		TaskType: taskType,
		priority: priority,
		workFunc: workFunc,
	}
}

func (et *ExtensionTask) Execute(ctx context.Context) (any, error) {
	if et.workFunc != nil {
		return et.workFunc(ctx)
	}

	// 默认任务执行逻辑
	processingTime := time.Duration(50+rand.Intn(150)) * time.Millisecond
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(processingTime):
		return fmt.Sprintf("Task %d (%s) of type %s completed in %v", et.ID, et.Name, et.TaskType, processingTime), nil
	}
}

func (et *ExtensionTask) Priority() int {
	return et.priority
}

// TimingMiddleware 计时中间件
type TimingMiddleware struct {
	name      string
	taskCount int64
}

func NewTimingMiddleware(name string) *TimingMiddleware {
	return &TimingMiddleware{name: name}
}

func (tm *TimingMiddleware) Before(ctx context.Context, task pool.Task) context.Context {
	atomic.AddInt64(&tm.taskCount, 1)
	fmt.Printf("[%s] Task #%d started (priority: %d)\n", tm.name, tm.taskCount, task.Priority())
	return context.WithValue(ctx, "timing_start", time.Now())
}

func (tm *TimingMiddleware) After(ctx context.Context, task pool.Task, result any, err error) {
	startTime, ok := ctx.Value("timing_start").(time.Time)
	if !ok {
		startTime = time.Now()
	}
	duration := time.Since(startTime)

	if err != nil {
		fmt.Printf("[%s] Task failed after %v: %v\n", tm.name, duration, err)
	} else {
		fmt.Printf("[%s] Task completed after %v\n", tm.name, duration)
	}
}

// SecurityMiddleware 安全检查中间件
type SecurityMiddleware struct {
	name              string
	blockedTasks      int64
	allowedTasks      int64
	maxPriority       int
	blockHighPriority bool
}

func NewSecurityMiddleware(name string, maxPriority int, blockHighPriority bool) *SecurityMiddleware {
	return &SecurityMiddleware{
		name:              name,
		maxPriority:       maxPriority,
		blockHighPriority: blockHighPriority,
	}
}

func (sm *SecurityMiddleware) Before(ctx context.Context, task pool.Task) context.Context {
	if sm.blockHighPriority && task.Priority() > sm.maxPriority {
		atomic.AddInt64(&sm.blockedTasks, 1)
		fmt.Printf("[%s] BLOCKED high priority task (priority: %d > %d)\n", sm.name, task.Priority(), sm.maxPriority)
		return context.WithValue(ctx, "security_blocked", true)
	}

	atomic.AddInt64(&sm.allowedTasks, 1)
	fmt.Printf("[%s] Task security check passed (priority: %d)\n", sm.name, task.Priority())
	return ctx
}

func (sm *SecurityMiddleware) After(ctx context.Context, task pool.Task, result any, err error) {
	if blocked, _ := ctx.Value("security_blocked").(bool); blocked {
		fmt.Printf("[%s] Blocked task completed with security violation\n", sm.name)
	}
}

func (sm *SecurityMiddleware) GetStats() (blocked, allowed int64) {
	return atomic.LoadInt64(&sm.blockedTasks), atomic.LoadInt64(&sm.allowedTasks)
}

// PerformanceMonitorPlugin 性能监控插件
type PerformanceMonitorPlugin struct {
	name           string
	pool           pool.Pool
	ticker         *time.Ticker
	stopChan       chan struct{}
	lastCompleted  int64
	lastFailed     int64
	peakThroughput float64
}

func NewPerformanceMonitorPlugin(name string, interval time.Duration) *PerformanceMonitorPlugin {
	return &PerformanceMonitorPlugin{
		name:     name,
		ticker:   time.NewTicker(interval),
		stopChan: make(chan struct{}),
	}
}

func (pmp *PerformanceMonitorPlugin) Name() string {
	return pmp.name
}

func (pmp *PerformanceMonitorPlugin) Init(p pool.Pool) error {
	pmp.pool = p
	fmt.Printf("[%s] Performance monitor plugin initialized\n", pmp.name)
	return nil
}

func (pmp *PerformanceMonitorPlugin) Start() error {
	fmt.Printf("[%s] Performance monitor plugin started\n", pmp.name)
	go pmp.monitor()
	return nil
}

func (pmp *PerformanceMonitorPlugin) Stop() error {
	fmt.Printf("[%s] Performance monitor plugin stopping\n", pmp.name)
	close(pmp.stopChan)
	pmp.ticker.Stop()
	return nil
}

func (pmp *PerformanceMonitorPlugin) monitor() {
	for {
		select {
		case <-pmp.ticker.C:
			if pmp.pool != nil {
				stats := pmp.pool.Stats()

				// 计算增量吞吐量
				completedDelta := stats.CompletedTasks - pmp.lastCompleted
				failedDelta := stats.FailedTasks - pmp.lastFailed

				if stats.ThroughputTPS > pmp.peakThroughput {
					pmp.peakThroughput = stats.ThroughputTPS
				}

				fmt.Printf("[%s] Performance Report:\n", pmp.name)
				fmt.Printf("  - Active Workers: %d\n", stats.ActiveWorkers)
				fmt.Printf("  - Queue Length: %d\n", stats.QueuedTasks)
				fmt.Printf("  - Completed (Δ): %d (+%d)\n", stats.CompletedTasks, completedDelta)
				fmt.Printf("  - Failed (Δ): %d (+%d)\n", stats.FailedTasks, failedDelta)
				fmt.Printf("  - Current TPS: %.2f\n", stats.ThroughputTPS)
				fmt.Printf("  - Peak TPS: %.2f\n", pmp.peakThroughput)
				fmt.Printf("  - Avg Duration: %v\n", stats.AvgTaskDuration)
				fmt.Printf("  - Memory Usage: %d bytes\n", stats.MemoryUsage)
				fmt.Println()

				pmp.lastCompleted = stats.CompletedTasks
				pmp.lastFailed = stats.FailedTasks
			}
		case <-pmp.stopChan:
			return
		}
	}
}

// DetailedEventListener 详细事件监听器
type DetailedEventListener struct {
	name        string
	eventCounts map[string]int64
	startTime   time.Time
}

func NewDetailedEventListener(name string) *DetailedEventListener {
	return &DetailedEventListener{
		name:        name,
		eventCounts: make(map[string]int64),
		startTime:   time.Now(),
	}
}

func (del *DetailedEventListener) OnPoolStart(p pool.Pool) {
	atomic.AddInt64(&del.eventCounts["pool_start"], 1)
	fmt.Printf("[%s] 🚀 Pool started at %v\n", del.name, time.Now().Format("15:04:05"))
}

func (del *DetailedEventListener) OnPoolShutdown(p pool.Pool) {
	atomic.AddInt64(&del.eventCounts["pool_shutdown"], 1)
	uptime := time.Since(del.startTime)
	fmt.Printf("[%s] 🛑 Pool shutdown after %v uptime\n", del.name, uptime)
}

func (del *DetailedEventListener) OnTaskSubmit(task pool.Task) {
	atomic.AddInt64(&del.eventCounts["task_submit"], 1)
	fmt.Printf("[%s] 📥 Task submitted (priority: %d)\n", del.name, task.Priority())
}

func (del *DetailedEventListener) OnTaskComplete(task pool.Task, result any) {
	atomic.AddInt64(&del.eventCounts["task_complete"], 1)
	fmt.Printf("[%s] ✅ Task completed (priority: %d)\n", del.name, task.Priority())
}

func (del *DetailedEventListener) OnWorkerPanic(workerID int, panicValue any) {
	atomic.AddInt64(&del.eventCounts["worker_panic"], 1)
	fmt.Printf("[%s] 💥 Worker %d panicked: %v\n", del.name, workerID, panicValue)
}

func (del *DetailedEventListener) GetEventCounts() map[string]int64 {
	result := make(map[string]int64)
	for k, v := range del.eventCounts {
		result[k] = atomic.LoadInt64(&v)
	}
	return result
}

func main() {
	fmt.Println("=== Goroutine Pool Extension Usage Example ===")

	// 创建协程池配置
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(20).
		WithTaskTimeout(5 * time.Second).
		WithMetrics(true).
		WithMetricsInterval(2 * time.Second).
		Build()

	if err != nil {
		log.Fatalf("Failed to create config: %v", err)
	}

	// 创建协程池
	p, err := pool.NewPool(config)
	if err != nil {
		log.Fatalf("Failed to create pool: %v", err)
	}

	fmt.Printf("Pool created with %d workers\n", config.WorkerCount)

	// 1. 添加中间件
	fmt.Println("\n=== 1. Adding Middleware ===")

	// 添加计时中间件
	timingMiddleware := NewTimingMiddleware("TimingMiddleware")
	p.AddMiddleware(timingMiddleware)

	// 添加安全中间件
	securityMiddleware := NewSecurityMiddleware("SecurityMiddleware", pool.PriorityNormal, false)
	p.AddMiddleware(securityMiddleware)

	// 添加内置日志中间件
	loggingMiddleware := pool.NewLoggingMiddleware(nil)
	p.AddMiddleware(loggingMiddleware)

	// 添加内置指标中间件
	metricsMiddleware := pool.NewMetricsMiddleware()
	p.AddMiddleware(metricsMiddleware)

	fmt.Println("All middleware added successfully")

	// 2. 注册插件
	fmt.Println("\n=== 2. Registering Plugins ===")

	// 注册性能监控插件
	perfMonitor := NewPerformanceMonitorPlugin("PerfMonitor", 3*time.Second)
	err = p.RegisterPlugin(perfMonitor)
	if err != nil {
		log.Fatalf("Failed to register performance monitor plugin: %v", err)
	}

	// 注册内置监控插件
	builtinMonitor := pool.NewMonitoringPlugin("BuiltinMonitor", 4*time.Second, nil)
	err = p.RegisterPlugin(builtinMonitor)
	if err != nil {
		log.Fatalf("Failed to register builtin monitor plugin: %v", err)
	}

	fmt.Println("All plugins registered successfully")

	// 3. 添加事件监听器
	fmt.Println("\n=== 3. Adding Event Listeners ===")

	// 添加详细事件监听器
	detailedListener := NewDetailedEventListener("DetailedListener")
	p.AddEventListener(detailedListener)

	// 添加内置日志事件监听器
	loggingListener := pool.NewLoggingEventListener(nil)
	p.AddEventListener(loggingListener)

	fmt.Println("All event listeners added successfully")

	// 4. 提交各种类型的任务
	fmt.Println("\n=== 4. Submitting Various Task Types ===")

	// 提交普通任务
	for i := 1; i <= 3; i++ {
		task := NewExtensionTask(i, fmt.Sprintf("normal-task-%d", i), "NORMAL", pool.PriorityNormal, nil)
		if err := p.Submit(task); err != nil {
			fmt.Printf("Failed to submit normal task %d: %v\n", i, err)
		}
	}

	// 提交高优先级任务
	for i := 1; i <= 2; i++ {
		task := NewExtensionTask(10+i, fmt.Sprintf("high-priority-task-%d", i), "HIGH", pool.PriorityHigh,
			func(ctx context.Context) (any, error) {
				time.Sleep(200 * time.Millisecond)
				return "High priority task completed with extra processing", nil
			})
		if err := p.Submit(task); err != nil {
			fmt.Printf("Failed to submit high priority task %d: %v\n", i, err)
		}
	}

	// 提交会失败的任务
	failTask := NewExtensionTask(20, "fail-task", "ERROR", pool.PriorityNormal,
		func(ctx context.Context) (any, error) {
			time.Sleep(100 * time.Millisecond)
			return nil, fmt.Errorf("intentional task failure for testing")
		})
	if err := p.Submit(failTask); err != nil {
		fmt.Printf("Failed to submit fail task: %v\n", err)
	}

	// 提交会panic的任务
	panicTask := NewExtensionTask(21, "panic-task", "PANIC", pool.PriorityLow,
		func(ctx context.Context) (any, error) {
			time.Sleep(50 * time.Millisecond)
			panic("intentional panic for testing middleware and event handling")
		})
	if err := p.Submit(panicTask); err != nil {
		fmt.Printf("Failed to submit panic task: %v\n", err)
	}

	// 提交CPU密集型任务
	cpuTask := NewExtensionTask(22, "cpu-intensive-task", "CPU", pool.PriorityNormal,
		func(ctx context.Context) (any, error) {
			result := int64(0)
			for i := 0; i < 500000; i++ {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					result += int64(i * i)
				}
			}
			return fmt.Sprintf("CPU intensive task completed: result=%d", result%1000000), nil
		})
	if err := p.Submit(cpuTask); err != nil {
		fmt.Printf("Failed to submit CPU task: %v\n", err)
	}

	// 5. 等待任务执行并观察扩展功能
	fmt.Println("\n=== 5. Observing Extension Functionality ===")
	time.Sleep(8 * time.Second)

	// 6. 获取各种统计信息
	fmt.Println("\n=== 6. Extension Statistics ===")

	// 获取指标中间件统计
	metrics := metricsMiddleware.GetMetrics()
	fmt.Printf("Metrics Middleware Stats:\n")
	fmt.Printf("  - Total Tasks: %d\n", metrics.TaskCount)
	fmt.Printf("  - Successful: %d\n", metrics.SuccessCount)
	fmt.Printf("  - Failed: %d\n", metrics.FailureCount)
	fmt.Printf("  - Average Duration: %v\n", metrics.AvgDuration)
	fmt.Printf("  - Success Rate: %.2f%%\n", metrics.SuccessRate*100)

	// 获取安全中间件统计
	blocked, allowed := securityMiddleware.GetStats()
	fmt.Printf("\nSecurity Middleware Stats:\n")
	fmt.Printf("  - Blocked Tasks: %d\n", blocked)
	fmt.Printf("  - Allowed Tasks: %d\n", allowed)

	// 获取事件监听器统计
	eventCounts := detailedListener.GetEventCounts()
	fmt.Printf("\nEvent Listener Stats:\n")
	for event, count := range eventCounts {
		fmt.Printf("  - %s: %d\n", event, count)
	}

	// 获取协程池统计
	poolStats := p.Stats()
	fmt.Printf("\nPool Stats:\n")
	fmt.Printf("  - Active Workers: %d\n", poolStats.ActiveWorkers)
	fmt.Printf("  - Queued Tasks: %d\n", poolStats.QueuedTasks)
	fmt.Printf("  - Completed Tasks: %d\n", poolStats.CompletedTasks)
	fmt.Printf("  - Failed Tasks: %d\n", poolStats.FailedTasks)
	fmt.Printf("  - Throughput: %.2f TPS\n", poolStats.ThroughputTPS)

	// 7. 测试插件管理
	fmt.Println("\n=== 7. Testing Plugin Management ===")

	// 获取插件
	if plugin, exists := p.GetPlugin("PerfMonitor"); exists {
		fmt.Printf("Found plugin: %s\n", plugin.Name())
	}

	// 注销插件
	if err := p.UnregisterPlugin("PerfMonitor"); err != nil {
		fmt.Printf("Failed to unregister plugin: %v\n", err)
	} else {
		fmt.Println("Performance monitor plugin unregistered successfully")
	}

	// 8. 测试中间件移除
	fmt.Println("\n=== 8. Testing Middleware Removal ===")

	// 移除安全中间件
	p.RemoveMiddleware(securityMiddleware)
	fmt.Println("Security middleware removed")

	// 提交一个高优先级任务验证安全中间件已移除
	testTask := NewExtensionTask(99, "test-after-removal", "TEST", pool.PriorityCritical, nil)
	if err := p.Submit(testTask); err != nil {
		fmt.Printf("Failed to submit test task: %v\n", err)
	} else {
		fmt.Println("High priority task submitted successfully after security middleware removal")
	}

	time.Sleep(2 * time.Second)

	// 9. 优雅关闭
	fmt.Println("\n=== 9. Graceful Shutdown ===")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("Failed to shutdown pool: %v\n", err)
	} else {
		fmt.Println("Pool shutdown successfully")
	}

	// 最终统计
	fmt.Println("\n=== Final Statistics ===")
	finalStats := p.Stats()
	fmt.Printf("Final Pool Stats:\n")
	fmt.Printf("  - Total Completed: %d\n", finalStats.CompletedTasks)
	fmt.Printf("  - Total Failed: %d\n", finalStats.FailedTasks)
	fmt.Printf("  - Final Memory Usage: %d bytes\n", finalStats.MemoryUsage)

	fmt.Println("\n=== Extension Usage Example Completed ===")
}
