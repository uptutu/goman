package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// Worker 工作协程实现
type Worker struct {
	id           int                // 工作协程ID
	pool         pool.Pool          // 协程池引用
	taskChan     chan *TaskWrapper  // 任务通道
	localQueue   []*TaskWrapper     // 本地任务队列
	localHead    int                // 本地队列头部索引
	localTail    int                // 本地队列尾部索引
	localMutex   sync.RWMutex       // 本地队列读写锁
	lastActive   int64              // 最后活跃时间（Unix纳秒时间戳）
	taskCount    int64              // 已处理任务数量
	failedCount  int64              // 失败任务数量
	isActive     int32              // 是否活跃（0=非活跃，1=活跃）
	isRunning    int32              // 是否正在运行（0=停止，1=运行）
	state        int32              // 工作协程状态（0=空闲，1=忙碌，2=错误）
	ctx          context.Context    // 工作协程上下文
	cancel       context.CancelFunc // 取消函数
	wg           *sync.WaitGroup    // 等待组
	config       *pool.Config       // 配置
	errorHandler pool.ErrorHandler  // 错误处理器
	monitor      pool.Monitor       // 监控器
	objectCache  *ObjectCache       // 本地对象缓存
	restartCount int32              // 重启次数
	maxRestarts  int32              // 最大重启次数
	backoffBase  time.Duration      // 退避基础时间
}

// TaskWrapper 任务包装器
type TaskWrapper struct {
	task       pool.Task          // 原始任务
	future     pool.Future        // Future对象
	submitTime time.Time          // 提交时间
	priority   int                // 任务优先级
	ctx        context.Context    // 任务上下文
	cancel     context.CancelFunc // 取消函数
	next       *TaskWrapper       // 链表指针（用于对象池）
}

// ObjectCache 本地对象缓存
type ObjectCache struct {
	taskWrappers []*TaskWrapper    // 任务包装器缓存
	contexts     []context.Context // 上下文缓存
	cacheSize    int               // 缓存大小
	taskIndex    int               // 任务包装器索引
	ctxIndex     int               // 上下文索引
}

// NewWorker 创建新的工作协程
func NewWorker(id int, poolRef pool.Pool, config *pool.Config, errorHandler pool.ErrorHandler, monitor pool.Monitor) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建本地对象缓存
	objectCache := &ObjectCache{
		taskWrappers: make([]*TaskWrapper, pool.LocalQueueSize),
		contexts:     make([]context.Context, pool.LocalQueueSize),
		cacheSize:    pool.LocalQueueSize,
	}

	// 预分配任务包装器
	for i := 0; i < pool.LocalQueueSize; i++ {
		objectCache.taskWrappers[i] = &TaskWrapper{}
	}

	worker := &Worker{
		id:           id,
		pool:         poolRef,
		taskChan:     make(chan *TaskWrapper, 1), // 小缓冲区
		localQueue:   make([]*TaskWrapper, pool.LocalQueueSize),
		lastActive:   time.Now().UnixNano(),
		ctx:          ctx,
		cancel:       cancel,
		config:       config,
		errorHandler: errorHandler,
		monitor:      monitor,
		objectCache:  objectCache,
		maxRestarts:  10,                     // 最大重启10次
		backoffBase:  time.Millisecond * 100, // 基础退避时间100ms
	}

	return worker
}

// NewTaskWrapper 创建新的任务包装器
func NewTaskWrapper(task pool.Task, future pool.Future, ctx context.Context) *TaskWrapper {
	if ctx == nil {
		ctx = context.Background()
	}

	taskCtx, cancel := context.WithCancel(ctx)

	return &TaskWrapper{
		task:       task,
		future:     future,
		submitTime: time.Now(),
		priority:   task.Priority(),
		ctx:        taskCtx,
		cancel:     cancel,
	}
}

// Start 启动工作协程
func (w *Worker) Start(wg *sync.WaitGroup) {
	w.wg = wg
	atomic.StoreInt32(&w.isRunning, 1)
	w.wg.Add(1)
	go w.run()
}

// Stop 停止工作协程
func (w *Worker) Stop() {
	atomic.StoreInt32(&w.isRunning, 0)
	w.cancel()
}

// ID 获取工作协程ID
func (w *Worker) ID() int {
	return w.id
}

// IsActive 检查是否活跃
func (w *Worker) IsActive() bool {
	return atomic.LoadInt32(&w.isActive) == 1
}

// TaskCount 获取已处理任务数量
func (w *Worker) TaskCount() int64 {
	return atomic.LoadInt64(&w.taskCount)
}

// LastActiveTime 获取最后活跃时间
func (w *Worker) LastActiveTime() time.Time {
	timestamp := atomic.LoadInt64(&w.lastActive)
	return time.Unix(0, timestamp)
}

