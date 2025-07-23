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

// TestLongRunningStability 测试长时间运行的稳定性
func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long running stability test in short mode")
	}

	t.Run("continuous operation", func(t *testing.T) {
		testContinuousOperation(t)
	})

	t.Run("memory stability", func(t *testing.T) {
		testMemoryStability(t)
	})

	t.Run("worker recovery", func(t *testing.T) {
		testWorkerRecovery(t)
	})

	t.Run("resource cleanup", func(t *testing.T) {
		testResourceCleanup(t)
	})
}

// testContinuousOperation 测试连续运行稳定性
func testContinuousOperation(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
		WithTaskTimeout(time.Second * 2).
		WithMetrics(true).
		WithMetricsInterval(time.Millisecond * 500).
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

	// 运行时间（可以根据需要调整）
	testDuration := time.Minute * 2 // 2分钟的连续测试
	if testing.Short() {
		testDuration = time.Second * 30 // 短模式下30秒
	}

	var stats struct {
		totalSubmitted int64
		totalCompleted int64
		totalFailed    int64
		totalPanics    int64
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// 启动多个提交协程
	var wg sync.WaitGroup
	submitterCount := 3

	for i := 0; i < submitterCount; i++ {
		wg.Add(1)
		go func(submitterID int) {
			defer wg.Done()

			taskID := 0
			ticker := time.NewTicker(time.Millisecond * 10) // 每10ms提交一个任务
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					taskID++
					task := &StabilityTask{
						id:          fmt.Sprintf("s%d-t%d", submitterID, taskID),
						duration:    time.Millisecond * time.Duration(10+taskID%50),
						shouldFail:  taskID%100 == 0, // 1%的任务失败
						shouldPanic: taskID%500 == 0, // 0.2%的任务panic
						onComplete: func() {
							atomic.AddInt64(&stats.totalCompleted, 1)
						},
						onError: func() {
							atomic.AddInt64(&stats.totalFailed, 1)
						},
						onPanic: func() {
							atomic.AddInt64(&stats.totalPanics, 1)
						},
					}

					if err := pool.Submit(task); err != nil {
						atomic.AddInt64(&stats.totalFailed, 1)
					} else {
						atomic.AddInt64(&stats.totalSubmitted, 1)
					}
				}
			}
		}(i)
	}

	// 监控协程，定期输出统计信息
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
				t.Logf("Stability test progress:")
				t.Logf("  Submitted: %d, Completed: %d, Failed: %d, Panics: %d",
					atomic.LoadInt64(&stats.totalSubmitted),
					atomic.LoadInt64(&stats.totalCompleted),
					atomic.LoadInt64(&stats.totalFailed),
					atomic.LoadInt64(&stats.totalPanics))
				t.Logf("  Pool stats: Active=%d, Queued=%d, Completed=%d, Failed=%d",
					poolStats.ActiveWorkers, poolStats.QueuedTasks,
					poolStats.CompletedTasks, poolStats.FailedTasks)
				t.Logf("  Memory: %d bytes, GC: %d", poolStats.MemoryUsage, poolStats.GCCount)
			}
		}
	}()

	// 等待测试完成
	wg.Wait()

	// 等待剩余任务完成
	time.Sleep(time.Second * 2)

	// 最终统计
	finalStats := pool.Stats()
	t.Logf("Final stability test results:")
	t.Logf("  Test duration: %v", testDuration)
	t.Logf("  Total submitted: %d", stats.totalSubmitted)
	t.Logf("  Total completed: %d", stats.totalCompleted)
	t.Logf("  Total failed: %d", stats.totalFailed)
	t.Logf("  Total panics: %d", stats.totalPanics)
	t.Logf("  Pool final stats: %+v", finalStats)

	// 基本验证
	if stats.totalSubmitted == 0 {
		t.Error("No tasks were submitted during stability test")
	}

	if stats.totalCompleted == 0 {
		t.Error("No tasks were completed during stability test")
	}

	// 验证协程池仍然正常运行
	if !pool.IsRunning() {
		t.Error("Pool should still be running after stability test")
	}

	// 提交一个测试任务验证功能正常
	testTask := &StabilityTask{
		id:       "final-test",
		duration: time.Millisecond * 10,
	}
	if err := pool.Submit(testTask); err != nil {
		t.Errorf("Pool should still accept tasks after stability test: %v", err)
	}
}

