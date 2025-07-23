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

// TestFinalIntegrationSuite 最终集成测试套件
func TestFinalIntegrationSuite(t *testing.T) {
	t.Run("complete system integration", func(t *testing.T) {
		testCompleteSystemIntegration(t)
	})

	t.Run("production simulation", func(t *testing.T) {
		testProductionSimulation(t)
	})

	t.Run("disaster recovery", func(t *testing.T) {
		testDisasterRecovery(t)
	})

	t.Run("performance regression", func(t *testing.T) {
		testPerformanceRegression(t)
	})

	t.Run("resource lifecycle validation", func(t *testing.T) {
		testResourceLifecycleValidation(t)
	})
}

// testCompleteSystemIntegration 完整系统集成测试
func testCompleteSystemIntegration(t *testing.T) {
	t.Log("Starting complete system integration test")

	// 系统配置
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU()).
		WithQueueSize(500).
		WithTaskTimeout(time.Second * 5).
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

	// 系统状态跟踪
	var systemStats struct {
		totalPhases       int
		completedPhases   int
		totalTasks        int64
		completedTasks    int64
		failedTasks       int64
		systemErrors      int64
		recoveryEvents    int64
		performanceIssues int64
	}

	// 阶段1: 系统初始化验证
	t.Log("Phase 1: System initialization validation")
	systemStats.totalPhases++

	if !pool.IsRunning() {
		t.Error("Pool should be running after initialization")
		atomic.AddInt64(&systemStats.systemErrors, 1)
	}

	initialStats := pool.Stats()
	if initialStats.ActiveWorkers < 0 || initialStats.QueuedTasks < 0 {
		t.Error("Invalid initial statistics")
		atomic.AddInt64(&systemStats.systemErrors, 1)
	}
	systemStats.completedPhases++

	// 阶段2: 基础功能验证
	t.Log("Phase 2: Basic functionality validation")
	systemStats.totalPhases++

	basicTaskCount := 100
	var basicWg sync.WaitGroup
	var basicCompleted int64

	for i := 0; i < basicTaskCount; i++ {
		basicWg.Add(1)
		task := &SystemIntegrationTask{
			id:       fmt.Sprintf("basic-%d", i),
			taskType: "basic",
			duration: time.Millisecond * 50,
			onComplete: func() {
				atomic.AddInt64(&basicCompleted, 1)
				atomic.AddInt64(&systemStats.completedTasks, 1)
				basicWg.Done()
			},
			onError: func() {
				atomic.AddInt64(&systemStats.failedTasks, 1)
				basicWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit basic task: %v", err)
			atomic.AddInt64(&systemStats.systemErrors, 1)
			basicWg.Done()
		} else {
			atomic.AddInt64(&systemStats.totalTasks, 1)
		}
	}

	basicWg.Wait()
	if basicCompleted != int64(basicTaskCount) {
		t.Errorf("Basic functionality failed: expected %d, got %d", basicTaskCount, basicCompleted)
		atomic.AddInt64(&systemStats.systemErrors, 1)
	} else {
		systemStats.completedPhases++
	}

	// 阶段3: 并发压力测试
	t.Log("Phase 3: Concurrent stress testing")
	systemStats.totalPhases++

	concurrentGoroutines := 20
	tasksPerGoroutine := 50
	var concurrentWg sync.WaitGroup
	var concurrentCompleted int64

	for g := 0; g < concurrentGoroutines; g++ {
		concurrentWg.Add(1)
		go func(goroutineID int) {
			defer concurrentWg.Done()

			for i := 0; i < tasksPerGoroutine; i++ {
				task := &SystemIntegrationTask{
					id:       fmt.Sprintf("concurrent-g%d-t%d", goroutineID, i),
					taskType: "concurrent",
					duration: time.Millisecond * time.Duration(10+i%40),
					onComplete: func() {
						atomic.AddInt64(&concurrentCompleted, 1)
						atomic.AddInt64(&systemStats.completedTasks, 1)
					},
					onError: func() {
						atomic.AddInt64(&systemStats.failedTasks, 1)
					},
				}

				if err := pool.Submit(task); err != nil {
					atomic.AddInt64(&systemStats.systemErrors, 1)
				} else {
					atomic.AddInt64(&systemStats.totalTasks, 1)
				}
			}
		}(g)
	}

	concurrentWg.Wait()
	expectedConcurrent := int64(concurrentGoroutines * tasksPerGoroutine)
	if concurrentCompleted < expectedConcurrent*8/10 { // 允许20%失败
		t.Errorf("Concurrent stress test failed: expected at least %d, got %d",
			expectedConcurrent*8/10, concurrentCompleted)
		atomic.AddInt64(&systemStats.performanceIssues, 1)
	} else {
		systemStats.completedPhases++
	}

	// 阶段4: 错误恢复测试
	t.Log("Phase 4: Error recovery testing")
	systemStats.totalPhases++

	errorTaskCount := 30
	var errorWg sync.WaitGroup
	var errorRecovered int64

	for i := 0; i < errorTaskCount; i++ {
		errorWg.Add(1)

		var taskType string
		switch i % 3 {
		case 0:
			taskType = "panic"
		case 1:
			taskType = "error"
		case 2:
			taskType = "timeout"
		}

		task := &SystemIntegrationTask{
			id:       fmt.Sprintf("error-%d", i),
			taskType: taskType,
			duration: time.Millisecond * 100,
			onComplete: func() {
				atomic.AddInt64(&errorRecovered, 1)
				atomic.AddInt64(&systemStats.recoveryEvents, 1)
				atomic.AddInt64(&systemStats.completedTasks, 1)
				errorWg.Done()
			},
			onError: func() {
				atomic.AddInt64(&systemStats.failedTasks, 1)
				errorWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			atomic.AddInt64(&systemStats.systemErrors, 1)
			errorWg.Done()
		} else {
			atomic.AddInt64(&systemStats.totalTasks, 1)
		}
	}

	errorWg.Wait()

	// 验证系统仍然可用
	testTask := &SystemIntegrationTask{
		id:       "post-error-test",
		taskType: "basic",
		duration: time.Millisecond * 10,
	}

	if err := pool.Submit(testTask); err != nil {
		t.Error("System should still be functional after error recovery test")
		atomic.AddInt64(&systemStats.systemErrors, 1)
	} else {
		systemStats.completedPhases++
	}

	// 阶段5: 资源清理验证
	t.Log("Phase 5: Resource cleanup validation")
	systemStats.totalPhases++

	// 记录关闭前状态
	preShutdownStats := pool.Stats()
	preShutdownGoroutines := runtime.NumGoroutine()

	// 优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	shutdownStart := time.Now()
	if err := pool.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
		atomic.AddInt64(&systemStats.systemErrors, 1)
	}
	shutdownDuration := time.Since(shutdownStart)

	// 验证关闭状态
	if !pool.IsClosed() {
		t.Error("Pool should be closed after shutdown")
		atomic.AddInt64(&systemStats.systemErrors, 1)
	}

	// 等待资源清理
	time.Sleep(time.Second)
	runtime.GC()

	postShutdownGoroutines := runtime.NumGoroutine()
	goroutineLeak := postShutdownGoroutines - preShutdownGoroutines

	if goroutineLeak > 5 { // 允许少量协程增长
		t.Errorf("Possible goroutine leak: %d", goroutineLeak)
		atomic.AddInt64(&systemStats.systemErrors, 1)
	} else {
		systemStats.completedPhases++
	}

	// 最终系统报告
	t.Logf("Complete system integration test results:")
	t.Logf("  Phases completed: %d/%d", systemStats.completedPhases, systemStats.totalPhases)
	t.Logf("  Total tasks: %d", systemStats.totalTasks)
	t.Logf("  Completed tasks: %d", systemStats.completedTasks)
	t.Logf("  Failed tasks: %d", systemStats.failedTasks)
	t.Logf("  System errors: %d", systemStats.systemErrors)
	t.Logf("  Recovery events: %d", systemStats.recoveryEvents)
	t.Logf("  Performance issues: %d", systemStats.performanceIssues)
	t.Logf("  Shutdown duration: %v", shutdownDuration)
	t.Logf("  Pre-shutdown stats: %+v", preShutdownStats)
	t.Logf("  Goroutine leak: %d", goroutineLeak)

	// 系统健康评估
	healthScore := float64(systemStats.completedPhases) / float64(systemStats.totalPhases)
	t.Logf("System health score: %.2f%%", healthScore*100)

	if healthScore < 0.8 { // 80%健康阈值
		t.Errorf("System health score too low: %.2f%%", healthScore*100)
	}

	if systemStats.systemErrors > 5 {
		t.Errorf("Too many system errors: %d", systemStats.systemErrors)
	}
}

