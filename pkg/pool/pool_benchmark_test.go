package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkTask 基准测试任务
type BenchmarkTask struct {
	id       int64
	workload time.Duration
	priority int
	result   any
	executed int32
}

func NewBenchmarkTask(id int64, workload time.Duration) *BenchmarkTask {
	return &BenchmarkTask{
		id:       id,
		workload: workload,
		priority: PriorityNormal,
	}
}

func (t *BenchmarkTask) Execute(ctx context.Context) (any, error) {
	atomic.StoreInt32(&t.executed, 1)

	if t.workload > 0 {
		select {
		case <-time.After(t.workload):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return fmt.Sprintf("task-%d-result", t.id), nil
}

func (t *BenchmarkTask) Priority() int {
	return t.priority
}

func (t *BenchmarkTask) SetPriority(priority int) {
	t.priority = priority
}

func (t *BenchmarkTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

// BenchmarkPoolSubmit_DifferentWorkerCounts 测试不同协程数的性能表现
func BenchmarkPoolSubmit_DifferentWorkerCounts(b *testing.B) {
	workerCounts := []int{1, 2, 4, 8, 16, 32}

	for _, workerCount := range workerCounts {
		b.Run(fmt.Sprintf("Workers%d", workerCount), func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(workerCount).
				WithQueueSize(workerCount * 100).
				WithMetrics(false).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var taskID int64
				for pb.Next() {
					id := atomic.AddInt64(&taskID, 1)
					task := NewBenchmarkTask(id, 0) // 无工作负载
					pool.Submit(task)
				}
			})
		})
	}
}

// BenchmarkPoolSubmit_DifferentQueueSizes 测试不同队列大小的性能表现
func BenchmarkPoolSubmit_DifferentQueueSizes(b *testing.B) {
	queueSizes := []int{100, 500, 1000, 5000, 10000}

	for _, queueSize := range queueSizes {
		b.Run(fmt.Sprintf("Queue%d", queueSize), func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(8).
				WithQueueSize(queueSize).
				WithMetrics(false).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var taskID int64
				for pb.Next() {
					id := atomic.AddInt64(&taskID, 1)
					task := NewBenchmarkTask(id, 0)
					pool.Submit(task)
				}
			})
		})
	}
}

// BenchmarkPoolSubmit_WithObjectPool 对比使用对象池前后的性能差异
func BenchmarkPoolSubmit_WithObjectPool(b *testing.B) {
	testCases := []struct {
		name           string
		objectPoolSize int
		preAlloc       bool
	}{
		{"NoObjectPool", 0, false},
		{"SmallObjectPool", 50, false},
		{"DefaultObjectPool", DefaultObjectPoolSize, false},
		{"LargeObjectPool", 500, false},
		{"PreAllocObjectPool", DefaultObjectPoolSize, true},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(8).
				WithQueueSize(1000).
				WithObjectPoolSize(tc.objectPoolSize).
				WithPreAlloc(tc.preAlloc).
				WithMetrics(false).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var taskID int64
				for pb.Next() {
					id := atomic.AddInt64(&taskID, 1)
					task := NewBenchmarkTask(id, 0)
					pool.Submit(task)
				}
			})
		})
	}
}

// BenchmarkPoolHighConcurrency 测试高并发场景下的稳定性
func BenchmarkPoolHighConcurrency(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU() * 2).
		WithQueueSize(50000).
		WithMetrics(false).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	const numGoroutines = 100

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		var successCount int64

		for j := 0; j < numGoroutines; j++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for k := 0; k < 10; k++ {
					taskID := int64(goroutineID*10 + k)
					task := NewBenchmarkTask(taskID, time.Microsecond*time.Duration(k%10))

					err := pool.Submit(task)
					if err == nil {
						atomic.AddInt64(&successCount, 1)
					}
				}
			}(j)
		}

		wg.Wait()

		expectedTasks := int64(numGoroutines * 10)
		if successCount < expectedTasks*8/10 { // 至少80%成功
			b.Errorf("High concurrency test failed: expected at least %d successful tasks, got %d",
				expectedTasks*8/10, successCount)
		}
	}
}

