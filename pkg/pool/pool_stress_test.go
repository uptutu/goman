package pool

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestStressScenarios 压力测试场景
func TestStressScenarios(t *testing.T) {
	t.Run("high throughput stress", func(t *testing.T) {
		testHighThroughputStress(t)
	})

	t.Run("memory pressure stress", func(t *testing.T) {
		testMemoryPressureStress(t)
	})

	t.Run("concurrent pool stress", func(t *testing.T) {
		testConcurrentPoolStress(t)
	})

	t.Run("mixed workload stress", func(t *testing.T) {
		testMixedWorkloadStress(t)
	})

	t.Run("failure injection stress", func(t *testing.T) {
		testFailureInjectionStress(t)
	})
}

// testHighThroughputStress 高吞吐量压力测试
func testHighThroughputStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU() * 4).
		WithQueueSize(10000).
		WithTaskTimeout(time.Second * 5).
		WithMetrics(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 压力测试参数
	testDuration := time.Second * 30
	if testing.Short() {
		testDuration = time.Second * 5
	}

	submitterCount := runtime.NumCPU() * 2
	var stats struct {
		submitted int64
		completed int64
		failed    int64
		errors    int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup
	startTime := time.Now()

	// 启动多个提交协程
	for i := 0; i < submitterCount; i++ {
		wg.Add(1)
		go func(submitterID int) {
			defer wg.Done()

			taskID := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					taskID++
					task := &StressTask{
						id:       fmt.Sprintf("s%d-t%d", submitterID, taskID),
						workType: "cpu",
						duration: time.Microsecond * time.Duration(100+rand.Intn(900)), // 100-1000μs
						onComplete: func() {
							atomic.AddInt64(&stats.completed, 1)
						},
						onError: func() {
							atomic.AddInt64(&stats.errors, 1)
						},
					}

					if err := pool.Submit(task); err != nil {
						atomic.AddInt64(&stats.failed, 1)
					} else {
						atomic.AddInt64(&stats.submitted, 1)
					}

					// 控制提交速率，避免过度消耗CPU
					if taskID%1000 == 0 {
						time.Sleep(time.Microsecond * 10)
					}
				}
			}
		}(i)
	}

	// 监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(startTime)
				submitted := atomic.LoadInt64(&stats.submitted)
				completed := atomic.LoadInt64(&stats.completed)
				failed := atomic.LoadInt64(&stats.failed)
				errors := atomic.LoadInt64(&stats.errors)

				throughput := float64(completed) / elapsed.Seconds()
				poolStats := pool.Stats()

				t.Logf("Stress test progress (%.1fs):", elapsed.Seconds())
				t.Logf("  Submitted: %d, Completed: %d, Failed: %d, Errors: %d", submitted, completed, failed, errors)
				t.Logf("  Throughput: %.0f tasks/sec", throughput)
				t.Logf("  Pool: Active=%d, Queued=%d, Memory=%d bytes",
					poolStats.ActiveWorkers, poolStats.QueuedTasks, poolStats.MemoryUsage)
			}
		}
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 2)

	totalDuration := time.Since(startTime)
	finalStats := pool.Stats()

	t.Logf("High throughput stress test results:")
	t.Logf("  Duration: %v", totalDuration)
	t.Logf("  Submitted: %d", stats.submitted)
	t.Logf("  Completed: %d", stats.completed)
	t.Logf("  Failed: %d", stats.failed)
	t.Logf("  Errors: %d", stats.errors)
	t.Logf("  Average throughput: %.0f tasks/sec", float64(stats.completed)/totalDuration.Seconds())
	t.Logf("  Final pool stats: %+v", finalStats)

	// 基本验证
	if stats.submitted == 0 {
		t.Error("No tasks were submitted during stress test")
	}

	if stats.completed == 0 {
		t.Error("No tasks were completed during stress test")
	}

	// 验证完成率
	completionRate := float64(stats.completed) / float64(stats.submitted)
	if completionRate < 0.8 { // 至少80%的任务应该完成
		t.Errorf("Low completion rate: %.2f%%", completionRate*100)
	}
}

