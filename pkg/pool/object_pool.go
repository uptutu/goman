package pool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ObjectPoolStats 对象池统计信息
type ObjectPoolStats struct {
	// 基础统计
	TaskWrapperPoolSize int64 // taskWrapper池大小
	FuturePoolSize      int64 // Future池大小
	ContextPoolSize     int64 // Context池大小

	// 使用统计
	TaskWrapperGets int64 // taskWrapper获取次数
	TaskWrapperPuts int64 // taskWrapper归还次数
	FutureGets      int64 // Future获取次数
	FuturePuts      int64 // Future归还次数
	ContextGets     int64 // Context获取次数
	ContextPuts     int64 // Context归还次数

	// 性能统计
	TaskWrapperHitRate float64 // taskWrapper命中率
	FutureHitRate      float64 // Future命中率
	ContextHitRate     float64 // Context命中率

	// 内存统计
	TotalAllocated int64 // 总分配对象数
	TotalReused    int64 // 总复用对象数
}

// ObjectPool 对象池管理器
type ObjectPool struct {
	// 对象池
	taskWrapperPool *sync.Pool
	futurePool      *sync.Pool
	contextPool     *sync.Pool

	// 配置
	config *Config

	// 统计信息
	stats *ObjectPoolStats

	// 预分配对象
	preAllocated bool

	// 监控
	mu sync.RWMutex
}

// taskWrapper 任务包装器
type taskWrapper struct {
	task       Task
	future     *future
	submitTime time.Time
	priority   int
	timeout    time.Duration // 任务超时时间
	ctx        context.Context
	cancel     context.CancelFunc // 取消函数

	// 对象池复用字段
	next *taskWrapper // 用于链表
}

// future Future接口的实现
type future struct {
	result    any
	err       error
	done      int32 // 使用atomic操作
	cancelled int32 // 使用atomic操作

	// 同步原语
	ch chan struct{}

	// 对象池复用字段
	mu sync.RWMutex
}

// NewObjectPool 创建对象池
func NewObjectPool(config *Config) *ObjectPool {
	pool := &ObjectPool{
		config: config,
		stats:  &ObjectPoolStats{},
	}

	// 初始化taskWrapper池
	pool.taskWrapperPool = &sync.Pool{
		New: func() any {
			atomic.AddInt64(&pool.stats.TotalAllocated, 1)
			return &taskWrapper{}
		},
	}

	// 初始化Future池
	pool.futurePool = &sync.Pool{
		New: func() any {
			atomic.AddInt64(&pool.stats.TotalAllocated, 1)
			return &future{
				ch: make(chan struct{}, 1),
			}
		},
	}

	// 初始化Context池
	pool.contextPool = &sync.Pool{
		New: func() any {
			atomic.AddInt64(&pool.stats.TotalAllocated, 1)
			return context.Background()
		},
	}

	// 预分配对象
	if config.PreAlloc && config.ObjectPoolSize > 0 {
		pool.preAllocateObjects()
		pool.preAllocated = true
	}

	return pool
}

// preAllocateObjects 预分配对象
func (p *ObjectPool) preAllocateObjects() {
	// 预分配taskWrapper对象
	for i := 0; i < p.config.ObjectPoolSize; i++ {
		wrapper := &taskWrapper{}
		p.taskWrapperPool.Put(wrapper)
	}

	// 预分配Future对象
	for i := 0; i < p.config.ObjectPoolSize; i++ {
		fut := &future{
			ch: make(chan struct{}, 1),
		}
		p.futurePool.Put(fut)
	}

	// 预分配Context对象
	for i := 0; i < p.config.ObjectPoolSize/2; i++ {
		ctx := context.Background()
		p.contextPool.Put(ctx)
	}
}

// GetTaskWrapper 获取taskWrapper对象
func (p *ObjectPool) GetTaskWrapper() *taskWrapper {
	atomic.AddInt64(&p.stats.TaskWrapperGets, 1)

	wrapper := p.taskWrapperPool.Get().(*taskWrapper)

	// 重置对象状态
	wrapper.task = nil
	wrapper.future = nil
	wrapper.submitTime = time.Time{}
	wrapper.priority = PriorityNormal
	wrapper.timeout = 0
	wrapper.ctx = nil
	wrapper.cancel = nil
	wrapper.next = nil

	atomic.AddInt64(&p.stats.TotalReused, 1)
	return wrapper
}