// BenchmarkPoolObjectPoolDetailedComparison 详细对象池性能对比测试
func BenchmarkPoolObjectPoolDetailedComparison(b *testing.B) {
	testCases := []struct {
		name           string
		objectPoolSize int
		preAlloc       bool
		description    string
	}{
		{"NoPool", 0, false, "无对象池"},
		{"Pool_10", 10, false, "对象池大小10"},
		{"Pool_50", 50, false, "对象池大小50"},
		{"Pool_100", 100, false, "对象池大小100"},
		{"Pool_200", 200, false, "对象池大小200"},
		{"Pool_500", 500, false, "对象池大小500"},
		{"Pool_1000", 1000, false, "对象池大小1000"},
		{"PreAlloc_50", 50, true, "预分配对象池50"},
		{"PreAlloc_100", 100, true, "预分配对象池100"},
		{"PreAlloc_200", 200, true, "预分配对象池200"},
		{"PreAlloc_500", 500, true, "预分配对象池500"},
		{"PreAlloc_1000", 1000, true, "预分配对象池1000"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(8).
				WithQueueSize(2000).
				WithObjectPoolSize(tc.objectPoolSize).
				WithPreAlloc(tc.preAlloc).
				WithMetrics(false).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			// 预热对象池
			if tc.objectPoolSize > 0 {
				for i := 0; i < tc.objectPoolSize; i++ {
					task := NewBenchmarkTask(int64(i), 0)
					pool.Submit(task)
				}
				time.Sleep(time.Millisecond * 100)
			}

			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			b.ResetTimer()
			start := time.Now()

			// 并行提交任务测试对象池效率
			b.RunParallel(func(pb *testing.PB) {
				var taskID int64
				for pb.Next() {
					id := atomic.AddInt64(&taskID, 1)
					task := NewBenchmarkTask(id, 0)
					pool.Submit(task)
				}
			})

			duration := time.Since(start)
			runtime.GC()
			runtime.ReadMemStats(&m2)

			// 计算性能指标
			throughput := float64(b.N) / duration.Seconds()
			allocDiff := m2.TotalAlloc - m1.TotalAlloc
			mallocDiff := m2.Mallocs - m1.Mallocs
			gcDiff := m2.NumGC - m1.NumGC

			b.ReportMetric(throughput, "tasks/sec")
			b.ReportMetric(float64(allocDiff)/float64(b.N), "bytes/task")
			b.ReportMetric(float64(mallocDiff)/float64(b.N), "allocs/task")
			b.ReportMetric(float64(gcDiff), "gc_count")

			b.Logf("%s: %.2f tasks/sec, %.2f bytes/task, %.2f allocs/task, %d GC",
				tc.description, throughput, float64(allocDiff)/float64(b.N),
				float64(mallocDiff)/float64(b.N), gcDiff)
		})
	}
}

