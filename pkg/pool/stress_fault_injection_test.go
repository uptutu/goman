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

// TestStressFaultInjection 压力测试与故障注入
func TestStressFaultInjection(t *testing.T) {
	t.Run("extreme load test", func(t *testing.T) {
		testExtremeLoadTest(t)
	})

	t.Run("chaos engineering", func(t *testing.T) {
		testChaosEngineering(t)
	})

	t.Run("resource exhaustion", func(t *testing.T) {
		testResourceExhaustion(t)
	})

	t.Run("network partition simulation", func(t *testing.T) {
		testNetworkPartitionSimulation(t)
	})

	t.Run("memory pressure test", func(t *testing.T) {
		testMemoryPressureTest(t)
	})

	t.Run("cascading failure test", func(t *testing.T) {
		testCascadingFailureTest(t)
	})
}

// testExtremeLoadTest 极限负载测试
func testExtremeLoadTest(t *testing.T) {
	// 使用系统最大资源
	maxWorkers := runtime.NumCPU() * 4
	maxQueueSize := 2000

	config, err := NewConfigBuilder().
		WithWorkerCount(maxWorkers).
		WithQueueSize(maxQueueSize).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 极限负载参数
	testDuration := time.Minute * 3
	if testing.Short() {
		testDuration = time.Second * 30
	}

	submitterCount := runtime.NumCPU() * 2
	tasksPerSecond := 1000

	var stats struct {
		submitted       int64
		completed       int64
		failed          int64
		queueFullErrors int64
		timeouts        int64
		panics          int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// 启动多个高强度提交协程
	var wg sync.WaitGroup
	for i := 0; i < submitterCount; i++ {
		wg.Add(1)
		go func(submitterID int) {
			defer wg.Done()

			taskID := 0
			interval := time.Duration(int64(time.Second) / int64(tasksPerSecond/submitterCount))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					taskID++

					// 随机任务类型和故障注入
					taskType := []string{"cpu_intensive", "memory_intensive", "io_intensive", "mixed"}[rand.Intn(4)]
					shouldFail := rand.Float64() < 0.05 // 5%失败率

					task := &ExtremeLoadTask{
						id:         fmt.Sprintf("extreme-s%d-t%d", submitterID, taskID),
						taskType:   taskType,
						shouldFail: shouldFail,
						onComplete: func() {
							atomic.AddInt64(&stats.completed, 1)
						},
						onError: func(errType string) {
							atomic.AddInt64(&stats.failed, 1)
							switch errType {
							case "timeout":
								atomic.AddInt64(&stats.timeouts, 1)
							case "panic":
								atomic.AddInt64(&stats.panics, 1)
							}
						},
					}

					if err := pool.Submit(task); err != nil {
						if IsQueueFullError(err) {
							atomic.AddInt64(&stats.queueFullErrors, 1)
						}
						atomic.AddInt64(&stats.failed, 1)
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
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poolStats := pool.Stats()
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)

				t.Logf("Extreme load test progress:")
				t.Logf("  Submitted: %d, Completed: %d, Failed: %d",
					atomic.LoadInt64(&stats.submitted),
					atomic.LoadInt64(&stats.completed),
					atomic.LoadInt64(&stats.failed))
				t.Logf("  Queue full: %d, Timeouts: %d, Panics: %d",
					atomic.LoadInt64(&stats.queueFullErrors),
					atomic.LoadInt64(&stats.timeouts),
					atomic.LoadInt64(&stats.panics))
				t.Logf("  Pool: Active=%d, Queued=%d, Memory=%d",
					poolStats.ActiveWorkers, poolStats.QueuedTasks, poolStats.MemoryUsage)
				t.Logf("  System: Goroutines=%d, Memory=%d MB",
					runtime.NumGoroutine(), memStats.Alloc/1024/1024)
			}
		}
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 5)

	// 最终统计
	finalStats := pool.Stats()
	t.Logf("Extreme load test results:")
	t.Logf("  Duration: %v", testDuration)
	t.Logf("  Submitted: %d", stats.submitted)
	t.Logf("  Completed: %d", stats.completed)
	t.Logf("  Failed: %d", stats.failed)
	t.Logf("  Queue full errors: %d", stats.queueFullErrors)
	t.Logf("  Timeouts: %d", stats.timeouts)
	t.Logf("  Panics: %d", stats.panics)
	t.Logf("  Final pool stats: %+v", finalStats)

	// 验证系统在极限负载下的表现
	if stats.submitted == 0 {
		t.Error("No tasks were submitted during extreme load test")
	}

	if stats.completed == 0 {
		t.Error("No tasks were completed during extreme load test")
	}

	// 计算成功率
	if stats.submitted > 0 {
		successRate := float64(stats.completed) / float64(stats.submitted)
		t.Logf("Success rate under extreme load: %.2f%%", successRate*100)

		// 在极限负载下，允许较低的成功率
		if successRate < 0.7 { // 70%
			t.Errorf("Success rate too low under extreme load: %.2f%%", successRate*100)
		}
	}

	// 验证系统仍然响应
	if pool.IsRunning() {
		testTask := &ExtremeLoadTask{id: "post-extreme-test", taskType: "simple"}
		if err := pool.Submit(testTask); err != nil {
			t.Errorf("Pool should still accept tasks after extreme load: %v", err)
		}
	}
}

