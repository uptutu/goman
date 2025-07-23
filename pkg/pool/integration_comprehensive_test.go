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

// TestComprehensiveIntegration 综合集成测试
func TestComprehensiveIntegration(t *testing.T) {
	t.Run("full lifecycle integration", func(t *testing.T) {
		testFullLifecycleIntegration(t)
	})

	t.Run("stress test with fault injection", func(t *testing.T) {
		testStressWithFaultInjection(t)
	})

	t.Run("long running stability", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping long running test in short mode")
		}
		testLongRunningStability(t)
	})

	t.Run("resource leak validation", func(t *testing.T) {
		testResourceLeakValidation(t)
	})

	t.Run("concurrent pool operations", func(t *testing.T) {
		testConcurrentPoolOperations(t)
	})

	t.Run("error recovery scenarios", func(t *testing.T) {
		testErrorRecoveryScenarios(t)
	})

	t.Run("performance degradation detection", func(t *testing.T) {
		testPerformanceDegradationDetection(t)
	})
}

// testFullLifecycleIntegration 测试完整生命周期集成
func testFullLifecycleIntegration(t *testing.T) {
	// 阶段1: 创建和初始化
	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(200).
		WithTaskTimeout(time.Second * 3).
		WithShutdownTimeout(time.Second * 10).
		WithMetrics(true).
		WithMetricsInterval(time.Millisecond * 100).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	// 验证初始状态
	if !pool.IsRunning() {
		t.Error("Pool should be running after creation")
	}

	initialStats := pool.Stats()
	t.Logf("Initial stats: %+v", initialStats)

	// 阶段2: 多阶段任务提交和执行
	phases := []struct {
		name      string
		taskCount int
		taskType  string
		duration  time.Duration
	}{
		{"warmup", 50, "fast", time.Millisecond * 10},
		{"normal_load", 200, "mixed", time.Millisecond * 50},
		{"peak_load", 500, "varied", time.Millisecond * 100},
		{"cooldown", 100, "slow", time.Millisecond * 200},
	}

	var totalCompleted int64
	var totalFailed int64

	for _, phase := range phases {
		t.Logf("Starting phase: %s", phase.name)
		phaseStart := time.Now()

		var phaseCompleted int64
		var phaseFailed int64
		var wg sync.WaitGroup

		for i := 0; i < phase.taskCount; i++ {
			wg.Add(1)
			task := &ComprehensiveTask{
				id:       fmt.Sprintf("%s-%d", phase.name, i),
				taskType: phase.taskType,
				duration: phase.duration,
				onComplete: func() {
					atomic.AddInt64(&phaseCompleted, 1)
					atomic.AddInt64(&totalCompleted, 1)
					wg.Done()
				},
				onError: func() {
					atomic.AddInt64(&phaseFailed, 1)
					atomic.AddInt64(&totalFailed, 1)
					wg.Done()
				},
			}

			if err := pool.Submit(task); err != nil {
				t.Errorf("Failed to submit task in phase %s: %v", phase.name, err)
				wg.Done()
			}
		}

		// 等待阶段完成
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			phaseDuration := time.Since(phaseStart)
			t.Logf("Phase %s completed: %d tasks in %v (%.2f tasks/sec)",
				phase.name, phaseCompleted, phaseDuration,
				float64(phaseCompleted)/phaseDuration.Seconds())
		case <-time.After(time.Second * 30):
			t.Errorf("Phase %s timeout", phase.name)
		}

		// 阶段间统计
		phaseStats := pool.Stats()
		t.Logf("Phase %s stats: Active=%d, Queued=%d, Completed=%d, Failed=%d",
			phase.name, phaseStats.ActiveWorkers, phaseStats.QueuedTasks,
			phaseStats.CompletedTasks, phaseStats.FailedTasks)

		// 短暂休息
		time.Sleep(time.Millisecond * 100)
	}

	// 阶段3: 验证最终状态
	finalStats := pool.Stats()
	t.Logf("Final integration stats:")
	t.Logf("  Total completed: %d", totalCompleted)
	t.Logf("  Total failed: %d", totalFailed)
	t.Logf("  Pool stats: %+v", finalStats)

	// 验证统计一致性
	if finalStats.CompletedTasks < totalCompleted {
		t.Errorf("Stats inconsistency: pool reports %d completed, actual %d",
			finalStats.CompletedTasks, totalCompleted)
	}

	// 阶段4: 优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	shutdownStart := time.Now()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	t.Logf("Shutdown completed in %v", shutdownDuration)

	// 验证关闭后状态
	if !pool.IsClosed() {
		t.Error("Pool should be closed after shutdown")
	}

	// 尝试提交任务应该失败
	testTask := &ComprehensiveTask{id: "post-shutdown"}
	if err := pool.Submit(testTask); err == nil {
		t.Error("Submit should fail after shutdown")
	}
}

