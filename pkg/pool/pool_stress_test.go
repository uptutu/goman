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

// StressTestTask 压力测试任务
type StressTestTask struct {
	id         int64
	workload   time.Duration
	priority   int
	submitTime time.Time
	startTime  time.Time
	finishTime time.Time
	executed   int32
	cancelled  int32
}

func NewStressTestTask(id int64, workload time.Duration) *StressTestTask {
	return &StressTestTask{
		id:         id,
		workload:   workload,
		priority:   PriorityNormal,
		submitTime: time.Now(),
	}
}

func (t *StressTestTask) Execute(ctx context.Context) (any, error) {
	t.startTime = time.Now()
	atomic.StoreInt32(&t.executed, 1)

	if t.workload > 0 {
		select {
		case <-time.After(t.workload):
		case <-ctx.Done():
			atomic.StoreInt32(&t.cancelled, 1)
			return nil, ctx.Err()
		}
	}

	t.finishTime = time.Now()
	return fmt.Sprintf("stress-task-%d-result", t.id), nil
}

func (t *StressTestTask) Priority() int {
	return t.priority
}

func (t *StressTestTask) SetPriority(priority int) {
	t.priority = priority
}

func (t *StressTestTask) IsExecuted() bool {
	return atomic.LoadInt32(&t.executed) == 1
}

func (t *StressTestTask) IsCancelled() bool {
	return atomic.LoadInt32(&t.cancelled) == 1
}

func (t *StressTestTask) GetLatency() time.Duration {
	if t.startTime.IsZero() {
		return 0
	}
	return t.startTime.Sub(t.submitTime)
}

func (t *StressTestTask) GetExecutionTime() time.Duration {
	if t.startTime.IsZero() || t.finishTime.IsZero() {
		return 0
	}
	return t.finishTime.Sub(t.startTime)
}

// BenchmarkPoolExtremeStress 极限压力测试
func BenchmarkPoolExtremeStress(b *testing.B) {
	stressLevels := []struct {
		name        string
		workers     int
		queueSize   int
		goroutines  int
		tasksPerGor int
		workload    time.Duration
		duration    time.Duration
	}{
		{"Moderate", 8, 2000, 50, 100, time.Microsecond, time.Second * 5},
		{"Heavy", 16, 5000, 100, 100, time.Microsecond * 10, time.Second * 10},
		{"Extreme", 32, 10000, 200, 50, time.Microsecond * 5, time.Second * 15},
		{"Ultra", runtime.NumCPU() * 4, 20000, 500, 20, 0, time.Second * 20},
	}

	for _, level := range stressLevels {
		b.Run(level.name, func(b *testing.B) {
			if testing.Short() && level.name == "Ultra" {
				b.Skip("跳过Ultra级别测试在短测试模式下")
			}

			runExtremeStressTest(b, level.workers, level.queueSize, level.goroutines,
				level.tasksPerGor, level.workload, level.duration)
		})
	}
}

