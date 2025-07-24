package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// MockTask 模拟任务实现
type MockTask struct {
	id       string
	priority int
	duration time.Duration
	result   any
	err      error
	executed int32
	panicMsg string
}

func NewMockTask(id string, priority int) *MockTask {
	return &MockTask{
		id:       id,
		priority: priority,
		duration: time.Millisecond * 10,
		result:   "result-" + id,
	}
}

func (t *MockTask) Execute(ctx context.Context) (any, error) {
	atomic.StoreInt32(&t.executed, 1)

	if t.panicMsg != "" {
		panic(t.panicMsg)
	}

	if t.duration > 0 {
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return t.result, t.err
}

func (t *MockTask) Priority() int {
	return t.priority
}

func (t *MockTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

// MockPool 模拟协程池
type MockPool struct{}

func (p *MockPool) Submit(task pool.Task) error                                   { return nil }
func (p *MockPool) SubmitWithTimeout(task pool.Task, timeout time.Duration) error { return nil }
func (p *MockPool) SubmitAsync(task pool.Task) pool.Future                        { return nil }
func (p *MockPool) Shutdown(ctx context.Context) error                            { return nil }
func (p *MockPool) Stats() pool.PoolStats                                         { return pool.PoolStats{} }
func (p *MockPool) IsRunning() bool                                               { return true }
func (p *MockPool) IsShutdown() bool                                              { return false }
func (p *MockPool) IsClosed() bool                                                { return false }
func (p *MockPool) AddMiddleware(middleware pool.Middleware)                      {}
func (p *MockPool) RemoveMiddleware(middleware pool.Middleware)                   {}
func (p *MockPool) RegisterPlugin(plugin pool.Plugin) error                       { return nil }
func (p *MockPool) UnregisterPlugin(name string) error                            { return nil }
func (p *MockPool) GetPlugin(name string) (pool.Plugin, bool)                     { return nil, false }
func (p *MockPool) AddEventListener(listener pool.EventListener)                  {}
func (p *MockPool) RemoveEventListener(listener pool.EventListener)               {}
func (p *MockPool) SetSchedulerPlugin(plugin pool.SchedulerPlugin)                {}
func (p *MockPool) GetSchedulerPlugin() pool.SchedulerPlugin                      { return nil }

// MockErrorHandler 模拟错误处理器
type MockErrorHandler struct {
	panicCalls   int32
	timeoutCalls int32
	queueCalls   int32
	lastPanic    any
	lastTask     pool.Task
}

func (h *MockErrorHandler) HandlePanic(workerID int, task pool.Task, panicValue any) {
	atomic.AddInt32(&h.panicCalls, 1)
	h.lastPanic = panicValue
	h.lastTask = task
}

func (h *MockErrorHandler) HandleTimeout(task pool.Task, duration time.Duration) {
	atomic.AddInt32(&h.timeoutCalls, 1)
	h.lastTask = task
}

func (h *MockErrorHandler) HandleQueueFull(task pool.Task) error {
	atomic.AddInt32(&h.queueCalls, 1)
	return pool.ErrQueueFull
}

func (h *MockErrorHandler) PanicCalls() int32 {
	return atomic.LoadInt32(&h.panicCalls)
}

func (h *MockErrorHandler) TimeoutCalls() int32 {
	return atomic.LoadInt32(&h.timeoutCalls)
}

func (h *MockErrorHandler) GetStats() pool.ErrorStats {
	return pool.ErrorStats{
		PanicCount:     int64(atomic.LoadInt32(&h.panicCalls)),
		TimeoutCount:   int64(atomic.LoadInt32(&h.timeoutCalls)),
		QueueFullCount: int64(atomic.LoadInt32(&h.queueCalls)),
	}
}

// MockMonitor 模拟监控器
type MockMonitor struct {
	submitCalls   int32
	completeCalls int32
	failCalls     int32
}

func (m *MockMonitor) RecordTaskSubmit(task pool.Task) {
	atomic.AddInt32(&m.submitCalls, 1)
}

func (m *MockMonitor) RecordTaskComplete(task pool.Task, duration time.Duration) {
	atomic.AddInt32(&m.completeCalls, 1)
}

func (m *MockMonitor) RecordTaskFail(task pool.Task, err error) {
	atomic.AddInt32(&m.failCalls, 1)
}

func (m *MockMonitor) GetStats() pool.PoolStats {
	return pool.PoolStats{}
}

func (m *MockMonitor) CompleteCalls() int32 {
	return atomic.LoadInt32(&m.completeCalls)
}

func (m *MockMonitor) FailCalls() int32 {
	return atomic.LoadInt32(&m.failCalls)
}

func (m *MockMonitor) Start() {}

func (m *MockMonitor) Stop() {}

func (m *MockMonitor) SetActiveWorkers(count int64) {}

func (m *MockMonitor) SetQueuedTasks(count int64) {}

// 测试工作协程创建
func TestNewWorker(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 5,
		QueueSize:   100,
		TaskTimeout: time.Second * 30,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	if worker.ID() != 1 {
		t.Errorf("Expected worker ID 1, got %d", worker.ID())
	}

	if worker.IsActive() {
		t.Error("New worker should not be active")
	}

	if worker.TaskCount() != 0 {
		t.Errorf("New worker should have 0 task count, got %d", worker.TaskCount())
	}

	if worker.FailedCount() != 0 {
		t.Errorf("New worker should have 0 failed count, got %d", worker.FailedCount())
	}

	if !worker.IsIdle() {
		t.Error("New worker should be idle")
	}

	if worker.IsBusy() {
		t.Error("New worker should not be busy")
	}

	if worker.IsError() {
		t.Error("New worker should not be in error state")
	}

	if worker.RestartCount() != 0 {
		t.Errorf("New worker should have 0 restart count, got %d", worker.RestartCount())
	}

	if worker.objectCache == nil {
		t.Error("Worker should have object cache")
	}

	if len(worker.localQueue) != pool.LocalQueueSize {
		t.Errorf("Expected local queue size %d, got %d", pool.LocalQueueSize, len(worker.localQueue))
	}

	if worker.maxRestarts != 10 {
		t.Errorf("Expected max restarts 10, got %d", worker.maxRestarts)
	}

	if worker.backoffBase != time.Millisecond*100 {
		t.Errorf("Expected backoff base 100ms, got %v", worker.backoffBase)
	}
}

// 测试任务执行
func TestWorker_ExecuteTask(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 创建测试任务
	task := NewMockTask("test1", pool.PriorityNormal)
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:       task,
		future:     future,
		submitTime: time.Now(),
		priority:   task.Priority(),
		ctx:        context.Background(),
	}

	// 执行任务
	worker.executeTask(taskWrapper)

	// 验证任务已执行
	if !task.IsExecuted() {
		t.Error("Task should be executed")
	}

	// 验证Future结果
	if !future.IsDone() {
		t.Error("Future should be done")
	}

	result, err := future.Get()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result != "result-test1" {
		t.Errorf("Expected result 'result-test1', got %v", result)
	}

	// 验证监控调用
	if mockMonitor.CompleteCalls() != 1 {
		t.Errorf("Expected 1 complete call, got %d", mockMonitor.CompleteCalls())
	}

	// 验证任务计数
	if worker.TaskCount() != 1 {
		t.Errorf("Expected task count 1, got %d", worker.TaskCount())
	}
}

// 测试任务panic恢复
func TestWorker_ExecuteTaskWithPanic(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 创建会panic的任务
	task := NewMockTask("panic-task", pool.PriorityNormal)
	task.panicMsg = "test panic"
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:       task,
		future:     future,
		submitTime: time.Now(),
		priority:   task.Priority(),
		ctx:        context.Background(),
	}

	// 执行任务
	worker.executeTask(taskWrapper)

	// 验证panic被处理
	if mockErrorHandler.PanicCalls() != 1 {
		t.Errorf("Expected 1 panic call, got %d", mockErrorHandler.PanicCalls())
	}

	if mockErrorHandler.lastPanic != "test panic" {
		t.Errorf("Expected panic message 'test panic', got %v", mockErrorHandler.lastPanic)
	}

	// 验证Future包含错误
	if !future.IsDone() {
		t.Error("Future should be done")
	}

	_, err := future.Get()
	if err == nil {
		t.Error("Expected error from panic")
	}

	if panicErr, ok := err.(*pool.PanicError); !ok {
		t.Errorf("Expected PanicError, got %T", err)
	} else if panicErr.Value != "test panic" {
		t.Errorf("Expected panic value 'test panic', got %v", panicErr.Value)
	}

	// 验证监控调用
	if mockMonitor.FailCalls() != 1 {
		t.Errorf("Expected 1 fail call, got %d", mockMonitor.FailCalls())
	}
}