// BenchmarkPoolExtremeConcurrency 极端并发场景稳定性测试
func BenchmarkPoolExtremeConcurrency(b *testing.B) {
	scenarios := []struct {
		name         string
		goroutines   int
		tasksPerGor  int
		workers      int
		queueSize    int
		taskDuration time.Duration
	}{
		{"Burst_100G_10T", 100, 10, 16, 5000, 0},
		{"Burst_500G_5T", 500, 5, 32, 10000, 0},
		{"Burst_1000G_2T", 1000, 2, 64, 20000, 0},
		{"Sustained_50G_100T", 50, 100, 16, 10000, time.Microsecond * 10},
		{"Sustained_100G_50T", 100, 50, 32, 15000, time.Microsecond * 10},
		{"Mixed_200G_25T", 200, 25, 32, 20000, time.Microsecond * 5},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(scenario.workers).
				WithQueueSize(scenario.queueSize).
				WithObjectPoolSize(DefaultObjectPoolSize).
				WithPreAlloc(true).
				WithMetrics(true).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				var successCount, errorCount int64
				var totalLatency int64

				start := time.Now()

				// 启动大量协程并发提交任务
				for j := 0; j < scenario.goroutines; j++ {
					wg.Add(1)
					go func(goroutineID int) {
						defer wg.Done()

						for k := 0; k < scenario.tasksPerGor; k++ {
							taskStart := time.Now()
							taskID := int64(goroutineID*scenario.tasksPerGor + k)
							task := NewBenchmarkTask(taskID, scenario.taskDuration)

							err := pool.Submit(task)
							submitLatency := time.Since(taskStart)
							atomic.AddInt64(&totalLatency, submitLatency.Nanoseconds())

							if err != nil {
								atomic.AddInt64(&errorCount, 1)
							} else {
								atomic.AddInt64(&successCount, 1)
							}
						}
					}(j)
				}

				wg.Wait()
				totalDuration := time.Since(start)

				// 等待任务完成
				time.Sleep(time.Millisecond * 500)

				// 计算性能指标
				totalTasks := int64(scenario.goroutines * scenario.tasksPerGor)
				successRate := float64(successCount) / float64(totalTasks) * 100
				throughput := float64(successCount) / totalDuration.Seconds()
				avgLatency := float64(totalLatency) / float64(totalTasks) / 1e6 // 转换为毫秒

				stats := pool.Stats()

				b.ReportMetric(successRate, "success_rate_%")
				b.ReportMetric(throughput, "tasks/sec")
				b.ReportMetric(avgLatency, "avg_latency_ms")
				b.ReportMetric(float64(errorCount), "errors")
				b.ReportMetric(float64(stats.ActiveWorkers), "active_workers")
				b.ReportMetric(float64(stats.QueuedTasks), "queued_tasks")

				// 验证稳定性
				if successRate < 70 {
					b.Errorf("Extreme concurrency test failed: success rate %.2f%% too low", successRate)
				}

				b.Logf("场景: %s, 成功率: %.2f%%, 吞吐量: %.2f tasks/sec, 平均延迟: %.2f ms, 错误: %d",
					scenario.name, successRate, throughput, avgLatency, errorCount)
			}
		})
	}
}

// BenchmarkPoolPerformanceRegression 性能回归测试
func BenchmarkPoolPerformanceRegression(b *testing.B) {
	// 标准配置的性能基准测试，用于检测性能回归
	standardConfigs := []struct {
		name        string
		workers     int
		queueSize   int
		objectPool  int
		preAlloc    bool
		description string
	}{
		{"Standard_Small", 4, 1000, 100, false, "小型标准配置"},
		{"Standard_Medium", 8, 2000, 200, false, "中型标准配置"},
		{"Standard_Large", 16, 5000, 500, true, "大型标准配置"},
		{"Standard_Optimized", runtime.NumCPU(), runtime.NumCPU() * 1000, DefaultObjectPoolSize, true, "优化配置"},
	}

	for _, config := range standardConfigs {
		b.Run(config.name, func(b *testing.B) {
			poolConfig, err := NewConfigBuilder().
				WithWorkerCount(config.workers).
				WithQueueSize(config.queueSize).
				WithObjectPoolSize(config.objectPool).
				WithPreAlloc(config.preAlloc).
				WithMetrics(true).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(poolConfig)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			// 预热
			for i := 0; i < config.workers*10; i++ {
				task := NewBenchmarkTask(int64(i), 0)
				pool.Submit(task)
			}
			time.Sleep(time.Millisecond * 100)

			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			b.ResetTimer()
			start := time.Now()

			// 标准化的性能测试
			const testTasks = 10000
			var completed int64

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if atomic.LoadInt64(&completed) >= testTasks {
						return
					}

					taskID := atomic.AddInt64(&completed, 1)
					if taskID > testTasks {
						return
					}

					task := NewBenchmarkTask(taskID, time.Microsecond*10)
					pool.Submit(task)
				}
			})

			duration := time.Since(start)
			runtime.GC()
			runtime.ReadMemStats(&m2)

			stats := pool.Stats()
			throughput := float64(completed) / duration.Seconds()
			allocDiff := m2.TotalAlloc - m1.TotalAlloc
			efficiency := throughput / float64(config.workers)

			// 性能基准指标
			b.ReportMetric(throughput, "tasks/sec")
			b.ReportMetric(efficiency, "tasks/sec/worker")
			b.ReportMetric(float64(allocDiff)/float64(completed), "bytes/task")
			b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
			b.ReportMetric(float64(stats.FailedTasks), "failed_tasks")

			// 性能阈值检查（可根据实际情况调整）
			minThroughput := float64(config.workers) * 100 // 每个worker至少100 tasks/sec
			if throughput < minThroughput {
				b.Errorf("Performance regression detected: throughput %.2f < %.2f", throughput, minThroughput)
			}

			b.Logf("%s: %.2f tasks/sec, %.2f tasks/sec/worker, %.2f bytes/task",
				config.description, throughput, efficiency, float64(allocDiff)/float64(completed))
		})
	}
}

