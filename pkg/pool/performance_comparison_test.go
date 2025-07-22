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

// PerformanceTestSuite 性能测试套件
type PerformanceTestSuite struct {
	name        string
	description string
	scenarios   []PerformanceScenario
}

// PerformanceScenario 性能测试场景
type PerformanceScenario struct {
	name        string
	workers     int
	queueSize   int
	objectPool  int
	preAlloc    bool
	taskCount   int
	workload    time.Duration
	concurrency int
	expectedTPS float64 // 期望的每秒任务处理数
	maxMemoryMB float64 // 最大内存使用MB
}

// BenchmarkPoolPerformanceComparison 全面性能对比测试
func BenchmarkPoolPerformanceComparison(b *testing.B) {
	suites := []PerformanceTestSuite{
		{
			name:        "WorkerScaling",
			description: "工作协程数量扩展性能测试",
			scenarios: []PerformanceScenario{
				{"1Worker", 1, 1000, 100, false, 5000, 0, 10, 1000, 10},
				{"2Workers", 2, 1000, 100, false, 5000, 0, 10, 2000, 10},
				{"4Workers", 4, 1000, 100, false, 5000, 0, 10, 4000, 10},
				{"8Workers", 8, 1000, 100, false, 5000, 0, 10, 8000, 10},
				{"16Workers", 16, 1000, 100, false, 5000, 0, 10, 15000, 15},
				{"32Workers", 32, 1000, 100, false, 5000, 0, 10, 25000, 20},
			},
		},
		{
			name:        "QueueSizeImpact",
			description: "队列大小对性能的影响",
			scenarios: []PerformanceScenario{
				{"Queue100", 8, 100, 100, false, 5000, 0, 20, 8000, 10},
				{"Queue500", 8, 500, 100, false, 5000, 0, 20, 8000, 10},
				{"Queue1000", 8, 1000, 100, false, 5000, 0, 20, 8000, 10},
				{"Queue2000", 8, 2000, 100, false, 5000, 0, 20, 8000, 10},
				{"Queue5000", 8, 5000, 100, false, 5000, 0, 20, 8000, 10},
				{"Queue10000", 8, 10000, 100, false, 5000, 0, 20, 8000, 10},
			},
		},
		{
			name:        "ObjectPoolEfficiency",
			description: "对象池效率对比测试",
			scenarios: []PerformanceScenario{
				{"NoObjectPool", 8, 2000, 0, false, 10000, 0, 20, 8000, 20},
				{"SmallPool50", 8, 2000, 50, false, 10000, 0, 20, 8000, 15},
				{"MediumPool100", 8, 2000, 100, false, 10000, 0, 20, 8000, 12},
				{"LargePool500", 8, 2000, 500, false, 10000, 0, 20, 8000, 10},
				{"PreAllocPool100", 8, 2000, 100, true, 10000, 0, 20, 8000, 8},
				{"PreAllocPool500", 8, 2000, 500, true, 10000, 0, 20, 8000, 8},
			},
		},
		{
			name:        "ConcurrencyStress",
			description: "高并发压力测试",
			scenarios: []PerformanceScenario{
				{"LowConcurrency", 8, 2000, 200, true, 5000, time.Microsecond, 10, 8000, 15},
				{"MediumConcurrency", 8, 2000, 200, true, 5000, time.Microsecond, 50, 8000, 15},
				{"HighConcurrency", 8, 2000, 200, true, 5000, time.Microsecond, 100, 8000, 15},
				{"VeryHighConcurrency", 16, 5000, 500, true, 5000, time.Microsecond, 200, 15000, 25},
				{"ExtremeConcurrency", 32, 10000, 1000, true, 5000, time.Microsecond, 500, 25000, 50},
			},
		},
	}

	for _, suite := range suites {
		b.Run(suite.name, func(b *testing.B) {
			runPerformanceTestSuite(b, suite)
		})
	}
}

// runPerformanceTestSuite 运行性能测试套件
func runPerformanceTestSuite(b *testing.B, suite PerformanceTestSuite) {
	b.Logf("开始执行性能测试套件: %s - %s", suite.name, suite.description)

	results := make([]PerformanceResult, 0, len(suite.scenarios))

	for _, scenario := range suite.scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			result := runPerformanceScenario(b, scenario)
			results = append(results, result)
		})
	}

	// 生成性能对比报告
	generatePerformanceReport(b, suite, results)
}

// PerformanceResult 性能测试结果
type PerformanceResult struct {
	scenario     PerformanceScenario
	throughput   float64
	avgLatency   time.Duration
	memoryUsage  float64
	successRate  float64
	gcCount      uint32
	allocsPerOp  float64
	passed       bool
	errorMessage string
}

