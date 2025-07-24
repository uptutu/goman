package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// WorkerTask 工作任务实现
type WorkerTask struct {
	ID       int
	Name     string
	WorkType string
	priority int
	workFunc func(ctx context.Context) (any, error)
}

func NewWorkerTask(id int, name, workType string, priority int, workFunc func(ctx context.Context) (any, error)) *WorkerTask {
	return &WorkerTask{
		ID:       id,
		Name:     name,
		WorkType: workType,
		priority: priority,
		workFunc: workFunc,
	}
}

func (t *WorkerTask) Execute(ctx context.Context) (any, error) {
	if t.workFunc != nil {
		return t.workFunc(ctx)
	}
	return fmt.Sprintf("Task %s (%s) completed", t.Name, t.WorkType), nil
}

func (t *WorkerTask) Priority() int {
	return t.priority
}

// WorkerStatsCollector 工作协程统计收集器
type WorkerStatsCollector struct {
	taskSubmitted int64
	taskCompleted int64
	taskFailed    int64
	totalDuration int64
}

func (wsc *WorkerStatsCollector) RecordSubmit() {
	atomic.AddInt64(&wsc.taskSubmitted, 1)
}

func (wsc *WorkerStatsCollector) RecordComplete(duration time.Duration) {
	atomic.AddInt64(&wsc.taskCompleted, 1)
	atomic.AddInt64(&wsc.totalDuration, int64(duration))
}

func (wsc *WorkerStatsCollector) RecordFail() {
	atomic.AddInt64(&wsc.taskFailed, 1)
}

func (wsc *WorkerStatsCollector) GetStats() (submitted, completed, failed int64, avgDuration time.Duration) {
	submitted = atomic.LoadInt64(&wsc.taskSubmitted)
	completed = atomic.LoadInt64(&wsc.taskCompleted)
	failed = atomic.LoadInt64(&wsc.taskFailed)

	totalDur := atomic.LoadInt64(&wsc.totalDuration)
	if completed > 0 {
		avgDuration = time.Duration(totalDur / completed)
	}
	return
}