// testMemoryStability 测试内存稳定性
func testMemoryStability(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		pool.Shutdown(ctx)
	}()

	// 记录初始内存状态
	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)

	// 运行多轮任务，检查内存是否稳定
	rounds := 10
	tasksPerRound := 1000

	var memoryGrowth []uint64

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup

		// 提交一轮任务
		for i := 0; i < tasksPerRound; i++ {
			wg.Add(1)
			task := &StabilityTask{
				id:       fmt.Sprintf("r%d-t%d", round, i),
				duration: time.Millisecond * 5,
				data:     make([]byte, 1024), // 每个任务分配1KB数据
				onComplete: func() {
					wg.Done()
				},
			}

			if err := pool.Submit(task); err != nil {
				t.Errorf("Failed to submit task in round %d: %v", round, err)
				wg.Done()
			}
		}

		// 等待这轮任务完成
		wg.Wait()

		// 强制GC并检查内存使用
		runtime.GC()
		runtime.GC() // 两次GC确保清理完成

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memoryGrowth = append(memoryGrowth, memStats.Alloc)

		t.Logf("Round %d: Memory usage = %d bytes, Objects = %d",
			round, memStats.Alloc, memStats.Mallocs-memStats.Frees)

		// 短暂休息
		time.Sleep(time.Millisecond * 100)
	}

	// 分析内存增长趋势
	if len(memoryGrowth) >= 3 {
		// 检查最后几轮的内存使用是否稳定
		lastThree := memoryGrowth[len(memoryGrowth)-3:]
		maxMem := lastThree[0]
		minMem := lastThree[0]

		for _, mem := range lastThree {
			if mem > maxMem {
				maxMem = mem
			}
			if mem < minMem {
				minMem = mem
			}
		}

		// 内存波动不应该超过50%
		if maxMem > 0 && float64(maxMem-minMem)/float64(minMem) > 0.5 {
			t.Errorf("Memory usage is not stable. Min: %d, Max: %d, Growth: %.2f%%",
				minMem, maxMem, float64(maxMem-minMem)/float64(minMem)*100)
		}
	}

	// 检查是否有明显的内存泄漏
	finalMem := memoryGrowth[len(memoryGrowth)-1]
	if finalMem > initialMemStats.Alloc*10 { // 内存增长超过10倍认为可能有泄漏
		t.Errorf("Possible memory leak detected. Initial: %d, Final: %d",
			initialMemStats.Alloc, finalMem)
	}
}