func runExtremeStressTest(b *testing.B, workers, queueSize, goroutines, tasksPerGor int,
	workload, duration time.Duration) {

	config, err := NewConfigBuilder().
		WithWorkerCount(workers).
		WithQueueSize(queueSize).
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

	var totalSubmitted, totalErrors, totalPanics int64
	var totalLatency, totalExecTime int64
	var completedTasks []*StressTestTask
	var tasksMutex sync.Mutex

	b.ResetTimer()
	start := time.Now()

	var wg sync.WaitGroup

	// 启动压力测试协程
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&totalPanics, 1)
					b.Errorf("Goroutine %d panicked: %v", goroutineID, r)
				}
			}()

			localTasks := make([]*StressTestTask, 0, tasksPerGor)

			for j := 0; j < tasksPerGor; j++ {
				if time.Since(start) >= duration {
					break
				}

				taskID := int64(goroutineID*tasksPerGor + j)
				task := NewStressTestTask(taskID, workload)
				localTasks = append(localTasks, task)

				err := pool.Submit(task)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
				} else {
					atomic.AddInt64(&totalSubmitted, 1)
				}

				// 随机延迟模拟真实场景
				if j%10 == 0 {
					time.Sleep(time.Microsecond * time.Duration(j%5))
				}
			}

			// 等待任务完成并收集统计信息
			time.Sleep(time.Millisecond * 100)
			for _, task := range localTasks {
				if task.IsExecuted() {
					latency := task.GetLatency()
					execTime := task.GetExecutionTime()
					atomic.AddInt64(&totalLatency, int64(latency))
					atomic.AddInt64(&totalExecTime, int64(execTime))

					tasksMutex.Lock()
					completedTasks = append(completedTasks, task)
					tasksMutex.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()
	actualDuration := time.Since(start)

	// 等待剩余任务完成
	time.Sleep(time.Second)
	stats := pool.Stats()

	// 计算性能指标
	totalTasks := totalSubmitted + totalErrors
	successRate := float64(totalSubmitted) / float64(totalTasks) * 100
	throughput := float64(totalSubmitted) / actualDuration.Seconds()

	var avgLatency, avgExecTime time.Duration
	if len(completedTasks) > 0 {
		avgLatency = time.Duration(totalLatency / int64(len(completedTasks)))
		avgExecTime = time.Duration(totalExecTime / int64(len(completedTasks)))
	}

	// 内存统计
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 报告指标
	b.ReportMetric(throughput, "tasks/sec")
	b.ReportMetric(successRate, "success_rate_%")
	b.ReportMetric(float64(totalErrors), "errors")
	b.ReportMetric(float64(totalPanics), "panics")
	b.ReportMetric(float64(avgLatency.Nanoseconds()), "avg_latency_ns")
	b.ReportMetric(float64(avgExecTime.Nanoseconds()), "avg_exec_time_ns")
	b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")
	b.ReportMetric(float64(stats.ActiveWorkers), "active_workers")
	b.ReportMetric(float64(m.HeapAlloc)/1024/1024, "heap_mb")

	// 验证稳定性
	if successRate < 85 {
		b.Errorf("压力测试失败: 成功率 %.2f%% < 85%%", successRate)
	}
	if totalPanics > 0 {
		b.Errorf("压力测试失败: 发生了 %d 次panic", totalPanics)
	}
	if throughput < 100 {
		b.Errorf("压力测试失败: 吞吐量 %.2f < 100 tasks/sec", throughput)
	}

	b.Logf("压力测试结果: 协程%d, 队列%d, 并发%d, 吞吐量%.2f tasks/sec, 成功率%.2f%%, 延迟%v, 错误%d, Panic%d",
		workers, queueSize, goroutines, throughput, successRate, avgLatency, totalErrors, totalPanics)
}

// BenchmarkPoolLongRunningStabilityStress 长时间运行稳定性压力测试
func BenchmarkPoolLongRunningStabilityStress(b *testing.B) {
	if testing.Short() {
		b.Skip("跳过长时间运行测试在短测试模式下")
	}

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

	// 长时间运行测试（30秒）
	testDuration := time.Second * 30
	checkInterval := time.Second * 5

	var totalSubmitted, totalErrors int64
	var memorySnapshots []uint64
	var throughputSnapshots []float64

	b.ResetTimer()
	start := time.Now()

	// 启动任务提交协程
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	// 任务提交协程
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			var taskID int64

			for {
				select {
				case <-stopChan:
					return
				default:
					id := atomic.AddInt64(&taskID, 1)
					workload := time.Microsecond * time.Duration((id%100)+1)
					task := NewStressTestTask(int64(workerID)*100000+id, workload)

					err := pool.Submit(task)
					if err != nil {
						atomic.AddInt64(&totalErrors, 1)
					} else {
						atomic.AddInt64(&totalSubmitted, 1)
					}

					// 控制提交频率
					if id%100 == 0 {
						time.Sleep(time.Millisecond)
					}
				}
			}
		}(i)
	}

	// 监控协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		lastSubmitted := int64(0)
		lastTime := start

		for {
			select {
			case <-stopChan:
				return
			case now := <-ticker.C:
				currentSubmitted := atomic.LoadInt64(&totalSubmitted)
				currentErrors := atomic.LoadInt64(&totalErrors)

				// 计算当前吞吐量
				duration := now.Sub(lastTime)
				throughput := float64(currentSubmitted-lastSubmitted) / duration.Seconds()
				throughputSnapshots = append(throughputSnapshots, throughput)

				// 记录内存使用
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memorySnapshots = append(memorySnapshots, m.HeapAlloc)

				// 获取统计信息
				stats := pool.Stats()

				b.Logf("时间: %v, 吞吐量: %.2f tasks/sec, 总提交: %d, 错误: %d, 活跃协程: %d, 队列: %d, 内存: %.2f MB",
					now.Sub(start).Round(time.Second),
					throughput,
					currentSubmitted,
					currentErrors,
					stats.ActiveWorkers,
					stats.QueuedTasks,
					float64(m.HeapAlloc)/1024/1024)

				lastSubmitted = currentSubmitted
				lastTime = now

				// 检查异常情况
				if throughput < 100 && now.Sub(start) > time.Second*10 {
					b.Errorf("吞吐量异常低: %.2f tasks/sec", throughput)
				}
				if stats.ActiveWorkers == 0 && currentSubmitted > 0 {
					b.Errorf("所有工作协程都停止了")
				}
			}
		}
	}()

	// 等待测试完成
	time.Sleep(testDuration)
	close(stopChan)
	wg.Wait()

	actualDuration := time.Since(start)
	finalSubmitted := atomic.LoadInt64(&totalSubmitted)
	finalErrors := atomic.LoadInt64(&totalErrors)

	// 计算最终指标
	totalTasks := finalSubmitted + finalErrors
	successRate := float64(finalSubmitted) / float64(totalTasks) * 100
	avgThroughput := float64(finalSubmitted) / actualDuration.Seconds()

	// 分析内存趋势
	var memoryGrowth float64
	if len(memorySnapshots) >= 2 {
		initialMemory := memorySnapshots[0]
		finalMemory := memorySnapshots[len(memorySnapshots)-1]
		memoryGrowth = float64(finalMemory-initialMemory) / float64(initialMemory) * 100
	}

	// 分析吞吐量稳定性
	var throughputVariance float64
	if len(throughputSnapshots) > 1 {
		var sum, sumSquares float64
		for _, tp := range throughputSnapshots {
			sum += tp
			sumSquares += tp * tp
		}
		mean := sum / float64(len(throughputSnapshots))
		throughputVariance = (sumSquares/float64(len(throughputSnapshots)) - mean*mean) / mean * 100
	}

	// 报告指标
	b.ReportMetric(avgThroughput, "avg_tasks/sec")
	b.ReportMetric(successRate, "success_rate_%")
	b.ReportMetric(memoryGrowth, "memory_growth_%")
	b.ReportMetric(throughputVariance, "throughput_variance_%")
	b.ReportMetric(float64(finalErrors), "total_errors")

	// 稳定性验证
	if successRate < 90 {
		b.Errorf("长时间稳定性测试失败: 成功率 %.2f%% < 90%%", successRate)
	}
	if memoryGrowth > 50 {
		b.Errorf("长时间稳定性测试失败: 内存增长 %.2f%% > 50%%", memoryGrowth)
	}
	if throughputVariance > 100 {
		b.Errorf("长时间稳定性测试失败: 吞吐量方差 %.2f%% > 100%%", throughputVariance)
	}

	b.Logf("长时间稳定性测试结果: 运行时间%v, 平均吞吐量%.2f tasks/sec, 成功率%.2f%%, 内存增长%.2f%%, 吞吐量方差%.2f%%",
		actualDuration, avgThroughput, successRate, memoryGrowth, throughputVariance)
}