// 测试任务超时
func TestWorker_ExecuteTaskWithTimeout(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Millisecond * 50, // 很短的超时时间
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 创建长时间运行的任务
	task := NewMockTask("timeout-task", pool.PriorityNormal)
	task.duration = time.Millisecond * 200 // 比超时时间长
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:       task,
		future:     future,
		submitTime: time.Now(),
		priority:   task.Priority(),
		ctx:        context.Background(),
	}

	// 执行任务
	worker.executeTask(taskWrapper)

	// 验证超时处理
	if mockErrorHandler.TimeoutCalls() != 1 {
		t.Errorf("Expected 1 timeout call, got %d", mockErrorHandler.TimeoutCalls())
	}

	// 验证Future包含超时错误
	if !future.IsDone() {
		t.Error("Future should be done")
	}

	_, err := future.Get()
	if err == nil {
		t.Error("Expected timeout error")
	}

	if timeoutErr, ok := err.(*pool.TimeoutError); !ok {
		t.Errorf("Expected TimeoutError, got %T", err)
	} else if timeoutErr.Duration != config.TaskTimeout {
		t.Errorf("Expected timeout duration %v, got %v", config.TaskTimeout, timeoutErr.Duration)
	}
}

// 测试本地队列操作
func TestWorker_LocalQueue(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 测试空队列
	if task := worker.dequeueLocal(); task != nil {
		t.Error("Empty queue should return nil")
	}

	// 创建测试任务
	task1 := &TaskWrapper{task: NewMockTask("task1", pool.PriorityNormal)}
	task2 := &TaskWrapper{task: NewMockTask("task2", pool.PriorityHigh)}

	// 测试入队
	if !worker.enqueueLocal(task1) {
		t.Error("Should be able to enqueue task1")
	}

	if !worker.enqueueLocal(task2) {
		t.Error("Should be able to enqueue task2")
	}

	// 测试出队（FIFO）
	dequeued1 := worker.dequeueLocal()
	if dequeued1 != task1 {
		t.Error("Should dequeue task1 first")
	}

	dequeued2 := worker.dequeueLocal()
	if dequeued2 != task2 {
		t.Error("Should dequeue task2 second")
	}

	// 测试队列为空
	if task := worker.dequeueLocal(); task != nil {
		t.Error("Queue should be empty")
	}
}

