package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/uptutu/goman/internal/worker"
	"github.com/uptutu/goman/pkg/pool"
)

// ExampleTask 示例任务实现
type ExampleTask struct {
	id       string
	priority int
	work     func() (any, error)
}

func NewExampleTask(id string, priority int, work func() (any, error)) *ExampleTask {
	return &ExampleTask{
		id:       id,
		priority: priority,
		work:     work,
	}
}

func (t *ExampleTask) Execute(ctx context.Context) (any, error) {
	if t.work != nil {
		return t.work()
	}
	return fmt.Sprintf("Task %s completed", t.id), nil
}

func (t *ExampleTask) Priority() int {
	return t.priority
}

// ExampleErrorHandler 示例错误处理器
type ExampleErrorHandler struct{}

func (h *ExampleErrorHandler) HandlePanic(workerID int, task pool.Task, panicValue any) {
	log.Printf("Worker %d panic: %v", workerID, panicValue)
}

func (h *ExampleErrorHandler) HandleTimeout(task pool.Task, duration time.Duration) {
	log.Printf("Task timeout after %v", duration)
}

func (h *ExampleErrorHandler) HandleQueueFull(task pool.Task) error {
	log.Printf("Queue full, rejecting task")
	return pool.ErrQueueFull
}

// ExampleMonitor 示例监控器
type ExampleMonitor struct {
	submitCount   int64
	completeCount int64
	failCount     int64
}

func (m *ExampleMonitor) RecordTaskSubmit(task pool.Task) {
	m.submitCount++
	log.Printf("Task submitted, total: %d", m.submitCount)
}

func (m *ExampleMonitor) RecordTaskComplete(task pool.Task, duration time.Duration) {
	m.completeCount++
	log.Printf("Task completed in %v, total: %d", duration, m.completeCount)
}

func (m *ExampleMonitor) RecordTaskFail(task pool.Task, err error) {
	m.failCount++
	log.Printf("Task failed: %v, total: %d", err, m.failCount)
}

func (m *ExampleMonitor) GetStats() pool.PoolStats {
	return pool.PoolStats{
		CompletedTasks: m.completeCount,
		FailedTasks:    m.failCount,
	}
}

// MockPool 模拟协程池
type MockPool struct{}

func (p *MockPool) Submit(task pool.Task) error                                   { return nil }
func (p *MockPool) SubmitWithTimeout(task pool.Task, timeout time.Duration) error { return nil }
func (p *MockPool) SubmitAsync(task pool.Task) pool.Future                        { return nil }
func (p *MockPool) Shutdown(ctx context.Context) error                            { return nil }
func (p *MockPool) Stats() pool.PoolStats                                         { return pool.PoolStats{} }