// testChaosEngineering 混沌工程测试
func testChaosEngineering(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(200).
		WithTaskTimeout(time.Second * 3).
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

	// 混沌事件配置
	chaosEvents := []struct {
		name        string
		probability float64
		action      func(*ChaosTask)
	}{
		{"random_panic", 0.08, func(t *ChaosTask) { t.shouldPanic = true }},
		{"random_error", 0.12, func(t *ChaosTask) { t.shouldError = true }},
		{"random_timeout", 0.10, func(t *ChaosTask) { t.shouldTimeout = true }},
		{"memory_bomb", 0.05, func(t *ChaosTask) { t.memoryBomb = true }},
		{"cpu_spike", 0.08, func(t *ChaosTask) { t.cpuSpike = true }},
		{"deadlock_risk", 0.03, func(t *ChaosTask) { t.deadlockRisk = true }},
	}

	testDuration := time.Minute * 2
	if testing.Short() {
		testDuration = time.Second * 30
	}

	var stats struct {
		totalTasks    int64
		normalTasks   int64
		chaosEvents   int64
		recovered     int64
		systemFailure int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// 正常任务流
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		taskID := 0
		ticker := time.NewTicker(time.Millisecond * 20)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				taskID++
				atomic.AddInt64(&stats.totalTasks, 1)

				task := &ChaosTask{
					id: fmt.Sprintf("chaos-%d", taskID),
					onComplete: func() {
						atomic.AddInt64(&stats.normalTasks, 1)
					},
					onChaos: func() {
						atomic.AddInt64(&stats.chaosEvents, 1)
					},
					onRecover: func() {
						atomic.AddInt64(&stats.recovered, 1)
					},
				}

				// 随机注入混沌事件
				for _, event := range chaosEvents {
					if rand.Float64() < event.probability {
						event.action(task)
						break
					}
				}

				if err := pool.Submit(task); err != nil {
					atomic.AddInt64(&stats.systemFailure, 1)
				}
			}
		}
	}()

	// 系统健康监控
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

				t.Logf("Chaos engineering progress:")
				t.Logf("  Total tasks: %d", atomic.LoadInt64(&stats.totalTasks))
				t.Logf("  Normal completion: %d", atomic.LoadInt64(&stats.normalTasks))
				t.Logf("  Chaos events: %d", atomic.LoadInt64(&stats.chaosEvents))
				t.Logf("  Recovered: %d", atomic.LoadInt64(&stats.recovered))
				t.Logf("  System failures: %d", atomic.LoadInt64(&stats.systemFailure))
				t.Logf("  Pool health: Active=%d, Queued=%d",
					poolStats.ActiveWorkers, poolStats.QueuedTasks)

				// 检查系统是否仍然健康
				if !pool.IsRunning() {
					t.Logf("WARNING: Pool stopped running during chaos test")
					return
				}
			}
		}
	}()

	wg.Wait()

	// 最终分析
	t.Logf("Chaos engineering test results:")
	t.Logf("  Duration: %v", testDuration)
	t.Logf("  Total tasks: %d", stats.totalTasks)
	t.Logf("  Normal completion: %d", stats.normalTasks)
	t.Logf("  Chaos events: %d", stats.chaosEvents)
	t.Logf("  Recovery events: %d", stats.recovered)
	t.Logf("  System failures: %d", stats.systemFailure)

	// 验证系统韧性
	if stats.totalTasks > 0 {
		resilienceRate := float64(stats.normalTasks+stats.recovered) / float64(stats.totalTasks)
		t.Logf("System resilience rate: %.2f%%", resilienceRate*100)

		if resilienceRate < 0.8 { // 80%韧性率
			t.Errorf("System resilience too low: %.2f%%", resilienceRate*100)
		}
	}

	// 验证系统最终状态
	if pool.IsRunning() {
		finalStats := pool.Stats()
		t.Logf("Final system state: %+v", finalStats)
	} else {
		t.Error("System should still be running after chaos engineering test")
	}
}