// runPerformanceScenario 运行单个性能测试场景
func runPerformanceScenario(b *testing.B, scenario PerformanceScenario) PerformanceResult {
	config, err := NewConfigBuilder().
		WithWorkerCount(scenario.workers).
		WithQueueSize(scenario.queueSize).
		WithObjectPoolSize(scenario.objectPool).
		WithPreAlloc(scenario.preAlloc).
		WithMetrics(true).
		Build()
	if err != nil {
		return PerformanceResult{
			scenario:     scenario,
			passed:       false,
			errorMessage: fmt.Sprintf("配置创建失败: %v", err),
		}
	}

	pool, err := NewPool(config)
	if err != nil {
		return PerformanceResult{
			scenario:     scenario,
			passed:       false,
			errorMessage: fmt.Sprintf("协程池创建失败: %v", err),
		}
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 预热
	for i := 0; i < scenario.workers*10; i++ {
		task := NewBenchmarkTask(int64(i), 0)
		pool.Submit(task)
	}
	time.Sleep(time.Millisecond * 100)

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	start := time.Now()

	var successCount, errorCount int64
	var totalLatency int64

	// 执行测试
	var wg sync.WaitGroup
	tasksPerGoroutine := scenario.taskCount / scenario.concurrency

	for i := 0; i < scenario.concurrency; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				taskStart := time.Now()
				taskID := int64(goroutineID*tasksPerGoroutine + j)
				task := NewBenchmarkTask(taskID, scenario.workload)

				err := pool.Submit(task)
				latency := time.Since(taskStart)
				atomic.AddInt64(&totalLatency, int64(latency))

				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// 等待任务完成
	time.Sleep(time.Millisecond * 200)

	runtime.GC()
	runtime.ReadMemStats(&m2)

	// 计算性能指标
	totalTasks := successCount + errorCount
	throughput := float64(successCount) / duration.Seconds()
	avgLatency := time.Duration(totalLatency / totalTasks)
	successRate := float64(successCount) / float64(totalTasks) * 100
	memoryUsage := float64(m2.HeapAlloc-m1.HeapAlloc) / 1024 / 1024 // MB
	allocsPerOp := float64(m2.Mallocs-m1.Mallocs) / float64(totalTasks)

	// 验证性能指标
	passed := true
	var errorMessage string

	if throughput < scenario.expectedTPS*0.8 { // 允许20%的性能波动
		passed = false
		errorMessage = fmt.Sprintf("吞吐量不达标: %.2f < %.2f", throughput, scenario.expectedTPS*0.8)
	}

	if memoryUsage > scenario.maxMemoryMB {
		passed = false
		if errorMessage != "" {
			errorMessage += "; "
		}
		errorMessage += fmt.Sprintf("内存使用超标: %.2f MB > %.2f MB", memoryUsage, scenario.maxMemoryMB)
	}

	if successRate < 95 {
		passed = false
		if errorMessage != "" {
			errorMessage += "; "
		}
		errorMessage += fmt.Sprintf("成功率过低: %.2f%% < 95%%", successRate)
	}

	// 报告指标
	b.ReportMetric(throughput, "tasks/sec")
	b.ReportMetric(float64(avgLatency.Nanoseconds()), "avg_latency_ns")
	b.ReportMetric(memoryUsage, "memory_mb")
	b.ReportMetric(successRate, "success_rate_%")
	b.ReportMetric(allocsPerOp, "allocs/task")

	result := PerformanceResult{
		scenario:     scenario,
		throughput:   throughput,
		avgLatency:   avgLatency,
		memoryUsage:  memoryUsage,
		successRate:  successRate,
		gcCount:      m2.NumGC - m1.NumGC,
		allocsPerOp:  allocsPerOp,
		passed:       passed,
		errorMessage: errorMessage,
	}

	if !passed {
		b.Errorf("性能测试失败 [%s]: %s", scenario.name, errorMessage)
	}

	b.Logf("场景: %s, 吞吐量: %.2f tasks/sec, 延迟: %v, 内存: %.2f MB, 成功率: %.2f%%, 通过: %v",
		scenario.name, throughput, avgLatency, memoryUsage, successRate, passed)

	return result
}

// generatePerformanceReport 生成性能对比报告
func generatePerformanceReport(b *testing.B, suite PerformanceTestSuite, results []PerformanceResult) {
	b.Logf("\n=== 性能测试套件报告: %s ===", suite.name)
	b.Logf("描述: %s", suite.description)
	b.Logf("测试场景数量: %d", len(results))

	// 统计通过率
	passedCount := 0
	for _, result := range results {
		if result.passed {
			passedCount++
		}
	}
	passRate := float64(passedCount) / float64(len(results)) * 100
	b.Logf("总体通过率: %.2f%% (%d/%d)", passRate, passedCount, len(results))

	// 性能指标统计
	var maxThroughput, minThroughput, avgThroughput float64
	var maxMemory, minMemory, avgMemory float64
	var maxLatency, minLatency, avgLatency time.Duration

	if len(results) > 0 {
		maxThroughput = results[0].throughput
		minThroughput = results[0].throughput
		maxMemory = results[0].memoryUsage
		minMemory = results[0].memoryUsage
		maxLatency = results[0].avgLatency
		minLatency = results[0].avgLatency

		var totalThroughput, totalMemory float64
		var totalLatency time.Duration

		for _, result := range results {
			totalThroughput += result.throughput
			totalMemory += result.memoryUsage
			totalLatency += result.avgLatency

			if result.throughput > maxThroughput {
				maxThroughput = result.throughput
			}
			if result.throughput < minThroughput {
				minThroughput = result.throughput
			}
			if result.memoryUsage > maxMemory {
				maxMemory = result.memoryUsage
			}
			if result.memoryUsage < minMemory {
				minMemory = result.memoryUsage
			}
			if result.avgLatency > maxLatency {
				maxLatency = result.avgLatency
			}
			if result.avgLatency < minLatency {
				minLatency = result.avgLatency
			}
		}

		avgThroughput = totalThroughput / float64(len(results))
		avgMemory = totalMemory / float64(len(results))
		avgLatency = totalLatency / time.Duration(len(results))
	}

	b.Logf("\n性能指标统计:")
	b.Logf("吞吐量 - 最大: %.2f, 最小: %.2f, 平均: %.2f tasks/sec", maxThroughput, minThroughput, avgThroughput)
	b.Logf("内存使用 - 最大: %.2f, 最小: %.2f, 平均: %.2f MB", maxMemory, minMemory, avgMemory)
	b.Logf("平均延迟 - 最大: %v, 最小: %v, 平均: %v", maxLatency, minLatency, avgLatency)

	// 详细结果表格
	b.Logf("\n详细测试结果:")
	b.Logf("%-20s %-12s %-12s %-10s %-12s %-8s %s", "场景", "吞吐量", "延迟", "内存(MB)", "成功率", "通过", "错误信息")
	b.Logf("%s", "=====================================================================================================")

	for _, result := range results {
		status := "✓"
		if !result.passed {
			status = "✗"
		}
		b.Logf("%-20s %-12.2f %-12v %-10.2f %-12.2f %-8s %s",
			result.scenario.name,
			result.throughput,
			result.avgLatency,
			result.memoryUsage,
			result.successRate,
			status,
			result.errorMessage)
	}

	b.Logf("=== 报告结束 ===\n")
}

// BenchmarkPoolStabilityUnderLoad 负载下的稳定性测试
func BenchmarkPoolStabilityUnderLoad(b *testing.B) {
	// 长时间运行稳定性测试
	config, err := NewConfigBuilder().
		WithWorkerCount(16).
		WithQueueSize(5000).
		WithObjectPoolSize(500).
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

	// 测试持续时间
	testDuration := time.Second * 10
	if testing.Short() {
		testDuration = time.Second * 2
	}

	b.ResetTimer()
	start := time.Now()

	var totalSubmitted, totalErrors, totalPanics int64
	var wg sync.WaitGroup

	// 启动多个协程持续提交任务
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&totalPanics, 1)
				}
			}()

			var taskID int64
			for time.Since(start) < testDuration {
				id := atomic.AddInt64(&taskID, 1)
				workload := time.Microsecond * time.Duration((id%100)+1)
				task := NewBenchmarkTask(int64(workerID)*10000+id, workload)

				err := pool.Submit(task)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
				} else {
					atomic.AddInt64(&totalSubmitted, 1)
				}

				// 随机休眠，模拟真实场景
				if id%100 == 0 {
					time.Sleep(time.Microsecond * time.Duration(id%10))
				}
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(start)

	// 等待任务完成
	time.Sleep(time.Second)
	stats := pool.Stats()

	// 计算稳定性指标
	totalTasks := totalSubmitted + totalErrors
	successRate := float64(totalSubmitted) / float64(totalTasks) * 100
	throughput := float64(totalSubmitted) / actualDuration.Seconds()

	b.ReportMetric(throughput, "tasks/sec")
	b.ReportMetric(successRate, "success_rate_%")
	b.ReportMetric(float64(totalErrors), "errors")
	b.ReportMetric(float64(totalPanics), "panics")
	b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
	b.ReportMetric(float64(stats.ActiveWorkers), "active_workers")

	// 稳定性验证
	if successRate < 90 {
		b.Errorf("稳定性测试失败: 成功率 %.2f%% < 90%%", successRate)
	}
	if totalPanics > 0 {
		b.Errorf("稳定性测试失败: 发生了 %d 次panic", totalPanics)
	}
	if throughput < 1000 {
		b.Errorf("稳定性测试失败: 吞吐量 %.2f < 1000 tasks/sec", throughput)
	}

	b.Logf("稳定性测试结果: 运行时间: %v, 吞吐量: %.2f tasks/sec, 成功率: %.2f%%, 错误: %d, Panic: %d",
		actualDuration, throughput, successRate, totalErrors, totalPanics)
}