// BenchmarkPoolResourceUtilization 资源利用率测试
func BenchmarkPoolResourceUtilization(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU()).
		WithQueueSize(10000).
		WithObjectPoolSize(DefaultObjectPoolSize).
		WithPreAlloc(true).
		WithMetrics(true).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 监控资源使用情况
	var maxHeapSize, maxGoroutines uint64
	var gcCount uint32

	// 启动资源监控协程
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)

				if m.HeapAlloc > maxHeapSize {
					maxHeapSize = m.HeapAlloc
				}

				goroutines := uint64(runtime.NumGoroutine())
				if goroutines > maxGoroutines {
					maxGoroutines = goroutines
				}

				gcCount = m.NumGC

			case <-done:
				return
			}
		}
	}()

	b.ResetTimer()

	// 高强度任务提交测试资源利用率
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		const batchSize = 1000

		for batch := 0; batch < 10; batch++ {
			wg.Add(1)
			go func(batchID int) {
				defer wg.Done()
				for j := 0; j < batchSize; j++ {
					taskID := int64(batchID*batchSize + j)
					task := NewBenchmarkTask(taskID, time.Microsecond*50)
					pool.Submit(task)
				}
			}(batch)
		}

		wg.Wait()
		time.Sleep(time.Millisecond * 100) // 等待任务完成
	}

	close(done)

	stats := pool.Stats()

	b.ReportMetric(float64(maxHeapSize)/1024/1024, "max_heap_mb")
	b.ReportMetric(float64(maxGoroutines), "max_goroutines")
	b.ReportMetric(float64(gcCount), "gc_count")
	b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
	b.ReportMetric(float64(stats.ActiveWorkers), "active_workers")

	b.Logf("资源利用率 - 最大堆内存: %.2f MB, 最大协程数: %d, GC次数: %d, 完成任务: %d",
		float64(maxHeapSize)/1024/1024, maxGoroutines, gcCount, stats.CompletedTasks)
}

