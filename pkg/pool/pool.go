package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/uptutu/goman/internal/queue"
)

// pool 协程池实现
type pool struct {
	config       *Config              // 配置
	workers      []*poolWorker        // 工作协程列表
	taskQueue    *queue.LockFreeQueue // 全局任务队列
	monitor      Monitor              // 监控器
	objectPool   *poolObjectPool      // 对象池
	errorHandler ErrorHandler         // 错误处理器

	// 状态管理
	state  int32              // 协程池状态 (PoolStateRunning/PoolStateShutdown/PoolStateClosed)
	wg     sync.WaitGroup     // 等待组
	ctx    context.Context    // 上下文
	cancel context.CancelFunc // 取消函数

	// 统计信息
	stats        *poolStats // 统计信息
	startTime    time.Time  // 启动时间
	shutdownTime time.Time  // 关闭时间

	// 生命周期管理
	initOnce     sync.Once // 确保只初始化一次
	shutdownOnce sync.Once // 确保只关闭一次

	// 扩展机制
	extensionManager *ExtensionManager // 扩展管理器
	interceptor      TaskInterceptor   // 任务拦截器

	// 调度相关
	loadBalancer poolLoadBalancer // 负载均衡器
	scheduler    *poolScheduler   // 任务调度器
}

// poolStats 协程池统计信息实现
type poolStats struct {
	// 基础指标
	activeWorkers  int64 // 活跃工作协程数
	queuedTasks    int64 // 排队任务数
	completedTasks int64 // 已完成任务数
	failedTasks    int64 // 失败任务数
	submittedTasks int64 // 已提交任务数

	// 性能指标
	totalDuration   int64   // 总执行时间（纳秒）
	avgTaskDuration int64   // 平均任务执行时间（纳秒）
	throughputTPS   float64 // 每秒处理任务数

	// 资源指标
	memoryUsage int64 // 内存使用量
	gcCount     int64 // GC次数

	// 时间戳
	lastUpdateTime int64 // 最后更新时间
}

// poolObjectPool 对象池管理
type poolObjectPool struct {
	taskPool    *sync.Pool // 任务包装器池
	futurePool  *sync.Pool // Future对象池
	contextPool *sync.Pool // Context对象池
}

// poolTaskWrapper 任务包装器
type poolTaskWrapper struct {
	task       Task               // 原始任务
	future     *poolFuture        // Future对象
	submitTime time.Time          // 提交时间
	priority   int                // 任务优先级
	ctx        context.Context    // 任务上下文
	cancel     context.CancelFunc // 取消函数
	next       *poolTaskWrapper   // 链表指针（用于对象池）
}

// poolFuture Future接口的实现
type poolFuture struct {
	result   any
	err      error
	done     int32
	mu       sync.RWMutex
	waitCh   chan struct{}
	canceled int32
}

// poolWorker 工作协程实现
type poolWorker struct {
	id           int                   // 工作协程ID
	pool         *pool                 // 协程池引用
	taskChan     chan *poolTaskWrapper // 任务通道
	localQueue   []*poolTaskWrapper    // 本地任务队列
	localHead    int                   // 本地队列头部索引
	localTail    int                   // 本地队列尾部索引
	localMutex   sync.RWMutex          // 本地队列读写锁
	lastActive   int64                 // 最后活跃时间（Unix纳秒时间戳）
	taskCount    int64                 // 已处理任务数量
	failedCount  int64                 // 失败任务数量
	isActive     int32                 // 是否活跃（0=非活跃，1=活跃）
	isRunning    int32                 // 是否正在运行（0=停止，1=运行）
	state        int32                 // 工作协程状态（0=空闲，1=忙碌，2=错误）
	ctx          context.Context       // 工作协程上下文
	cancel       context.CancelFunc    // 取消函数
	wg           *sync.WaitGroup       // 等待组
	config       *Config               // 配置
	errorHandler ErrorHandler          // 错误处理器
	monitor      Monitor               // 监控器
	restartCount int32                 // 重启次数
}

// poolScheduler 任务调度器
type poolScheduler struct {
	pool           *pool
	isRunning      int32
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	totalSubmitted int64
	totalScheduled int64
	totalFailed    int64
}

// poolLoadBalancer 负载均衡器接口
type poolLoadBalancer interface {
	SelectWorker(workers []*poolWorker) *poolWorker
}

// roundRobinBalancer 轮询负载均衡器
type roundRobinBalancer struct {
	counter uint64
}