// testProductionSimulation 生产环境模拟测试
func testProductionSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping production simulation in short mode")
	}

	t.Log("Starting production environment simulation")

	// 生产级配置
	config, err := NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU() * 2).
		WithQueueSize(1000).
		WithTaskTimeout(time.Second * 10).
		WithShutdownTimeout(time.Second * 30).
		WithMetrics(true).
		WithMetricsInterval(time.Second).
		Build()
	if err != nil {
		t.Fatalf("Failed to create production config: %v", err)
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create production pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 生产模拟参数
	simulationDuration := time.Minute * 5 // 5分钟生产模拟
	if testing.Short() {
		simulationDuration = time.Minute // 短模式1分钟
	}

	var prodStats struct {
		totalRequests     int64
		successfulReqs    int64
		failedReqs        int64
		timeoutReqs       int64
		peakMemory        uint64
		avgResponseTime   int64
		maxResponseTime   int64
		throughputSamples []float64
	}

	ctx, cancel := context.WithTimeout(context.Background(), simulationDuration)
	defer cancel()

	// 模拟不同类型的生产负载
	var wg sync.WaitGroup

	// Web请求模拟
	wg.Add(1)
	go func() {
		defer wg.Done()

		requestID := 0
		ticker := time.NewTicker(time.Millisecond * 50) // 20 RPS
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				requestID++

				task := &ProductionTask{
					id:       fmt.Sprintf("web-req-%d", requestID),
					taskType: "web_request",
					duration: time.Millisecond * time.Duration(50+requestID%200),
					onComplete: func(duration time.Duration) {
						atomic.AddInt64(&prodStats.successfulReqs, 1)

						// 更新响应时间统计
						durationNs := int64(duration)
						atomic.AddInt64(&prodStats.avgResponseTime, durationNs)

						for {
							current := atomic.LoadInt64(&prodStats.maxResponseTime)
							if durationNs <= current || atomic.CompareAndSwapInt64(&prodStats.maxResponseTime, current, durationNs) {
								break
							}
						}
					},
					onError: func() {
						atomic.AddInt64(&prodStats.failedReqs, 1)
					},
					onTimeout: func() {
						atomic.AddInt64(&prodStats.timeoutReqs, 1)
					},
				}

				if err := pool.Submit(task); err != nil {
					atomic.AddInt64(&prodStats.failedReqs, 1)
				} else {
					atomic.AddInt64(&prodStats.totalRequests, 1)
				}
			}
		}
	}()

	// 批处理任务模拟
	wg.Add(1)
	go func() {
		defer wg.Done()

		batchID := 0
		ticker := time.NewTicker(time.Second * 5) // 每5秒一个批处理
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				batchID++

				// 提交批处理任务
				for i := 0; i < 10; i++ {
					task := &ProductionTask{
						id:       fmt.Sprintf("batch-%d-item-%d", batchID, i),
						taskType: "batch_processing",
						duration: time.Millisecond * 500,
						onComplete: func(duration time.Duration) {
							atomic.AddInt64(&prodStats.successfulReqs, 1)
						},
						onError: func() {
							atomic.AddInt64(&prodStats.failedReqs, 1)
						},
					}

					if err := pool.Submit(task); err != nil {
						atomic.AddInt64(&prodStats.failedReqs, 1)
					} else {
						atomic.AddInt64(&prodStats.totalRequests, 1)
					}
				}
			}
		}
	}()

	// 系统监控
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

				// 记录峰值内存
				if memStats.Alloc > prodStats.peakMemory {
					atomic.StoreUint64(&prodStats.peakMemory, memStats.Alloc)
				}

				// 计算吞吐量
				throughput := float64(poolStats.CompletedTasks) / time.Since(time.Now().Add(-simulationDuration)).Seconds()
				prodStats.throughputSamples = append(prodStats.throughputSamples, throughput)

				t.Logf("Production simulation status:")
				t.Logf("  Requests: total=%d, success=%d, failed=%d, timeout=%d",
					atomic.LoadInt64(&prodStats.totalRequests),
					atomic.LoadInt64(&prodStats.successfulReqs),
					atomic.LoadInt64(&prodStats.failedReqs),
					atomic.LoadInt64(&prodStats.timeoutReqs))
				t.Logf("  Pool: active=%d, queued=%d, completed=%d",
					poolStats.ActiveWorkers, poolStats.QueuedTasks, poolStats.CompletedTasks)
				t.Logf("  Memory: current=%d MB, peak=%d MB",
					memStats.Alloc/1024/1024, prodStats.peakMemory/1024/1024)
			}
		}
	}()

	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 3)

	// 生产模拟结果分析
	finalStats := pool.Stats()

	// 计算平均响应时间
	avgResponseTime := time.Duration(0)
	if prodStats.successfulReqs > 0 {
		avgResponseTime = time.Duration(prodStats.avgResponseTime / prodStats.successfulReqs)
	}

	// 计算平均吞吐量
	avgThroughput := 0.0
	if len(prodStats.throughputSamples) > 0 {
		sum := 0.0
		for _, sample := range prodStats.throughputSamples {
			sum += sample
		}
		avgThroughput = sum / float64(len(prodStats.throughputSamples))
	}

	t.Logf("Production simulation results:")
	t.Logf("  Duration: %v", simulationDuration)
	t.Logf("  Total requests: %d", prodStats.totalRequests)
	t.Logf("  Successful: %d", prodStats.successfulReqs)
	t.Logf("  Failed: %d", prodStats.failedReqs)
	t.Logf("  Timeouts: %d", prodStats.timeoutReqs)
	t.Logf("  Success rate: %.2f%%", float64(prodStats.successfulReqs)/float64(prodStats.totalRequests)*100)
	t.Logf("  Average response time: %v", avgResponseTime)
	t.Logf("  Max response time: %v", time.Duration(prodStats.maxResponseTime))
	t.Logf("  Average throughput: %.2f tasks/sec", avgThroughput)
	t.Logf("  Peak memory usage: %d MB", prodStats.peakMemory/1024/1024)
	t.Logf("  Final pool stats: %+v", finalStats)

	// 生产级别验证
	successRate := float64(prodStats.successfulReqs) / float64(prodStats.totalRequests)
	if successRate < 0.95 { // 95%成功率
		t.Errorf("Production success rate too low: %.2f%%", successRate*100)
	}

	if avgResponseTime > time.Second { // 1秒响应时间阈值
		t.Errorf("Average response time too high: %v", avgResponseTime)
	}

	if prodStats.peakMemory > 500*1024*1024 { // 500MB内存阈值
		t.Errorf("Peak memory usage too high: %d MB", prodStats.peakMemory/1024/1024)
	}
}