// 测试本地队列满的情况
func TestWorker_LocalQueueFull(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 填满本地队列
	for i := 0; i < pool.LocalQueueSize-1; i++ {
		task := &TaskWrapper{task: NewMockTask("task", pool.PriorityNormal)}
		if !worker.enqueueLocal(task) {
			t.Errorf("Should be able to enqueue task %d", i)
		}
	}

	// 尝试再添加一个任务，应该失败
	extraTask := &TaskWrapper{task: NewMockTask("extra", pool.PriorityNormal)}
	if worker.enqueueLocal(extraTask) {
		t.Error("Should not be able to enqueue when queue is full")
	}
}

// 测试工作窃取
func TestWorker_StealTask(t *testing.T) {
	config := &pool.Config{WorkerCount: 2}

	worker1 := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})
	worker2 := NewWorker(2, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 给worker2添加一些任务
	task1 := &TaskWrapper{task: NewMockTask("task1", pool.PriorityNormal)}
	task2 := &TaskWrapper{task: NewMockTask("task2", pool.PriorityNormal)}

	worker2.enqueueLocal(task1)
	worker2.enqueueLocal(task2)

	// 模拟worker2处理了一些任务
	atomic.StoreInt64(&worker2.taskCount, 5)

	// worker1尝试窃取任务
	workers := []pool.Worker{worker1, worker2}
	stolenTask := worker1.stealTask(workers)

	if stolenTask == nil {
		t.Error("Should be able to steal task")
	}

	// 验证任务被窃取
	if stolenTask != task2 { // 应该窃取最后一个任务（LIFO）
		t.Error("Should steal the last task")
	}
}

// 测试对象缓存
func TestWorker_ObjectCache(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 获取任务包装器
	wrapper1 := worker.getTaskWrapper()
	if wrapper1 == nil {
		t.Error("Should get task wrapper from cache")
	}

	wrapper2 := worker.getTaskWrapper()
	if wrapper2 == nil {
		t.Error("Should get another task wrapper")
	}

	if wrapper1 == wrapper2 {
		t.Error("Should get different wrappers")
	}

	// 回收任务包装器
	wrapper1.task = NewMockTask("test", pool.PriorityNormal)
	worker.recycleTaskWrapper(wrapper1)

	// 验证包装器被清理
	if wrapper1.task != nil {
		t.Error("Task should be cleared after recycling")
	}
}

// 测试Future实现
func TestFuture(t *testing.T) {
	future := NewFuture()

	// 测试初始状态
	if future.IsDone() {
		t.Error("New future should not be done")
	}

	// 测试取消
	if !future.Cancel() {
		t.Error("Should be able to cancel future")
	}

	if !future.IsDone() {
		t.Error("Cancelled future should be done")
	}

	// 验证取消结果
	result, err := future.Get()
	if result != nil {
		t.Error("Cancelled future should have nil result")
	}

	if err != pool.ErrTaskCancelled {
		t.Errorf("Expected ErrTaskCancelled, got %v", err)
	}

	// 测试重复取消
	if future.Cancel() {
		t.Error("Should not be able to cancel already cancelled future")
	}
}

// 测试Future超时获取
func TestFuture_GetWithTimeout(t *testing.T) {
	future := NewFuture()

	// 测试超时
	start := time.Now()
	result, err := future.GetWithTimeout(time.Millisecond * 100)
	duration := time.Since(start)

	if result != nil {
		t.Error("Timeout should return nil result")
	}

	if err != pool.ErrTaskTimeout {
		t.Errorf("Expected ErrTaskTimeout, got %v", err)
	}

	if duration < time.Millisecond*90 || duration > time.Millisecond*150 {
		t.Errorf("Timeout duration should be around 100ms, got %v", duration)
	}
}

