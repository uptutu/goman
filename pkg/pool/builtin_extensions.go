package pool

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// LoggingMiddleware 日志中间件
type LoggingMiddleware struct {
	logger ExtendedLogger
}

// NewLoggingMiddleware 创建日志中间件
func NewLoggingMiddleware(logger ExtendedLogger) *LoggingMiddleware {
	if logger == nil {
		logger = &DefaultExtendedLogger{}
	}
	return &LoggingMiddleware{logger: logger}
}

// Before 任务执行前记录日志
func (lm *LoggingMiddleware) Before(ctx context.Context, task Task) context.Context {
	lm.logger.Infof("Task started: priority=%d", task.Priority())
	return context.WithValue(ctx, "start_time", time.Now())
}

// After 任务执行后记录日志
func (lm *LoggingMiddleware) After(ctx context.Context, task Task, result any, err error) {
	startTime, ok := ctx.Value("start_time").(time.Time)
	if !ok {
		startTime = time.Now()
	}
	duration := time.Since(startTime)

	if err != nil {
		lm.logger.Errorf("Task failed: priority=%d, duration=%v, error=%v", task.Priority(), duration, err)
	} else {
		lm.logger.Infof("Task completed: priority=%d, duration=%v", task.Priority(), duration)
	}
}

// MetricsMiddleware 指标收集中间件
type MetricsMiddleware struct {
	taskCount    int64
	successCount int64
	failureCount int64
	totalTime    int64 // 纳秒
}

// NewMetricsMiddleware 创建指标收集中间件
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{}
}

// Before 任务执行前
func (mm *MetricsMiddleware) Before(ctx context.Context, task Task) context.Context {
	atomic.AddInt64(&mm.taskCount, 1)
	return context.WithValue(ctx, "metrics_start_time", time.Now())
}

// After 任务执行后收集指标
func (mm *MetricsMiddleware) After(ctx context.Context, task Task, result any, err error) {
	startTime, ok := ctx.Value("metrics_start_time").(time.Time)
	if !ok {
		return
	}

	duration := time.Since(startTime)
	atomic.AddInt64(&mm.totalTime, duration.Nanoseconds())

	if err != nil {
		atomic.AddInt64(&mm.failureCount, 1)
	} else {
		atomic.AddInt64(&mm.successCount, 1)
	}
}

// GetMetrics 获取指标
func (mm *MetricsMiddleware) GetMetrics() MetricsData {
	return MetricsData{
		TaskCount:    atomic.LoadInt64(&mm.taskCount),
		SuccessCount: atomic.LoadInt64(&mm.successCount),
		FailureCount: atomic.LoadInt64(&mm.failureCount),
		AvgDuration:  time.Duration(atomic.LoadInt64(&mm.totalTime) / max(atomic.LoadInt64(&mm.taskCount), 1)),
		SuccessRate:  float64(atomic.LoadInt64(&mm.successCount)) / float64(max(atomic.LoadInt64(&mm.taskCount), 1)),
	}
}

// MetricsData 指标数据
type MetricsData struct {
	TaskCount    int64
	SuccessCount int64
	FailureCount int64
	AvgDuration  time.Duration
	SuccessRate  float64
}

// TimeoutMiddleware 超时控制中间件
type TimeoutMiddleware struct {
	timeout time.Duration
}

// NewTimeoutMiddleware 创建超时控制中间件
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{timeout: timeout}
}

// Before 设置超时上下文
func (tm *TimeoutMiddleware) Before(ctx context.Context, task Task) context.Context {
	timeoutCtx, cancel := context.WithTimeout(ctx, tm.timeout)
	// 将cancel函数存储在context中，以便在After中调用
	return context.WithValue(timeoutCtx, "timeout_cancel", cancel)
}

// After 清理超时上下文
func (tm *TimeoutMiddleware) After(ctx context.Context, task Task, result any, err error) {
	if cancel, ok := ctx.Value("timeout_cancel").(context.CancelFunc); ok {
		cancel()
	}
}

// MonitoringPlugin 监控插件
type MonitoringPlugin struct {
	name     string
	pool     Pool
	ticker   *time.Ticker
	stopChan chan struct{}
	logger   ExtendedLogger
}

// NewMonitoringPlugin 创建监控插件
func NewMonitoringPlugin(name string, interval time.Duration, logger ExtendedLogger) *MonitoringPlugin {
	if logger == nil {
		logger = &DefaultExtendedLogger{}
	}
	return &MonitoringPlugin{
		name:     name,
		ticker:   time.NewTicker(interval),
		stopChan: make(chan struct{}),
		logger:   logger,
	}
}

// Name 返回插件名称
func (mp *MonitoringPlugin) Name() string {
	return mp.name
}

// Init 初始化插件
func (mp *MonitoringPlugin) Init(pool Pool) error {
	mp.pool = pool
	return nil
}

// Start 启动插件
func (mp *MonitoringPlugin) Start() error {
	go mp.monitor()
	return nil
}

// Stop 停止插件
func (mp *MonitoringPlugin) Stop() error {
	close(mp.stopChan)
	mp.ticker.Stop()
	return nil
}