// testDisasterRecovery 灾难恢复测试
func testDisasterRecovery(t *testing.T) {
	t.Log("Starting disaster recovery test")

	config, err := NewConfigBuilder().
		WithWorkerCount(6).
		WithQueueSize(100).
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

	var disasterStats struct {
		preDisasterTasks  int64
		duringDisaster    int64
		postRecoveryTasks int64
		totalRecovered    int64
		systemFailures    int64
	}

	// 阶段1: 正常运行建立基线
	t.Log("Phase 1: Establishing baseline")
	baselineTasks := 50
	var baselineWg sync.WaitGroup

	for i := 0; i < baselineTasks; i++ {
		baselineWg.Add(1)
		task := &DisasterRecoveryTask{
			id:       fmt.Sprintf("baseline-%d", i),
			taskType: "normal",
			onComplete: func() {
				atomic.AddInt64(&disasterStats.preDisasterTasks, 1)
				baselineWg.Done()
			},
			onError: func() {
				baselineWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit baseline task: %v", err)
			baselineWg.Done()
		}
	}

	baselineWg.Wait()
	t.Logf("Baseline established: %d tasks completed", disasterStats.preDisasterTasks)

	// 阶段2: 模拟灾难场景
	t.Log("Phase 2: Simulating disaster scenarios")

	// 灾难场景1: 大量panic任务
	panicTasks := 20
	var panicWg sync.WaitGroup

	for i := 0; i < panicTasks; i++ {
		panicWg.Add(1)
		task := &DisasterRecoveryTask{
			id:       fmt.Sprintf("panic-%d", i),
			taskType: "panic",
			onComplete: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				panicWg.Done()
			},
			onError: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				panicWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			atomic.AddInt64(&disasterStats.systemFailures, 1)
			panicWg.Done()
		}
	}

	panicWg.Wait()

	// 验证系统仍然响应
	if !pool.IsRunning() {
		t.Error("Pool should still be running after panic disaster")
		atomic.AddInt64(&disasterStats.systemFailures, 1)
	}

	// 灾难场景2: 资源耗尽
	t.Log("Disaster scenario 2: Resource exhaustion")
	exhaustionTasks := 30
	var exhaustionWg sync.WaitGroup

	for i := 0; i < exhaustionTasks; i++ {
		exhaustionWg.Add(1)
		task := &DisasterRecoveryTask{
			id:       fmt.Sprintf("exhaustion-%d", i),
			taskType: "resource_exhaustion",
			onComplete: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				exhaustionWg.Done()
			},
			onError: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				exhaustionWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			atomic.AddInt64(&disasterStats.systemFailures, 1)
			exhaustionWg.Done()
		}
	}

	exhaustionWg.Wait()

	// 灾难场景3: 级联超时
	t.Log("Disaster scenario 3: Cascading timeouts")
	timeoutTasks := 25
	var timeoutWg sync.WaitGroup

	for i := 0; i < timeoutTasks; i++ {
		timeoutWg.Add(1)
		task := &DisasterRecoveryTask{
			id:       fmt.Sprintf("timeout-%d", i),
			taskType: "timeout",
			onComplete: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				timeoutWg.Done()
			},
			onError: func() {
				atomic.AddInt64(&disasterStats.duringDisaster, 1)
				timeoutWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			atomic.AddInt64(&disasterStats.systemFailures, 1)
			timeoutWg.Done()
		}
	}

	timeoutWg.Wait()

	// 阶段3: 恢复验证
	t.Log("Phase 3: Recovery validation")

	// 等待系统稳定
	time.Sleep(time.Second * 2)

	// 提交恢复任务
	recoveryTasks := 40
	var recoveryWg sync.WaitGroup

	for i := 0; i < recoveryTasks; i++ {
		recoveryWg.Add(1)
		task := &DisasterRecoveryTask{
			id:       fmt.Sprintf("recovery-%d", i),
			taskType: "normal",
			onComplete: func() {
				atomic.AddInt64(&disasterStats.postRecoveryTasks, 1)
				atomic.AddInt64(&disasterStats.totalRecovered, 1)
				recoveryWg.Done()
			},
			onError: func() {
				recoveryWg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit recovery task: %v", err)
			atomic.AddInt64(&disasterStats.systemFailures, 1)
			recoveryWg.Done()
		}
	}

	recoveryWg.Wait()

	// 灾难恢复结果分析
	t.Logf("Disaster recovery test results:")
	t.Logf("  Pre-disaster tasks: %d", disasterStats.preDisasterTasks)
	t.Logf("  During disaster: %d", disasterStats.duringDisaster)
	t.Logf("  Post-recovery tasks: %d", disasterStats.postRecoveryTasks)
	t.Logf("  Total recovered: %d", disasterStats.totalRecovered)
	t.Logf("  System failures: %d", disasterStats.systemFailures)

	// 恢复率计算
	recoveryRate := float64(disasterStats.postRecoveryTasks) / float64(recoveryTasks)
	t.Logf("Recovery rate: %.2f%%", recoveryRate*100)

	// 验证恢复能力
	if recoveryRate < 0.9 { // 90%恢复率
		t.Errorf("Recovery rate too low: %.2f%%", recoveryRate*100)
	}

	if disasterStats.systemFailures > 10 {
		t.Errorf("Too many system failures during disaster: %d", disasterStats.systemFailures)
	}

	// 最终系统健康检查
	if !pool.IsRunning() {
		t.Error("Pool should be running after disaster recovery")
	}

	finalStats := pool.Stats()
	t.Logf("Final system state: %+v", finalStats)
}