// IsRunning 检查是否正在运行
func (w *Worker) IsRunning() bool {
	return atomic.LoadInt32(&w.isRunning) == 1
}

// RestartCount 获取重启次数
func (w *Worker) RestartCount() int32 {
	return atomic.LoadInt32(&w.restartCount)
}

// FailedCount 获取失败任务数量
func (w *Worker) FailedCount() int64 {
	return atomic.LoadInt64(&w.failedCount)
}

// State 获取工作协程状态
func (w *Worker) State() int32 {
	return atomic.LoadInt32(&w.state)
}

// IsIdle 检查是否空闲
func (w *Worker) IsIdle() bool {
	return atomic.LoadInt32(&w.state) == 0
}

// IsBusy 检查是否忙碌
func (w *Worker) IsBusy() bool {
	return atomic.LoadInt32(&w.state) == 1
}

// IsError 检查是否处于错误状态
func (w *Worker) IsError() bool {
	return atomic.LoadInt32(&w.state) == 2
}

// CanStealWork 检查是否可以从该工作协程窃取任务
func (w *Worker) CanStealWork() bool {
	return w.LocalQueueSize() > 1
}

// StealWork 从该工作协程窃取一个任务（供其他工作协程调用）
func (w *Worker) StealWork() *TaskWrapper {
	if !w.CanStealWork() {
		return nil
	}
	return w.stealFromLocal()
}

// GetStats 获取工作协程统计信息
func (w *Worker) GetStats() WorkerStats {
	return WorkerStats{
		ID:             w.id,
		IsActive:       w.IsActive(),
		IsRunning:      w.IsRunning(),
		State:          w.State(),
		TaskCount:      w.TaskCount(),
		FailedCount:    w.FailedCount(),
		RestartCount:   w.RestartCount(),
		LocalQueueSize: w.LocalQueueSize(),
		LastActiveTime: w.LastActiveTime(),
	}
}

// WorkerStats 工作协程统计信息
type WorkerStats struct {
	ID             int       // 工作协程ID
	IsActive       bool      // 是否活跃
	IsRunning      bool      // 是否运行中
	State          int32     // 当前状态
	TaskCount      int64     // 已处理任务数
	FailedCount    int64     // 失败任务数
	RestartCount   int32     // 重启次数
	LocalQueueSize int       // 本地队列大小
	LastActiveTime time.Time // 最后活跃时间
}

// LocalQueueSize 获取本地队列当前大小
func (w *Worker) LocalQueueSize() int {
	w.localMutex.RLock()
	defer w.localMutex.RUnlock()

	if w.localTail >= w.localHead {
		return w.localTail - w.localHead
	}
	return pool.LocalQueueSize - w.localHead + w.localTail
}

