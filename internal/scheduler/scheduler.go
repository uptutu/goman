package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/uptutu/goman/internal/queue"
	"github.com/uptutu/goman/internal/worker"
	"github.com/uptutu/goman/pkg/pool"
)

// Scheduler 任务调度器实现
type Scheduler struct {
	workers      []pool.Worker        // 工作协程列表
	taskQueue    *queue.LockFreeQueue // 全局任务队列
	loadBalancer pool.LoadBalancer    // 负载均衡器
	config       *pool.Config         // 配置
	monitor      pool.Monitor         // 监控器
	errorHandler pool.ErrorHandler    // 错误处理器

	// 状态管理
	isRunning int32              // 是否运行中
	ctx       context.Context    // 上下文
	cancel    context.CancelFunc // 取消函数
	wg        sync.WaitGroup     // 等待组

	// 调度统计
	totalSubmitted int64 // 总提交任务数
	totalScheduled int64 // 总调度任务数
	totalFailed    int64 // 总失败任务数

	// 工作窃取相关
	stealEnabled   bool          // 是否启用工作窃取
	stealThreshold int           // 窃取阈值
	stealInterval  time.Duration // 窃取检查间隔

	// 优先级调度
	priorityQueues map[int]*queue.LockFreeQueue // 优先级队列映射
	priorityMutex  sync.RWMutex                 // 优先级队列锁
}

// NewScheduler 创建新的调度器
func NewScheduler(workers []pool.Worker, taskQueue *queue.LockFreeQueue, config *pool.Config, monitor pool.Monitor, errorHandler pool.ErrorHandler) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建优先级队列
	priorityQueues := make(map[int]*queue.LockFreeQueue)
	for priority := pool.PriorityLow; priority <= pool.PriorityCritical; priority++ {
		priorityQueues[priority] = queue.NewLockFreeQueue(uint64(config.QueueSize / 4)) // 每个优先级分配1/4的队列空间
	}

	scheduler := &Scheduler{
		workers:        workers,
		taskQueue:      taskQueue,
		config:         config,
		monitor:        monitor,
		errorHandler:   errorHandler,
		ctx:            ctx,
		cancel:         cancel,
		stealEnabled:   true,
		stealThreshold: 2,                      // 当本地队列大小超过2时允许被窃取
		stealInterval:  time.Millisecond * 100, // 每100ms检查一次窃取机会
		priorityQueues: priorityQueues,
	}

	// 默认使用轮询负载均衡
	scheduler.loadBalancer = NewRoundRobinBalancer()

	return scheduler
}

// SetLoadBalancer 设置负载均衡器
func (s *Scheduler) SetLoadBalancer(lb pool.LoadBalancer) {
	s.loadBalancer = lb
}

// EnableWorkStealing 启用工作窃取
func (s *Scheduler) EnableWorkStealing(threshold int, interval time.Duration) {
	s.stealEnabled = true
	s.stealThreshold = threshold
	s.stealInterval = interval
}

// DisableWorkStealing 禁用工作窃取
func (s *Scheduler) DisableWorkStealing() {
	s.stealEnabled = false
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 0, 1) {
		return pool.NewPoolError("scheduler.Start", errors.New("scheduler is already running"))
	}

	// 启动调度协程
	s.wg.Add(1)
	go s.scheduleLoop()

	// 如果启用工作窃取，启动工作窃取协程
	if s.stealEnabled {
		s.wg.Add(1)
		go s.workStealingLoop()
	}

	// 启动优先级调度协程
	s.wg.Add(1)
	go s.priorityScheduleLoop()

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.isRunning, 1, 0) {
		return nil // 已经停止
	}

	s.cancel()
	s.wg.Wait()

	return nil
}