// BenchmarkPoolMemoryLeakDetection 内存泄漏检测测试
func BenchmarkPoolMemoryLeakDetection(b *testing.B) {
	const iterations = 5
	const tasksPerIteration = 10000

	memoryUsages := make([]uint64, iterations)

	for i := 0; i < iterations; i++ {
		config, err := NewConfigBuilder().
			WithWorkerCount(8).
			WithQueueSize(2000).
			WithObjectPoolSize(200).
			WithMetrics(false).
			Build()
		if err != nil {
			b.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			b.Fatalf("Failed to create pool: %v", err)
		}

		// 提交大量任务
		for j := 0; j < tasksPerIteration; j++ {
			task := NewBenchmarkTask(int64(j), time.Microsecond)
			pool.Submit(task)
		}

		// 等待任务完成
		time.Sleep(time.Millisecond * 500)

		// 关闭协程池
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		pool.Shutdown(ctx)
		cancel()

		// 强制GC并测量内存
		runtime.GC()
		runtime.GC() // 执行两次确保清理完成

		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memoryUsages[i] = m.HeapAlloc

		b.Logf("迭代 %d: 堆内存使用 %d bytes", i+1, m.HeapAlloc)
	}

	// 检查内存泄漏
	if len(memoryUsages) >= 2 {
		firstUsage := memoryUsages[0]
		lastUsage := memoryUsages[len(memoryUsages)-1]

		// 允许20%的内存增长
		maxAllowedUsage := firstUsage + (firstUsage * 20 / 100)

		if lastUsage > maxAllowedUsage {
			b.Errorf("可能存在内存泄漏: 初始内存 %d bytes, 最终内存 %d bytes, 增长 %.2f%%",
				firstUsage, lastUsage, float64(lastUsage-firstUsage)/float64(firstUsage)*100)
		}

		b.ReportMetric(float64(firstUsage), "initial_memory_bytes")
		b.ReportMetric(float64(lastUsage), "final_memory_bytes")
		b.ReportMetric(float64(lastUsage-firstUsage), "memory_growth_bytes")
	}
}