// testPerformanceRegression 性能回归测试
func testPerformanceRegression(t *testing.T) {
	t.Log("Starting performance regression test")

	// 基准配置
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(200).
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

	// 性能基准测试
	benchmarks := []struct {
		name      string
		taskCount int
		taskType  string
		duration  time.Duration
	}{
		{"lightweight", 1000, "light", time.Microsecond * 100},
		{"medium", 500, "medium", time.Millisecond * 10},
		{"heavy", 100, "heavy", time.Millisecond * 100},
		{"mixed", 300, "mixed", time.Millisecond * 50},
	}

	var regressionResults []FinalPerformanceResult

	for _, benchmark := range benchmarks {
		t.Logf("Running benchmark: %s", benchmark.name)

		result := runFinalPerformanceBenchmark(t, pool, benchmark.name, benchmark.taskCount, benchmark.taskType, benchmark.duration)
		regressionResults = append(regressionResults, result)

		t.Logf("Benchmark %s results:", benchmark.name)
		t.Logf("  Tasks: %d", result.TaskCount)
		t.Logf("  Duration: %v", result.TotalDuration)
		t.Logf("  Throughput: %.2f tasks/sec", result.Throughput)
		t.Logf("  Avg task time: %v", result.AvgTaskDuration)
		t.Logf("  Memory usage: %d bytes", result.MemoryUsage)

		// 短暂休息让系统稳定
		time.Sleep(time.Millisecond * 500)
	}

	// 性能回归分析
	t.Log("Performance regression analysis:")

	// 预期性能阈值（这些值应该基于历史基准调整）
	expectedThresholds := map[string]struct {
		minThroughput float64
		maxAvgTime    time.Duration
		maxMemory     uint64
	}{
		"lightweight": {500.0, time.Millisecond * 5, 10 * 1024 * 1024},  // 500 TPS, 5ms, 10MB
		"medium":      {200.0, time.Millisecond * 20, 20 * 1024 * 1024}, // 200 TPS, 20ms, 20MB
		"heavy":       {50.0, time.Millisecond * 150, 50 * 1024 * 1024}, // 50 TPS, 150ms, 50MB
		"mixed":       {100.0, time.Millisecond * 80, 30 * 1024 * 1024}, // 100 TPS, 80ms, 30MB
	}

	regressionCount := 0
	for _, result := range regressionResults {
		threshold, exists := expectedThresholds[result.BenchmarkName]
		if !exists {
			continue
		}

		t.Logf("Analyzing %s performance:", result.BenchmarkName)

		if result.Throughput < threshold.minThroughput {
			t.Errorf("Throughput regression in %s: %.2f < %.2f TPS",
				result.BenchmarkName, result.Throughput, threshold.minThroughput)
			regressionCount++
		}

		if result.AvgTaskDuration > threshold.maxAvgTime {
			t.Errorf("Response time regression in %s: %v > %v",
				result.BenchmarkName, result.AvgTaskDuration, threshold.maxAvgTime)
			regressionCount++
		}

		if result.MemoryUsage > threshold.maxMemory {
			t.Errorf("Memory usage regression in %s: %d > %d bytes",
				result.BenchmarkName, result.MemoryUsage, threshold.maxMemory)
			regressionCount++
		}
	}

	t.Logf("Performance regression summary:")
	t.Logf("  Total benchmarks: %d", len(regressionResults))
	t.Logf("  Regressions detected: %d", regressionCount)

	if regressionCount > 0 {
		t.Errorf("Performance regressions detected: %d", regressionCount)
	}
}