// SubmitTask 提交任务到工作协程
func (w *Worker) SubmitTask(taskWrapper *TaskWrapper) bool {
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
func (w *Worker) run() {
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

			// 检查是否应该重启（避免无限重启）
			restartCount := atomic.LoadInt32(&w.restartCount)
			if restartCount < w.maxRestarts && atomic.LoadInt32(&w.isRunning) == 1 {
				// 指数退避重启策略
				backoffDuration := time.Duration(restartCount) * w.backoffBase
				if backoffDuration > time.Second*10 {
					backoffDuration = time.Second * 10 // 最大退避时间10秒
				}
				time.Sleep(backoffDuration)

				// 重置状态并重新启动工作协程
				atomic.StoreInt32(&w.state, 0) // 重置为空闲状态
				w.wg.Add(1)
				go w.run()
			} else {
				// 达到最大重启次数，停止工作协程
				atomic.StoreInt32(&w.isRunning, 0)
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
func (w *Worker) executeTask(taskWrapper *TaskWrapper) {
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
			err = pool.NewPanicError(r, taskWrapper.task)
			atomic.StoreInt32(&w.state, 2) // 设置为错误状态
			atomic.AddInt64(&w.failedCount, 1)

			if w.errorHandler != nil {
				w.errorHandler.HandlePanic(w.id, taskWrapper.task, r)
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

		// 设置Future结果
		if future, ok := taskWrapper.future.(*Future); ok {
			future.setResult(result, err)
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
		err = pool.NewTimeoutError(w.config.TaskTimeout, taskWrapper.task)
		if w.errorHandler != nil {
			w.errorHandler.HandleTimeout(taskWrapper.task, w.config.TaskTimeout)
		}
	}
}

// enqueueLocal 将任务加入本地队列
func (w *Worker) enqueueLocal(taskWrapper *TaskWrapper) bool {
	w.localMutex.Lock()
	defer w.localMutex.Unlock()

	// 检查本地队列是否已满
	nextTail := (w.localTail + 1) % pool.LocalQueueSize
	if nextTail == w.localHead {
		return false // 队列已满
	}

	w.localQueue[w.localTail] = taskWrapper
	w.localTail = nextTail
	return true
}

// dequeueLocal 从本地队列取出任务
func (w *Worker) dequeueLocal() *TaskWrapper {
	w.localMutex.Lock()
	defer w.localMutex.Unlock()

	if w.localHead == w.localTail {
		return nil // 队列为空
	}

	task := w.localQueue[w.localHead]
	w.localQueue[w.localHead] = nil // 清空引用
	w.localHead = (w.localHead + 1) % pool.LocalQueueSize
	return task
}

// stealTask 从其他工作协程窃取任务
func (w *Worker) stealTask(workers []pool.Worker) *TaskWrapper {
	// 随机选择一个工作协程进行窃取
	if len(workers) <= 1 {
		return nil
	}

	// 简单的窃取策略：从最忙的工作协程窃取
	var targetWorker *Worker
	maxTasks := int64(0)

	for _, worker := range workers {
		if worker.ID() == w.id {
			continue // 跳过自己
		}

		if w, ok := worker.(*Worker); ok && w.TaskCount() > maxTasks {
			maxTasks = w.TaskCount()
			targetWorker = w
		}
	}

	if targetWorker != nil {
		return targetWorker.stealFromLocal()
	}

	return nil
}

// stealFromLocal 从本地队列中被窃取任务
func (w *Worker) stealFromLocal() *TaskWrapper {
	w.localMutex.Lock()
	defer w.localMutex.Unlock()

	// 从队列尾部窃取（LIFO策略）
	if w.localHead == w.localTail {
		return nil
	}

	// 原子性地窃取任务
	w.localTail = (w.localTail - 1 + pool.LocalQueueSize) % pool.LocalQueueSize
	task := w.localQueue[w.localTail]
	w.localQueue[w.localTail] = nil
	return task
}

// getTaskWrapper 从对象缓存获取任务包装器
func (w *Worker) getTaskWrapper() *TaskWrapper {
	if w.objectCache.taskIndex < len(w.objectCache.taskWrappers) {
		wrapper := w.objectCache.taskWrappers[w.objectCache.taskIndex]
		w.objectCache.taskIndex++
		return wrapper
	}

	// 缓存用完，创建新的
	return &TaskWrapper{}
}

// recycleTaskWrapper 回收任务包装器
func (w *Worker) recycleTaskWrapper(wrapper *TaskWrapper) {
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

	// 如果缓存未满，回收到缓存
	if w.objectCache.taskIndex > 0 {
		w.objectCache.taskIndex--
		w.objectCache.taskWrappers[w.objectCache.taskIndex] = wrapper
	}
}

// cleanup 清理资源
func (w *Worker) cleanup() {
	// 处理剩余的本地队列任务
	for {
		task := w.dequeueLocal()
		if task == nil {
			break
		}

		// 取消剩余任务
		if future, ok := task.future.(*Future); ok {
			future.setResult(nil, pool.ErrPoolClosed)
		}

		w.recycleTaskWrapper(task)
	}

	// 处理通道中的剩余任务
	for {
		select {
		case task := <-w.taskChan:
			if future, ok := task.future.(*Future); ok {
				future.setResult(nil, pool.ErrPoolClosed)
			}
			w.recycleTaskWrapper(task)
		default:
			return
		}
	}
}

// Future Future接口的实现
type Future struct {
	result   any
	err      error
	done     int32
	mu       sync.RWMutex
	waitCh   chan struct{}
	canceled int32
}

// NewFuture 创建新的Future
func NewFuture() *Future {
	return &Future{
		waitCh: make(chan struct{}),
	}
}

// Get 获取结果（阻塞）
func (f *Future) Get() (any, error) {
	<-f.waitCh
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.result, f.err
}

// GetWithTimeout 带超时获取结果
func (f *Future) GetWithTimeout(timeout time.Duration) (any, error) {
	select {
	case <-f.waitCh:
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.result, f.err
	case <-time.After(timeout):
		return nil, pool.ErrTaskTimeout
	}
}

// IsDone 检查是否完成
func (f *Future) IsDone() bool {
	return atomic.LoadInt32(&f.done) == 1
}

// Cancel 取消任务
func (f *Future) Cancel() bool {
	if atomic.CompareAndSwapInt32(&f.canceled, 0, 1) {
		f.setResult(nil, pool.ErrTaskCancelled)
		return true
	}
	return false
}

// setResult 设置结果（内部方法）
func (f *Future) setResult(result any, err error) {
	if atomic.CompareAndSwapInt32(&f.done, 0, 1) {
		f.mu.Lock()
		f.result = result
		f.err = err
		f.mu.Unlock()
		close(f.waitCh)
	}
}

// SetResult 设置结果（公开方法，供外部调用）
func (f *Future) SetResult(result any, err error) {
	f.setResult(result, err)
}