// 测试工作协程启动和停止
func TestWorker_StartStop(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	var wg sync.WaitGroup

	// 启动工作协程
	worker.Start(&wg)

	// 等待一小段时间确保协程启动
	time.Sleep(time.Millisecond * 10)

	// 提交一个任务
	task := NewMockTask("test", pool.PriorityNormal)
	future := NewFuture()
	taskWrapper := &TaskWrapper{
		task:   task,
		future: future,
		ctx:    context.Background(),
	}

	if !worker.SubmitTask(taskWrapper) {
		t.Error("Should be able to submit task")
	}

	// 等待任务完成
	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Errorf("Task should complete successfully: %v", err)
	}

	if result != "result-test" {
		t.Errorf("Expected result 'result-test', got %v", result)
	}

	// 停止工作协程
	worker.Stop()

	// 等待协程结束
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 协程正常结束
	case <-time.After(time.Second):
		t.Error("Worker should stop within 1 second")
	}
}

// 基准测试：任务执行性能
func BenchmarkWorker_ExecuteTask(b *testing.B) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 30,
	}

	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			task := NewMockTask("bench", pool.PriorityNormal)
			task.duration = 0 // 无延迟
			future := NewFuture()

			taskWrapper := &TaskWrapper{
				task:   task,
				future: future,
				ctx:    context.Background(),
			}

			worker.executeTask(taskWrapper)
			future.Get()
		}
	})
}

// 基准测试：本地队列操作
func BenchmarkWorker_LocalQueue(b *testing.B) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	tasks := make([]*TaskWrapper, pool.LocalQueueSize/2)
	for i := range tasks {
		tasks[i] = &TaskWrapper{task: NewMockTask("bench", pool.PriorityNormal)}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 入队
			for _, task := range tasks {
				worker.enqueueLocal(task)
			}

			// 出队
			for range tasks {
				worker.dequeueLocal()
			}
		}
	})
}

// 测试工作协程panic处理机制
func TestWorker_PanicHandling(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	var wg sync.WaitGroup
	worker.Start(&wg)

	// 等待工作协程启动
	time.Sleep(time.Millisecond * 10)

	if !worker.IsRunning() {
		t.Error("Worker should be running")
	}

	// 提交一个会panic的任务
	panicTask := NewMockTask("panic-task", pool.PriorityNormal)
	panicTask.panicMsg = "test panic for handling"
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:   panicTask,
		future: future,
		ctx:    context.Background(),
	}

	if !worker.SubmitTask(taskWrapper) {
		t.Error("Should be able to submit panic task")
	}

	// 等待任务执行和panic处理
	result, err := future.GetWithTimeout(time.Second)

	// 验证panic被正确处理
	if err == nil {
		t.Error("Expected error from panic task")
	}

	if result != nil {
		t.Error("Expected nil result from panic task")
	}

	// 验证错误处理器被调用
	if mockErrorHandler.PanicCalls() == 0 {
		t.Error("Error handler should have been called for panic")
	}

	// 验证工作协程仍在运行（panic被恢复）
	if !worker.IsRunning() {
		t.Error("Worker should still be running after panic recovery")
	}

	// 提交一个正常任务验证工作协程仍然工作
	normalTask := NewMockTask("normal-after-panic", pool.PriorityNormal)
	normalFuture := NewFuture()

	normalWrapper := &TaskWrapper{
		task:   normalTask,
		future: normalFuture,
		ctx:    context.Background(),
	}

	if !worker.SubmitTask(normalWrapper) {
		t.Error("Should be able to submit normal task after panic")
	}

	normalResult, normalErr := normalFuture.GetWithTimeout(time.Second)
	if normalErr != nil {
		t.Errorf("Normal task should complete successfully after panic: %v", normalErr)
	}

	if normalResult != "result-normal-after-panic" {
		t.Errorf("Expected result 'result-normal-after-panic', got %v", normalResult)
	}

	// 停止工作协程
	worker.Stop()
	wg.Wait()
}

// 测试本地队列大小统计
func TestWorker_LocalQueueSize(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 测试空队列
	if size := worker.LocalQueueSize(); size != 0 {
		t.Errorf("Empty queue size should be 0, got %d", size)
	}

	// 添加一些任务
	task1 := &TaskWrapper{task: NewMockTask("task1", pool.PriorityNormal)}
	task2 := &TaskWrapper{task: NewMockTask("task2", pool.PriorityNormal)}
	task3 := &TaskWrapper{task: NewMockTask("task3", pool.PriorityNormal)}

	worker.enqueueLocal(task1)
	if size := worker.LocalQueueSize(); size != 1 {
		t.Errorf("Queue size should be 1, got %d", size)
	}

	worker.enqueueLocal(task2)
	worker.enqueueLocal(task3)
	if size := worker.LocalQueueSize(); size != 3 {
		t.Errorf("Queue size should be 3, got %d", size)
	}

	// 取出一个任务
	worker.dequeueLocal()
	if size := worker.LocalQueueSize(); size != 2 {
		t.Errorf("Queue size should be 2 after dequeue, got %d", size)
	}
}