func main() {
	fmt.Println("=== Goroutine Pool Worker Usage Example ===")

	// 创建协程池配置
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(3).
		WithQueueSize(50).
		WithTaskTimeout(10 * time.Second).
		WithMetrics(true).
		WithMetricsInterval(1 * time.Second).
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

	// 创建统计收集器
	statsCollector := &WorkerStatsCollector{}

	// 示例1: CPU密集型任务
	fmt.Println("\n=== Example 1: CPU Intensive Tasks ===")
	var wg sync.WaitGroup

	for i := 1; i <= 5; i++ {
		wg.Add(1)
		task := NewWorkerTask(i, fmt.Sprintf("cpu-task-%d", i), "CPU", pool.PriorityNormal,
			func(ctx context.Context) (any, error) {
				defer wg.Done()

				// 模拟CPU密集型计算
				result := int64(0)
				iterations := 1000000

				for j := 0; j < iterations; j++ {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
						result += int64(j * j)
						if j%100000 == 0 {
							// 定期检查取消信号
							if ctx.Err() != nil {
								return nil, ctx.Err()
							}
						}
					}
				}

				return fmt.Sprintf("CPU task completed: calculated %d iterations, result=%d", iterations, result%1000000), nil
			})

		statsCollector.RecordSubmit()
		future := p.SubmitAsync(task)

		go func(taskID int, f pool.Future) {
			start := time.Now()
			result, err := f.GetWithTimeout(5 * time.Second)
			duration := time.Since(start)

			if err != nil {
				statsCollector.RecordFail()
				fmt.Printf("CPU Task %d failed: %v\n", taskID, err)
			} else {
				statsCollector.RecordComplete(duration)
				fmt.Printf("CPU Task %d result: %v (took %v)\n", taskID, result, duration)
			}
		}(i, future)
	}

	// 示例2: IO密集型任务
	fmt.Println("\n=== Example 2: IO Intensive Tasks ===")

	for i := 1; i <= 4; i++ {
		wg.Add(1)
		task := NewWorkerTask(100+i, fmt.Sprintf("io-task-%d", i), "IO", pool.PriorityHigh,
			func(ctx context.Context) (any, error) {
				defer wg.Done()

				// 模拟IO操作
				ioDelay := time.Duration(200+rand.Intn(300)) * time.Millisecond

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(ioDelay):
					// 模拟读取文件或网络请求
					dataSize := rand.Intn(5000) + 1000
					return fmt.Sprintf("IO operation completed: processed %d bytes in %v", dataSize, ioDelay), nil
				}
			})

		statsCollector.RecordSubmit()
		future := p.SubmitAsync(task)

		go func(taskID int, f pool.Future) {
			start := time.Now()
			result, err := f.GetWithTimeout(3 * time.Second)
			duration := time.Since(start)

			if err != nil {
				statsCollector.RecordFail()
				fmt.Printf("IO Task %d failed: %v\n", taskID, err)
			} else {
				statsCollector.RecordComplete(duration)
				fmt.Printf("IO Task %d result: %v (took %v)\n", taskID, result, duration)
			}
		}(100+i, future)
	}

	// 示例3: 混合任务类型
	fmt.Println("\n=== Example 3: Mixed Task Types ===")

	taskTypes := []struct {
		name     string
		priority int
		workFunc func(ctx context.Context) (any, error)
	}{
		{
			name:     "database-query",
			priority: pool.PriorityHigh,
			workFunc: func(ctx context.Context) (any, error) {
				// 模拟数据库查询
				queryTime := time.Duration(100+rand.Intn(200)) * time.Millisecond
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(queryTime):
					recordCount := rand.Intn(1000) + 100
					return fmt.Sprintf("Database query completed: found %d records in %v", recordCount, queryTime), nil
				}
			},
		},
		{
			name:     "image-processing",
			priority: pool.PriorityNormal,
			workFunc: func(ctx context.Context) (any, error) {
				// 模拟图像处理
				processTime := time.Duration(300+rand.Intn(500)) * time.Millisecond
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(processTime):
					width, height := 1920+rand.Intn(1080), 1080+rand.Intn(720)
					return fmt.Sprintf("Image processed: %dx%d pixels in %v", width, height, processTime), nil
				}
			},
		},
		{
			name:     "api-call",
			priority: pool.PriorityNormal,
			workFunc: func(ctx context.Context) (any, error) {
				// 模拟API调用
				callTime := time.Duration(150+rand.Intn(250)) * time.Millisecond
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(callTime):
					statusCode := 200
					if rand.Float32() < 0.1 { // 10% 失败率
						statusCode = 500
						return nil, fmt.Errorf("API call failed with status %d", statusCode)
					}
					return fmt.Sprintf("API call successful: status %d, response time %v", statusCode, callTime), nil
				}
			},
		},
	}

	for i, taskType := range taskTypes {
		for j := 1; j <= 3; j++ {
			wg.Add(1)
			taskID := 200 + i*10 + j
			task := NewWorkerTask(taskID, fmt.Sprintf("%s-%d", taskType.name, j), taskType.name, taskType.priority, taskType.workFunc)

			statsCollector.RecordSubmit()
			future := p.SubmitAsync(task)

			go func(id int, name string, f pool.Future) {
				defer wg.Done()
				start := time.Now()
				result, err := f.GetWithTimeout(2 * time.Second)
				duration := time.Since(start)

				if err != nil {
					statsCollector.RecordFail()
					fmt.Printf("Mixed Task %d (%s) failed: %v\n", id, name, err)
				} else {
					statsCollector.RecordComplete(duration)
					fmt.Printf("Mixed Task %d (%s) result: %v (took %v)\n", id, name, result, duration)
				}
			}(taskID, taskType.name, future)
		}
	}

	// 示例4: 错误处理和恢复
	fmt.Println("\n=== Example 4: Error Handling and Recovery ===")

	// 提交一个会panic的任务
	panicTask := NewWorkerTask(300, "panic-task", "ERROR", pool.PriorityNormal,
		func(ctx context.Context) (any, error) {
			time.Sleep(100 * time.Millisecond) // 短暂延迟
			panic("intentional panic for testing error recovery")
		})

	statsCollector.RecordSubmit()
	panicFuture := p.SubmitAsync(panicTask)

	go func() {
		result, err := panicFuture.GetWithTimeout(1 * time.Second)
		if err != nil {
			statsCollector.RecordFail()
			fmt.Printf("Panic task handled gracefully: %v\n", err)
		} else {
			fmt.Printf("Panic task unexpected success: %v\n", result)
		}
	}()

	// 提交一个超时任务
	timeoutTask := NewWorkerTask(301, "timeout-task", "TIMEOUT", pool.PriorityLow,
		func(ctx context.Context) (any, error) {
			// 故意超过任务超时时间
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(15 * time.Second): // 超过配置的10秒超时
				return "should not reach here", nil
			}
		})

	statsCollector.RecordSubmit()
	timeoutFuture := p.SubmitAsync(timeoutTask)

	go func() {
		result, err := timeoutFuture.GetWithTimeout(12 * time.Second)
		if err != nil {
			statsCollector.RecordFail()
			fmt.Printf("Timeout task handled correctly: %v\n", err)
		} else {
			fmt.Printf("Timeout task unexpected success: %v\n", result)
		}
	}()

	// 等待所有任务完成
	fmt.Println("\n=== Waiting for all tasks to complete ===")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All tasks completed")
	case <-time.After(20 * time.Second):
		fmt.Println("Timeout waiting for tasks to complete")
	}

	// 显示统计信息
	fmt.Println("\n=== Task Statistics ===")
	submitted, completed, failed, avgDuration := statsCollector.GetStats()
	fmt.Printf("Tasks Submitted: %d\n", submitted)
	fmt.Printf("Tasks Completed: %d\n", completed)
	fmt.Printf("Tasks Failed: %d\n", failed)
	fmt.Printf("Average Duration: %v\n", avgDuration)
	fmt.Printf("Success Rate: %.2f%%\n", float64(completed)/float64(submitted)*100)

	// 显示协程池统计信息
	fmt.Println("\n=== Pool Statistics ===")
	poolStats := p.Stats()
	fmt.Printf("Active Workers: %d\n", poolStats.ActiveWorkers)
	fmt.Printf("Queued Tasks: %d\n", poolStats.QueuedTasks)
	fmt.Printf("Completed Tasks: %d\n", poolStats.CompletedTasks)
	fmt.Printf("Failed Tasks: %d\n", poolStats.FailedTasks)
	fmt.Printf("Average Task Duration: %v\n", poolStats.AvgTaskDuration)
	fmt.Printf("Throughput (TPS): %.2f\n", poolStats.ThroughputTPS)
	fmt.Printf("Memory Usage: %d bytes\n", poolStats.MemoryUsage)

	// 优雅关闭协程池
	fmt.Println("\n=== Shutting down pool ===")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown pool: %v", err)
	} else {
		fmt.Println("Pool shutdown successfully")
	}

	fmt.Println("\n=== Worker Usage Example Complete ===")
}