// testMemoryPressureStress 内存压力测试
func testMemoryPressureStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(1000).
		WithMetrics(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 记录初始内存状态
	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)

	var stats struct {
		submitted int64
		completed int64
		failed    int64
	}

	// 提交大量内存密集型任务
	rounds := 20
	tasksPerRound := 500

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup

		for i := 0; i < tasksPerRound; i++ {
			wg.Add(1)
			task := &StressTask{
				id:       fmt.Sprintf("mem-r%d-t%d", round, i),
				workType: "memory",
				dataSize: 1024 * (1 + rand.Intn(10)),                         // 1-10KB per task
				duration: time.Millisecond * time.Duration(10+rand.Intn(40)), // 10-50ms
				onComplete: func() {
					atomic.AddInt64(&stats.completed, 1)
					wg.Done()
				},
				onError: func() {
					atomic.AddInt64(&stats.failed, 1)
					wg.Done()
				},
			}

			if err := pool.Submit(task); err != nil {
				atomic.AddInt64(&stats.failed, 1)
				wg.Done()
			} else {
				atomic.AddInt64(&stats.submitted, 1)
			}
		}

		wg.Wait()

		// 每几轮检查内存使用
		if round%5 == 0 {
			runtime.GC()
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			t.Logf("Round %d: Memory = %d bytes, GC = %d",
				round, memStats.Alloc, memStats.NumGC)

			// 检查内存是否过度增长
			if memStats.Alloc > initialMemStats.Alloc*20 { // 20倍增长认为异常
				t.Errorf("Excessive memory growth at round %d: %d -> %d bytes",
					round, initialMemStats.Alloc, memStats.Alloc)
				break
			}
		}
	}

	// 最终内存检查
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Millisecond * 100)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	t.Logf("Memory pressure stress test results:")
	t.Logf("  Submitted: %d", stats.submitted)
	t.Logf("  Completed: %d", stats.completed)
	t.Logf("  Failed: %d", stats.failed)
	t.Logf("  Initial memory: %d bytes", initialMemStats.Alloc)
	t.Logf("  Final memory: %d bytes", finalMemStats.Alloc)
	t.Logf("  Memory growth: %.2fx", float64(finalMemStats.Alloc)/float64(initialMemStats.Alloc))
	t.Logf("  GC count: %d -> %d", initialMemStats.NumGC, finalMemStats.NumGC)

	// 验证内存使用合理
	memoryGrowth := float64(finalMemStats.Alloc) / float64(initialMemStats.Alloc)
	if memoryGrowth > 5.0 { // 允许5倍内存增长
		t.Errorf("Excessive memory growth: %.2fx", memoryGrowth)
	}
}

// testConcurrentPoolStress 并发协程池压力测试
func testConcurrentPoolStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent pool stress test in short mode")
	}

	poolCount := 5
	var pools []Pool
	var wg sync.WaitGroup

	var globalStats struct {
		totalSubmitted int64
		totalCompleted int64
		totalFailed    int64
	}

	// 创建多个协程池
	for i := 0; i < poolCount; i++ {
		config, err := NewConfigBuilder().
			WithWorkerCount(4).
			WithQueueSize(200).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config for pool %d: %v", i, err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool %d: %v", i, err)
		}

		pools = append(pools, pool)
	}

	// 为每个协程池启动任务提交协程
	for poolID, pool := range pools {
		wg.Add(1)
		go func(pid int, p Pool) {
			defer wg.Done()

			var localStats struct {
				submitted int64
				completed int64
				failed    int64
			}

			// 每个协程池提交1000个任务
			for i := 0; i < 1000; i++ {
				task := &StressTask{
					id:       fmt.Sprintf("p%d-t%d", pid, i),
					workType: "mixed",
					duration: time.Millisecond * time.Duration(5+rand.Intn(20)), // 5-25ms
					onComplete: func() {
						atomic.AddInt64(&localStats.completed, 1)
						atomic.AddInt64(&globalStats.totalCompleted, 1)
					},
					onError: func() {
						atomic.AddInt64(&localStats.failed, 1)
						atomic.AddInt64(&globalStats.totalFailed, 1)
					},
				}

				if err := p.Submit(task); err != nil {
					atomic.AddInt64(&localStats.failed, 1)
					atomic.AddInt64(&globalStats.totalFailed, 1)
				} else {
					atomic.AddInt64(&localStats.submitted, 1)
					atomic.AddInt64(&globalStats.totalSubmitted, 1)
				}

				// 随机延迟，模拟真实场景
				if i%100 == 0 {
					time.Sleep(time.Microsecond * time.Duration(rand.Intn(1000)))
				}
			}

			t.Logf("Pool %d: Submitted=%d, Completed=%d, Failed=%d",
				pid, localStats.submitted, localStats.completed, localStats.failed)
		}(poolID, pool)
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(time.Second * 3)

	// 关闭所有协程池
	for i, pool := range pools {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := pool.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown pool %d: %v", i, err)
		}
		cancel()
	}

	t.Logf("Concurrent pool stress test results:")
	t.Logf("  Pools: %d", poolCount)
	t.Logf("  Total submitted: %d", globalStats.totalSubmitted)
	t.Logf("  Total completed: %d", globalStats.totalCompleted)
	t.Logf("  Total failed: %d", globalStats.totalFailed)

	// 验证结果
	if globalStats.totalSubmitted == 0 {
		t.Error("No tasks were submitted")
	}

	if globalStats.totalCompleted == 0 {
		t.Error("No tasks were completed")
	}

	// 验证所有协程池都已关闭
	for i, pool := range pools {
		if !pool.IsClosed() {
			t.Errorf("Pool %d should be closed", i)
		}
	}
}

