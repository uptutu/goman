package main

import (
	"context"
	"fmt"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

type testTask struct {
	id       int
	duration time.Duration
}

func (t *testTask) Execute(ctx context.Context) (any, error) {
	time.Sleep(t.duration)
	return fmt.Sprintf("Task %d completed", t.id), nil
}

func (t *testTask) Priority() int {
	return 1
}

func main() {
	// 创建配置
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(10).
		WithMetrics(true).
		WithMetricsInterval(1 * time.Second).
		Build()
	if err != nil {
		panic(err)
	}

	// 创建协程池
	p, err := pool.NewPool(config)
	if err != nil {
		panic(err)
	}

	fmt.Println("Pool created successfully")

	// 提交一些任务
	for i := 0; i < 10; i++ {
		task := &testTask{
			id:       i,
			duration: 100 * time.Millisecond,
		}
		if err := p.Submit(task); err != nil {
			fmt.Printf("Failed to submit task %d: %v\n", i, err)
		}
	}

	fmt.Println("Tasks submitted")

	// 等待任务完成
	time.Sleep(2 * time.Second)

	// 获取统计信息
	stats := p.Stats()
	fmt.Printf("Pool Stats:\n")
	fmt.Printf("  Active Workers: %d\n", stats.ActiveWorkers)
	fmt.Printf("  Queued Tasks: %d\n", stats.QueuedTasks)
	fmt.Printf("  Completed Tasks: %d\n", stats.CompletedTasks)
	fmt.Printf("  Failed Tasks: %d\n", stats.FailedTasks)
	fmt.Printf("  Avg Task Duration: %v\n", stats.AvgTaskDuration)
	fmt.Printf("  Throughput TPS: %.2f\n", stats.ThroughputTPS)
	fmt.Printf("  Memory Usage: %d bytes\n", stats.MemoryUsage)
	fmt.Printf("  GC Count: %d\n", stats.GCCount)

	// 关闭协程池
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Shutdown(ctx); err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	fmt.Println("Pool shutdown completed")
}