// NewPool 创建新的协程池
func NewPool(config *Config) (Pool, error) {
	if config == nil {
		config = NewConfig()
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, NewPoolError("NewPool", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 创建对象池
	objectPool := &poolObjectPool{
		taskPool: &sync.Pool{
			New: func() any {
				return &poolTaskWrapper{}
			},
		},
		futurePool: &sync.Pool{
			New: func() any {
				return newPoolFuture()
			},
		},
		contextPool: &sync.Pool{
			New: func() any {
				return context.Background()
			},
		},
	}

	// 创建统计信息
	stats := &poolStats{
		lastUpdateTime: time.Now().UnixNano(),
	}

	// 初始化内存使用情况
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	atomic.StoreInt64(&stats.memoryUsage, int64(memStats.Alloc))
	atomic.StoreInt64(&stats.gcCount, int64(memStats.NumGC))

	// 创建协程池实例
	p := &pool{
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		objectPool:       objectPool,
		stats:            stats,
		startTime:        time.Now(),
		state:            PoolStateRunning,
		loadBalancer:     &roundRobinBalancer{},
		extensionManager: NewExtensionManager(),
	}

	// 初始化组件
	if err := p.initialize(); err != nil {
		return nil, NewPoolError("NewPool", err)
	}

	return p, nil
}

// initialize 初始化协程池组件
func (p *pool) initialize() error {
	var initErr error
	p.initOnce.Do(func() {
		// 创建全局任务队列
		p.taskQueue = queue.NewLockFreeQueue(uint64(p.config.QueueSize))

		// 创建默认错误处理器
		if p.config.PanicHandler != nil {
			p.errorHandler = NewErrorHandlerWithLogger(&defaultLogger{})
		} else {
			p.errorHandler = NewDefaultErrorHandler()
		}

		// 创建监控器
		p.monitor = NewMonitor(p.config)

		// 创建工作协程
		p.workers = make([]*poolWorker, p.config.WorkerCount)
		for i := 0; i < p.config.WorkerCount; i++ {
			p.workers[i] = newPoolWorker(i, p, p.config, p.errorHandler, p.monitor)
		}

		// 创建调度器
		p.scheduler = newPoolScheduler(p)

		// 设置任务拦截器
		if p.config.TaskInterceptor != nil {
			p.interceptor = p.config.TaskInterceptor
		}

		// 启动组件
		initErr = p.startComponents()
	})

	return initErr
}

// startComponents 启动所有组件
func (p *pool) startComponents() error {
	// 启动监控器
	p.monitor.Start()

	// 启动调度器
	if err := p.scheduler.start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// 启动工作协程
	for _, w := range p.workers {
		w.start(&p.wg)
	}

	// 启动扩展
	if err := p.extensionManager.StartAllPlugins(); err != nil {
		return fmt.Errorf("failed to start plugins: %w", err)
	}

	// 触发启动事件
	p.extensionManager.EmitPoolStart()

	return nil
}

// Submit 提交任务到协程池
func (p *pool) Submit(task Task) error {
	state := atomic.LoadInt32(&p.state)
	switch state {
	case PoolStateShutdown:
		return NewPoolError("Submit", ErrShutdownInProgress)
	case PoolStateClosed:
		return NewPoolError("Submit", ErrPoolClosed)
	case PoolStateRunning:
		// 继续执行
	default:
		return NewPoolError("Submit", ErrPoolClosed)
	}

	if task == nil {
		return NewPoolError("Submit", fmt.Errorf("task cannot be nil"))
	}

	// 增加提交计数
	atomic.AddInt64(&p.stats.submittedTasks, 1)

	// 应用中间件和拦截器
	ctx := p.ctx
	if p.interceptor != nil {
		ctx = p.interceptor.Before(ctx, task)
	}

	// 应用扩展管理器中的中间件
	ctx = p.extensionManager.ExecuteMiddlewareBefore(ctx, task)

	// 调度任务
	if err := p.scheduler.schedule(task); err != nil {
		atomic.AddInt64(&p.stats.failedTasks, 1)
		return NewPoolError("Submit", err)
	}

	// 触发任务提交事件
	p.extensionManager.EmitTaskSubmit(task)

	return nil
}

// SubmitWithTimeout 带超时的任务提交
func (p *pool) SubmitWithTimeout(task Task, timeout time.Duration) error {
	state := atomic.LoadInt32(&p.state)
	switch state {
	case PoolStateShutdown:
		return NewPoolError("SubmitWithTimeout", ErrShutdownInProgress)
	case PoolStateClosed:
		return NewPoolError("SubmitWithTimeout", ErrPoolClosed)
	case PoolStateRunning:
		// 继续执行
	default:
		return NewPoolError("SubmitWithTimeout", ErrPoolClosed)
	}

	if task == nil {
		return NewPoolError("SubmitWithTimeout", fmt.Errorf("task cannot be nil"))
	}

	if timeout <= 0 {
		return NewPoolError("SubmitWithTimeout", fmt.Errorf("timeout must be positive"))
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()

	// 创建包装任务
	wrappedTask := &timeoutTask{
		originalTask: task,
		timeout:      timeout,
		ctx:          ctx,
	}

	return p.Submit(wrappedTask)
}

// SubmitAsync 异步提交任务，返回Future
func (p *pool) SubmitAsync(task Task) Future {
	state := atomic.LoadInt32(&p.state)
	future := newPoolFuture()

	switch state {
	case PoolStateShutdown:
		go func() {
			future.setResult(nil, NewPoolError("SubmitAsync", ErrShutdownInProgress))
		}()
		return future
	case PoolStateClosed:
		go func() {
			future.setResult(nil, NewPoolError("SubmitAsync", ErrPoolClosed))
		}()
		return future
	case PoolStateRunning:
		// 继续执行
	default:
		go func() {
			future.setResult(nil, NewPoolError("SubmitAsync", ErrPoolClosed))
		}()
		return future
	}

	if task == nil {
		future := newPoolFuture()
		go func() {
			future.setResult(nil, NewPoolError("SubmitAsync", fmt.Errorf("task cannot be nil")))
		}()
		return future
	}

	// 创建Future
	future = p.objectPool.futurePool.Get().(*poolFuture)

	// 创建异步任务包装器
	asyncTask := &asyncTask{
		originalTask: task,
		future:       future,
	}

	// 提交任务
	if err := p.Submit(asyncTask); err != nil {
		go func() {
			future.setResult(nil, err)
		}()
	}

	return future
}

// Shutdown 优雅关闭协程池
func (p *pool) Shutdown(ctx context.Context) error {
	var shutdownErr error
	p.shutdownOnce.Do(func() {
		// 设置关闭状态
		atomic.StoreInt32(&p.state, PoolStateShutdown)
		p.shutdownTime = time.Now()

		// 创建关闭超时上下文
		shutdownCtx := ctx
		if shutdownCtx == nil {
			var cancel context.CancelFunc
			shutdownCtx, cancel = context.WithTimeout(context.Background(), p.config.ShutdownTimeout)
			defer cancel()
		}

		// 触发关闭事件
		p.extensionManager.EmitPoolShutdown()

		// 执行优雅关闭流程
		shutdownErr = p.performGracefulShutdown(shutdownCtx)
	})

	return shutdownErr
}

// performGracefulShutdown 执行优雅关闭流程
func (p *pool) performGracefulShutdown(ctx context.Context) error {
	// 第一阶段：停止接收新任务
	p.cancel()

	// 第二阶段：停止调度器，不再分发新任务
	if p.scheduler != nil {
		if err := p.scheduler.stop(); err != nil {
			return NewPoolError("Shutdown", fmt.Errorf("failed to stop scheduler: %w", err))
		}
	}

	// 第三阶段：等待正在执行的任务完成
	if err := p.waitForRunningTasks(ctx); err != nil {
		return err
	}

	// 第四阶段：停止工作协程
	p.stopWorkers()

	// 第五阶段：等待工作协程完全停止
	if err := p.waitForWorkers(ctx); err != nil {
		return err
	}

	// 第六阶段：清理剩余资源
	if err := p.cleanupResources(); err != nil {
		return err
	}

	// 第七阶段：停止监控和插件
	if err := p.stopMonitoringAndPlugins(); err != nil {
		return err
	}

	// 设置最终状态
	atomic.StoreInt32(&p.state, PoolStateClosed)
	return nil
}

// waitForRunningTasks 等待正在执行的任务完成
func (p *pool) waitForRunningTasks(ctx context.Context) error {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// 超时，执行强制关闭
			return p.forceShutdown()
		case <-ticker.C:
			// 检查是否还有活跃的工作协程
			activeWorkers := int64(0)
			for _, w := range p.workers {
				if w.IsActive() {
					activeWorkers++
				}
			}

			// 如果没有活跃的工作协程，说明所有任务都已完成
			if activeWorkers == 0 {
				return nil
			}
		}
	}
}

// forceShutdown 强制关闭，取消所有正在执行的任务
func (p *pool) forceShutdown() error {
	// 强制取消所有工作协程的上下文
	for _, w := range p.workers {
		w.cancel()
	}

	// 清理队列中的剩余任务
	p.clearPendingTasks()

	return NewPoolError("Shutdown", fmt.Errorf("shutdown timeout exceeded, forced shutdown"))
}

// stopWorkers 停止所有工作协程
func (p *pool) stopWorkers() {
	for _, w := range p.workers {
		w.stop()
	}
}

// waitForWorkers 等待所有工作协程停止
func (p *pool) waitForWorkers(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.wg.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return NewPoolError("Shutdown", fmt.Errorf("timeout waiting for workers to stop"))
	}
}