func main() {
	fmt.Println("=== Worker Management Example ===")

	// 创建配置
	config := &pool.Config{
		WorkerCount: 2,
		QueueSize:   100,
		TaskTimeout: time.Second * 5,
	}

	// 创建错误处理器和监控器
	errorHandler := &ExampleErrorHandler{}
	monitor := &ExampleMonitor{}
	mockPool := &MockPool{}

	// 创建工作协程
	worker1 := worker.NewWorker(1, mockPool, config, errorHandler, monitor)
	worker2 := worker.NewWorker(2, mockPool, config, errorHandler, monitor)

	var wg sync.WaitGroup

	// 启动工作协程
	fmt.Println("Starting workers...")
	worker1.Start(&wg)
	worker2.Start(&wg)

	// 等待工作协程启动
	time.Sleep(time.Millisecond * 100)

	fmt.Printf("Worker 1 running: %v\n", worker1.IsRunning())
	fmt.Printf("Worker 2 running: %v\n", worker2.IsRunning())

	// 示例1: 提交普通任务
	fmt.Println("\n=== Example 1: Normal Tasks ===")
	for i := 0; i < 5; i++ {
		task := NewExampleTask(fmt.Sprintf("normal-%d", i), pool.PriorityNormal, func() (any, error) {
			time.Sleep(time.Millisecond * 100) // 模拟工作
			return "success", nil
		})

		future := worker.NewFuture()
		taskWrapper := &worker.TaskWrapper{
			Task:       task,
			Future:     future,
			SubmitTime: time.Now(),
			Priority:   task.Priority(),
			Ctx:        context.Background(),
		}

		workerToUse := worker1
		if i%2 == 1 {
			workerToUse = worker2
		}

		if workerToUse.SubmitTask(taskWrapper) {
			go func(id int, f *worker.Future) {
				result, err := f.GetWithTimeout(time.Second * 2)
				if err != nil {
					fmt.Printf("Task normal-%d failed: %v\n", id, err)
				} else {
					fmt.Printf("Task normal-%d result: %v\n", id, result)
				}
			}(i, future)
		}
	}

	// 等待任务完成
	time.Sleep(time.Second)

	// 示例2: 测试panic恢复
	fmt.Println("\n=== Example 2: Panic Recovery ===")
	panicTask := NewExampleTask("panic-task", pool.PriorityHigh, func() (any, error) {
		panic("intentional panic for testing")
	})

	panicFuture := worker.NewFuture()
	panicWrapper := &worker.TaskWrapper{
		Task:       panicTask,
		Future:     panicFuture,
		SubmitTime: time.Now(),
		Priority:   panicTask.Priority(),
		Ctx:        context.Background(),
	}

	if worker1.SubmitTask(panicWrapper) {
		result, err := panicFuture.GetWithTimeout(time.Second)
		fmt.Printf("Panic task result: %v, error: %v\n", result, err)
	}

	// 验证工作协程仍在运行
	time.Sleep(time.Millisecond * 100)
	fmt.Printf("Worker 1 still running after panic: %v\n", worker1.IsRunning())

	// 示例3: 测试超时处理
	fmt.Println("\n=== Example 3: Timeout Handling ===")
	timeoutTask := NewExampleTask("timeout-task", pool.PriorityNormal, func() (any, error) {
		time.Sleep(time.Second * 10) // 超过配置的超时时间
		return "should not reach here", nil
	})

	timeoutFuture := worker.NewFuture()
	timeoutWrapper := &worker.TaskWrapper{
		Task:       timeoutTask,
		Future:     timeoutFuture,
		SubmitTime: time.Now(),
		Priority:   timeoutTask.Priority(),
		Ctx:        context.Background(),
	}

	if worker2.SubmitTask(timeoutWrapper) {
		result, err := timeoutFuture.GetWithTimeout(time.Second * 6)
		fmt.Printf("Timeout task result: %v, error: %v\n", result, err)
	}

	// 示例4: 测试本地队列和工作窃取
	fmt.Println("\n=== Example 4: Local Queue and Work Stealing ===")

	// 给worker2添加大量任务
	for i := 0; i < 10; i++ {
		task := NewExampleTask(fmt.Sprintf("queue-%d", i), pool.PriorityNormal, func() (any, error) {
			time.Sleep(time.Millisecond * 50)
			return "queued task done", nil
		})

		future := worker.NewFuture()
		taskWrapper := &worker.TaskWrapper{
			Task:       task,
			Future:     future,
			SubmitTime: time.Now(),
			Priority:   task.Priority(),
			Ctx:        context.Background(),
		}

		worker2.SubmitTask(taskWrapper)
	}

	fmt.Printf("Worker 2 local queue size: %d\n", worker2.LocalQueueSize())

	// 模拟工作窃取
	workers := []pool.Worker{worker1, worker2}
	if stolenTask := worker1.StealTask(workers); stolenTask != nil {
		fmt.Println("Worker 1 successfully stole a task from Worker 2")
	}

	fmt.Printf("Worker 2 local queue size after steal: %d\n", worker2.LocalQueueSize())

	// 示例5: 统计信息
	fmt.Println("\n=== Example 5: Worker Statistics ===")
	fmt.Printf("Worker 1 - ID: %d, Active: %v, Task Count: %d, Last Active: %v\n",
		worker1.ID(), worker1.IsActive(), worker1.TaskCount(), worker1.LastActiveTime())
	fmt.Printf("Worker 2 - ID: %d, Active: %v, Task Count: %d, Last Active: %v\n",
		worker2.ID(), worker2.IsActive(), worker2.TaskCount(), worker2.LastActiveTime())

	// 等待所有任务完成
	time.Sleep(time.Second * 2)

	// 停止工作协程
	fmt.Println("\n=== Shutting Down Workers ===")
	worker1.Stop()
	worker2.Stop()

	// 等待工作协程结束
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All workers stopped successfully")
	case <-time.After(time.Second * 5):
		fmt.Println("Timeout waiting for workers to stop")
	}

	// 最终统计
	fmt.Printf("\nFinal Statistics:\n")
	fmt.Printf("Worker 1 - Task Count: %d, Running: %v\n", worker1.TaskCount(), worker1.IsRunning())
	fmt.Printf("Worker 2 - Task Count: %d, Running: %v\n", worker2.TaskCount(), worker2.IsRunning())

	fmt.Println("\n=== Worker Management Example Complete ===")
}