// PutTaskWrapper 归还taskWrapper对象
func (p *ObjectPool) PutTaskWrapper(wrapper *taskWrapper) {
	if wrapper == nil {
		return
	}

	atomic.AddInt64(&p.stats.TaskWrapperPuts, 1)

	// 清理对象状态并取消上下文
	if wrapper.cancel != nil {
		wrapper.cancel()
	}
	wrapper.task = nil
	wrapper.future = nil
	wrapper.submitTime = time.Time{}
	wrapper.priority = PriorityNormal
	wrapper.timeout = 0
	wrapper.ctx = nil
	wrapper.cancel = nil
	wrapper.next = nil

	p.taskWrapperPool.Put(wrapper)
}

// GetFuture 获取Future对象
func (p *ObjectPool) GetFuture() *future {
	atomic.AddInt64(&p.stats.FutureGets, 1)

	fut := p.futurePool.Get().(*future)

	// 重置对象状态
	fut.result = nil
	fut.err = nil
	atomic.StoreInt32(&fut.done, 0)
	atomic.StoreInt32(&fut.cancelled, 0)

	// 清空通道
	select {
	case <-fut.ch:
	default:
	}

	atomic.AddInt64(&p.stats.TotalReused, 1)
	return fut
}

// PutFuture 归还Future对象
func (p *ObjectPool) PutFuture(fut *future) {
	if fut == nil {
		return
	}

	atomic.AddInt64(&p.stats.FuturePuts, 1)

	// 清理对象状态
	fut.mu.Lock()
	fut.result = nil
	fut.err = nil
	atomic.StoreInt32(&fut.done, 0)
	atomic.StoreInt32(&fut.cancelled, 0)

	// 清空通道
	select {
	case <-fut.ch:
	default:
	}
	fut.mu.Unlock()

	p.futurePool.Put(fut)
}

// GetContext 获取Context对象
func (p *ObjectPool) GetContext() context.Context {
	atomic.AddInt64(&p.stats.ContextGets, 1)

	ctx := p.contextPool.Get().(context.Context)
	atomic.AddInt64(&p.stats.TotalReused, 1)
	return ctx
}

// PutContext 归还Context对象
func (p *ObjectPool) PutContext(ctx context.Context) {
	if ctx == nil {
		return
	}

	atomic.AddInt64(&p.stats.ContextPuts, 1)

	// 只归还background context
	if ctx == context.Background() {
		p.contextPool.Put(ctx)
	}
}

// GetStats 获取对象池统计信息
func (p *ObjectPool) GetStats() ObjectPoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := *p.stats

	// 计算命中率
	if stats.TaskWrapperGets > 0 {
		stats.TaskWrapperHitRate = float64(stats.TaskWrapperPuts) / float64(stats.TaskWrapperGets)
	}
	if stats.FutureGets > 0 {
		stats.FutureHitRate = float64(stats.FuturePuts) / float64(stats.FutureGets)
	}
	if stats.ContextGets > 0 {
		stats.ContextHitRate = float64(stats.ContextPuts) / float64(stats.ContextGets)
	}

	return stats
}

// Reset 重置对象池统计信息
func (p *ObjectPool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats = &ObjectPoolStats{}
}

// Warm 预热对象池
func (p *ObjectPool) Warm(count int) {
	if count <= 0 {
		return
	}

	// 预热taskWrapper池
	wrappers := make([]*taskWrapper, count)
	for i := 0; i < count; i++ {
		wrappers[i] = p.GetTaskWrapper()
	}
	for _, wrapper := range wrappers {
		p.PutTaskWrapper(wrapper)
	}

	// 预热Future池
	futures := make([]*future, count)
	for i := 0; i < count; i++ {
		futures[i] = p.GetFuture()
	}
	for _, fut := range futures {
		p.PutFuture(fut)
	}
}

// Future接口实现