// clearPendingTasks 清理队列中的待处理任务
func (p *pool) clearPendingTasks() {
	clearedCount := int64(0)

	// 清理全局队列
	for {
		taskWrapper := p.taskQueue.Dequeue()
		if taskWrapper == nil {
			break
		}

		tw := (*poolTaskWrapper)(taskWrapper)
		if tw.future != nil {
			tw.future.setResult(nil, ErrPoolClosed)
		}

		// 回收任务包装器
		p.recycleTaskWrapper(tw)
		clearedCount++
	}

	// 更新统计信息
	atomic.AddInt64(&p.stats.failedTasks, clearedCount)
}

// cleanupResources 清理资源
func (p *pool) cleanupResources() error {
	// 清理对象池
	if p.objectPool != nil {
		// 对象池会被GC自动回收，这里主要是确保没有循环引用
		p.objectPool.taskPool = nil
		p.objectPool.futurePool = nil
		p.objectPool.contextPool = nil
	}

	// 清理任务队列
	if p.taskQueue != nil {
		p.clearPendingTasks()
	}

	return nil
}

// stopMonitoringAndPlugins 停止监控器和插件
func (p *pool) stopMonitoringAndPlugins() error {
	// 停止监控器
	p.monitor.Stop()

	// 停止扩展管理器中的插件
	if err := p.extensionManager.StopAllPlugins(); err != nil {
		return NewPoolError("Shutdown", fmt.Errorf("failed to stop plugins: %w", err))
	}

	return nil
}