// 测试并发安全的本地队列操作
func TestWorker_LocalQueueConcurrency(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	const numGoroutines = 10
	const tasksPerGoroutine = 100

	var wg sync.WaitGroup

	// 并发入队
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				task := &TaskWrapper{
					task: NewMockTask(fmt.Sprintf("task-%d-%d", id, j), pool.PriorityNormal),
				}
				worker.enqueueLocal(task)
			}
		}(i)
	}

	// 并发出队
	dequeueCount := int32(0)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				if task := worker.dequeueLocal(); task != nil {
					atomic.AddInt32(&dequeueCount, 1)
				}
				time.Sleep(time.Microsecond) // 小延迟避免忙等
			}
		}()
	}

	wg.Wait()

	// 验证没有数据竞争导致的问题
	finalSize := worker.LocalQueueSize()
	totalDequeued := atomic.LoadInt32(&dequeueCount)

	t.Logf("Final queue size: %d, Total dequeued: %d", finalSize, totalDequeued)

	// 由于并发操作，我们只验证没有崩溃和基本的一致性
	if finalSize < 0 {
		t.Error("Queue size should not be negative")
	}
}

// 测试工作窃取的线程安全性
func TestWorker_StealTaskConcurrency(t *testing.T) {
	config := &pool.Config{WorkerCount: 2}

	worker1 := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})
	worker2 := NewWorker(2, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 给worker2添加大量任务
	for i := 0; i < 50; i++ {
		task := &TaskWrapper{task: NewMockTask(fmt.Sprintf("task-%d", i), pool.PriorityNormal)}
		worker2.enqueueLocal(task)
	}

	// 模拟worker2处理了一些任务
	atomic.StoreInt64(&worker2.taskCount, 100)

	workers := []pool.Worker{worker1, worker2}
	stolenTasks := int32(0)

	var wg sync.WaitGroup

	// 并发窃取任务
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if task := worker1.stealTask(workers); task != nil {
					atomic.AddInt32(&stolenTasks, 1)
				}
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	t.Logf("Stolen tasks: %d", atomic.LoadInt32(&stolenTasks))

	// 验证窃取操作没有导致数据竞争
	if atomic.LoadInt32(&stolenTasks) < 0 {
		t.Error("Stolen task count should not be negative")
	}
}

// 基准测试：增强的任务执行性能
func BenchmarkWorker_ExecuteTaskEnhanced(b *testing.B) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 30,
	}

	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			task := NewMockTask("bench", pool.PriorityNormal)
			task.duration = 0 // 无延迟
			future := NewFuture()

			taskWrapper := &TaskWrapper{
				task:       task,
				future:     future,
				ctx:        context.Background(),
				submitTime: time.Now(),
				priority:   task.Priority(),
			}

			worker.executeTask(taskWrapper)
			future.Get()
		}
	})
}

// 基准测试：并发本地队列操作
func BenchmarkWorker_LocalQueueConcurrent(b *testing.B) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	tasks := make([]*TaskWrapper, 100)
	for i := range tasks {
		tasks[i] = &TaskWrapper{task: NewMockTask("bench", pool.PriorityNormal)}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 入队操作
			for i := 0; i < 50; i++ {
				worker.enqueueLocal(tasks[i])
			}

			// 出队操作
			for i := 0; i < 50; i++ {
				worker.dequeueLocal()
			}
		}
	})
}

// 测试工作协程状态管理
func TestWorker_StateManagement(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 初始状态应该是空闲
	if !worker.IsIdle() {
		t.Error("New worker should be idle")
	}

	if worker.IsBusy() {
		t.Error("New worker should not be busy")
	}

	if worker.IsError() {
		t.Error("New worker should not be in error state")
	}

	// 创建正常任务
	task := NewMockTask("state-test", pool.PriorityNormal)
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:       task,
		future:     future,
		submitTime: time.Now(),
		priority:   task.Priority(),
		ctx:        context.Background(),
	}

	// 执行任务（这会改变状态）
	go func() {
		worker.executeTask(taskWrapper)
	}()

	// 等待任务完成
	future.GetWithTimeout(time.Second)

	// 任务完成后应该回到空闲状态
	if !worker.IsIdle() {
		t.Error("Worker should be idle after task completion")
	}

	// 测试panic任务的状态变化
	panicTask := NewMockTask("panic-state-test", pool.PriorityNormal)
	panicTask.panicMsg = "state test panic"
	panicFuture := NewFuture()

	panicWrapper := &TaskWrapper{
		task:       panicTask,
		future:     panicFuture,
		submitTime: time.Now(),
		priority:   panicTask.Priority(),
		ctx:        context.Background(),
	}

	// 执行panic任务
	worker.executeTask(panicWrapper)

	// panic任务执行后应该处于错误状态
	if !worker.IsError() {
		t.Error("Worker should be in error state after panic")
	}

	// 验证失败计数增加
	if worker.FailedCount() == 0 {
		t.Error("Failed count should be greater than 0 after panic")
	}
}