// Schedule 调度任务
func (s *Scheduler) Schedule(task pool.Task) error {
	if atomic.LoadInt32(&s.isRunning) == 0 {
		return pool.NewPoolError("scheduler.Schedule", pool.ErrPoolClosed)
	}

	atomic.AddInt64(&s.totalSubmitted, 1)

	// 记录任务提交
	if s.monitor != nil {
		s.monitor.RecordTaskSubmit(task)
	}

	// 根据任务优先级选择队列
	priority := task.Priority()
	if priority < pool.PriorityLow || priority > pool.PriorityCritical {
		priority = pool.PriorityNormal // 默认优先级
	}

	// 创建任务包装器
	taskWrapper := worker.NewTaskWrapper(task, worker.NewFuture(), s.ctx)

	// 尝试直接分配给工作协程
	if s.tryDirectSchedule(taskWrapper) {
		atomic.AddInt64(&s.totalScheduled, 1)
		return nil
	}

	// 直接分配失败，放入优先级队列
	s.priorityMutex.RLock()
	priorityQueue := s.priorityQueues[priority]
	s.priorityMutex.RUnlock()

	if priorityQueue != nil && priorityQueue.Enqueue(unsafe.Pointer(taskWrapper)) {
		return nil
	}

	// 优先级队列也满了，尝试放入全局队列
	if s.taskQueue.Enqueue(unsafe.Pointer(taskWrapper)) {
		return nil
	}

	// 所有队列都满了
	atomic.AddInt64(&s.totalFailed, 1)
	if s.errorHandler != nil {
		return s.errorHandler.HandleQueueFull(task)
	}

	return pool.NewPoolError("scheduler.Schedule", pool.ErrQueueFull)
}

// tryDirectSchedule 尝试直接调度任务到工作协程
func (s *Scheduler) tryDirectSchedule(taskWrapper *worker.TaskWrapper) bool {
	// 使用负载均衡器选择工作协程
	selectedWorker := s.loadBalancer.SelectWorker(s.workers)
	if selectedWorker == nil {
		return false
	}

	// 尝试提交任务到选中的工作协程
	if w, ok := selectedWorker.(*worker.Worker); ok {
		return w.SubmitTask(taskWrapper)
	}

	return false
}

// scheduleLoop 主调度循环
func (s *Scheduler) scheduleLoop() {
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
func (s *Scheduler) processGlobalQueue() {
	// 批量处理任务以提高效率
	batchSize := 10
	for i := 0; i < batchSize; i++ {
		taskWrapper := s.taskQueue.Dequeue()
		if taskWrapper == nil {
			break
		}

		tw := (*worker.TaskWrapper)(taskWrapper)
		if s.tryDirectSchedule(tw) {
			atomic.AddInt64(&s.totalScheduled, 1)
		} else {
			// 重新放回队列
			s.taskQueue.Enqueue(unsafe.Pointer(tw))
			break // 避免无限循环
		}
	}
}

// priorityScheduleLoop 优先级调度循环
func (s *Scheduler) priorityScheduleLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Millisecond * 5) // 更频繁地处理优先级任务
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processPriorityQueues()
		}
	}
}

// processPriorityQueues 处理优先级队列
func (s *Scheduler) processPriorityQueues() {
	// 按优先级从高到低处理
	priorities := []int{pool.PriorityCritical, pool.PriorityHigh, pool.PriorityNormal, pool.PriorityLow}

	for _, priority := range priorities {
		s.priorityMutex.RLock()
		priorityQueue := s.priorityQueues[priority]
		s.priorityMutex.RUnlock()

		if priorityQueue == nil {
			continue
		}

		// 根据优先级决定处理的任务数量
		batchSize := s.getBatchSizeForPriority(priority)

		for i := 0; i < batchSize; i++ {
			taskWrapper := priorityQueue.Dequeue()
			if taskWrapper == nil {
				break
			}

			tw := (*worker.TaskWrapper)(taskWrapper)
			if s.tryDirectSchedule(tw) {
				atomic.AddInt64(&s.totalScheduled, 1)
			} else {
				// 重新放回队列
				priorityQueue.Enqueue(unsafe.Pointer(tw))
				break
			}
		}
	}
}