// recycleTaskWrapper 回收任务包装器
func (p *pool) recycleTaskWrapper(wrapper *poolTaskWrapper) {
	if wrapper == nil {
		return
	}

	// 清理包装器
	wrapper.task = nil
	wrapper.future = nil
	wrapper.ctx = nil
	wrapper.cancel = nil
	wrapper.next = nil
	wrapper.submitTime = time.Time{}
	wrapper.priority = 0

	// 回收到对象池
	if p.objectPool != nil && p.objectPool.taskPool != nil {
		p.objectPool.taskPool.Put(wrapper)
	}
}

// Stats 获取协程池统计信息
func (p *pool) Stats() PoolStats {
	// 更新实时统计信息
	p.updateStats()

	return PoolStats{
		ActiveWorkers:   atomic.LoadInt64(&p.stats.activeWorkers),
		QueuedTasks:     atomic.LoadInt64(&p.stats.queuedTasks),
		CompletedTasks:  atomic.LoadInt64(&p.stats.completedTasks),
		FailedTasks:     atomic.LoadInt64(&p.stats.failedTasks),
		AvgTaskDuration: time.Duration(atomic.LoadInt64(&p.stats.avgTaskDuration)),
		ThroughputTPS:   p.stats.throughputTPS,
		MemoryUsage:     atomic.LoadInt64(&p.stats.memoryUsage),
		GCCount:         atomic.LoadInt64(&p.stats.gcCount),
	}
}

// updateStats 更新统计信息
func (p *pool) updateStats() {
	now := time.Now().UnixNano()
	lastUpdate := atomic.LoadInt64(&p.stats.lastUpdateTime)

	// 避免频繁更新
	if now-lastUpdate < int64(time.Second) {
		return
	}

	if !atomic.CompareAndSwapInt64(&p.stats.lastUpdateTime, lastUpdate, now) {
		return // 其他协程正在更新
	}

	// 更新活跃工作协程数
	activeCount := int64(0)
	for _, w := range p.workers {
		if w.IsActive() {
			activeCount++
		}
	}
	atomic.StoreInt64(&p.stats.activeWorkers, activeCount)

	// 更新队列任务数
	queueSize := int64(p.taskQueue.Size())
	atomic.StoreInt64(&p.stats.queuedTasks, queueSize)

	// 更新内存使用情况
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	atomic.StoreInt64(&p.stats.memoryUsage, int64(memStats.Alloc))
	atomic.StoreInt64(&p.stats.gcCount, int64(memStats.NumGC))

	// 计算吞吐量
	completedTasks := atomic.LoadInt64(&p.stats.completedTasks)
	if completedTasks > 0 {
		duration := time.Since(p.startTime).Seconds()
		if duration > 0 {
			p.stats.throughputTPS = float64(completedTasks) / duration
		}
	}
}