// BenchmarkPoolLongRunningStability 长时间运行稳定性测试
func BenchmarkPoolLongRunningStability(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping long running test in short mode")
	}

	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(2000).
		WithObjectPoolSize(DefaultObjectPoolSize).
		WithPreAlloc(true).
		WithMetrics(true).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	b.ResetTimer()

	// 长时间运行测试（每次迭代运行5秒）
	for i := 0; i < b.N; i++ {
		var totalTasks, successTasks, errorTasks int64
		var initialMem, finalMem runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&initialMem)

		start := time.Now()
		end := start.Add(time.Second * 5) // 每次迭代运行5秒

		// 持续提交任务
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ { // 4个提交协程
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				var taskID int64

				for time.Now().Before(end) {
					id := atomic.AddInt64(&taskID, 1)
					globalTaskID := int64(goroutineID)*10000 + id

					// 混合工作负载
					var workload time.Duration
					switch globalTaskID % 4 {
					case 0:
						workload = 0 // 快速任务
					case 1:
						workload = time.Microsecond * 10 // 短任务
					case 2:
						workload = time.Microsecond * 100 // 中等任务
					case 3:
						workload = time.Millisecond // 长任务
					}

					task := NewBenchmarkTask(globalTaskID, workload)
					err := pool.Submit(task)

					atomic.AddInt64(&totalTasks, 1)
					if err != nil {
						atomic.AddInt64(&errorTasks, 1)
					} else {
						atomic.AddInt64(&successTasks, 1)
					}

					// 控制提交速率
					if id%100 == 0 {
						time.Sleep(time.Microsecond * 100)
					}
				}
			}(j)
		}

		wg.Wait()

		// 等待任务完成
		time.Sleep(time.Second)

		runtime.GC()
		runtime.ReadMemStats(&finalMem)

		duration := time.Since(start)
		stats := pool.Stats()

		// 计算性能指标
		successRate := float64(successTasks) / float64(totalTasks) * 100
		throughput := float64(successTasks) / duration.Seconds()
		memoryGrowth := int64(finalMem.HeapAlloc) - int64(initialMem.HeapAlloc)
		gcCount := finalMem.NumGC - initialMem.NumGC

		b.ReportMetric(successRate, "success_rate_%")
		b.ReportMetric(throughput, "tasks/sec")
		b.ReportMetric(float64(memoryGrowth)/1024/1024, "memory_growth_mb")
		b.ReportMetric(float64(gcCount), "gc_count")
		b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
		b.ReportMetric(float64(errorTasks), "error_tasks")

		// 稳定性检查
		if successRate < 95 {
			b.Errorf("Long running stability test failed: success rate %.2f%% too low", successRate)
		}

		if memoryGrowth > 100*1024*1024 { // 100MB
			b.Errorf("Memory leak detected: growth %d MB", memoryGrowth/1024/1024)
		}

		b.Logf("长时间运行 - 成功率: %.2f%%, 吞吐量: %.2f tasks/sec, 内存增长: %.2f MB, GC: %d",
			successRate, throughput, float64(memoryGrowth)/1024/1024, gcCount)
	}
}

// BenchmarkPoolObjectPoolMemoryImpact 对象池内存影响详细测试
func BenchmarkPoolObjectPoolMemoryImpact(b *testing.B) {
	scenarios := []struct {
		name           string
		objectPoolSize int
		preAlloc       bool
		taskCount      int
		description    string
	}{
		{"NoPool_1K", 0, false, 1000, "无对象池-1K任务"},
		{"NoPool_10K", 0, false, 10000, "无对象池-10K任务"},
		{"NoPool_100K", 0, false, 100000, "无对象池-100K任务"},
		{"Pool50_1K", 50, false, 1000, "对象池50-1K任务"},
		{"Pool50_10K", 50, false, 10000, "对象池50-10K任务"},
		{"Pool50_100K", 50, false, 100000, "对象池50-100K任务"},
		{"Pool200_1K", 200, false, 1000, "对象池200-1K任务"},
		{"Pool200_10K", 200, false, 10000, "对象池200-10K任务"},
		{"Pool200_100K", 200, false, 100000, "对象池200-100K任务"},
		{"PreAlloc200_1K", 200, true, 1000, "预分配200-1K任务"},
		{"PreAlloc200_10K", 200, true, 10000, "预分配200-10K任务"},
		{"PreAlloc200_100K", 200, true, 100000, "预分配200-100K任务"},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(8).
				WithQueueSize(scenario.taskCount + 1000).
				WithObjectPoolSize(scenario.objectPoolSize).
				WithPreAlloc(scenario.preAlloc).
				WithMetrics(false).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var m1, m2 runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&m1)

				start := time.Now()

				// 批量提交任务
				var wg sync.WaitGroup
				batchSize := scenario.taskCount / 10
				for batch := 0; batch < 10; batch++ {
					wg.Add(1)
					go func(batchID int) {
						defer wg.Done()
						for j := 0; j < batchSize; j++ {
							taskID := int64(batchID*batchSize + j)
							task := NewBenchmarkTask(taskID, 0)
							pool.Submit(task)
						}
					}(batch)
				}

				wg.Wait()
				duration := time.Since(start)

				// 等待任务完成
				time.Sleep(time.Millisecond * 500)

				runtime.GC()
				runtime.ReadMemStats(&m2)

				// 计算内存指标
				allocDiff := m2.TotalAlloc - m1.TotalAlloc
				mallocDiff := m2.Mallocs - m1.Mallocs
				gcDiff := m2.NumGC - m1.NumGC
				throughput := float64(scenario.taskCount) / duration.Seconds()

				b.ReportMetric(throughput, "tasks/sec")
				b.ReportMetric(float64(allocDiff)/1024/1024, "total_alloc_mb")
				b.ReportMetric(float64(allocDiff)/float64(scenario.taskCount), "bytes/task")
				b.ReportMetric(float64(mallocDiff)/float64(scenario.taskCount), "allocs/task")
				b.ReportMetric(float64(gcDiff), "gc_count")

				b.Logf("%s: %.2f tasks/sec, %.2f MB总分配, %.2f bytes/task, %.2f allocs/task, %d GC",
					scenario.description, throughput, float64(allocDiff)/1024/1024,
					float64(allocDiff)/float64(scenario.taskCount),
					float64(mallocDiff)/float64(scenario.taskCount), gcDiff)
			}
		})
	}
}