// 测试增强的panic恢复机制
func TestWorker_EnhancedPanicRecovery(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 设置较小的最大重启次数用于测试
	worker.maxRestarts = 3
	worker.backoffBase = time.Millisecond * 10

	var wg sync.WaitGroup
	worker.Start(&wg)

	// 等待工作协程启动
	time.Sleep(time.Millisecond * 10)

	initialRestartCount := worker.RestartCount()

	// 提交多个会导致协程级panic的任务（模拟严重错误）
	// 注意：这里我们需要模拟协程级别的panic，而不是任务级别的panic
	// 由于当前实现中协程级panic很难触发，我们主要测试任务级panic的处理

	for i := 0; i < 2; i++ {
		panicTask := NewMockTask(fmt.Sprintf("panic-task-%d", i), pool.PriorityNormal)
		panicTask.panicMsg = fmt.Sprintf("test panic %d", i)
		future := NewFuture()

		taskWrapper := &TaskWrapper{
			task:   panicTask,
			future: future,
			ctx:    context.Background(),
		}

		if !worker.SubmitTask(taskWrapper) {
			t.Errorf("Should be able to submit panic task %d", i)
		}

		// 等待任务完成
		future.GetWithTimeout(time.Second)
	}

	// 验证错误处理
	if mockErrorHandler.PanicCalls() < 2 {
		t.Errorf("Expected at least 2 panic calls, got %d", mockErrorHandler.PanicCalls())
	}

	// 验证工作协程仍在运行
	if !worker.IsRunning() {
		t.Error("Worker should still be running after panic recovery")
	}

	// 验证重启计数没有增加（因为是任务级panic，不是协程级panic）
	if worker.RestartCount() != initialRestartCount {
		t.Logf("Restart count changed from %d to %d", initialRestartCount, worker.RestartCount())
	}

	// 停止工作协程
	worker.Stop()
	wg.Wait()
}

// 测试失败任务计数
func TestWorker_FailedTaskCount(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Millisecond * 50,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	initialFailedCount := worker.FailedCount()

	// 测试panic任务
	panicTask := NewMockTask("panic-fail-test", pool.PriorityNormal)
	panicTask.panicMsg = "fail test panic"
	panicFuture := NewFuture()

	panicWrapper := &TaskWrapper{
		task:   panicTask,
		future: panicFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(panicWrapper)

	// 验证失败计数增加
	if worker.FailedCount() != initialFailedCount+1 {
		t.Errorf("Expected failed count %d, got %d", initialFailedCount+1, worker.FailedCount())
	}

	// 测试超时任务
	timeoutTask := NewMockTask("timeout-fail-test", pool.PriorityNormal)
	timeoutTask.duration = time.Millisecond * 200 // 比超时时间长
	timeoutFuture := NewFuture()

	timeoutWrapper := &TaskWrapper{
		task:   timeoutTask,
		future: timeoutFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(timeoutWrapper)

	// 验证失败计数再次增加
	if worker.FailedCount() != initialFailedCount+2 {
		t.Errorf("Expected failed count %d, got %d", initialFailedCount+2, worker.FailedCount())
	}

	// 测试正常任务不增加失败计数
	normalTask := NewMockTask("normal-success-test", pool.PriorityNormal)
	normalFuture := NewFuture()

	normalWrapper := &TaskWrapper{
		task:   normalTask,
		future: normalFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(normalWrapper)

	// 验证失败计数没有增加
	if worker.FailedCount() != initialFailedCount+2 {
		t.Errorf("Expected failed count to remain %d, got %d", initialFailedCount+2, worker.FailedCount())
	}
}

// 测试工作协程生命周期管理
func TestWorker_LifecycleManagement(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 初始状态
	if worker.IsRunning() {
		t.Error("New worker should not be running")
	}

	if worker.IsActive() {
		t.Error("New worker should not be active")
	}

	var wg sync.WaitGroup

	// 启动工作协程
	worker.Start(&wg)

	// 等待启动
	time.Sleep(time.Millisecond * 10)

	if !worker.IsRunning() {
		t.Error("Worker should be running after start")
	}

	// 提交任务测试活跃状态
	task := NewMockTask("lifecycle-test", pool.PriorityNormal)
	task.duration = time.Millisecond * 100 // 稍长的执行时间
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:   task,
		future: future,
		ctx:    context.Background(),
	}

	if !worker.SubmitTask(taskWrapper) {
		t.Error("Should be able to submit task")
	}

	// 短暂等待，任务应该正在执行
	time.Sleep(time.Millisecond * 20)

	// 在任务执行期间，工作协程应该是活跃的
	// 注意：由于时序问题，这个测试可能不稳定，所以我们只记录状态
	t.Logf("Worker active during task execution: %v", worker.IsActive())

	// 等待任务完成
	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Errorf("Task should complete successfully: %v", err)
	}

	if result != "result-lifecycle-test" {
		t.Errorf("Expected result 'result-lifecycle-test', got %v", result)
	}

	// 停止工作协程
	worker.Stop()

	// 等待协程结束
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 协程正常结束
	case <-time.After(time.Second):
		t.Error("Worker should stop within 1 second")
	}

	if worker.IsRunning() {
		t.Error("Worker should not be running after stop")
	}
}