// IsRunning 检查协程池是否正在运行
func (p *pool) IsRunning() bool {
	return atomic.LoadInt32(&p.state) == PoolStateRunning
}

// IsShutdown 检查协程池是否正在关闭
func (p *pool) IsShutdown() bool {
	return atomic.LoadInt32(&p.state) == PoolStateShutdown
}

// IsClosed 检查协程池是否已关闭
func (p *pool) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == PoolStateClosed
}

// Extension management methods implementation

// AddMiddleware 添加中间件
func (p *pool) AddMiddleware(middleware Middleware) {
	p.extensionManager.AddMiddleware(middleware)
}

// RemoveMiddleware 移除中间件
func (p *pool) RemoveMiddleware(middleware Middleware) {
	p.extensionManager.RemoveMiddleware(middleware)
}

// RegisterPlugin 注册插件
func (p *pool) RegisterPlugin(plugin Plugin) error {
	return p.extensionManager.RegisterPlugin(plugin)
}

// UnregisterPlugin 注销插件
func (p *pool) UnregisterPlugin(name string) error {
	return p.extensionManager.UnregisterPlugin(name)
}

// GetPlugin 获取插件
func (p *pool) GetPlugin(name string) (Plugin, bool) {
	return p.extensionManager.GetPlugin(name)
}

// AddEventListener 添加事件监听器
func (p *pool) AddEventListener(listener EventListener) {
	p.extensionManager.AddEventListener(listener)
}

// RemoveEventListener 移除事件监听器
func (p *pool) RemoveEventListener(listener EventListener) {
	p.extensionManager.RemoveEventListener(listener)
}

// SetSchedulerPlugin 设置调度器插件
func (p *pool) SetSchedulerPlugin(plugin SchedulerPlugin) {
	p.extensionManager.SetSchedulerPlugin(plugin)
}

// GetSchedulerPlugin 获取调度器插件
func (p *pool) GetSchedulerPlugin() SchedulerPlugin {
	return p.extensionManager.GetSchedulerPlugin()
}

// newPoolFuture 创建新的Future
func newPoolFuture() *poolFuture {
	return &poolFuture{
		waitCh: make(chan struct{}),
	}
}

// Get 获取结果（阻塞）
func (f *poolFuture) Get() (any, error) {
	<-f.waitCh
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.result, f.err
}

// GetWithTimeout 带超时获取结果
func (f *poolFuture) GetWithTimeout(timeout time.Duration) (any, error) {
	select {
	case <-f.waitCh:
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.result, f.err
	case <-time.After(timeout):
		return nil, ErrTaskTimeout
	}
}

// IsDone 检查是否完成
func (f *poolFuture) IsDone() bool {
	return atomic.LoadInt32(&f.done) == 1
}

// Cancel 取消任务
func (f *poolFuture) Cancel() bool {
	if atomic.CompareAndSwapInt32(&f.canceled, 0, 1) {
		f.setResult(nil, ErrTaskCancelled)
		return true
	}
	return false
}

// setResult 设置结果（内部方法）
func (f *poolFuture) setResult(result any, err error) {
	if atomic.CompareAndSwapInt32(&f.done, 0, 1) {
		f.mu.Lock()
		f.result = result
		f.err = err
		f.mu.Unlock()
		close(f.waitCh)
	}
}

// newPoolWorker 创建新的工作协程
func newPoolWorker(id int, poolRef *pool, config *Config, errorHandler ErrorHandler, monitor Monitor) *poolWorker {
	ctx, cancel := context.WithCancel(context.Background())

	worker := &poolWorker{
		id:           id,
		pool:         poolRef,
		taskChan:     make(chan *poolTaskWrapper, 1), // 小缓冲区
		localQueue:   make([]*poolTaskWrapper, LocalQueueSize),
		lastActive:   time.Now().UnixNano(),
		ctx:          ctx,
		cancel:       cancel,
		config:       config,
		errorHandler: errorHandler,
		monitor:      monitor,
	}

	return worker
}

// start 启动工作协程
func (w *poolWorker) start(wg *sync.WaitGroup) {
	w.wg = wg
	atomic.StoreInt32(&w.isRunning, 1)
	w.wg.Add(1)
	go w.run()
}

// stop 停止工作协程
func (w *poolWorker) stop() {
	atomic.StoreInt32(&w.isRunning, 0)
	w.cancel()
}

// ID 获取工作协程ID
func (w *poolWorker) ID() int {
	return w.id
}

// IsActive 检查是否活跃（实现Worker接口）
func (w *poolWorker) IsActive() bool {
	return atomic.LoadInt32(&w.isActive) == 1
}

// IsRunning 检查是否正在运行
func (w *poolWorker) IsRunning() bool {
	return atomic.LoadInt32(&w.isRunning) == 1
}