// BenchmarkPoolBurstTraffic 突发流量测试
func BenchmarkPoolBurstTraffic(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(16).
		WithQueueSize(10000).
		WithObjectPoolSize(1000).
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

	// 突发流量模式：正常 -> 突发 -> 正常 -> 突发
	phases := []struct {
		name        string
		duration    time.Duration
		concurrency int
		taskRate    int // 每秒任务数
		workload    time.Duration
	}{
		{"Normal1", time.Second * 2, 10, 1000, time.Microsecond * 10},
		{"Burst1", time.Second * 3, 100, 5000, time.Microsecond * 5},
		{"Normal2", time.Second * 2, 10, 1000, time.Microsecond * 10},
		{"Burst2", time.Second * 3, 200, 8000, 0},
		{"Recovery", time.Second * 2, 5, 500, time.Microsecond * 20},
	}

	var totalSubmitted, totalErrors int64
	var phaseResults []struct {
		name        string
		submitted   int64
		errors      int64
		throughput  float64
		successRate float64
	}

	b.ResetTimer()

	for _, phase := range phases {
		b.Logf("开始阶段: %s", phase.name)
		phaseStart := time.Now()
		var phaseSubmitted, phaseErrors int64

		var wg sync.WaitGroup
		stopChan := make(chan struct{})

		// 启动并发协程
		for i := 0; i < phase.concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				var taskID int64
				interval := time.Second / time.Duration(phase.taskRate/phase.concurrency)

				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				for {
					select {
					case <-stopChan:
						return
					case <-ticker.C:
						id := atomic.AddInt64(&taskID, 1)
						task := NewStressTestTask(int64(workerID)*10000+id, phase.workload)

						err := pool.Submit(task)
						if err != nil {
							atomic.AddInt64(&phaseErrors, 1)
							atomic.AddInt64(&totalErrors, 1)
						} else {
							atomic.AddInt64(&phaseSubmitted, 1)
							atomic.AddInt64(&totalSubmitted, 1)
						}
					}
				}
			}(i)
		}

		// 等待阶段完成
		time.Sleep(phase.duration)
		close(stopChan)
		wg.Wait()

		phaseDuration := time.Since(phaseStart)
		phaseThroughput := float64(phaseSubmitted) / phaseDuration.Seconds()
		phaseSuccessRate := float64(phaseSubmitted) / float64(phaseSubmitted+phaseErrors) * 100

		phaseResults = append(phaseResults, struct {
			name        string
			submitted   int64
			errors      int64
			throughput  float64
			successRate float64
		}{
			name:        phase.name,
			submitted:   phaseSubmitted,
			errors:      phaseErrors,
			throughput:  phaseThroughput,
			successRate: phaseSuccessRate,
		})

		stats := pool.Stats()
		b.Logf("阶段 %s 完成: 提交%d, 错误%d, 吞吐量%.2f tasks/sec, 成功率%.2f%%, 队列%d, 活跃协程%d",
			phase.name, phaseSubmitted, phaseErrors, phaseThroughput, phaseSuccessRate,
			stats.QueuedTasks, stats.ActiveWorkers)
	}

	// 等待所有任务完成
	time.Sleep(time.Second * 2)

	// 分析结果
	totalTasks := totalSubmitted + totalErrors
	overallSuccessRate := float64(totalSubmitted) / float64(totalTasks) * 100

	b.ReportMetric(float64(totalSubmitted), "total_submitted")
	b.ReportMetric(float64(totalErrors), "total_errors")
	b.ReportMetric(overallSuccessRate, "overall_success_rate_%")

	// 验证突发流量处理能力
	burstPhases := []string{"Burst1", "Burst2"}
	for _, burstPhase := range burstPhases {
		for _, result := range phaseResults {
			if result.name == burstPhase {
				if result.successRate < 80 {
					b.Errorf("突发流量处理失败 [%s]: 成功率 %.2f%% < 80%%", burstPhase, result.successRate)
				}
				if result.throughput < 1000 {
					b.Errorf("突发流量处理失败 [%s]: 吞吐量 %.2f < 1000 tasks/sec", burstPhase, result.throughput)
				}
			}
		}
	}

	// 打印详细结果
	b.Logf("\n=== 突发流量测试结果 ===")
	for _, result := range phaseResults {
		b.Logf("阶段: %-10s 提交: %6d 错误: %4d 吞吐量: %8.2f tasks/sec 成功率: %6.2f%%",
			result.name, result.submitted, result.errors, result.throughput, result.successRate)
	}
	b.Logf("总体成功率: %.2f%%", overallSuccessRate)
}