// testStressWithFaultInjection 压力测试与故障注入
func testStressWithFaultInjection(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU()).
		WithQueueSize(500).
		WithTaskTimeout(time.Second * 2).
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

	// 故障注入配置
	faultConfig := struct {
		panicRate    float64 // panic概率
		errorRate    float64 // 错误概率
		timeoutRate  float64 // 超时概率
		slowTaskRate float64 // 慢任务概率
	}{
		panicRate:    0.02, // 2%
		errorRate:    0.05, // 5%
		timeoutRate:  0.03, // 3%
		slowTaskRate: 0.10, // 10%
	}

	// 压力测试参数
	stressConfig := struct {
		duration       time.Duration
		submitterCount int
		taskRate       int // 每秒提交任务数
	}{
		duration:       time.Minute * 2,
		submitterCount: 10,
		taskRate:       100,
	}

	if testing.Short() {
		stressConfig.duration = time.Second * 30
		stressConfig.taskRate = 50
	}

	var stats struct {
		submitted   int64
		completed   int64
		failed      int64
		panics      int64
		timeouts    int64
		submitFails int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), stressConfig.duration)
	defer cancel()

	// 启动多个提交协程
	var wg sync.WaitGroup
	for i := 0; i < stressConfig.submitterCount; i++ {
		wg.Add(1)
		go func(submitterID int) {
			defer wg.Done()

			taskID := 0
			ticker := time.NewTicker(time.Duration(int64(time.Second) / int64(stressConfig.taskRate/stressConfig.submitterCount)))
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					taskID++

					// 随机故障注入
					var faultType string
					r := rand.Float64()
					switch {
					case r < faultConfig.panicRate:
						faultType = "panic"
					case r < faultConfig.panicRate+faultConfig.errorRate:
						faultType = "error"
					case r < faultConfig.panicRate+faultConfig.errorRate+faultConfig.timeoutRate:
						faultType = "timeout"
					case r < faultConfig.panicRate+faultConfig.errorRate+faultConfig.timeoutRate+faultConfig.slowTaskRate:
						faultType = "slow"
					default:
						faultType = "normal"
					}

					task := &ComprehensiveStressTask{
						id:        fmt.Sprintf("s%d-t%d", submitterID, taskID),
						faultType: faultType,
						onComplete: func() {
							atomic.AddInt64(&stats.completed, 1)
						},
						onError: func() {
							atomic.AddInt64(&stats.failed, 1)
						},
						onPanic: func() {
							atomic.AddInt64(&stats.panics, 1)
						},
						onTimeout: func() {
							atomic.AddInt64(&stats.timeouts, 1)
						},
					}

					if err := pool.Submit(task); err != nil {
						atomic.AddInt64(&stats.submitFails, 1)
					} else {
						atomic.AddInt64(&stats.submitted, 1)
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
				poolStats := pool.Stats()
				t.Logf("Stress test progress:")
				t.Logf("  Submitted: %d, Completed: %d, Failed: %d",
					atomic.LoadInt64(&stats.submitted),
					atomic.LoadInt64(&stats.completed),
					atomic.LoadInt64(&stats.failed))
				t.Logf("  Panics: %d, Timeouts: %d, Submit fails: %d",
					atomic.LoadInt64(&stats.panics),
					atomic.LoadInt64(&stats.timeouts),
					atomic.LoadInt64(&stats.submitFails))
				t.Logf("  Pool: Active=%d, Queued=%d, Memory=%d",
					poolStats.ActiveWorkers, poolStats.QueuedTasks, poolStats.MemoryUsage)
			}
		}
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 3)

	// 最终统计
	finalStats := pool.Stats()
	t.Logf("Stress test with fault injection results:")
	t.Logf("  Duration: %v", stressConfig.duration)
	t.Logf("  Submitted: %d", stats.submitted)
	t.Logf("  Completed: %d", stats.completed)
	t.Logf("  Failed: %d", stats.failed)
	t.Logf("  Panics: %d", stats.panics)
	t.Logf("  Timeouts: %d", stats.timeouts)
	t.Logf("  Submit failures: %d", stats.submitFails)
	t.Logf("  Final pool stats: %+v", finalStats)

	// 验证系统稳定性
	if stats.submitted == 0 {
		t.Error("No tasks were submitted during stress test")
	}

	if stats.completed == 0 {
		t.Error("No tasks were completed during stress test")
	}

	// 验证协程池仍然可用
	if !pool.IsRunning() {
		t.Error("Pool should still be running after stress test")
	}

	// 提交测试任务验证功能
	testTask := &ComprehensiveStressTask{id: "post-stress-test", faultType: "normal"}
	if err := pool.Submit(testTask); err != nil {
		t.Errorf("Pool should still accept tasks after stress test: %v", err)
	}
}