// TaskCount 获取已处理任务数量
func (w *poolWorker) TaskCount() int64 {
	return atomic.LoadInt64(&w.taskCount)
}

// LastActiveTime 获取最后活跃时间
func (w *poolWorker) LastActiveTime() time.Time {
	timestamp := atomic.LoadInt64(&w.lastActive)
	return time.Unix(0, timestamp)
}

// RestartCount 获取重启次数
func (w *poolWorker) RestartCount() int32 {
	return atomic.LoadInt32(&w.restartCount)
}

// LocalQueueSize 获取本地队列当前大小
func (w *poolWorker) LocalQueueSize() int {
	w.localMutex.RLock()
	defer w.localMutex.RUnlock()

	if w.localTail >= w.localHead {
		return w.localTail - w.localHead
	}
	return LocalQueueSize - w.localHead + w.localTail
}

// submitTask 提交任务到工作协程
func (w *poolWorker) submitTask(taskWrapper *poolTaskWrapper) bool {
	// 首先尝试放入本地队列
	if w.enqueueLocal(taskWrapper) {
		return true
	}

	// 本地队列满，尝试通过通道发送
	select {
	case w.taskChan <- taskWrapper:
		return true
	default:
		return false // 通道也满了
	}
}

// run 工作协程主循环
func (w *poolWorker) run() {
	defer func() {
		atomic.StoreInt32(&w.isRunning, 0)
		w.wg.Done()
		w.cleanup()
	}()

	// 设置panic恢复
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt32(&w.restartCount, 1)
			atomic.StoreInt32(&w.state, 2) // 设置为错误状态

			if w.errorHandler != nil {
				w.errorHandler.HandlePanic(w.id, nil, r)
			}
		}
	}()

	// 工作协程主循环
	idleCount := 0
	for atomic.LoadInt32(&w.isRunning) == 1 {
		select {
		case <-w.ctx.Done():
			return
		case taskWrapper := <-w.taskChan:
			w.executeTask(taskWrapper)
			idleCount = 0 // 重置空闲计数
		default:
			// 检查本地队列
			if taskWrapper := w.dequeueLocal(); taskWrapper != nil {
				w.executeTask(taskWrapper)
				idleCount = 0 // 重置空闲计数
			} else {
				// 没有任务，增加空闲计数
				idleCount++

				// 自适应休眠策略
				if idleCount < 10 {
					// 短暂自旋等待
					continue
				} else if idleCount < 100 {
					// 短暂休眠
					time.Sleep(time.Microsecond * 10)
				} else {
					// 较长休眠，避免CPU浪费
					time.Sleep(time.Microsecond * 100)
				}
			}
		}
	}
}

// executeTask 执行任务
func (w *poolWorker) executeTask(taskWrapper *poolTaskWrapper) {
	if taskWrapper == nil {
		return
	}

	// 更新活跃状态和时间
	atomic.StoreInt32(&w.isActive, 1)
	atomic.StoreInt32(&w.state, 1) // 设置为忙碌状态
	atomic.StoreInt64(&w.lastActive, time.Now().UnixNano())

	startTime := time.Now()
	var result any
	var err error
	var panicRecovered bool

	// 设置任务级别的panic恢复
	defer func() {
		if r := recover(); r != nil {
			panicRecovered = true
			err = NewPanicError(r, taskWrapper.task)
			atomic.StoreInt32(&w.state, 2) // 设置为错误状态
			atomic.AddInt64(&w.failedCount, 1)

			if w.errorHandler != nil {
				w.errorHandler.HandlePanic(w.id, taskWrapper.task, r)
			}

			// 触发工作协程panic事件
			if w.pool.extensionManager != nil {
				w.pool.extensionManager.EmitWorkerPanic(w.id, r)
			}
		}

		// 更新统计信息
		duration := time.Since(startTime)
		atomic.AddInt64(&w.taskCount, 1)
		atomic.StoreInt32(&w.isActive, 0)

		// 如果没有panic，恢复到空闲状态
		if !panicRecovered {
			if err != nil {
				atomic.AddInt64(&w.failedCount, 1)
			}
			atomic.StoreInt32(&w.state, 0) // 设置为空闲状态
		}

		// 记录监控信息
		if w.monitor != nil {
			if err != nil {
				w.monitor.RecordTaskFail(taskWrapper.task, err)
			} else {
				w.monitor.RecordTaskComplete(taskWrapper.task, duration)
			}
		}

		// 执行中间件后置处理
		if w.pool.extensionManager != nil {
			w.pool.extensionManager.ExecuteMiddlewareAfter(taskWrapper.ctx, taskWrapper.task, result, err)
		}

		// 执行任务拦截器后置处理
		if w.pool.interceptor != nil {
			w.pool.interceptor.After(taskWrapper.ctx, taskWrapper.task, result, err)
		}

		// 触发任务完成事件
		if w.pool.extensionManager != nil {
			w.pool.extensionManager.EmitTaskComplete(taskWrapper.task, result)
		}

		// 设置Future结果
		if taskWrapper.future != nil {
			taskWrapper.future.setResult(result, err)
		}

		// 回收任务包装器
		w.recycleTaskWrapper(taskWrapper)
	}()

	// 创建任务执行上下文
	taskCtx := taskWrapper.ctx
	if taskCtx == nil {
		taskCtx = context.Background()
	}

	// 添加超时控制
	if w.config.TaskTimeout > 0 {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(taskCtx, w.config.TaskTimeout)
		defer cancel()
	}

	// 执行任务
	result, err = taskWrapper.task.Execute(taskCtx)

	// 检查是否超时
	if err != nil && taskCtx.Err() == context.DeadlineExceeded {
		err = NewTimeoutError(w.config.TaskTimeout, taskWrapper.task)
		if w.errorHandler != nil {
			w.errorHandler.HandleTimeout(taskWrapper.task, w.config.TaskTimeout)
		}
	}
}