// testResourceExhaustion 资源耗尽测试
func testResourceExhaustion(t *testing.T) {
	// 创建资源受限的协程池
	config, err := NewConfigBuilder().
		WithWorkerCount(2).
		WithQueueSize(10).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var stats struct {
		submitted       int64
		completed       int64
		queueFull       int64
		resourceBlocked int64
		recovered       int64
	}

	// 阶段1: 填满队列
	t.Log("Phase 1: Filling queue to capacity")
	for i := 0; i < 20; i++ {
		task := &ResourceExhaustionTask{
			id:       fmt.Sprintf("fill-%d", i),
			duration: time.Second, // 长时间任务
			onComplete: func() {
				atomic.AddInt64(&stats.completed, 1)
			},
		}

		if err := pool.Submit(task); err != nil {
			if IsQueueFullError(err) {
				atomic.AddInt64(&stats.queueFull, 1)
			}
		} else {
			atomic.AddInt64(&stats.submitted, 1)
		}
	}

	// 等待队列填满
	time.Sleep(time.Millisecond * 100)

	// 阶段2: 持续尝试提交，测试背压处理
	t.Log("Phase 2: Testing backpressure handling")
	for i := 0; i < 50; i++ {
		task := &ResourceExhaustionTask{
			id:       fmt.Sprintf("backpressure-%d", i),
			duration: time.Millisecond * 100,
			onComplete: func() {
				atomic.AddInt64(&stats.completed, 1)
			},
		}

		if err := pool.Submit(task); err != nil {
			if IsQueueFullError(err) {
				atomic.AddInt64(&stats.queueFull, 1)
			} else {
				atomic.AddInt64(&stats.resourceBlocked, 1)
			}
		} else {
			atomic.AddInt64(&stats.submitted, 1)
		}

		// 短暂间隔
		time.Sleep(time.Millisecond * 10)
	}

	// 阶段3: 等待资源释放并测试恢复
	t.Log("Phase 3: Testing resource recovery")
	time.Sleep(time.Second * 2) // 等待长任务完成

	for i := 0; i < 20; i++ {
		task := &ResourceExhaustionTask{
			id:       fmt.Sprintf("recovery-%d", i),
			duration: time.Millisecond * 50,
			onComplete: func() {
				atomic.AddInt64(&stats.completed, 1)
				atomic.AddInt64(&stats.recovered, 1)
			},
		}

		if err := pool.Submit(task); err != nil {
			if IsQueueFullError(err) {
				atomic.AddInt64(&stats.queueFull, 1)
			}
		} else {
			atomic.AddInt64(&stats.submitted, 1)
		}
	}

	// 等待恢复阶段完成
	time.Sleep(time.Second * 2)

	// 结果分析
	t.Logf("Resource exhaustion test results:")
	t.Logf("  Submitted: %d", stats.submitted)
	t.Logf("  Completed: %d", stats.completed)
	t.Logf("  Queue full errors: %d", stats.queueFull)
	t.Logf("  Resource blocked: %d", stats.resourceBlocked)
	t.Logf("  Recovered: %d", stats.recovered)

	// 验证背压处理
	if stats.queueFull == 0 {
		t.Error("Expected queue full errors during resource exhaustion")
	}

	// 验证恢复能力
	if stats.recovered == 0 {
		t.Error("Expected some tasks to complete during recovery phase")
	}

	// 验证系统仍然可用
	if !pool.IsRunning() {
		t.Error("Pool should still be running after resource exhaustion test")
	}
}