// testResourceLifecycleValidation 资源生命周期验证测试
func testResourceLifecycleValidation(t *testing.T) {
	t.Log("Starting resource lifecycle validation test")

	// 记录初始系统状态
	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)
	initialGoroutines := runtime.NumGoroutine()

	t.Logf("Initial system state:")
	t.Logf("  Memory: %d bytes", initialMemStats.Alloc)
	t.Logf("  Goroutines: %d", initialGoroutines)

	// 多轮资源生命周期测试
	cycles := 5
	poolsPerCycle := 4

	var lifecycleStats struct {
		totalPools     int
		successfulOps  int
		failedOps      int
		memoryLeaks    int
		goroutineLeaks int
	}

	for cycle := 0; cycle < cycles; cycle++ {
		t.Logf("Resource lifecycle cycle %d/%d", cycle+1, cycles)

		var cyclePools []Pool
		var cycleMemStats []runtime.MemStats

		// 创建多个协程池
		for p := 0; p < poolsPerCycle; p++ {
			config, err := NewConfigBuilder().
				WithWorkerCount(3).
				WithQueueSize(50).
				WithTaskTimeout(time.Second).
				Build()
			if err != nil {
				t.Errorf("Failed to create config in cycle %d: %v", cycle, err)
				lifecycleStats.failedOps++
				continue
			}

			pool, err := NewPool(config)
			if err != nil {
				t.Errorf("Failed to create pool in cycle %d: %v", cycle, err)
				lifecycleStats.failedOps++
				continue
			}

			cyclePools = append(cyclePools, pool)
			lifecycleStats.totalPools++

			// 向每个池提交任务
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				task := &ResourceLifecycleTask{
					id:   fmt.Sprintf("c%d-p%d-t%d", cycle, p, i),
					data: make([]byte, 1024), // 1KB数据
					onComplete: func() {
						wg.Done()
					},
					onError: func() {
						wg.Done()
					},
				}

				if err := pool.Submit(task); err != nil {
					lifecycleStats.failedOps++
					wg.Done()
				} else {
					lifecycleStats.successfulOps++
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
				t.Errorf("Timeout waiting for tasks in cycle %d pool %d", cycle, p)
				lifecycleStats.failedOps++
			}

			// 记录内存状态
			runtime.GC()
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			cycleMemStats = append(cycleMemStats, memStats)
		}

		// 关闭所有协程池
		for p, pool := range cyclePools {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			if err := pool.Shutdown(ctx); err != nil {
				t.Errorf("Failed to shutdown pool in cycle %d: %v", cycle, err)
				lifecycleStats.failedOps++
			}
			cancel()

			if !pool.IsClosed() {
				t.Errorf("Pool should be closed after shutdown in cycle %d pool %d", cycle, p)
				lifecycleStats.failedOps++
			}
		}

		// 强制GC并检查资源释放
		runtime.GC()
		runtime.GC()
		time.Sleep(time.Millisecond * 200)

		var cycleEndMemStats runtime.MemStats
		runtime.ReadMemStats(&cycleEndMemStats)
		cycleEndGoroutines := runtime.NumGoroutine()

		// 检查内存泄漏
		memoryGrowth := int64(cycleEndMemStats.Alloc) - int64(initialMemStats.Alloc)
		if memoryGrowth > int64(cycle+1)*5*1024*1024 { // 每轮允许5MB增长
			t.Logf("Possible memory leak in cycle %d: %d bytes growth", cycle, memoryGrowth)
			lifecycleStats.memoryLeaks++
		}

		// 检查协程泄漏
		goroutineGrowth := cycleEndGoroutines - initialGoroutines
		if goroutineGrowth > (cycle+1)*3 { // 每轮允许3个协程增长
			t.Logf("Possible goroutine leak in cycle %d: %d goroutines growth", cycle, goroutineGrowth)
			lifecycleStats.goroutineLeaks++
		}

		t.Logf("Cycle %d completed: Memory=%d bytes, Goroutines=%d",
			cycle, cycleEndMemStats.Alloc, cycleEndGoroutines)
	}

	// 最终资源状态检查
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Second)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	finalGoroutines := runtime.NumGoroutine()

	finalMemoryGrowth := int64(finalMemStats.Alloc) - int64(initialMemStats.Alloc)
	finalGoroutineGrowth := finalGoroutines - initialGoroutines

	t.Logf("Resource lifecycle validation results:")
	t.Logf("  Cycles completed: %d", cycles)
	t.Logf("  Total pools created: %d", lifecycleStats.totalPools)
	t.Logf("  Successful operations: %d", lifecycleStats.successfulOps)
	t.Logf("  Failed operations: %d", lifecycleStats.failedOps)
	t.Logf("  Memory leaks detected: %d", lifecycleStats.memoryLeaks)
	t.Logf("  Goroutine leaks detected: %d", lifecycleStats.goroutineLeaks)
	t.Logf("  Final memory growth: %d bytes", finalMemoryGrowth)
	t.Logf("  Final goroutine growth: %d", finalGoroutineGrowth)

	// 验证资源生命周期管理
	if lifecycleStats.memoryLeaks > 1 {
		t.Errorf("Too many memory leaks detected: %d", lifecycleStats.memoryLeaks)
	}

	if lifecycleStats.goroutineLeaks > 1 {
		t.Errorf("Too many goroutine leaks detected: %d", lifecycleStats.goroutineLeaks)
	}

	if finalMemoryGrowth > 20*1024*1024 { // 20MB最终增长阈值
		t.Errorf("Excessive final memory growth: %d bytes", finalMemoryGrowth)
	}

	if finalGoroutineGrowth > 10 { // 10个协程最终增长阈值
		t.Errorf("Excessive final goroutine growth: %d", finalGoroutineGrowth)
	}

	// 成功率验证
	if lifecycleStats.totalPools > 0 {
		successRate := float64(lifecycleStats.successfulOps) / float64(lifecycleStats.successfulOps+lifecycleStats.failedOps)
		t.Logf("Operation success rate: %.2f%%", successRate*100)

		if successRate < 0.95 { // 95%成功率
			t.Errorf("Operation success rate too low: %.2f%%", successRate*100)
		}
	}
}