// testMixedWorkloadStress 混合工作负载压力测试
func testMixedWorkloadStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mixed workload stress test in short mode")
	}

	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(500).
		WithTaskTimeout(time.Second * 2).
		WithMetrics(true).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var stats struct {
		fastCompleted   int64
		slowCompleted   int64
		cpuCompleted    int64
		memoryCompleted int64
		ioCompleted     int64
		errorTasks      int64
		panicTasks      int64
		timeoutTasks    int64
	}

	var wg sync.WaitGroup
	taskTypes := []string{"fast", "slow", "cpu", "memory", "io", "error", "panic", "timeout"}

	// 为每种任务类型启动提交协程
	for _, taskType := range taskTypes {
		wg.Add(1)
		go func(tType string) {
			defer wg.Done()

			taskCount := 200 // 每种类型200个任务
			for i := 0; i < taskCount; i++ {
				task := createStressTaskByType(tType, i, &stats)

				if err := pool.Submit(task); err != nil {
					t.Logf("Failed to submit %s task %d: %v", tType, i, err)
				}

				// 控制提交速率
				if i%50 == 0 {
					time.Sleep(time.Millisecond * 10)
				}
			}
		}(taskType)
	}

	wg.Wait()

	// 等待所有任务完成
	timeout := time.After(time.Second * 30)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Log("Timeout waiting for mixed workload to complete")
			goto results
		case <-ticker.C:
			total := atomic.LoadInt64(&stats.fastCompleted) +
				atomic.LoadInt64(&stats.slowCompleted) +
				atomic.LoadInt64(&stats.cpuCompleted) +
				atomic.LoadInt64(&stats.memoryCompleted) +
				atomic.LoadInt64(&stats.ioCompleted) +
				atomic.LoadInt64(&stats.errorTasks) +
				atomic.LoadInt64(&stats.panicTasks) +
				atomic.LoadInt64(&stats.timeoutTasks)

			if total >= int64(float64(len(taskTypes)*200)*0.9) { // 90%完成即可
				goto results
			}
		}
	}

results:
	poolStats := pool.Stats()

	t.Logf("Mixed workload stress test results:")
	t.Logf("  Fast tasks: %d", stats.fastCompleted)
	t.Logf("  Slow tasks: %d", stats.slowCompleted)
	t.Logf("  CPU tasks: %d", stats.cpuCompleted)
	t.Logf("  Memory tasks: %d", stats.memoryCompleted)
	t.Logf("  IO tasks: %d", stats.ioCompleted)
	t.Logf("  Error tasks: %d", stats.errorTasks)
	t.Logf("  Panic tasks: %d", stats.panicTasks)
	t.Logf("  Timeout tasks: %d", stats.timeoutTasks)
	t.Logf("  Pool stats: %+v", poolStats)

	// 验证协程池仍然正常工作
	if !pool.IsRunning() {
		t.Error("Pool should still be running after mixed workload stress")
	}
}