// testNetworkPartitionSimulation 网络分区模拟测试
func testNetworkPartitionSimulation(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(6).
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var stats struct {
		normalTasks    int64
		partitionTasks int64
		timeoutTasks   int64
		recoveredTasks int64
	}

	// 模拟网络分区场景
	partitionActive := int32(0)

	// 正常任务流
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 200; i++ {
			isPartitioned := atomic.LoadInt32(&partitionActive) == 1

			task := &NetworkPartitionTask{
				id:            fmt.Sprintf("net-%d", i),
				isPartitioned: isPartitioned,
				onComplete: func(taskType string) {
					switch taskType {
					case "normal":
						atomic.AddInt64(&stats.normalTasks, 1)
					case "partition":
						atomic.AddInt64(&stats.partitionTasks, 1)
					case "recovered":
						atomic.AddInt64(&stats.recoveredTasks, 1)
					}
				},
				onTimeout: func() {
					atomic.AddInt64(&stats.timeoutTasks, 1)
				},
			}

			pool.Submit(task)
			time.Sleep(time.Millisecond * 20)
		}
	}()

	// 分区控制协程
	wg.Add(1)
	go func() {
		defer wg.Done()

		// 正常运行
		time.Sleep(time.Second * 2)

		// 激活分区
		t.Log("Activating network partition simulation")
		atomic.StoreInt32(&partitionActive, 1)
		time.Sleep(time.Second * 3)

		// 恢复网络
		t.Log("Recovering from network partition")
		atomic.StoreInt32(&partitionActive, 0)
		time.Sleep(time.Second * 2)
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second)

	t.Logf("Network partition simulation results:")
	t.Logf("  Normal tasks: %d", stats.normalTasks)
	t.Logf("  Partition tasks: %d", stats.partitionTasks)
	t.Logf("  Timeout tasks: %d", stats.timeoutTasks)
	t.Logf("  Recovered tasks: %d", stats.recoveredTasks)

	// 验证分区处理
	if stats.partitionTasks == 0 && stats.timeoutTasks == 0 {
		t.Error("Expected some partition or timeout effects")
	}

	// 验证恢复
	if stats.recoveredTasks == 0 {
		t.Error("Expected some tasks to complete after partition recovery")
	}
}

// testMemoryPressureTest 内存压力测试
func testMemoryPressureTest(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(50).
		WithTaskTimeout(time.Second * 3).
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
		memoryTasks    int64
		completedTasks int64
		failedTasks    int64
		gcCount        int64
	}

	// 内存压力测试
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)

		// 创建不同大小的内存压力任务
		memorySize := []int{1024, 10240, 102400, 1048576}[i%4] // 1KB到1MB

		task := &MemoryPressureTask{
			id:         fmt.Sprintf("mem-%d", i),
			memorySize: memorySize,
			onComplete: func() {
				atomic.AddInt64(&stats.completedTasks, 1)
				wg.Done()
			},
			onError: func() {
				atomic.AddInt64(&stats.failedTasks, 1)
				wg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			atomic.AddInt64(&stats.failedTasks, 1)
			wg.Done()
		} else {
			atomic.AddInt64(&stats.memoryTasks, 1)
		}
	}

	// 监控内存使用
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	var memorySnapshots []uint64
	for {
		select {
		case <-done:
			goto completed
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			memorySnapshots = append(memorySnapshots, memStats.Alloc)

			if len(memorySnapshots)%10 == 0 {
				t.Logf("Memory pressure: %d MB, GC: %d",
					memStats.Alloc/1024/1024, memStats.NumGC)
			}
		}
	}