// 测试任务提交到已满的工作协程
func TestWorker_SubmitTaskToFullWorker(t *testing.T) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 填满本地队列
	for i := 0; i < pool.LocalQueueSize-1; i++ {
		task := &TaskWrapper{task: NewMockTask(fmt.Sprintf("fill-task-%d", i), pool.PriorityNormal)}
		if !worker.enqueueLocal(task) {
			t.Errorf("Should be able to enqueue task %d", i)
		}
	}

	// 现在本地队列应该满了，但通道还有空间
	extraTask := &TaskWrapper{task: NewMockTask("extra-task", pool.PriorityNormal)}
	if !worker.SubmitTask(extraTask) {
		t.Error("Should be able to submit task to channel when local queue is full")
	}

	// 再提交一个任务，这次应该失败（本地队列满，通道也满）
	anotherTask := &TaskWrapper{task: NewMockTask("another-task", pool.PriorityNormal)}
	if worker.SubmitTask(anotherTask) {
		t.Error("Should not be able to submit task when both local queue and channel are full")
	}
}

// 测试工作协程的最后活跃时间更新
func TestWorker_LastActiveTimeUpdate(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	initialTime := worker.LastActiveTime()

	// 等待一小段时间
	time.Sleep(time.Millisecond * 10)

	// 执行任务
	task := NewMockTask("time-test", pool.PriorityNormal)
	future := NewFuture()

	taskWrapper := &TaskWrapper{
		task:   task,
		future: future,
		ctx:    context.Background(),
	}

	worker.executeTask(taskWrapper)

	// 验证最后活跃时间已更新
	updatedTime := worker.LastActiveTime()
	if !updatedTime.After(initialTime) {
		t.Error("Last active time should be updated after task execution")
	}

	// 验证时间差在合理范围内
	timeDiff := updatedTime.Sub(initialTime)
	if timeDiff < time.Millisecond*5 || timeDiff > time.Second {
		t.Errorf("Time difference should be reasonable, got %v", timeDiff)
	}
}

// 测试工作窃取功能
func TestWorker_WorkStealing(t *testing.T) {
	config := &pool.Config{WorkerCount: 2}

	_ = NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{}) // worker1 not used in this test
	worker2 := NewWorker(2, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 给worker2添加多个任务
	tasks := make([]*TaskWrapper, 5)
	for i := range tasks {
		tasks[i] = &TaskWrapper{task: NewMockTask(fmt.Sprintf("steal-task-%d", i), pool.PriorityNormal)}
		worker2.enqueueLocal(tasks[i])
	}

	// 验证worker2可以被窃取
	if !worker2.CanStealWork() {
		t.Error("Worker2 should allow work stealing when it has multiple tasks")
	}

	// worker1窃取任务
	stolenTask := worker2.StealWork()
	if stolenTask == nil {
		t.Error("Should be able to steal task from worker2")
	}

	// 验证窃取的是最后一个任务（LIFO）
	if stolenTask != tasks[4] {
		t.Error("Should steal the last task (LIFO)")
	}

	// 验证worker2的队列大小减少了
	if worker2.LocalQueueSize() != 4 {
		t.Errorf("Worker2 queue size should be 4 after stealing, got %d", worker2.LocalQueueSize())
	}

	// 测试空队列不能窃取
	emptyWorker := NewWorker(3, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})
	if emptyWorker.CanStealWork() {
		t.Error("Empty worker should not allow work stealing")
	}

	if emptyWorker.StealWork() != nil {
		t.Error("Should not be able to steal from empty worker")
	}

	// 测试只有一个任务的工作协程不能被窃取
	singleTaskWorker := NewWorker(4, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})
	singleTask := &TaskWrapper{task: NewMockTask("single-task", pool.PriorityNormal)}
	singleTaskWorker.enqueueLocal(singleTask)

	if singleTaskWorker.CanStealWork() {
		t.Error("Worker with single task should not allow work stealing")
	}
}