// 测试任务类型定义

// SystemIntegrationTask 系统集成测试任务
type SystemIntegrationTask struct {
	id         string
	taskType   string
	duration   time.Duration
	onComplete func()
	onError    func()
}

func (t *SystemIntegrationTask) Execute(ctx context.Context) (any, error) {
	switch t.taskType {
	case "panic":
		panic(fmt.Sprintf("test panic from %s", t.id))
	case "error":
		if t.onError != nil {
			t.onError()
		}
		return nil, fmt.Errorf("test error from %s", t.id)
	case "timeout":
		select {
		case <-time.After(time.Second * 10): // 长时间执行
		case <-ctx.Done():
			if t.onError != nil {
				t.onError()
			}
			return nil, ctx.Err()
		}
	default: // basic, concurrent
		if t.duration > 0 {
			select {
			case <-time.After(t.duration):
			case <-ctx.Done():
				if t.onError != nil {
					t.onError()
				}
				return nil, ctx.Err()
			}
		}
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *SystemIntegrationTask) Priority() int {
	return PriorityNormal
}

// ProductionTask 生产环境测试任务
type ProductionTask struct {
	id         string
	taskType   string
	duration   time.Duration
	onComplete func(time.Duration)
	onError    func()
	onTimeout  func()
}

func (t *ProductionTask) Execute(ctx context.Context) (any, error) {
	start := time.Now()

	switch t.taskType {
	case "web_request":
		// 模拟Web请求处理
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			if t.onTimeout != nil {
				t.onTimeout()
			}
			return nil, ctx.Err()
		}
	case "batch_processing":
		// 模拟批处理任务
		select {
		case <-time.After(t.duration):
		case <-ctx.Done():
			if t.onTimeout != nil {
				t.onTimeout()
			}
			return nil, ctx.Err()
		}
	default:
		time.Sleep(t.duration)
	}

	duration := time.Since(start)
	if t.onComplete != nil {
		t.onComplete(duration)
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ProductionTask) Priority() int {
	return PriorityNormal
}

// DisasterRecoveryTask 灾难恢复测试任务
type DisasterRecoveryTask struct {
	id         string
	taskType   string
	onComplete func()
	onError    func()
}

func (t *DisasterRecoveryTask) Execute(ctx context.Context) (any, error) {
	switch t.taskType {
	case "panic":
		panic(fmt.Sprintf("disaster panic from %s", t.id))
	case "resource_exhaustion":
		// 模拟资源耗尽
		data := make([][]byte, 1000)
		for i := range data {
			data[i] = make([]byte, 10240) // 10KB each
		}
		time.Sleep(time.Millisecond * 100)
	case "timeout":
		select {
		case <-time.After(time.Second * 5):
		case <-ctx.Done():
			if t.onError != nil {
				t.onError()
			}
			return nil, ctx.Err()
		}
	default: // normal
		time.Sleep(time.Millisecond * 50)
	}

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *DisasterRecoveryTask) Priority() int {
	return PriorityNormal
}

// ResourceLifecycleTask 资源生命周期测试任务
type ResourceLifecycleTask struct {
	id         string
	data       []byte
	onComplete func()
	onError    func()
}

func (t *ResourceLifecycleTask) Execute(ctx context.Context) (any, error) {
	// 使用数据以防止优化
	temp := make([]byte, len(t.data))
	copy(temp, t.data)

	time.Sleep(time.Millisecond * 20)

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *ResourceLifecycleTask) Priority() int {
	return PriorityNormal
}

// FinalPerformanceResult 最终性能测试结果
type FinalPerformanceResult struct {
	BenchmarkName   string
	TaskCount       int
	TotalDuration   time.Duration
	Throughput      float64
	AvgTaskDuration time.Duration
	MemoryUsage     uint64
}

// runFinalPerformanceBenchmark 运行最终性能基准测试
func runFinalPerformanceBenchmark(t *testing.T, pool Pool, name string, taskCount int, taskType string, taskDuration time.Duration) FinalPerformanceResult {
	var completed int64
	var totalTaskDuration int64
	var wg sync.WaitGroup

	// 记录开始内存状态
	var startMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&startMemStats)

	start := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		task := &FinalPerformanceBenchmarkTask{
			id:       fmt.Sprintf("%s-%d", name, i),
			taskType: taskType,
			duration: taskDuration,
			onComplete: func(duration time.Duration) {
				atomic.AddInt64(&completed, 1)
				atomic.AddInt64(&totalTaskDuration, int64(duration))
				wg.Done()
			},
			onError: func() {
				wg.Done()
			},
		}

		if err := pool.Submit(task); err != nil {
			t.Errorf("Failed to submit benchmark task: %v", err)
			wg.Done()
		}
	}

	wg.Wait()
	totalDuration := time.Since(start)

	// 记录结束内存状态
	var endMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&endMemStats)

	// 计算结果
	throughput := float64(completed) / totalDuration.Seconds()
	avgTaskDuration := time.Duration(0)
	if completed > 0 {
		avgTaskDuration = time.Duration(totalTaskDuration / completed)
	}
	memoryUsage := endMemStats.Alloc - startMemStats.Alloc

	return FinalPerformanceResult{
		BenchmarkName:   name,
		TaskCount:       int(completed),
		TotalDuration:   totalDuration,
		Throughput:      throughput,
		AvgTaskDuration: avgTaskDuration,
		MemoryUsage:     memoryUsage,
	}
}