// BenchmarkPoolScalabilityTest 可扩展性测试
func BenchmarkPoolScalabilityTest(b *testing.B) {
	// 测试不同协程数和队列大小组合的可扩展性
	testMatrix := []struct {
		workers   int
		queueSize int
		name      string
	}{
		{1, 100, "1W_100Q"},
		{1, 1000, "1W_1000Q"},
		{2, 200, "2W_200Q"},
		{2, 2000, "2W_2000Q"},
		{4, 400, "4W_400Q"},
		{4, 4000, "4W_4000Q"},
		{8, 800, "8W_800Q"},
		{8, 8000, "8W_8000Q"},
		{16, 1600, "16W_1600Q"},
		{16, 16000, "16W_16000Q"},
		{32, 3200, "32W_3200Q"},
		{32, 32000, "32W_32000Q"},
	}

	for _, matrix := range testMatrix {
		b.Run(matrix.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(matrix.workers).
				WithQueueSize(matrix.queueSize).
				WithObjectPoolSize(DefaultObjectPoolSize).
				WithPreAlloc(true).
				WithMetrics(true).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()

			// 可扩展性测试：固定每个worker的任务数
			tasksPerWorker := 1000
			totalTasks := matrix.workers * tasksPerWorker

			start := time.Now()
			var completed int64

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if atomic.LoadInt64(&completed) >= int64(totalTasks) {
						return
					}

					taskID := atomic.AddInt64(&completed, 1)
					if taskID > int64(totalTasks) {
						return
					}

					task := NewBenchmarkTask(taskID, time.Microsecond*10)
					pool.Submit(task)
				}
			})

			duration := time.Since(start)
			stats := pool.Stats()

			throughput := float64(completed) / duration.Seconds()
			efficiency := throughput / float64(matrix.workers)
			utilization := float64(stats.ActiveWorkers) / float64(matrix.workers) * 100

			b.ReportMetric(throughput, "tasks/sec")
			b.ReportMetric(efficiency, "tasks/sec/worker")
			b.ReportMetric(utilization, "worker_utilization_%")
			b.ReportMetric(float64(stats.QueuedTasks), "avg_queued_tasks")

			b.Logf("Workers=%d, Queue=%d: %.2f tasks/sec, %.2f efficiency, %.2f%% utilization",
				matrix.workers, matrix.queueSize, throughput, efficiency, utilization)
		})
	}
}