// BenchmarkPoolResourceCleanup 资源清理测试
func BenchmarkPoolResourceCleanup(b *testing.B) {
	const poolCount = 100

	var initialGoroutines int
	var finalGoroutines int

	// 获取初始协程数量
	initialGoroutines = runtime.NumGoroutine()

	b.ResetTimer()

	for i := 0; i < poolCount; i++ {
		config, err := NewConfigBuilder().
			WithWorkerCount(4).
			WithQueueSize(100).
			WithObjectPoolSize(50).
			WithMetrics(false).
			Build()
		if err != nil {
			b.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			b.Fatalf("Failed to create pool: %v", err)
		}

		// 提交一些任务
		for j := 0; j < 10; j++ {
			task := NewBenchmarkTask(int64(j), 0)
			pool.Submit(task)
		}

		// 立即关闭
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		err = pool.Shutdown(ctx)
		cancel()

		if err != nil {
			b.Errorf("Pool %d shutdown failed: %v", i, err)
		}
	}

	// 等待资源清理
	time.Sleep(time.Second)
	runtime.GC()

	// 获取最终协程数量
	finalGoroutines = runtime.NumGoroutine()

	goroutineLeak := finalGoroutines - initialGoroutines
	b.ReportMetric(float64(initialGoroutines), "initial_goroutines")
	b.ReportMetric(float64(finalGoroutines), "final_goroutines")
	b.ReportMetric(float64(goroutineLeak), "goroutine_leak")

	// 允许少量协程泄漏（测试框架本身可能创建协程）
	if goroutineLeak > 10 {
		b.Errorf("协程泄漏检测失败: 初始 %d, 最终 %d, 泄漏 %d",
			initialGoroutines, finalGoroutines, goroutineLeak)
	}

	b.Logf("资源清理测试: 初始协程 %d, 最终协程 %d, 泄漏 %d",
		initialGoroutines, finalGoroutines, goroutineLeak)
}