// monitor 监控协程池状态
func (mp *MonitoringPlugin) monitor() {
	for {
		select {
		case <-mp.ticker.C:
			if mp.pool != nil {
				stats := mp.pool.Stats()
				mp.logger.Infof("Pool Stats - Active: %d, Queued: %d, Completed: %d, Failed: %d, TPS: %.2f",
					stats.ActiveWorkers, stats.QueuedTasks, stats.CompletedTasks, stats.FailedTasks, stats.ThroughputTPS)
			}
		case <-mp.stopChan:
			return
		}
	}
}

// LoggingEventListener 日志事件监听器
type LoggingEventListener struct {
	logger ExtendedLogger
}

// NewLoggingEventListener 创建日志事件监听器
func NewLoggingEventListener(logger ExtendedLogger) *LoggingEventListener {
	if logger == nil {
		logger = &DefaultExtendedLogger{}
	}
	return &LoggingEventListener{logger: logger}
}

// OnPoolStart 协程池启动事件
func (lel *LoggingEventListener) OnPoolStart(pool Pool) {
	lel.logger.Info("Pool started")
}

// OnPoolShutdown 协程池关闭事件
func (lel *LoggingEventListener) OnPoolShutdown(pool Pool) {
	lel.logger.Info("Pool shutdown")
}

// OnTaskSubmit 任务提交事件
func (lel *LoggingEventListener) OnTaskSubmit(task Task) {
	lel.logger.Debugf("Task submitted: priority=%d", task.Priority())
}

// OnTaskComplete 任务完成事件
func (lel *LoggingEventListener) OnTaskComplete(task Task, result any) {
	lel.logger.Debugf("Task completed: priority=%d", task.Priority())
}

// OnWorkerPanic 工作协程panic事件
func (lel *LoggingEventListener) OnWorkerPanic(workerID int, panicValue any) {
	lel.logger.Errorf("Worker %d panicked: %v", workerID, panicValue)
}

// RoundRobinSchedulerPlugin 轮询调度器插件
type RoundRobinSchedulerPlugin struct {
	priority int
}

// NewRoundRobinSchedulerPlugin 创建轮询调度器插件
func NewRoundRobinSchedulerPlugin(priority int) *RoundRobinSchedulerPlugin {
	return &RoundRobinSchedulerPlugin{priority: priority}
}

// Name 返回插件名称
func (rrsp *RoundRobinSchedulerPlugin) Name() string {
	return "round-robin-scheduler"
}

// CreateScheduler 创建调度器
func (rrsp *RoundRobinSchedulerPlugin) CreateScheduler(config *Config) (Scheduler, error) {
	// 这里应该返回实际的轮询调度器实现
	// 为了示例，返回一个简单的实现
	return &roundRobinScheduler{}, nil
}

// Priority 返回优先级
func (rrsp *RoundRobinSchedulerPlugin) Priority() int {
	return rrsp.priority
}

// roundRobinScheduler 轮询调度器实现
type roundRobinScheduler struct {
	// 实际实现会更复杂
}

// Schedule 调度任务
func (rrs *roundRobinScheduler) Schedule(task Task) error {
	// 简单实现，实际会有更复杂的逻辑
	return nil
}

// Start 启动调度器
func (rrs *roundRobinScheduler) Start() error {
	return nil
}

// Stop 停止调度器
func (rrs *roundRobinScheduler) Stop() error {
	return nil
}

// ExtendedLogger 扩展日志接口
type ExtendedLogger interface {
	Logger // 继承基础Logger接口
	Debug(args ...any)
	Debugf(format string, args ...any)
	Info(args ...any)
	Infof(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
}

// DefaultExtendedLogger 默认扩展日志实现
type DefaultExtendedLogger struct{}

// Printf 实现基础Logger接口
func (del *DefaultExtendedLogger) Printf(format string, v ...any) {
	log.Printf(format, v...)
}

// Println 实现基础Logger接口
func (del *DefaultExtendedLogger) Println(v ...any) {
	log.Println(v...)
}

// Debug 调试日志
func (del *DefaultExtendedLogger) Debug(args ...any) {
	log.Println(append([]any{"[DEBUG]"}, args...)...)
}

// Debugf 格式化调试日志
func (del *DefaultExtendedLogger) Debugf(format string, args ...any) {
	log.Printf("[DEBUG] "+format, args...)
}

// Info 信息日志
func (del *DefaultExtendedLogger) Info(args ...any) {
	log.Println(append([]any{"[INFO]"}, args...)...)
}

// Infof 格式化信息日志
func (del *DefaultExtendedLogger) Infof(format string, args ...any) {
	log.Printf("[INFO] "+format, args...)
}

// Warn 警告日志
func (del *DefaultExtendedLogger) Warn(args ...any) {
	log.Println(append([]any{"[WARN]"}, args...)...)
}

// Warnf 格式化警告日志
func (del *DefaultExtendedLogger) Warnf(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}

// Error 错误日志
func (del *DefaultExtendedLogger) Error(args ...any) {
	log.Println(append([]any{"[ERROR]"}, args...)...)
}

// Errorf 格式化错误日志
func (del *DefaultExtendedLogger) Errorf(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}

// max 返回两个数中的最大值
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