// FinalPerformanceBenchmarkTask 最终性能基准测试任务
type FinalPerformanceBenchmarkTask struct {
	id         string
	taskType   string
	duration   time.Duration
	onComplete func(time.Duration)
	onError    func()
}

func (t *FinalPerformanceBenchmarkTask) Execute(ctx context.Context) (any, error) {
	start := time.Now()

	switch t.taskType {
	case "light":
		// 轻量级任务
		time.Sleep(t.duration)
	case "medium":
		// 中等任务 - 一些CPU工作
		sum := 0
		for i := 0; i < 10000; i++ {
			sum += i
		}
		time.Sleep(t.duration)
	case "heavy":
		// 重任务 - 更多CPU和内存工作
		data := make([]int, 1000)
		for i := range data {
			data[i] = i * i
		}
		time.Sleep(t.duration)
	case "mixed":
		// 混合任务
		if t.id[len(t.id)-1]%2 == 0 {
			time.Sleep(t.duration / 2)
		} else {
			sum := 0
			for i := 0; i < 5000; i++ {
				sum += i
			}
			time.Sleep(t.duration)
		}
	default:
		time.Sleep(t.duration)
	}

	duration := time.Since(start)
	if t.onComplete != nil {
		t.onComplete(duration)
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *PerformanceBenchmarkTask) Priority() int {
	return PriorityNormal
}