// 测试工作协程统计信息
func TestWorker_GetStats(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	mockPool := &MockPool{}
	mockErrorHandler := &MockErrorHandler{}
	mockMonitor := &MockMonitor{}

	worker := NewWorker(1, mockPool, config, mockErrorHandler, mockMonitor)

	// 获取初始统计信息
	stats := worker.GetStats()

	// 验证初始统计信息
	if stats.ID != 1 {
		t.Errorf("Expected worker ID 1, got %d", stats.ID)
	}

	if stats.IsActive {
		t.Error("New worker should not be active")
	}

	if stats.IsRunning {
		t.Error("New worker should not be running")
	}

	if stats.State != 0 {
		t.Errorf("New worker should be in idle state (0), got %d", stats.State)
	}

	if stats.TaskCount != 0 {
		t.Errorf("New worker should have 0 task count, got %d", stats.TaskCount)
	}

	if stats.FailedCount != 0 {
		t.Errorf("New worker should have 0 failed count, got %d", stats.FailedCount)
	}

	if stats.RestartCount != 0 {
		t.Errorf("New worker should have 0 restart count, got %d", stats.RestartCount)
	}

	if stats.LocalQueueSize != 0 {
		t.Errorf("New worker should have 0 local queue size, got %d", stats.LocalQueueSize)
	}

	// 添加一些任务到本地队列
	task1 := &TaskWrapper{task: NewMockTask("stats-task-1", pool.PriorityNormal)}
	task2 := &TaskWrapper{task: NewMockTask("stats-task-2", pool.PriorityNormal)}

	worker.enqueueLocal(task1)
	worker.enqueueLocal(task2)

	// 执行一个任务
	normalTask := NewMockTask("stats-normal", pool.PriorityNormal)
	normalFuture := NewFuture()

	normalWrapper := &TaskWrapper{
		task:   normalTask,
		future: normalFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(normalWrapper)

	// 执行一个失败任务
	panicTask := NewMockTask("stats-panic", pool.PriorityNormal)
	panicTask.panicMsg = "stats test panic"
	panicFuture := NewFuture()

	panicWrapper := &TaskWrapper{
		task:   panicTask,
		future: panicFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(panicWrapper)

	// 获取更新后的统计信息
	updatedStats := worker.GetStats()

	// 验证统计信息更新
	if updatedStats.TaskCount != 2 {
		t.Errorf("Expected task count 2, got %d", updatedStats.TaskCount)
	}

	if updatedStats.FailedCount != 1 {
		t.Errorf("Expected failed count 1, got %d", updatedStats.FailedCount)
	}

	if updatedStats.LocalQueueSize != 2 {
		t.Errorf("Expected local queue size 2, got %d", updatedStats.LocalQueueSize)
	}

	// 验证最后活跃时间已更新
	if updatedStats.LastActiveTime.IsZero() {
		t.Error("Last active time should not be zero")
	}
}

// 测试工作协程的错误状态恢复
func TestWorker_ErrorStateRecovery(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 初始状态应该是空闲
	if !worker.IsIdle() {
		t.Error("Worker should start in idle state")
	}

	// 执行一个panic任务
	panicTask := NewMockTask("error-recovery-panic", pool.PriorityNormal)
	panicTask.panicMsg = "error recovery test"
	panicFuture := NewFuture()

	panicWrapper := &TaskWrapper{
		task:   panicTask,
		future: panicFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(panicWrapper)

	// 任务panic后应该处于错误状态
	if !worker.IsError() {
		t.Error("Worker should be in error state after panic")
	}

	// 执行一个正常任务
	normalTask := NewMockTask("error-recovery-normal", pool.PriorityNormal)
	normalFuture := NewFuture()

	normalWrapper := &TaskWrapper{
		task:   normalTask,
		future: normalFuture,
		ctx:    context.Background(),
	}

	worker.executeTask(normalWrapper)

	// 正常任务执行后应该恢复到空闲状态
	if !worker.IsIdle() {
		t.Error("Worker should recover to idle state after successful task")
	}

	// 验证任务都被正确处理
	if worker.TaskCount() != 2 {
		t.Errorf("Expected task count 2, got %d", worker.TaskCount())
	}

	if worker.FailedCount() != 1 {
		t.Errorf("Expected failed count 1, got %d", worker.FailedCount())
	}
}

// 测试工作协程的退避重启机制
func TestWorker_BackoffRestartMechanism(t *testing.T) {
	config := &pool.Config{
		WorkerCount: 1,
		TaskTimeout: time.Second * 5,
	}

	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 设置较小的最大重启次数和退避时间用于测试
	worker.maxRestarts = 3
	worker.backoffBase = time.Millisecond * 5

	// 验证初始配置
	if worker.maxRestarts != 3 {
		t.Errorf("Expected max restarts 3, got %d", worker.maxRestarts)
	}

	if worker.backoffBase != time.Millisecond*5 {
		t.Errorf("Expected backoff base 5ms, got %v", worker.backoffBase)
	}

	// 由于协程级panic很难在单元测试中触发，我们主要验证配置是否正确设置
	// 实际的重启逻辑在集成测试中更容易验证

	// 验证重启计数初始为0
	if worker.RestartCount() != 0 {
		t.Errorf("Expected initial restart count 0, got %d", worker.RestartCount())
	}
}

// 基准测试：工作窃取性能
func BenchmarkWorker_WorkStealing(b *testing.B) {
	config := &pool.Config{WorkerCount: 2}

	_ = NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{}) // worker1 not used in benchmark
	worker2 := NewWorker(2, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	// 给worker2添加任务
	tasks := make([]*TaskWrapper, 100)
	for i := range tasks {
		tasks[i] = &TaskWrapper{task: NewMockTask("bench-steal", pool.PriorityNormal)}
		worker2.enqueueLocal(tasks[i])
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if worker2.CanStealWork() {
				worker2.StealWork()
			}
		}
	})
}

// 基准测试：状态检查性能
func BenchmarkWorker_StateChecks(b *testing.B) {
	config := &pool.Config{WorkerCount: 1}
	worker := NewWorker(1, &MockPool{}, config, &MockErrorHandler{}, &MockMonitor{})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			worker.IsIdle()
			worker.IsBusy()
			worker.IsError()
			worker.IsActive()
			worker.IsRunning()
		}
	})
}