// Get 获取结果（阻塞）
func (f *future) Get() (any, error) {
	if atomic.LoadInt32(&f.cancelled) == 1 {
		return nil, ErrTaskCancelled
	}

	if atomic.LoadInt32(&f.done) == 1 {
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.result, f.err
	}

	<-f.ch

	if atomic.LoadInt32(&f.cancelled) == 1 {
		return nil, ErrTaskCancelled
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.result, f.err
}

// GetWithTimeout 带超时获取结果
func (f *future) GetWithTimeout(timeout time.Duration) (any, error) {
	if atomic.LoadInt32(&f.cancelled) == 1 {
		return nil, ErrTaskCancelled
	}

	if atomic.LoadInt32(&f.done) == 1 {
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.result, f.err
	}

	select {
	case <-f.ch:
		if atomic.LoadInt32(&f.cancelled) == 1 {
			return nil, ErrTaskCancelled
		}
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.result, f.err
	case <-time.After(timeout):
		return nil, ErrTaskTimeout
	}
}

// IsDone 检查是否完成
func (f *future) IsDone() bool {
	return atomic.LoadInt32(&f.done) == 1
}

// Cancel 取消任务
func (f *future) Cancel() bool {
	if atomic.LoadInt32(&f.done) == 1 {
		return false
	}

	if atomic.CompareAndSwapInt32(&f.cancelled, 0, 1) {
		// 通知等待的协程
		select {
		case f.ch <- struct{}{}:
		default:
		}
		return true
	}

	return false
}

// setResult 设置结果（内部使用）
func (f *future) setResult(result any, err error) {
	if f == nil {
		return
	}

	if atomic.LoadInt32(&f.cancelled) == 1 {
		return
	}

	f.mu.Lock()
	f.result = result
	f.err = err
	f.mu.Unlock()

	if atomic.CompareAndSwapInt32(&f.done, 0, 1) {
		// 通知等待的协程
		select {
		case f.ch <- struct{}{}:
		default:
		}
	}
}

// taskWrapper methods

// newTaskWrapper 创建新的任务包装器
func newTaskWrapper(task Task, timeout time.Duration, objectPool *ObjectPool) *taskWrapper {
	wrapper := objectPool.GetTaskWrapper()
	future := objectPool.GetFuture()

	// 设置上下文和取消函数
	ctx, cancel := context.WithCancel(context.Background())
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}

	wrapper.task = task
	wrapper.future = future
	wrapper.submitTime = time.Now()
	wrapper.priority = task.Priority()
	wrapper.timeout = timeout
	wrapper.ctx = ctx
	wrapper.cancel = cancel

	return wrapper
}

// Execute 执行任务
func (tw *taskWrapper) Execute() {
	defer func() {
		if r := recover(); r != nil {
			tw.future.setResult(nil, &PanicError{
				Value: r,
				Task:  tw.task,
			})
		}
	}()

	// 检查是否已被取消
	select {
	case <-tw.ctx.Done():
		tw.future.setResult(nil, tw.ctx.Err())
		return
	default:
	}

	// 执行任务
	result, err := tw.task.Execute(tw.ctx)
	tw.future.setResult(result, err)
}

// Cancel 取消任务
func (tw *taskWrapper) Cancel() bool {
	return tw.future.Cancel()
}

// IsCancelled 检查任务是否已被取消
func (tw *taskWrapper) IsCancelled() bool {
	return atomic.LoadInt32(&tw.future.cancelled) == 1
}

// IsTimeout 检查任务是否超时
func (tw *taskWrapper) IsTimeout() bool {
	select {
	case <-tw.ctx.Done():
		return tw.ctx.Err() == context.DeadlineExceeded
	default:
		return false
	}
}

// Reset 重置任务包装器，用于对象池复用
func (tw *taskWrapper) Reset() {
	if tw.cancel != nil {
		tw.cancel()
	}
	tw.task = nil
	tw.future = nil
	tw.submitTime = time.Time{}
	tw.priority = 0
	tw.timeout = 0
	tw.ctx = nil
	tw.cancel = nil
	tw.next = nil
}

// Helper task implementations

// SimpleTask 简单任务实现，用于快速创建任务
type SimpleTask struct {
	fn       func(ctx context.Context) (any, error)
	priority int
}

// NewSimpleTask 创建简单任务
func NewSimpleTask(fn func(ctx context.Context) (any, error), priority int) *SimpleTask {
	return &SimpleTask{
		fn:       fn,
		priority: priority,
	}
}

// Execute 执行任务
func (st *SimpleTask) Execute(ctx context.Context) (any, error) {
	return st.fn(ctx)
}

// Priority 获取任务优先级
func (st *SimpleTask) Priority() int {
	return st.priority
}

// FuncTask 函数任务，包装普通函数为任务
type FuncTask struct {
	fn       func() (any, error)
	priority int
}

// NewFuncTask 创建函数任务
func NewFuncTask(fn func() (any, error), priority int) *FuncTask {
	return &FuncTask{
		fn:       fn,
		priority: priority,
	}
}

// Execute 执行任务
func (ft *FuncTask) Execute(ctx context.Context) (any, error) {
	// 检查上下文是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return ft.fn()
}

// Priority 获取任务优先级
func (ft *FuncTask) Priority() int {
	return ft.priority
}

// Future interface extensions

// IsCancelled 检查Future是否已被取消
func (f *future) IsCancelled() bool {
	return atomic.LoadInt32(&f.cancelled) == 1
}
