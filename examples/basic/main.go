package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// DataProcessingTask 数据处理任务
type DataProcessingTask struct {
	ID       int
	Data     string
	priority int
}

func (t *DataProcessingTask) Execute(ctx context.Context) (any, error) {
	// 真实的数据处理工作
	processingTime := time.Duration(50+rand.Intn(100)) * time.Millisecond

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(processingTime):
		// 模拟实际的数据处理逻辑
		processedData := fmt.Sprintf("PROCESSED_%s", t.Data)
		result := fmt.Sprintf("Task %d processed '%s' -> '%s' in %v",
			t.ID, t.Data, processedData, processingTime)
		return result, nil
	}
}

func (t *DataProcessingTask) Priority() int {
	return t.priority
}

// ComputeTask 计算任务
type ComputeTask struct {
	ID     int
	Number int
}

func (t *ComputeTask) Execute(ctx context.Context) (any, error) {
	// 真实的计算密集型任务
	result := int64(0)
	iterations := t.Number * 1000

	for i := 0; i < iterations; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// 执行一些实际的计算
			result += int64(i * i)
			if i%10000 == 0 {
				// 每10000次迭代检查一次取消信号
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
		}
	}

	return fmt.Sprintf("Compute task %d: calculated sum of squares for %d iterations = %d",
		t.ID, iterations, result), nil
}

func (t *ComputeTask) Priority() int {
	return pool.PriorityNormal
}

// IOTask IO任务
type IOTask struct {
	ID       int
	Filename string
}

func (t *IOTask) Execute(ctx context.Context) (any, error) {
	// 模拟真实的IO操作
	ioDelay := time.Duration(100+rand.Intn(200)) * time.Millisecond

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(ioDelay):
		// 模拟文件处理结果
		fileSize := rand.Intn(10000) + 1000
		result := fmt.Sprintf("IO task %d: processed file '%s' (%d bytes) in %v",
			t.ID, t.Filename, fileSize, ioDelay)
		return result, nil
	}
}

func (t *IOTask) Priority() int {
	return pool.PriorityHigh
}

func main() {
	fmt.Println("=== Goroutine Pool SDK - Basic Usage Example ===")

	// 创建协程池配置
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
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

	// 示例1: 提交数据处理任务
	fmt.Println("\n1. Submitting data processing tasks...")
	for i := 1; i <= 5; i++ {
		task := &DataProcessingTask{
			ID:       i,
			Data:     fmt.Sprintf("dataset-%d", i),
			priority: pool.PriorityNormal,
		}

		if err := p.Submit(task); err != nil {
			log.Printf("Failed to submit data processing task %d: %v", i, err)
		} else {
			fmt.Printf("Submitted data processing task %d\n", i)
		}
	}

	// 示例2: 提交计算任务
	fmt.Println("\n2. Submitting compute tasks...")
	for i := 1; i <= 3; i++ {
		task := &ComputeTask{
			ID:     i,
			Number: 100 + i*50,
		}

		if err := p.Submit(task); err != nil {
			log.Printf("Failed to submit compute task %d: %v", i, err)
		} else {
			fmt.Printf("Submitted compute task %d\n", i)
		}
	}

	// 示例3: 提交高优先级IO任务
	fmt.Println("\n3. Submitting high priority IO tasks...")
	for i := 1; i <= 3; i++ {
		task := &IOTask{
			ID:       i,
			Filename: fmt.Sprintf("file-%d.txt", i),
		}

		if err := p.Submit(task); err != nil {
			log.Printf("Failed to submit IO task %d: %v", i, err)
		} else {
			fmt.Printf("Submitted IO task %d (high priority)\n", i)
		}
	}

	// 示例4: 异步提交任务并获取结果
	fmt.Println("\n4. Submitting async tasks...")
	futures := make([]pool.Future, 3)
	for i := 0; i < 3; i++ {
		task := &DataProcessingTask{
			ID:       100 + i,
			Data:     fmt.Sprintf("async-data-%d", i),
			priority: pool.PriorityNormal,
		}

		futures[i] = p.SubmitAsync(task)
		fmt.Printf("Submitted async task %d\n", 100+i)
	}

	// 等待异步任务完成并获取结果
	fmt.Println("\n5. Getting async task results...")
	for i, future := range futures {
		result, err := future.GetWithTimeout(3 * time.Second)
		if err != nil {
			log.Printf("Async task %d failed: %v", 100+i, err)
		} else {
			fmt.Printf("Async task %d result: %v\n", 100+i, result)
		}
	}

	// 示例5: 带超时的任务提交
	fmt.Println("\n6. Submitting tasks with timeout...")
	for i := 1; i <= 2; i++ {
		task := &ComputeTask{
			ID:     200 + i,
			Number: 500, // 较大的计算量
		}

		if err := p.SubmitWithTimeout(task, 1*time.Second); err != nil {
			log.Printf("Failed to submit timeout task %d: %v", 200+i, err)
		} else {
			fmt.Printf("Submitted timeout task %d\n", 200+i)
		}
	}

	// 等待任务执行
	fmt.Println("\n7. Waiting for tasks to complete...")
	time.Sleep(3 * time.Second)

	// 获取协程池统计信息
	fmt.Println("\n8. Pool statistics:")
	stats := p.Stats()
	fmt.Printf("Active Workers: %d\n", stats.ActiveWorkers)
	fmt.Printf("Queued Tasks: %d\n", stats.QueuedTasks)
	fmt.Printf("Completed Tasks: %d\n", stats.CompletedTasks)
	fmt.Printf("Failed Tasks: %d\n", stats.FailedTasks)
	fmt.Printf("Average Task Duration: %v\n", stats.AvgTaskDuration)
	fmt.Printf("Throughput (TPS): %.2f\n", stats.ThroughputTPS)
	fmt.Printf("Memory Usage: %d bytes\n", stats.MemoryUsage)
	fmt.Printf("GC Count: %d\n", stats.GCCount)

	// 检查协程池状态
	fmt.Printf("\nPool Status:\n")
	fmt.Printf("Is Running: %v\n", p.IsRunning())
	fmt.Printf("Is Shutdown: %v\n", p.IsShutdown())
	fmt.Printf("Is Closed: %v\n", p.IsClosed())

	// 优雅关闭协程池
	fmt.Println("\n9. Shutting down pool...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.Shutdown(shutdownCtx); err != nil {
		log.Printf("Failed to shutdown pool: %v", err)
	} else {
		fmt.Println("Pool shutdown successfully")
	}

	// 最终统计
	finalStats := p.Stats()
	fmt.Printf("\nFinal Statistics:\n")
	fmt.Printf("Total Completed Tasks: %d\n", finalStats.CompletedTasks)
	fmt.Printf("Total Failed Tasks: %d\n", finalStats.FailedTasks)

	fmt.Println("\n=== Basic Usage Example Completed ===")
}