// testFailureInjectionStress 故障注入压力测试
func testFailureInjectionStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failure injection stress test in short mode")
	}

	config, err := NewConfigBuilder().
		WithWorkerCount(6).
		WithQueueSize(300).
		WithTaskTimeout(time.Millisecond * 500).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var stats struct {
		normalCompleted int64
		errors          int64
		panics          int64
		timeouts        int64
		recoveries      int64
	}

	// 故障注入模式
	failurePatterns := []struct {
		name        string
		count       int
		failureRate float64 // 失败率
	}{
		{"normal", 500, 0.0},
		{"error_prone", 300, 0.3},
		{"panic_prone", 200, 0.2},
		{"timeout_prone", 150, 0.4},
	}

	var wg sync.WaitGroup

	for _, pattern := range failurePatterns {
		wg.Add(1)
		go func(p struct {
			name        string
			count       int
			failureRate float64
		}) {
			defer wg.Done()

			for i := 0; i < p.count; i++ {
				shouldFail := rand.Float64() < p.failureRate

				var task Task
				switch p.name {
				case "normal":
					task = &StressTask{
						id:       fmt.Sprintf("%s-%d", p.name, i),
						workType: "cpu",
						duration: time.Millisecond * time.Duration(10+rand.Intn(40)),
						onComplete: func() {
							atomic.AddInt64(&stats.normalCompleted, 1)
						},
					}
				case "error_prone":
					task = &StressTask{
						id:          fmt.Sprintf("%s-%d", p.name, i),
						workType:    "cpu",
						duration:    time.Millisecond * time.Duration(10+rand.Intn(40)),
						shouldError: shouldFail,
						onComplete: func() {
							atomic.AddInt64(&stats.normalCompleted, 1)
						},
						onError: func() {
							atomic.AddInt64(&stats.errors, 1)
						},
					}
				case "panic_prone":
					task = &StressTask{
						id:          fmt.Sprintf("%s-%d", p.name, i),
						workType:    "cpu",
						duration:    time.Millisecond * time.Duration(10+rand.Intn(40)),
						shouldPanic: shouldFail,
						onComplete: func() {
							atomic.AddInt64(&stats.normalCompleted, 1)
						},
						onPanic: func() {
							atomic.AddInt64(&stats.panics, 1)
						},
					}
				case "timeout_prone":
					duration := time.Millisecond * time.Duration(100+rand.Intn(400))
					if shouldFail {
						duration = time.Millisecond * 600 // 超过超时时间
					}
					task = &StressTask{
						id:       fmt.Sprintf("%s-%d", p.name, i),
						workType: "cpu",
						duration: duration,
						onComplete: func() {
							atomic.AddInt64(&stats.normalCompleted, 1)
						},
						onTimeout: func() {
							atomic.AddInt64(&stats.timeouts, 1)
						},
					}
				}

				if err := pool.Submit(task); err != nil {
					t.Logf("Failed to submit %s task %d: %v", p.name, i, err)
				}

				// 控制提交速率
				if i%100 == 0 {
					time.Sleep(time.Millisecond * 5)
				}
			}
		}(pattern)
	}

	wg.Wait()

	// 等待任务完成
	time.Sleep(time.Second * 5)

	// 验证恢复能力 - 提交一批正常任务
	var recoveryCount int64
	var recoveryWg sync.WaitGroup

	for i := 0; i < 100; i++ {
		recoveryWg.Add(1)
		task := &StressTask{
			id:       fmt.Sprintf("recovery-%d", i),
			workType: "cpu",
			duration: time.Millisecond * 10,
			onComplete: func() {
				atomic.AddInt64(&recoveryCount, 1)
				recoveryWg.Done()
			},
			onError: func() {
				recoveryWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			recoveryWg.Done()
		}
	}

	recoveryWg.Wait()

	poolStats := pool.Stats()

	t.Logf("Failure injection stress test results:")
	t.Logf("  Normal completed: %d", stats.normalCompleted)
	t.Logf("  Errors: %d", stats.errors)
	t.Logf("  Panics: %d", stats.panics)
	t.Logf("  Timeouts: %d", stats.timeouts)
	t.Logf("  Recovery tasks: %d/100", recoveryCount)
	t.Logf("  Pool stats: %+v", poolStats)

	// 验证协程池恢复能力
	if !pool.IsRunning() {
		t.Error("Pool should still be running after failure injection")
	}

	if recoveryCount < 90 { // 至少90%的恢复任务应该完成
		t.Errorf("Poor recovery performance: %d/100 tasks completed", recoveryCount)
	}
}