// testLongRunningStability 长时间运行稳定性测试
func testLongRunningStability(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(6).
		WithQueueSize(200).
		WithTaskTimeout(time.Second * 5).
		WithMetrics(true).
		WithMetricsInterval(time.Second).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 长时间运行配置
	testDuration := time.Minute * 10 // 10分钟
	if testing.Short() {
		testDuration = time.Minute * 2 // 短模式2分钟
	}

	var stats struct {
		totalSubmitted  int64
		totalCompleted  int64
		totalFailed     int64
		memorySnapshots []uint64
		gcSnapshots     []int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// 任务提交协程
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		taskID := 0
		ticker := time.NewTicker(time.Millisecond * 50) // 每50ms提交一个任务
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				taskID++

				// 创建不同类型的任务
				var taskType string
				switch taskID % 10 {
				case 0, 1, 2, 3, 4, 5: // 60% 快速任务
					taskType = "fast"
				case 6, 7, 8: // 30% 中等任务
					taskType = "medium"
				case 9: // 10% 慢任务
					taskType = "slow"
				}

				task := &LongRunningTask{
					id:       fmt.Sprintf("lr-%d", taskID),
					taskType: taskType,
					onComplete: func() {
						atomic.AddInt64(&stats.totalCompleted, 1)
					},
					onError: func() {
						atomic.AddInt64(&stats.totalFailed, 1)
					},
				}

				if err := pool.Submit(task); err != nil {
					atomic.AddInt64(&stats.totalFailed, 1)
				} else {
					atomic.AddInt64(&stats.totalSubmitted, 1)
				}
			}
		}
	}()

	// 监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second * 30)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poolStats := pool.Stats()

				// 记录内存快照
				stats.memorySnapshots = append(stats.memorySnapshots, uint64(poolStats.MemoryUsage))
				stats.gcSnapshots = append(stats.gcSnapshots, poolStats.GCCount)

				t.Logf("Long running stability check:")
				deadline, _ := ctx.Deadline()
				elapsed := time.Since(deadline.Add(-testDuration))
				t.Logf("  Runtime: %v", elapsed)
				t.Logf("  Submitted: %d, Completed: %d, Failed: %d",
					atomic.LoadInt64(&stats.totalSubmitted),
					atomic.LoadInt64(&stats.totalCompleted),
					atomic.LoadInt64(&stats.totalFailed))
				t.Logf("  Pool stats: %+v", poolStats)

				// 检查内存增长趋势
				if len(stats.memorySnapshots) >= 3 {
					recent := stats.memorySnapshots[len(stats.memorySnapshots)-3:]
					if isMemoryGrowthConcerning(recent) {
						t.Logf("WARNING: Concerning memory growth detected: %v", recent)
					}
				}
			}
		}
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 5)

	// 最终分析
	finalStats := pool.Stats()
	t.Logf("Long running stability test results:")
	t.Logf("  Test duration: %v", testDuration)
	t.Logf("  Total submitted: %d", stats.totalSubmitted)
	t.Logf("  Total completed: %d", stats.totalCompleted)
	t.Logf("  Total failed: %d", stats.totalFailed)
	t.Logf("  Final stats: %+v", finalStats)

	// 内存稳定性分析
	if len(stats.memorySnapshots) > 0 {
		analyzeMemoryStability(t, stats.memorySnapshots)
	}

	// GC频率分析
	if len(stats.gcSnapshots) > 1 {
		analyzeGCStability(t, stats.gcSnapshots, testDuration)
	}

	// 验证系统仍然健康
	if !pool.IsRunning() {
		t.Error("Pool should still be running after long stability test")
	}

	if stats.totalCompleted == 0 {
		t.Error("No tasks completed during long running test")
	}

	// 计算成功率
	if stats.totalSubmitted > 0 {
		successRate := float64(stats.totalCompleted) / float64(stats.totalSubmitted)
		t.Logf("Task success rate: %.2f%%", successRate*100)

		if successRate < 0.95 { // 95%成功率阈值
			t.Errorf("Success rate too low: %.2f%%", successRate*100)
		}
	}
}