// enqueueLocal 将任务加入本地队列
func (w *poolWorker) enqueueLocal(taskWrapper *poolTaskWrapper) bool {
	w.localMutex.Lock()
	defer w.localMutex.Unlock()

	// 检查本地队列是否已满
	nextTail := (w.localTail + 1) % LocalQueueSize
	if nextTail == w.localHead {
		return false // 队列已满
	}

	w.localQueue[w.localTail] = taskWrapper
	w.localTail = nextTail
	return true
}

// dequeueLocal 从本地队列取出任务
func (w *poolWorker) dequeueLocal() *poolTaskWrapper {
	w.localMutex.Lock()
	defer w.localMutex.Unlock()

	if w.localHead == w.localTail {
		return nil // 队列为空
	}

	task := w.localQueue[w.localHead]
	w.localQueue[w.localHead] = nil // 清空引用
	w.localHead = (w.localHead + 1) % LocalQueueSize
	return task
}

// recycleTaskWrapper 回收任务包装器
func (w *poolWorker) recycleTaskWrapper(wrapper *poolTaskWrapper) {
	if wrapper == nil {
		return
	}

	// 清理包装器
	wrapper.task = nil
	wrapper.future = nil
	wrapper.ctx = nil
	wrapper.cancel = nil
	wrapper.next = nil
	wrapper.submitTime = time.Time{}
	wrapper.priority = 0

	// 回收到对象池
	w.pool.objectPool.taskPool.Put(wrapper)
}

// cleanup 清理资源
func (w *poolWorker) cleanup() {
	// 处理剩余的本地队列任务
	for {
		task := w.dequeueLocal()
		if task == nil {
			break
		}

		// 取消剩余任务
		if task.future != nil {
			task.future.setResult(nil, ErrPoolClosed)
		}

		w.recycleTaskWrapper(task)
	}

	// 处理通道中的剩余任务
	for {
		select {
		case task := <-w.taskChan:
			if task.future != nil {
				task.future.setResult(nil, ErrPoolClosed)
			}
			w.recycleTaskWrapper(task)
		default:
			return
		}
	}
}

// newPoolScheduler 创建新的调度器
func newPoolScheduler(pool *pool) *poolScheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &poolScheduler{
		pool:   pool,
		ctx:    ctx,
		cancel: cancel,
	}
}

// start 启动调度器
func (s *poolScheduler) start() error {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 0, 1) {
		return fmt.Errorf("scheduler is already running")
	}

	// 启动调度协程
	s.wg.Add(1)
	go s.scheduleLoop()

	return nil
}

// stop 停止调度器
func (s *poolScheduler) stop() error {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 1, 0) {
		return nil // 已经停止
	}

	s.cancel()
	s.wg.Wait()

	return nil
}