// BenchmarkPoolStressTest 压力测试
func BenchmarkPoolStressTest(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping stress test in short mode")
	}

	stressScenarios := []struct {
		name         string
		duration     time.Duration
		goroutines   int
		submitRate   time.Duration // 每个协程的提交间隔
		taskDuration time.Duration
		workers      int
		queueSize    int
		description  string
	}{
		{"Burst_High", time.Second * 10, 100, 0, 0, 16, 10000, "突发高负载"},
		{"Burst_Medium", time.Second * 15, 50, time.Microsecond * 100, time.Microsecond * 10, 8, 5000, "突发中等负载"},
		{"Sustained_High", time.Second * 20, 20, time.Millisecond, time.Microsecond * 100, 8, 2000, "持续高负载"},
		{"Sustained_Low", time.Second * 30, 10, time.Millisecond * 10, time.Millisecond, 4, 1000, "持续低负载"},
		{"Mixed_Load", time.Second * 25, 30, time.Microsecond * 500, time.Microsecond * 50, 12, 8000, "混合负载"},
	}

	for _, scenario := range stressScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			config, err := NewConfigBuilder().
				WithWorkerCount(scenario.workers).
				WithQueueSize(scenario.queueSize).
				WithObjectPoolSize(DefaultObjectPoolSize).
				WithPreAlloc(true).
				WithMetrics(true).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var totalSubmitted, totalSuccess, totalErrors int64
				var maxQueueSize, maxActiveWorkers int64

				start := time.Now()
				end := start.Add(scenario.duration)

				// 启动监控协程
				monitorDone := make(chan bool)
				go func() {
					ticker := time.NewTicker(time.Millisecond * 100)
					defer ticker.Stop()

					for {
						select {
						case <-ticker.C:
							stats := pool.Stats()
							if stats.QueuedTasks > maxQueueSize {
								maxQueueSize = stats.QueuedTasks
							}
							if stats.ActiveWorkers > maxActiveWorkers {
								maxActiveWorkers = stats.ActiveWorkers
							}
						case <-monitorDone:
							return
						}
					}
				}()

				// 启动压力测试协程
				var wg sync.WaitGroup
				for j := 0; j < scenario.goroutines; j++ {
					wg.Add(1)
					go func(goroutineID int) {
						defer wg.Done()
						var taskID int64

						for time.Now().Before(end) {
							id := atomic.AddInt64(&taskID, 1)
							globalTaskID := int64(goroutineID)*100000 + id

							task := NewBenchmarkTask(globalTaskID, scenario.taskDuration)
							err := pool.Submit(task)

							atomic.AddInt64(&totalSubmitted, 1)
							if err != nil {
								atomic.AddInt64(&totalErrors, 1)
							} else {
								atomic.AddInt64(&totalSuccess, 1)
							}

							if scenario.submitRate > 0 {
								time.Sleep(scenario.submitRate)
							}
						}
					}(j)
				}

				wg.Wait()
				close(monitorDone)

				// 等待任务完成
				time.Sleep(time.Second * 2)

				actualDuration := time.Since(start)
				stats := pool.Stats()

				successRate := float64(totalSuccess) / float64(totalSubmitted) * 100
				throughput := float64(totalSuccess) / actualDuration.Seconds()
				errorRate := float64(totalErrors) / float64(totalSubmitted) * 100

				b.ReportMetric(successRate, "success_rate_%")
				b.ReportMetric(throughput, "tasks/sec")
				b.ReportMetric(errorRate, "error_rate_%")
				b.ReportMetric(float64(maxQueueSize), "max_queue_size")
				b.ReportMetric(float64(maxActiveWorkers), "max_active_workers")
				b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")

				// 压力测试通过标准
				if successRate < 80 {
					b.Errorf("Stress test failed: success rate %.2f%% too low", successRate)
				}
				if errorRate > 20 {
					b.Errorf("Stress test failed: error rate %.2f%% too high", errorRate)
				}

				b.Logf("%s: 成功率=%.2f%%, 吞吐量=%.2f tasks/sec, 错误率=%.2f%%, 最大队列=%d",
					scenario.description, successRate, throughput, errorRate, maxQueueSize)
			}
		})
	}
}