// testResourceLeakValidation 资源泄漏验证测试
func testResourceLeakValidation(t *testing.T) {
	// 记录初始状态
	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)
	initialGoroutines := runtime.NumGoroutine()

	t.Logf("Initial state: Memory=%d bytes, Goroutines=%d",
		initialMemStats.Alloc, initialGoroutines)

	// 创建和销毁多个协程池
	poolCount := 20
	for cycle := 0; cycle < 3; cycle++ { // 3个周期
		t.Logf("Starting resource leak test cycle %d", cycle+1)

		var pools []Pool

		// 创建多个协程池
		for i := 0; i < poolCount; i++ {
			config, err := NewConfigBuilder().
				WithWorkerCount(4).
				WithQueueSize(50).
				WithTaskTimeout(time.Second).
				Build()
			if err != nil {
				t.Fatalf("Failed to create config: %v", err)
			}

			pool, err := NewPool(config)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			pools = append(pools, pool)

			// 向每个协程池提交任务
			var wg sync.WaitGroup
			for j := 0; j < 20; j++ {
				wg.Add(1)
				task := &ResourceTestTask{
					id:   fmt.Sprintf("c%d-p%d-t%d", cycle, i, j),
					data: make([]byte, 1024), // 1KB数据
					onComplete: func() {
						wg.Done()
					},
				}

				if err := pool.Submit(task); err != nil {
					t.Errorf("Failed to submit task: %v", err)
					wg.Done()
				}
			}

			// 等待任务完成
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// 任务完成
			case <-time.After(time.Second * 5):
				t.Errorf("Timeout waiting for tasks in pool %d", i)
			}
		}

		// 关闭所有协程池
		for i, pool := range pools {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			if err := pool.Shutdown(ctx); err != nil {
				t.Errorf("Failed to shutdown pool %d: %v", i, err)
			}
			cancel()
		}

		// 强制GC并检查资源使用
		runtime.GC()
		runtime.GC()
		time.Sleep(time.Millisecond * 100)

		var cycleMemStats runtime.MemStats
		runtime.ReadMemStats(&cycleMemStats)
		cycleGoroutines := runtime.NumGoroutine()

		t.Logf("After cycle %d: Memory=%d bytes, Goroutines=%d",
			cycle+1, cycleMemStats.Alloc, cycleGoroutines)

		// 检查资源泄漏
		memoryGrowth := int64(cycleMemStats.Alloc) - int64(initialMemStats.Alloc)
		goroutineGrowth := cycleGoroutines - initialGoroutines

		if memoryGrowth > 10*1024*1024 { // 10MB阈值
			t.Errorf("Possible memory leak in cycle %d: %d bytes growth", cycle+1, memoryGrowth)
		}

		if goroutineGrowth > 10 { // 10个协程阈值
			t.Errorf("Possible goroutine leak in cycle %d: %d goroutines growth", cycle+1, goroutineGrowth)
		}
	}

	// 最终检查
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Second)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	finalGoroutines := runtime.NumGoroutine()

	t.Logf("Final state: Memory=%d bytes, Goroutines=%d",
		finalMemStats.Alloc, finalGoroutines)

	// 最终验证
	finalMemoryGrowth := int64(finalMemStats.Alloc) - int64(initialMemStats.Alloc)
	finalGoroutineGrowth := finalGoroutines - initialGoroutines

	t.Logf("Resource leak validation results:")
	t.Logf("  Memory growth: %d bytes", finalMemoryGrowth)
	t.Logf("  Goroutine growth: %d", finalGoroutineGrowth)

	// 允许一定的资源增长
	if finalMemoryGrowth > 5*1024*1024 { // 5MB
		t.Errorf("Significant memory leak detected: %d bytes", finalMemoryGrowth)
	}

	if finalGoroutineGrowth > 5 {
		t.Errorf("Significant goroutine leak detected: %d goroutines", finalGoroutineGrowth)
	}
}