// getBatchSizeForPriority 根据优先级获取批处理大小
func (s *Scheduler) getBatchSizeForPriority(priority int) int {
	switch priority {
	case pool.PriorityCritical:
		return 20 // 关键任务优先处理更多
	case pool.PriorityHigh:
		return 15
	case pool.PriorityNormal:
		return 10
	case pool.PriorityLow:
		return 5
	default:
		return 10
	}
}

// workStealingLoop 工作窃取循环
func (s *Scheduler) workStealingLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.stealInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performWorkStealing()
		}
	}
}

// performWorkStealing 执行工作窃取
func (s *Scheduler) performWorkStealing() {
	if len(s.workers) <= 1 {
		return
	}

	// 找到最忙和最闲的工作协程
	var busiestWorker, idlestWorker pool.Worker
	maxLoad, minLoad := int64(0), int64(^uint64(0)>>1) // 最大和最小负载

	for _, worker := range s.workers {
		if !worker.IsRunning() {
			continue
		}

		load := worker.TaskCount()
		localQueueSize := worker.LocalQueueSize()

		// 考虑本地队列大小和总任务数
		totalLoad := load + int64(localQueueSize)

		if totalLoad > maxLoad && localQueueSize > s.stealThreshold {
			maxLoad = totalLoad
			busiestWorker = worker
		}

		if totalLoad < minLoad {
			minLoad = totalLoad
			idlestWorker = worker
		}
	}

	// 如果负载差异足够大，执行窃取
	if busiestWorker != nil && idlestWorker != nil &&
		busiestWorker.ID() != idlestWorker.ID() &&
		maxLoad-minLoad > int64(s.stealThreshold) {

		s.stealTask(busiestWorker, idlestWorker)
	}
}

// stealTask 从忙碌的工作协程窃取任务给空闲的工作协程
func (s *Scheduler) stealTask(from, to pool.Worker) {
	if fromWorker, ok := from.(*worker.Worker); ok {
		if toWorker, ok := to.(*worker.Worker); ok {
			// 从忙碌的工作协程窃取任务
			stolenTask := fromWorker.StealWork()
			if stolenTask != nil {
				// 提交给空闲的工作协程
				if !toWorker.SubmitTask(stolenTask) {
					// 如果提交失败，放回全局队列
					s.taskQueue.Enqueue(unsafe.Pointer(stolenTask))
				}
			}
		}
	}
}

// GetStats 获取调度器统计信息
func (s *Scheduler) GetStats() SchedulerStats {
	return SchedulerStats{
		IsRunning:      atomic.LoadInt32(&s.isRunning) == 1,
		TotalSubmitted: atomic.LoadInt64(&s.totalSubmitted),
		TotalScheduled: atomic.LoadInt64(&s.totalScheduled),
		TotalFailed:    atomic.LoadInt64(&s.totalFailed),
		WorkerCount:    len(s.workers),
		QueueSize:      int(s.taskQueue.Size()),
		StealEnabled:   s.stealEnabled,
		LoadBalancer:   s.getLoadBalancerType(),
	}
}

// getLoadBalancerType 获取负载均衡器类型
func (s *Scheduler) getLoadBalancerType() string {
	switch s.loadBalancer.(type) {
	case *RoundRobinBalancer:
		return "RoundRobin"
	case *LeastLoadBalancer:
		return "LeastLoad"
	case *RandomBalancer:
		return "Random"
	case *WeightedRoundRobinBalancer:
		return "WeightedRoundRobin"
	default:
		return "Unknown"
	}
}

// SchedulerStats 调度器统计信息
type SchedulerStats struct {
	IsRunning      bool   // 是否运行中
	TotalSubmitted int64  // 总提交任务数
	TotalScheduled int64  // 总调度任务数
	TotalFailed    int64  // 总失败任务数
	WorkerCount    int    // 工作协程数量
	QueueSize      int    // 队列大小
	StealEnabled   bool   // 是否启用工作窃取
	LoadBalancer   string // 负载均衡器类型
}