// schedule 调度任务
func (s *poolScheduler) schedule(task Task) error {
	if atomic.LoadInt32(&s.isRunning) == 0 {
		return ErrPoolClosed
	}

	atomic.AddInt64(&s.totalSubmitted, 1)

	// 记录任务提交
	if s.pool.monitor != nil {
		s.pool.monitor.RecordTaskSubmit(task)
	}

	// 创建任务包装器
	taskWrapper := s.pool.objectPool.taskPool.Get().(*poolTaskWrapper)
	taskWrapper.task = task
	taskWrapper.future = s.pool.objectPool.futurePool.Get().(*poolFuture)
	taskWrapper.submitTime = time.Now()
	taskWrapper.priority = task.Priority()
	taskWrapper.ctx = s.pool.ctx

	// 尝试直接分配给工作协程
	if s.tryDirectSchedule(taskWrapper) {
		atomic.AddInt64(&s.totalScheduled, 1)
		return nil
	}

	// 直接分配失败，放入全局队列
	if s.pool.taskQueue.Enqueue(unsafe.Pointer(taskWrapper)) {
		return nil
	}

	// 队列满了
	atomic.AddInt64(&s.totalFailed, 1)
	if s.pool.errorHandler != nil {
		return s.pool.errorHandler.HandleQueueFull(task)
	}

	return ErrQueueFull
}

// tryDirectSchedule 尝试直接调度任务到工作协程
func (s *poolScheduler) tryDirectSchedule(taskWrapper *poolTaskWrapper) bool {
	// 使用负载均衡器选择工作协程
	selectedWorker := s.pool.loadBalancer.SelectWorker(s.pool.workers)
	if selectedWorker == nil {
		return false
	}

	// 尝试提交任务到选中的工作协程
	return selectedWorker.submitTask(taskWrapper)
}

// scheduleLoop 主调度循环
func (s *poolScheduler) scheduleLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Millisecond * 10) // 每10ms检查一次
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processGlobalQueue()
		}
	}
}

// processGlobalQueue 处理全局队列中的任务
func (s *poolScheduler) processGlobalQueue() {
	// 批量处理任务以提高效率
	batchSize := 10
	for i := 0; i < batchSize; i++ {
		taskWrapper := s.pool.taskQueue.Dequeue()
		if taskWrapper == nil {
			break
		}

		tw := (*poolTaskWrapper)(taskWrapper)
		if s.tryDirectSchedule(tw) {
			atomic.AddInt64(&s.totalScheduled, 1)
		} else {
			// 重新放回队列
			s.pool.taskQueue.Enqueue(unsafe.Pointer(tw))
			break // 避免无限循环
		}
	}
}

// SelectWorker 轮询负载均衡器选择工作协程
func (rb *roundRobinBalancer) SelectWorker(workers []*poolWorker) *poolWorker {
	if len(workers) == 0 {
		return nil
	}

	// 过滤出运行中的工作协程
	runningWorkers := make([]*poolWorker, 0, len(workers))
	for _, worker := range workers {
		if worker.IsRunning() {
			runningWorkers = append(runningWorkers, worker)
		}
	}

	if len(runningWorkers) == 0 {
		return nil
	}

	// 原子递增计数器并取模
	index := atomic.AddUint64(&rb.counter, 1) % uint64(len(runningWorkers))
	return runningWorkers[index]
}

// timeoutTask 带超时的任务包装器
type timeoutTask struct {
	originalTask Task
	timeout      time.Duration
	ctx          context.Context
}

func (t *timeoutTask) Execute(ctx context.Context) (any, error) {
	// 使用更严格的超时上下文
	timeoutCtx := t.ctx
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// 在超时上下文中执行原始任务
	return t.originalTask.Execute(timeoutCtx)
}

func (t *timeoutTask) Priority() int {
	return t.originalTask.Priority()
}

// asyncTask 异步任务包装器
type asyncTask struct {
	originalTask Task
	future       *poolFuture
}

func (t *asyncTask) Execute(ctx context.Context) (any, error) {
	result, err := t.originalTask.Execute(ctx)

	// 设置Future结果
	t.future.setResult(result, err)

	return result, err
}

func (t *asyncTask) Priority() int {
	return t.originalTask.Priority()
}

// defaultLogger 默认日志实现（用于错误处理器）
type defaultLogger struct{}

func (l *defaultLogger) Printf(format string, v ...any) {
	fmt.Printf(format, v...)
}

func (l *defaultLogger) Println(v ...any) {
	fmt.Println(v...)
}

// noopMonitor 空监控器（当监控被禁用时使用）
type noopMonitor struct{}

func (m *noopMonitor) RecordTaskSubmit(task Task)                           {}
func (m *noopMonitor) RecordTaskComplete(task Task, duration time.Duration) {}
func (m *noopMonitor) RecordTaskFail(task Task, err error)                  {}
func (m *noopMonitor) GetStats() PoolStats                                  { return PoolStats{} }
func (m *noopMonitor) Start()                                               {}
func (m *noopMonitor) Stop()                                                {}
func (m *noopMonitor) SetActiveWorkers(count int64)                         {}
func (m *noopMonitor) SetQueuedTasks(count int64)                           {}