// testConcurrentPoolOperations 并发协程池操作测试
func testConcurrentPoolOperations(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(100).
		WithTaskTimeout(time.Second * 2).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	var stats struct {
		syncSubmitted    int64
		asyncSubmitted   int64
		timeoutSubmitted int64
		syncCompleted    int64
		asyncCompleted   int64
		timeoutCompleted int64
		errors           int64
	}

	// 并发操作
	var wg sync.WaitGroup
	operationCount := 100

	// 并发同步提交
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operationCount; i++ {
			task := &ConcurrentTask{
				id:       fmt.Sprintf("sync-%d", i),
				taskType: "sync",
				onComplete: func() {
					atomic.AddInt64(&stats.syncCompleted, 1)
				},
			}

			if err := pool.Submit(task); err != nil {
				atomic.AddInt64(&stats.errors, 1)
			} else {
				atomic.AddInt64(&stats.syncSubmitted, 1)
			}
		}
	}()

	// 并发异步提交
	wg.Add(1)
	go func() {
		defer wg.Done()
		futures := make([]Future, operationCount)

		for i := 0; i < operationCount; i++ {
			task := &ConcurrentTask{
				id:       fmt.Sprintf("async-%d", i),
				taskType: "async",
			}

			futures[i] = pool.SubmitAsync(task)
			atomic.AddInt64(&stats.asyncSubmitted, 1)
		}

		// 获取所有结果
		for i, future := range futures {
			if _, err := future.GetWithTimeout(time.Second * 5); err != nil {
				atomic.AddInt64(&stats.errors, 1)
			} else {
				atomic.AddInt64(&stats.asyncCompleted, 1)
			}

			if i%20 == 0 {
				t.Logf("Async progress: %d/%d", i+1, operationCount)
			}
		}
	}()

	// 并发超时提交
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operationCount; i++ {
			task := &ConcurrentTask{
				id:       fmt.Sprintf("timeout-%d", i),
				taskType: "timeout",
				onComplete: func() {
					atomic.AddInt64(&stats.timeoutCompleted, 1)
				},
			}

			if err := pool.SubmitWithTimeout(task, time.Millisecond*500); err != nil {
				atomic.AddInt64(&stats.errors, 1)
			} else {
				atomic.AddInt64(&stats.timeoutSubmitted, 1)
			}
		}
	}()

	// 并发统计查询
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			stats := pool.Stats()
			_ = stats // 使用统计信息
			time.Sleep(time.Millisecond * 20)
		}
	}()

	// 等待所有操作完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All concurrent operations completed")
	case <-time.After(time.Second * 30):
		t.Fatal("Timeout waiting for concurrent operations")
	}

	// 等待剩余任务完成
	time.Sleep(time.Second * 2)

	// 验证结果
	t.Logf("Concurrent operations results:")
	t.Logf("  Sync: submitted=%d, completed=%d", stats.syncSubmitted, stats.syncCompleted)
	t.Logf("  Async: submitted=%d, completed=%d", stats.asyncSubmitted, stats.asyncCompleted)
	t.Logf("  Timeout: submitted=%d, completed=%d", stats.timeoutSubmitted, stats.timeoutCompleted)
	t.Logf("  Errors: %d", stats.errors)

	finalStats := pool.Stats()
	t.Logf("  Final pool stats: %+v", finalStats)

	// 关闭协程池
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := pool.Shutdown(ctx); err != nil {
		t.Errorf("Failed to shutdown pool: %v", err)
	}

	// 验证基本功能正常
	if stats.syncSubmitted == 0 && stats.asyncSubmitted == 0 && stats.timeoutSubmitted == 0 {
		t.Error("No tasks were submitted during concurrent test")
	}
}