completed:
	// 强制GC并获取最终状态
	runtime.GC()
	runtime.GC()

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	t.Logf("Memory pressure test results:")
	t.Logf("  Memory tasks: %d", stats.memoryTasks)
	t.Logf("  Completed: %d", stats.completedTasks)
	t.Logf("  Failed: %d", stats.failedTasks)
	t.Logf("  Initial memory: %d MB", initialMemStats.Alloc/1024/1024)
	t.Logf("  Final memory: %d MB", finalMemStats.Alloc/1024/1024)
	t.Logf("  GC count: %d", finalMemStats.NumGC-initialMemStats.NumGC)

	// 分析内存使用模式
	if len(memorySnapshots) > 0 {
		analyzeMemoryPressure(t, memorySnapshots)
	}

	// 验证内存没有严重泄漏
	memoryGrowth := int64(finalMemStats.Alloc) - int64(initialMemStats.Alloc)
	if memoryGrowth > 50*1024*1024 { // 50MB阈值
		t.Errorf("Excessive memory growth under pressure: %d MB", memoryGrowth/1024/1024)
	}
}

// testCascadingFailureTest 级联失败测试
func testCascadingFailureTest(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(6).
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var stats struct {
		normalTasks    int64
		failedTasks    int64
		cascadeTasks   int64
		recoveredTasks int64
	}

	// 级联失败模拟
	failureActive := int32(0)
	failureIntensity := int32(0) // 0=无, 1=低, 2=中, 3=高

	var wg sync.WaitGroup

	// 任务提交协程
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 300; i++ {
			intensity := atomic.LoadInt32(&failureIntensity)
			active := atomic.LoadInt32(&failureActive) == 1

			task := &CascadingFailureTask{
				id:               fmt.Sprintf("cascade-%d", i),
				failureActive:    active,
				failureIntensity: int(intensity),
				onComplete: func(taskType string) {
					switch taskType {
					case "normal":
						atomic.AddInt64(&stats.normalTasks, 1)
					case "recovered":
						atomic.AddInt64(&stats.recoveredTasks, 1)
					}
				},
				onFailure: func(taskType string) {
					switch taskType {
					case "failed":
						atomic.AddInt64(&stats.failedTasks, 1)
					case "cascade":
						atomic.AddInt64(&stats.cascadeTasks, 1)
					}
				},
			}

			pool.Submit(task)
			time.Sleep(time.Millisecond * 10)
		}
	}()

	// 级联失败控制协程
	wg.Add(1)
	go func() {
		defer wg.Done()

		// 正常运行
		time.Sleep(time.Second)

		// 触发级联失败
		t.Log("Triggering cascading failure - Low intensity")
		atomic.StoreInt32(&failureActive, 1)
		atomic.StoreInt32(&failureIntensity, 1)
		time.Sleep(time.Second)

		// 增加失败强度
		t.Log("Increasing failure intensity - Medium")
		atomic.StoreInt32(&failureIntensity, 2)
		time.Sleep(time.Second)

		// 最高强度
		t.Log("Maximum failure intensity - High")
		atomic.StoreInt32(&failureIntensity, 3)
		time.Sleep(time.Millisecond * 500)

		// 开始恢复
		t.Log("Starting recovery - Medium intensity")
		atomic.StoreInt32(&failureIntensity, 2)
		time.Sleep(time.Millisecond * 500)

		t.Log("Recovery continues - Low intensity")
		atomic.StoreInt32(&failureIntensity, 1)
		time.Sleep(time.Millisecond * 500)

		// 完全恢复
		t.Log("Full recovery")
		atomic.StoreInt32(&failureActive, 0)
		atomic.StoreInt32(&failureIntensity, 0)
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 2)

	t.Logf("Cascading failure test results:")
	t.Logf("  Normal tasks: %d", stats.normalTasks)
	t.Logf("  Failed tasks: %d", stats.failedTasks)
	t.Logf("  Cascade tasks: %d", stats.cascadeTasks)
	t.Logf("  Recovered tasks: %d", stats.recoveredTasks)

	// 验证级联失败处理
	if stats.cascadeTasks == 0 {
		t.Error("Expected some cascading failures")
	}

	// 验证系统恢复
	if stats.recoveredTasks == 0 {
		t.Error("Expected some recovery after cascading failure")
	}

	// 验证系统最终状态
	if !pool.IsRunning() {
		t.Error("Pool should still be running after cascading failure test")
	}

	finalStats := pool.Stats()
	t.Logf("Final pool state: %+v", finalStats)
}