// BenchmarkPoolGracefulDegradation 优雅降级测试
func BenchmarkPoolGracefulDegradation(b *testing.B) {
	// 测试在资源不足时的优雅降级
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100). // 故意设置较小的队列
		WithObjectPoolSize(50).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	var totalSubmitted, totalErrors int64
	var errorTypes = make(map[string]int64)
	var errorMutex sync.Mutex

	b.ResetTimer()

	// 大量并发提交，超过队列容量
	var wg sync.WaitGroup
	const goroutineCount = 50
	const tasksPerGoroutine = 100

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < tasksPerGoroutine; j++ {
				taskID := int64(workerID*tasksPerGoroutine + j)
				// 使用较长的工作负载增加队列压力
				task := NewStressTestTask(taskID, time.Millisecond*10)

				err := pool.Submit(task)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)

					errorMutex.Lock()
					errorTypes[err.Error()]++
					errorMutex.Unlock()
				} else {
					atomic.AddInt64(&totalSubmitted, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待任务完成
	time.Sleep(time.Second * 5)
	stats := pool.Stats()

	totalTasks := totalSubmitted + totalErrors
	successRate := float64(totalSubmitted) / float64(totalTasks) * 100
	errorRate := float64(totalErrors) / float64(totalTasks) * 100

	b.ReportMetric(successRate, "success_rate_%")
	b.ReportMetric(errorRate, "error_rate_%")
	b.ReportMetric(float64(totalSubmitted), "submitted_tasks")
	b.ReportMetric(float64(totalErrors), "error_tasks")
	b.ReportMetric(float64(stats.CompletedTasks), "completed_tasks")

	// 验证优雅降级
	if errorRate > 80 {
		b.Errorf("降级测试失败: 错误率过高 %.2f%% > 80%%", errorRate)
	}
	if successRate < 10 {
		b.Errorf("降级测试失败: 成功率过低 %.2f%% < 10%%", successRate)
	}

	// 分析错误类型
	b.Logf("优雅降级测试结果:")
	b.Logf("总任务: %d, 成功: %d (%.2f%%), 错误: %d (%.2f%%)",
		totalTasks, totalSubmitted, successRate, totalErrors, errorRate)
	b.Logf("完成任务: %d, 活跃协程: %d, 队列任务: %d",
		stats.CompletedTasks, stats.ActiveWorkers, stats.QueuedTasks)

	b.Logf("错误类型分布:")
	errorMutex.Lock()
	for errType, count := range errorTypes {
		b.Logf("  %s: %d", errType, count)
	}
	errorMutex.Unlock()
}

// BenchmarkPoolRecoveryAfterOverload 过载后恢复测试
func BenchmarkPoolRecoveryAfterOverload(b *testing.B) {
	config, err := NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(1000).
		WithObjectPoolSize(200).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	b.ResetTimer()

	// 阶段1: 正常负载
	b.Logf("阶段1: 正常负载")
	normalLoad := func() (int64, int64) {
		var submitted, errors int64
		for i := 0; i < 500; i++ {
			task := NewStressTestTask(int64(i), time.Microsecond*10)
			err := pool.Submit(task)
			if err != nil {
				errors++
			} else {
				submitted++
			}
		}
		return submitted, errors
	}

	normalSubmitted, normalErrors := normalLoad()
	normalSuccessRate := float64(normalSubmitted) / float64(normalSubmitted+normalErrors) * 100
	b.Logf("正常负载结果: 成功率 %.2f%%", normalSuccessRate)

	// 阶段2: 过载
	b.Logf("阶段2: 系统过载")
	overload := func() (int64, int64) {
		var submitted, errors int64
		var wg sync.WaitGroup

		// 大量并发提交造成过载
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					taskID := int64(workerID*50 + j)
					task := NewStressTestTask(taskID, time.Millisecond*50) // 长任务
					err := pool.Submit(task)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					} else {
						atomic.AddInt64(&submitted, 1)
					}
				}
			}(i)
		}

		wg.Wait()
		return submitted, errors
	}

	overloadSubmitted, overloadErrors := overload()
	overloadSuccessRate := float64(overloadSubmitted) / float64(overloadSubmitted+overloadErrors) * 100
	b.Logf("过载阶段结果: 成功率 %.2f%%", overloadSuccessRate)

	// 等待系统恢复
	b.Logf("等待系统恢复...")
	time.Sleep(time.Second * 3)

	// 阶段3: 恢复后测试
	b.Logf("阶段3: 恢复后测试")
	recoverySubmitted, recoveryErrors := normalLoad()
	recoverySuccessRate := float64(recoverySubmitted) / float64(recoverySubmitted+recoveryErrors) * 100
	b.Logf("恢复后结果: 成功率 %.2f%%", recoverySuccessRate)

	// 验证恢复能力
	recoveryRatio := recoverySuccessRate / normalSuccessRate
	if recoveryRatio < 0.8 {
		b.Errorf("恢复测试失败: 恢复后成功率 %.2f%% 相比正常 %.2f%% 下降过多 (比率: %.2f)",
			recoverySuccessRate, normalSuccessRate, recoveryRatio)
	}

	stats := pool.Stats()
	b.ReportMetric(normalSuccessRate, "normal_success_rate_%")
	b.ReportMetric(overloadSuccessRate, "overload_success_rate_%")
	b.ReportMetric(recoverySuccessRate, "recovery_success_rate_%")
	b.ReportMetric(recoveryRatio, "recovery_ratio")
	b.ReportMetric(float64(stats.CompletedTasks), "total_completed")

	b.Logf("过载恢复测试完成: 正常%.2f%% -> 过载%.2f%% -> 恢复%.2f%% (恢复比率: %.2f)",
		normalSuccessRate, overloadSuccessRate, recoverySuccessRate, recoveryRatio)
}