// testErrorRecoveryScenarios 错误恢复场景测试
func testErrorRecoveryScenarios(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(50).
		WithTaskTimeout(time.Millisecond * 200).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	scenarios := []struct {
		name        string
		errorType   string
		taskCount   int
		expectError bool
	}{
		{"normal_tasks", "normal", 20, false},
		{"panic_recovery", "panic", 10, true},
		{"timeout_handling", "timeout", 15, true},
		{"error_propagation", "error", 12, true},
		{"mixed_errors", "mixed", 25, true},
		{"recovery_validation", "normal", 30, false},
	}

	for _, scenario := range scenarios {
		t.Logf("Running error recovery scenario: %s", scenario.name)

		var completed int64
		var failed int64
		var wg sync.WaitGroup

		for i := 0; i < scenario.taskCount; i++ {
			wg.Add(1)
			task := &ErrorRecoveryTask{
				id:        fmt.Sprintf("%s-%d", scenario.name, i),
				errorType: scenario.errorType,
				onComplete: func() {
					atomic.AddInt64(&completed, 1)
					wg.Done()
				},
				onError: func() {
					atomic.AddInt64(&failed, 1)
					wg.Done()
				},
			}

			if err := pool.Submit(task); err != nil {
				t.Errorf("Failed to submit task in scenario %s: %v", scenario.name, err)
				wg.Done()
			}
		}

		// 等待场景完成
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Logf("Scenario %s: completed=%d, failed=%d", scenario.name, completed, failed)
		case <-time.After(time.Second * 10):
			t.Errorf("Timeout in scenario %s", scenario.name)
		}

		// 验证协程池仍然健康
		if !pool.IsRunning() {
			t.Errorf("Pool should still be running after scenario %s", scenario.name)
		}

		// 短暂休息
		time.Sleep(time.Millisecond * 100)
	}

	// 最终健康检查
	healthTask := &ErrorRecoveryTask{
		id:        "health-check",
		errorType: "normal",
	}

	future := pool.SubmitAsync(healthTask)
	if _, err := future.GetWithTimeout(time.Second); err != nil {
		t.Errorf("Health check failed after error recovery scenarios: %v", err)
	}

	finalStats := pool.Stats()
	t.Logf("Error recovery test final stats: %+v", finalStats)
}

// testPerformanceDegradationDetection 性能退化检测测试
func testPerformanceDegradationDetection(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
		WithMetrics(true).
		WithMetricsInterval(time.Millisecond * 100).
		Build()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 性能基准测试
	baselineResults := runPerformanceBenchmark(t, pool, "baseline", 100, time.Millisecond*10)

	// 增加负载
	heavyLoadResults := runPerformanceBenchmark(t, pool, "heavy_load", 500, time.Millisecond*20)

	// 恢复正常负载
	recoveryResults := runPerformanceBenchmark(t, pool, "recovery", 100, time.Millisecond*10)

	// 分析性能退化
	t.Logf("Performance degradation analysis:")
	t.Logf("  Baseline: %.2f tasks/sec, avg duration: %v",
		baselineResults.throughput, baselineResults.avgDuration)
	t.Logf("  Heavy load: %.2f tasks/sec, avg duration: %v",
		heavyLoadResults.throughput, heavyLoadResults.avgDuration)
	t.Logf("  Recovery: %.2f tasks/sec, avg duration: %v",
		recoveryResults.throughput, recoveryResults.avgDuration)

	// 检查性能退化
	degradationRatio := baselineResults.throughput / heavyLoadResults.throughput
	if degradationRatio > 3.0 { // 性能下降超过3倍
		t.Errorf("Significant performance degradation detected: %.2fx", degradationRatio)
	}

	// 检查恢复能力
	recoveryRatio := recoveryResults.throughput / baselineResults.throughput
	if recoveryRatio < 0.8 { // 恢复后性能低于基准的80%
		t.Errorf("Poor performance recovery: %.2f%% of baseline", recoveryRatio*100)
	}
}

// 辅助函数和结构体

// ComprehensiveTask 综合测试任务
type ComprehensiveTask struct {
	id         string
	taskType   string
	duration   time.Duration
	onComplete func()
	onError    func()
}