// createStressTaskByType 根据类型创建压力测试任务
func createStressTaskByType(taskType string, id int, stats *struct {
	fastCompleted   int64
	slowCompleted   int64
	cpuCompleted    int64
	memoryCompleted int64
	ioCompleted     int64
	errorTasks      int64
	panicTasks      int64
	timeoutTasks    int64
}) *StressTask {
	taskID := fmt.Sprintf("%s-%d", taskType, id)

	switch taskType {
	case "fast":
		return &StressTask{
			id:       taskID,
			workType: "cpu",
			duration: time.Microsecond * time.Duration(100+rand.Intn(400)), // 100-500μs
			onComplete: func() {
				atomic.AddInt64(&stats.fastCompleted, 1)
			},
		}
	case "slow":
		return &StressTask{
			id:       taskID,
			workType: "cpu",
			duration: time.Millisecond * time.Duration(50+rand.Intn(100)), // 50-150ms
			onComplete: func() {
				atomic.AddInt64(&stats.slowCompleted, 1)
			},
		}
	case "cpu":
		return &StressTask{
			id:       taskID,
			workType: "cpu",
			duration: time.Millisecond * time.Duration(10+rand.Intn(30)), // 10-40ms
			onComplete: func() {
				atomic.AddInt64(&stats.cpuCompleted, 1)
			},
		}
	case "memory":
		return &StressTask{
			id:       taskID,
			workType: "memory",
			dataSize: 1024 * (1 + rand.Intn(5)),                         // 1-5KB
			duration: time.Millisecond * time.Duration(5+rand.Intn(20)), // 5-25ms
			onComplete: func() {
				atomic.AddInt64(&stats.memoryCompleted, 1)
			},
		}
	case "io":
		return &StressTask{
			id:       taskID,
			workType: "io",
			duration: time.Millisecond * time.Duration(20+rand.Intn(80)), // 20-100ms
			onComplete: func() {
				atomic.AddInt64(&stats.ioCompleted, 1)
			},
		}
	case "error":
		return &StressTask{
			id:          taskID,
			workType:    "cpu",
			duration:    time.Millisecond * time.Duration(5+rand.Intn(15)),
			shouldError: true,
			onError: func() {
				atomic.AddInt64(&stats.errorTasks, 1)
			},
		}
	case "panic":
		return &StressTask{
			id:          taskID,
			workType:    "cpu",
			duration:    time.Millisecond * time.Duration(5+rand.Intn(15)),
			shouldPanic: true,
			onPanic: func() {
				atomic.AddInt64(&stats.panicTasks, 1)
			},
		}
	case "timeout":
		return &StressTask{
			id:       taskID,
			workType: "cpu",
			duration: time.Second * 3, // 超过默认超时时间
			onTimeout: func() {
				atomic.AddInt64(&stats.timeoutTasks, 1)
			},
		}
	default:
		return &StressTask{
			id:       taskID,
			workType: "cpu",
			duration: time.Millisecond * 10,
		}
	}
}

// StressTask 压力测试任务
type StressTask struct {
	id          string
	workType    string // "cpu", "memory", "io", "mixed"
	duration    time.Duration
	dataSize    int
	shouldError bool
	shouldPanic bool
	onComplete  func()
	onError     func()
	onPanic     func()
	onTimeout   func()
}

func (t *StressTask) Execute(ctx context.Context) (any, error) {
	if t.shouldPanic {
		if t.onPanic != nil {
			defer t.onPanic()
		}
		panic(fmt.Sprintf("stress test panic from task %s", t.id))
	}

	if t.shouldError {
		if t.onError != nil {
			t.onError()
		}
		return nil, fmt.Errorf("stress test error from task %s", t.id)
	}

	// 执行不同类型的工作负载
	switch t.workType {
	case "cpu":
		t.doCPUWork(ctx)
	case "memory":
		t.doMemoryWork(ctx)
	case "io":
		t.doIOWork(ctx)
	case "mixed":
		t.doMixedWork(ctx)
	}

	// 检查是否被取消或超时
	select {
	case <-ctx.Done():
		if t.onTimeout != nil {
			t.onTimeout()
		}
		return nil, ctx.Err()
	default:
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *StressTask) Priority() int {
	return PriorityNormal
}

// doCPUWork 执行CPU密集型工作
func (t *StressTask) doCPUWork(ctx context.Context) {
	start := time.Now()
	for time.Since(start) < t.duration {
		select {
		case <-ctx.Done():
			return
		default:
			// 简单的CPU计算
			sum := 0
			for i := 0; i < 1000; i++ {
				sum += i * i
			}
		}
	}
}

// doMemoryWork 执行内存密集型工作
func (t *StressTask) doMemoryWork(ctx context.Context) {
	if t.dataSize <= 0 {
		t.dataSize = 1024
	}

	// 分配和操作内存
	data := make([]byte, t.dataSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// 模拟内存操作
	start := time.Now()
	for time.Since(start) < t.duration {
		select {
		case <-ctx.Done():
			return
		default:
			// 简单的内存操作
			for i := 0; i < len(data); i += 64 {
				data[i] = byte(rand.Intn(256))
			}
		}
	}
}

// doIOWork 模拟IO密集型工作
func (t *StressTask) doIOWork(ctx context.Context) {
	// 使用sleep模拟IO等待
	select {
	case <-time.After(t.duration):
	case <-ctx.Done():
	}
}

// doMixedWork 执行混合工作负载
func (t *StressTask) doMixedWork(ctx context.Context) {
	// 混合CPU和内存工作
	halfDuration := t.duration / 2

	// 前半段做CPU工作
	t.duration = halfDuration
	t.doCPUWork(ctx)

	// 后半段做内存工作
	if t.dataSize == 0 {
		t.dataSize = 512
	}
	t.duration = halfDuration
	t.doMemoryWork(ctx)
}