// testWorkerRecovery 测试工作协程恢复能力
func testWorkerRecovery(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(50).
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
		normalCompleted int64
		panicRecovered  int64
		errorHandled    int64
	}

	// 提交正常任务
	for i := 0; i < 20; i++ {
		task := &StabilityTask{
			id:       fmt.Sprintf("normal-%d", i),
			duration: time.Millisecond * 10,
			onComplete: func() {
				atomic.AddInt64(&stats.normalCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 提交会panic的任务
	for i := 0; i < 10; i++ {
		task := &StabilityTask{
			id:          fmt.Sprintf("panic-%d", i),
			shouldPanic: true,
			onPanic: func() {
				atomic.AddInt64(&stats.panicRecovered, 1)
			},
		}
		pool.Submit(task)
	}

	// 提交会出错的任务
	for i := 0; i < 15; i++ {
		task := &StabilityTask{
			id:         fmt.Sprintf("error-%d", i),
			shouldFail: true,
			onError: func() {
				atomic.AddInt64(&stats.errorHandled, 1)
			},
		}
		pool.Submit(task)
	}

	// 再次提交正常任务，验证工作协程恢复正常
	for i := 0; i < 20; i++ {
		task := &StabilityTask{
			id:       fmt.Sprintf("recovery-%d", i),
			duration: time.Millisecond * 10,
			onComplete: func() {
				atomic.AddInt64(&stats.normalCompleted, 1)
			},
		}
		pool.Submit(task)
	}

	// 等待所有任务完成
	time.Sleep(time.Second * 3)

	t.Logf("Worker recovery test results:")
	t.Logf("  Normal completed: %d/40", stats.normalCompleted)
	t.Logf("  Panics recovered: %d/10", stats.panicRecovered)
	t.Logf("  Errors handled: %d/15", stats.errorHandled)

	// 验证协程池仍然正常工作
	if !pool.IsRunning() {
		t.Error("Pool should still be running after panic recovery")
	}

	// 验证正常任务都完成了
	if stats.normalCompleted != 40 {
		t.Errorf("Expected 40 normal tasks completed, got %d", stats.normalCompleted)
	}

	// 验证panic被正确处理
	if stats.panicRecovered != 10 {
		t.Errorf("Expected 10 panics recovered, got %d", stats.panicRecovered)
	}

	// 验证错误被正确处理
	if stats.errorHandled != 15 {
		t.Errorf("Expected 15 errors handled, got %d", stats.errorHandled)
	}
}

// testResourceCleanup 测试资源清理
func testResourceCleanup(t *testing.T) {
	// 创建多个协程池并关闭，检查资源是否正确清理
	poolCount := 5
	var pools []Pool

	// 创建多个协程池
	for i := 0; i < poolCount; i++ {
		config, err := NewConfigBuilder().
			WithWorkerCount(2).
			WithQueueSize(20).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config for pool %d: %v", i, err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool %d: %v", i, err)
		}

		pools = append(pools, pool)

		// 向每个协程池提交一些任务
		for j := 0; j < 10; j++ {
			task := &StabilityTask{
				id:       fmt.Sprintf("p%d-t%d", i, j),
				duration: time.Millisecond * 50,
			}
			pool.Submit(task)
		}
	}

	// 记录创建后的内存状态
	var afterCreateMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&afterCreateMemStats)

	// 等待任务完成
	time.Sleep(time.Second * 2)

	// 关闭所有协程池
	for i, pool := range pools {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		if err := pool.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown pool %d: %v", i, err)
		}
		cancel()

		if !pool.IsClosed() {
			t.Errorf("Pool %d should be closed after shutdown", i)
		}
	}

	// 强制GC并检查内存是否被释放
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Millisecond * 100) // 给GC一些时间

	var afterCleanupMemStats runtime.MemStats
	runtime.ReadMemStats(&afterCleanupMemStats)

	t.Logf("Resource cleanup test results:")
	t.Logf("  Pools created and closed: %d", poolCount)
	t.Logf("  Memory after create: %d bytes", afterCreateMemStats.Alloc)
	t.Logf("  Memory after cleanup: %d bytes", afterCleanupMemStats.Alloc)
	t.Logf("  Memory freed: %d bytes", int64(afterCreateMemStats.Alloc)-int64(afterCleanupMemStats.Alloc))

	// 验证内存使用没有显著增长
	memoryGrowth := float64(afterCleanupMemStats.Alloc) / float64(afterCreateMemStats.Alloc)
	if memoryGrowth > 1.5 { // 允许50%的内存增长
		t.Errorf("Memory not properly cleaned up. Growth ratio: %.2f", memoryGrowth)
	}

	// 验证所有协程池都已关闭
	for i, pool := range pools {
		if !pool.IsClosed() {
			t.Errorf("Pool %d should be closed", i)
		}

		// 尝试提交任务应该失败
		task := &StabilityTask{id: fmt.Sprintf("after-close-%d", i)}
		if err := pool.Submit(task); err == nil {
			t.Errorf("Pool %d should reject tasks after closure", i)
		}
	}
}

// StabilityTask 稳定性测试任务
type StabilityTask struct {
	id          string
	duration    time.Duration
	data        []byte
	shouldFail  bool
	shouldPanic bool
	onComplete  func()
	onError     func()
	onPanic     func()
}

func (t *StabilityTask) Execute(ctx context.Context) (any, error) {
	if t.shouldPanic {
		if t.onPanic != nil {
			defer t.onPanic()
		}
		panic(fmt.Sprintf("test panic from task %s", t.id))
	}

	if t.shouldFail {
		if t.onError != nil {
			t.onError()
		}
		return nil, fmt.Errorf("test error from task %s", t.id)
	}

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

	if t.onComplete != nil {
		t.onComplete()
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *StabilityTask) Priority() int {
	return PriorityNormal
}