func (t *ComprehensiveTask) Execute(ctx context.Context) (any, error) {
	// 根据任务类型调整行为
	switch t.taskType {
	case "fast":
		time.Sleep(t.duration / 2)
	case "mixed":
		if rand.Float64() < 0.1 { // 10%概率出错
			if t.onError != nil {
				t.onError()
			}
			return nil, fmt.Errorf("mixed task error")
		}
		time.Sleep(t.duration)
	case "varied":
		// 变化的执行时间
		variation := time.Duration(rand.Int63n(int64(t.duration)))
		time.Sleep(t.duration + variation)
	case "slow":
		time.Sleep(t.duration)
	default:
		time.Sleep(t.duration)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ComprehensiveTask) Priority() int {
	return PriorityNormal
}

// ComprehensiveStressTask 压力测试任务
type ComprehensiveStressTask struct {
	id         string
	faultType  string
	onComplete func()
	onError    func()
	onPanic    func()
	onTimeout  func()
}

func (t *ComprehensiveStressTask) Execute(ctx context.Context) (any, error) {
	switch t.faultType {
	case "panic":
		if t.onPanic != nil {
			defer t.onPanic()
		}
		panic(fmt.Sprintf("stress test panic from %s", t.id))
	case "error":
		if t.onError != nil {
			t.onError()
		}
		return nil, fmt.Errorf("stress test error from %s", t.id)
	case "timeout":
		select {
		case <-time.After(time.Second * 5): // 长时间执行
			if t.onTimeout != nil {
				t.onTimeout()
			}
		case <-ctx.Done():
			if t.onTimeout != nil {
				t.onTimeout()
			}
			return nil, ctx.Err()
		}
	case "slow":
		time.Sleep(time.Millisecond * 100)
	default: // normal
		time.Sleep(time.Millisecond * 10)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ComprehensiveStressTask) Priority() int {
	return PriorityNormal
}

// LongRunningTask 长时间运行测试任务
type LongRunningTask struct {
	id         string
	taskType   string
	onComplete func()
	onError    func()
}

func (t *LongRunningTask) Execute(ctx context.Context) (any, error) {
	var duration time.Duration
	switch t.taskType {
	case "fast":
		duration = time.Millisecond * 10
	case "medium":
		duration = time.Millisecond * 50
	case "slow":
		duration = time.Millisecond * 200
	default:
		duration = time.Millisecond * 25
	}

	select {
	case <-time.After(duration):
		if t.onComplete != nil {
			t.onComplete()
		}
		return fmt.Sprintf("result-%s", t.id), nil
	case <-ctx.Done():
		if t.onError != nil {
			t.onError()
		}
		return nil, ctx.Err()
	}
}

func (t *LongRunningTask) Priority() int {
	return PriorityNormal
}

// ResourceTestTask 资源测试任务
type ResourceTestTask struct {
	id         string
	data       []byte
	onComplete func()
}

func (t *ResourceTestTask) Execute(ctx context.Context) (any, error) {
	// 模拟一些内存操作
	temp := make([]byte, len(t.data))
	copy(temp, t.data)

	time.Sleep(time.Millisecond * 10)

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ResourceTestTask) Priority() int {
	return PriorityNormal
}

// ConcurrentTask 并发测试任务
type ConcurrentTask struct {
	id         string
	taskType   string
	onComplete func()
}

func (t *ConcurrentTask) Execute(ctx context.Context) (any, error) {
	switch t.taskType {
	case "sync":
		time.Sleep(time.Millisecond * 20)
	case "async":
		time.Sleep(time.Millisecond * 30)
	case "timeout":
		time.Sleep(time.Millisecond * 100)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ConcurrentTask) Priority() int {
	return PriorityNormal
}

// ErrorRecoveryTask 错误恢复测试任务
type ErrorRecoveryTask struct {
	id         string
	errorType  string
	onComplete func()
	onError    func()
}

func (t *ErrorRecoveryTask) Execute(ctx context.Context) (any, error) {
	switch t.errorType {
	case "panic":
		if t.onError != nil {
			defer t.onError()
		}
		panic(fmt.Sprintf("recovery test panic from %s", t.id))
	case "timeout":
		select {
		case <-time.After(time.Second):
			// 超时
		case <-ctx.Done():
			if t.onError != nil {
				t.onError()
			}
			return nil, ctx.Err()
		}
	case "error":
		if t.onError != nil {
			t.onError()
		}
		return nil, fmt.Errorf("recovery test error from %s", t.id)
	case "mixed":
		r := rand.Float64()
		if r < 0.3 {
			if t.onError != nil {
				defer t.onError()
			}
			panic("mixed panic")
		} else if r < 0.6 {
			if t.onError != nil {
				t.onError()
			}
			return nil, fmt.Errorf("mixed error")
		}
		// 否则正常执行
		fallthrough
	default: // normal
		time.Sleep(time.Millisecond * 10)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ErrorRecoveryTask) Priority() int {
	return PriorityNormal
}

// ComprehensivePerformanceResult 性能测试结果
type ComprehensivePerformanceResult struct {
	throughput  float64
	avgDuration time.Duration
	taskCount   int
}

// runPerformanceBenchmark 运行性能基准测试
func runPerformanceBenchmark(t *testing.T, pool Pool, name string, taskCount int, taskDuration time.Duration) ComprehensivePerformanceResult {
	var completed int64
	var totalDuration int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		task := &ComprehensiveBenchmarkTask{
			id:       fmt.Sprintf("%s-%d", name, i),
			duration: taskDuration,
			onComplete: func(duration time.Duration) {
				atomic.AddInt64(&completed, 1)
				atomic.AddInt64(&totalDuration, int64(duration))
				wg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit benchmark task: %v", err)
			wg.Done()
		}
	}

	wg.Wait()
	elapsed := time.Since(start)

	result := ComprehensivePerformanceResult{
		throughput:  float64(completed) / elapsed.Seconds(),
		avgDuration: time.Duration(totalDuration / completed),
		taskCount:   int(completed),
	}

	t.Logf("Benchmark %s: %d tasks in %v (%.2f tasks/sec, avg duration: %v)",
		name, result.taskCount, elapsed, result.throughput, result.avgDuration)

	return result
}

// ComprehensiveBenchmarkTask 基准测试任务
type ComprehensiveBenchmarkTask struct {
	id         string
	duration   time.Duration
	onComplete func(time.Duration)
}

func (t *ComprehensiveBenchmarkTask) Execute(ctx context.Context) (any, error) {
	start := time.Now()

	select {
	case <-time.After(t.duration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if t.onComplete != nil {
		t.onComplete(time.Since(start))
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ComprehensiveBenchmarkTask) Priority() int {
	return PriorityNormal
}

// 辅助分析函数

// isMemoryGrowthConcerning 检查内存增长是否令人担忧
func isMemoryGrowthConcerning(snapshots []uint64) bool {
	if len(snapshots) < 2 {
		return false
	}

	// 检查是否持续增长
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i] <= snapshots[i-1] {
			return false // 有下降，不是持续增长
		}
	}

	// 检查增长幅度
	growth := float64(snapshots[len(snapshots)-1]) / float64(snapshots[0])
	return growth > 2.0 // 增长超过2倍
}

// analyzeMemoryStability 分析内存稳定性
func analyzeMemoryStability(t *testing.T, snapshots []uint64) {
	if len(snapshots) < 3 {
		return
	}

	// 计算统计信息
	var sum, min, max uint64
	min = snapshots[0]
	max = snapshots[0]

	for _, snapshot := range snapshots {
		sum += snapshot
		if snapshot < min {
			min = snapshot
		}
		if snapshot > max {
			max = snapshot
		}
	}

	avg := sum / uint64(len(snapshots))
	variation := float64(max-min) / float64(avg)

	t.Logf("Memory stability analysis:")
	t.Logf("  Samples: %d", len(snapshots))
	t.Logf("  Min: %d bytes", min)
	t.Logf("  Max: %d bytes", max)
	t.Logf("  Avg: %d bytes", avg)
	t.Logf("  Variation: %.2f%%", variation*100)

	if variation > 0.5 { // 50%变化
		t.Logf("WARNING: High memory variation detected")
	}
}

// analyzeGCStability 分析GC稳定性
func analyzeGCStability(t *testing.T, gcSnapshots []int64, duration time.Duration) {
	if len(gcSnapshots) < 2 {
		return
	}

	totalGCs := gcSnapshots[len(gcSnapshots)-1] - gcSnapshots[0]
	gcRate := float64(totalGCs) / duration.Minutes()

	t.Logf("GC stability analysis:")
	t.Logf("  Total GCs: %d", totalGCs)
	t.Logf("  GC rate: %.2f GCs/minute", gcRate)

	if gcRate > 60 { // 每分钟超过60次GC
		t.Logf("WARNING: High GC frequency detected")
	}
}