// BenchmarkPoolComprehensiveComparison 综合性能对比测试
func BenchmarkPoolComprehensiveComparison(b *testing.B) {
	// 对比不同配置组合的综合性能
	configurations := []struct {
		name        string
		workers     int
		queueSize   int
		objectPool  int
		preAlloc    bool
		description string
	}{
		{"Minimal", 1, 100, 0, false, "最小配置"},
		{"Small", 2, 500, 50, false, "小型配置"},
		{"Medium", 4, 1000, 100, false, "中型配置"},
		{"Large", 8, 2000, 200, false, "大型配置"},
		{"XLarge", 16, 5000, 500, false, "超大配置"},
		{"Optimized_Small", 2, 500, 50, true, "优化小型"},
		{"Optimized_Medium", 4, 1000, 100, true, "优化中型"},
		{"Optimized_Large", 8, 2000, 200, true, "优化大型"},
		{"Optimized_XLarge", 16, 5000, 500, true, "优化超大"},
		{"CPU_Matched", runtime.NumCPU(), runtime.NumCPU() * 500, DefaultObjectPoolSize, true, "CPU匹配"},
	}

	for _, config := range configurations {
		b.Run(config.name, func(b *testing.B) {
			poolConfig, err := NewConfigBuilder().
				WithWorkerCount(config.workers).
				WithQueueSize(config.queueSize).
				WithObjectPoolSize(config.objectPool).
				WithPreAlloc(config.preAlloc).
				WithMetrics(true).
				Build()
			if err != nil {
				b.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(poolConfig)
			if err != nil {
				b.Fatalf("Failed to create pool: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				pool.Shutdown(ctx)
			}()

			// 预热
			for i := 0; i < config.workers*5; i++ {
				task := NewBenchmarkTask(int64(i), 0)
				pool.Submit(task)
			}
			time.Sleep(time.Millisecond * 100)

			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			b.ResetTimer()
			start := time.Now()

			// 混合负载测试
			var wg sync.WaitGroup
			const goroutines = 10
			tasksPerGoroutine := b.N / goroutines

			for i := 0; i < goroutines; i++ {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()
					for j := 0; j < tasksPerGoroutine; j++ {
						taskID := int64(goroutineID*tasksPerGoroutine + j)

						// 混合工作负载
						var workload time.Duration
						switch taskID % 5 {
						case 0, 1, 2:
							workload = 0 // 60% 快速任务
						case 3:
							workload = time.Microsecond * 10 // 20% 短任务
						case 4:
							workload = time.Microsecond * 100 // 20% 中等任务
						}

						task := NewBenchmarkTask(taskID, workload)
						pool.Submit(task)
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(start)

			// 等待任务完成
			time.Sleep(time.Millisecond * 200)

			runtime.GC()
			runtime.ReadMemStats(&m2)

			stats := pool.Stats()
			throughput := float64(b.N) / duration.Seconds()
			efficiency := throughput / float64(config.workers)
			allocDiff := m2.TotalAlloc - m1.TotalAlloc
			memoryPerTask := float64(allocDiff) / float64(b.N)

			b.ReportMetric(throughput, "tasks/sec")
			b.ReportMetric(efficiency, "tasks/sec/worker")
			b.ReportMetric(memoryPerTask, "bytes/task")
			b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
			b.ReportMetric(float64(m2.NumGC-m1.NumGC), "gc_count")

			b.Logf("%s: 吞吐量=%.2f tasks/sec, 效率=%.2f tasks/sec/worker, 内存=%.2f bytes/task, GC=%d",
				config.description, throughput, efficiency, memoryPerTask, m2.NumGC-m1.NumGC)
		})
	}
}