// 测试任务类型定义

// ExtremeLoadTask 极限负载测试任务
type ExtremeLoadTask struct {
	id         string
	taskType   string
	shouldFail bool
	onComplete func()
	onError    func(string)
}

func (t *ExtremeLoadTask) Execute(ctx context.Context) (any, error) {
	if t.shouldFail && rand.Float64() < 0.5 {
		if t.onError != nil {
			t.onError("random_failure")
		}
		return nil, fmt.Errorf("random failure in task %s", t.id)
	}

	switch t.taskType {
	case "cpu_intensive":
		// CPU密集型任务
		sum := 0
		for i := 0; i < 100000; i++ {
			sum += i * i
		}
		time.Sleep(time.Millisecond * 10)
	case "memory_intensive":
		// 内存密集型任务
		data := make([][]byte, 100)
		for i := range data {
			data[i] = make([]byte, 1024)
		}
		time.Sleep(time.Millisecond * 20)
	case "io_intensive":
		// IO密集型任务模拟
		time.Sleep(time.Millisecond * 50)
	case "mixed":
		// 混合任务
		time.Sleep(time.Millisecond * time.Duration(10+rand.Intn(40)))
	default:
		time.Sleep(time.Millisecond * 5)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ExtremeLoadTask) Priority() int {
	return PriorityNormal
}

// ChaosTask 混沌工程测试任务
type ChaosTask struct {
	id            string
	shouldPanic   bool
	shouldError   bool
	shouldTimeout bool
	memoryBomb    bool
	cpuSpike      bool
	deadlockRisk  bool
	onComplete    func()
	onChaos       func()
	onRecover     func()
}

func (t *ChaosTask) Execute(ctx context.Context) (any, error) {
	defer func() {
		if r := recover(); r != nil {
			if t.onChaos != nil {
				t.onChaos()
			}
			if t.onRecover != nil {
				t.onRecover()
			}
			panic(r) // 重新抛出panic
		}
	}()

	if t.shouldPanic {
		if t.onChaos != nil {
			t.onChaos()
		}
		panic(fmt.Sprintf("chaos panic from task %s", t.id))
	}

	if t.shouldError {
		if t.onChaos != nil {
			t.onChaos()
		}
		return nil, fmt.Errorf("chaos error from task %s", t.id)
	}

	if t.shouldTimeout {
		if t.onChaos != nil {
			t.onChaos()
		}
		select {
		case <-time.After(time.Second * 10):
			// 超时
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if t.memoryBomb {
		if t.onChaos != nil {
			t.onChaos()
		}
		// 分配大量内存
		data := make([][]byte, 1000)
		for i := range data {
			data[i] = make([]byte, 10240) // 10KB each
		}
		time.Sleep(time.Millisecond * 100)
	}

	if t.cpuSpike {
		if t.onChaos != nil {
			t.onChaos()
		}
		// CPU密集计算
		sum := 0
		for i := 0; i < 1000000; i++ {
			sum += i * i * i
		}
	}

	if t.deadlockRisk {
		if t.onChaos != nil {
			t.onChaos()
		}
		// 模拟死锁风险（长时间持有资源）
		var mu sync.Mutex
		mu.Lock()
		time.Sleep(time.Millisecond * 100)
		mu.Unlock()
	}

	// 正常执行
	time.Sleep(time.Millisecond * 10)

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ChaosTask) Priority() int {
	return PriorityNormal
}

// ResourceExhaustionTask 资源耗尽测试任务
type ResourceExhaustionTask struct {
	id         string
	duration   time.Duration
	onComplete func()
}

func (t *ResourceExhaustionTask) Execute(ctx context.Context) (any, error) {
	select {
	case <-time.After(t.duration):
		if t.onComplete != nil {
			t.onComplete()
		}
		return fmt.Sprintf("result-%s", t.id), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *ResourceExhaustionTask) Priority() int {
	return PriorityNormal
}

// NetworkPartitionTask 网络分区测试任务
type NetworkPartitionTask struct {
	id            string
	isPartitioned bool
	onComplete    func(string)
	onTimeout     func()
}

func (t *NetworkPartitionTask) Execute(ctx context.Context) (any, error) {
	if t.isPartitioned {
		// 模拟网络分区导致的延迟和超时
		select {
		case <-time.After(time.Second * 3): // 长延迟
			if t.onComplete != nil {
				t.onComplete("partition")
			}
		case <-ctx.Done():
			if t.onTimeout != nil {
				t.onTimeout()
			}
			return nil, ctx.Err()
		}
	} else {
		// 正常执行
		time.Sleep(time.Millisecond * 20)
		taskType := "normal"
		if rand.Float64() < 0.3 { // 30%概率是恢复任务
			taskType = "recovered"
		}
		if t.onComplete != nil {
			t.onComplete(taskType)
		}
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *NetworkPartitionTask) Priority() int {
	return PriorityNormal
}

// MemoryPressureTask 内存压力测试任务
type MemoryPressureTask struct {
	id         string
	memorySize int
	onComplete func()
	onError    func()
}

func (t *MemoryPressureTask) Execute(ctx context.Context) (any, error) {
	// 分配指定大小的内存
	data := make([]byte, t.memorySize)

	// 使用内存以防止优化
	for i := 0; i < len(data); i += 1024 {
		data[i] = byte(i % 256)
	}

	// 持有内存一段时间
	time.Sleep(time.Millisecond * 100)

	// 模拟内存操作
	temp := make([]byte, t.memorySize/2)
	copy(temp, data[:len(temp)])

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *MemoryPressureTask) Priority() int {
	return PriorityNormal
}

// CascadingFailureTask 级联失败测试任务
type CascadingFailureTask struct {
	id               string
	failureActive    bool
	failureIntensity int
	onComplete       func(string)
	onFailure        func(string)
}

func (t *CascadingFailureTask) Execute(ctx context.Context) (any, error) {
	if t.failureActive {
		// 根据失败强度决定失败概率
		failureProbability := float64(t.failureIntensity) * 0.2 // 20%, 40%, 60%

		if rand.Float64() < failureProbability {
			// 决定失败类型
			if rand.Float64() < 0.3 {
				// 级联失败
				if t.onFailure != nil {
					t.onFailure("cascade")
				}
				return nil, fmt.Errorf("cascading failure from %s", t.id)
			} else {
				// 普通失败
				if t.onFailure != nil {
					t.onFailure("failed")
				}
				return nil, fmt.Errorf("failure from %s", t.id)
			}
		}
	}

	// 正常执行或恢复
	time.Sleep(time.Millisecond * 20)

	taskType := "normal"
	if t.failureActive && rand.Float64() < 0.2 {
		taskType = "recovered"
	}

	if t.onComplete != nil {
		t.onComplete(taskType)
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *CascadingFailureTask) Priority() int {
	return PriorityNormal
}

// 辅助分析函数

// analyzeMemoryPressure 分析内存压力模式
func analyzeMemoryPressure(t *testing.T, snapshots []uint64) {
	if len(snapshots) < 3 {
		return
	}

	var max, min uint64
	max = snapshots[0]
	min = snapshots[0]

	for _, snapshot := range snapshots {
		if snapshot > max {
			max = snapshot
		}
		if snapshot < min {
			min = snapshot
		}
	}

	// 计算内存压力指标
	pressureRatio := float64(max) / float64(min)

	t.Logf("Memory pressure analysis:")
	t.Logf("  Peak memory: %d MB", max/1024/1024)
	t.Logf("  Low memory: %d MB", min/1024/1024)
	t.Logf("  Pressure ratio: %.2fx", pressureRatio)

	if pressureRatio > 10.0 {
		t.Logf("WARNING: High memory pressure detected")
	}
}
