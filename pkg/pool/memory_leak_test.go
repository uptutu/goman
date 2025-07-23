package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestMemoryLeakDetection 内存泄漏检测测试
func TestMemoryLeakDetection(t *testing.T) {
	t.Run("task wrapper recycling", func(t *testing.T) {
		testTaskWrapperRecycling(t)
	})

	t.Run("future object recycling", func(t *testing.T) {
		testFutureObjectRecycling(t)
	})

	t.Run("goroutine leak detection", func(t *testing.T) {
		testGoroutineLeakDetection(t)
	})

	t.Run("channel leak detection", func(t *testing.T) {
		testChannelLeakDetection(t)
	})

	t.Run("context leak detection", func(t *testing.T) {
		testContextLeakDetection(t)
	})
}

// testTaskWrapperRecycling 测试任务包装器回收
func testTaskWrapperRecycling(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
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

	// 运行多轮任务，每轮之间检查内存使用
	rounds := 20
	tasksPerRound := 500

	var memoryUsage []uint64

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup

		// 提交大量任务
		for i := 0; i < tasksPerRound; i++ {
			wg.Add(1)
			task := &MemoryTestTask{
				id:       fmt.Sprintf("r%d-t%d", round, i),
				duration: time.Microsecond * 100,
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
		wg.Wait()

		// 强制GC并记录内存使用
		runtime.GC()
		runtime.GC()

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memoryUsage = append(memoryUsage, memStats.Alloc)

		if round%5 == 0 {
			t.Logf("Round %d: Memory = %d bytes, Objects = %d",
				round, memStats.Alloc, memStats.Mallocs-memStats.Frees)
		}
	}

	// 分析内存使用趋势
	if len(memoryUsage) >= 10 {
		// 检查后半段的内存使用是否稳定
		halfPoint := len(memoryUsage) / 2
		firstHalf := memoryUsage[:halfPoint]
		secondHalf := memoryUsage[halfPoint:]

		firstHalfAvg := calculateAverage(firstHalf)
		secondHalfAvg := calculateAverage(secondHalf)

		growthRatio := secondHalfAvg / firstHalfAvg

		t.Logf("Memory analysis:")
		t.Logf("  First half average: %.0f bytes", firstHalfAvg)
		t.Logf("  Second half average: %.0f bytes", secondHalfAvg)
		t.Logf("  Growth ratio: %.3f", growthRatio)

		// 内存增长不应该超过20%
		if growthRatio > 1.2 {
			t.Errorf("Possible memory leak in task wrapper recycling. Growth ratio: %.3f", growthRatio)
		}
	}
}

// testFutureObjectRecycling 测试Future对象回收
func testFutureObjectRecycling(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
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

	// 运行多轮异步任务
	rounds := 15
	futuresPerRound := 200

	var memoryUsage []uint64

	for round := 0; round < rounds; round++ {
		futures := make([]Future, futuresPerRound)

		// 提交异步任务
		for i := 0; i < futuresPerRound; i++ {
			task := &MemoryTestTask{
				id:       fmt.Sprintf("async-r%d-t%d", round, i),
				duration: time.Microsecond * 50,
				result:   fmt.Sprintf("result-%d-%d", round, i),
			}

			futures[i] = pool.SubmitAsync(task)
		}

		// 获取所有结果
		for i, future := range futures {
			result, err := future.GetWithTimeout(time.Second)
			if err != nil {
				t.Errorf("Future %d failed: %v", i, err)
			} else {
				expected := fmt.Sprintf("result-%d-%d", round, i)
				if result != expected {
					t.Errorf("Future %d: expected %s, got %v", i, expected, result)
				}
			}
		}

		// 清空futures引用
		for i := range futures {
			futures[i] = nil
		}
		futures = nil

		// 强制GC并记录内存使用
		runtime.GC()
		runtime.GC()

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		memoryUsage = append(memoryUsage, memStats.Alloc)

		if round%5 == 0 {
			t.Logf("Round %d: Memory = %d bytes", round, memStats.Alloc)
		}
	}

	// 分析内存使用趋势
	if len(memoryUsage) >= 10 {
		lastFive := memoryUsage[len(memoryUsage)-5:]
		maxMem := lastFive[0]
		minMem := lastFive[0]

		for _, mem := range lastFive {
			if mem > maxMem {
				maxMem = mem
			}
			if mem < minMem {
				minMem = mem
			}
		}

		// 最后几轮的内存波动不应该超过30%
		if maxMem > 0 && float64(maxMem-minMem)/float64(minMem) > 0.3 {
			t.Errorf("Future object memory usage is not stable. Min: %d, Max: %d, Variation: %.2f%%",
				minMem, maxMem, float64(maxMem-minMem)/float64(minMem)*100)
		}
	}
}

// testGoroutineLeakDetection 测试协程泄漏检测
func testGoroutineLeakDetection(t *testing.T) {
	// 记录初始协程数量
	initialGoroutines := runtime.NumGoroutine()

	// 创建和销毁多个协程池
	poolCount := 10
	for i := 0; i < poolCount; i++ {
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

		// 提交一些任务
		var wg sync.WaitGroup
		for j := 0; j < 20; j++ {
			wg.Add(1)
			task := &MemoryTestTask{
				id:       fmt.Sprintf("p%d-t%d", i, j),
				duration: time.Millisecond * 10,
				onComplete: func() {
					wg.Done()
				},
			}
			pool.Submit(task)
		}

		// 等待任务完成
		wg.Wait()

		// 关闭协程池
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		if err := pool.Shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown pool %d: %v", i, err)
		}
		cancel()

		// 检查协程数量
		currentGoroutines := runtime.NumGoroutine()
		t.Logf("Pool %d: Goroutines after shutdown = %d", i, currentGoroutines)
	}

	// 等待一段时间让协程完全清理
	time.Sleep(time.Second)
	runtime.GC()

	// 检查最终协程数量
	finalGoroutines := runtime.NumGoroutine()
	goroutineLeak := finalGoroutines - initialGoroutines

	t.Logf("Goroutine leak detection:")
	t.Logf("  Initial goroutines: %d", initialGoroutines)
	t.Logf("  Final goroutines: %d", finalGoroutines)
	t.Logf("  Potential leak: %d", goroutineLeak)

	// 允许少量协程增长（测试框架本身可能创建协程）
	if goroutineLeak > 5 {
		t.Errorf("Possible goroutine leak detected. Leaked: %d goroutines", goroutineLeak)
	}
}

// testChannelLeakDetection 测试通道泄漏检测
func testChannelLeakDetection(t *testing.T) {
	// 这个测试主要通过内存使用情况来间接检测通道泄漏
	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)

	// 创建多个协程池，每个都有通道
	poolCount := 20
	for i := 0; i < poolCount; i++ {
		config, err := NewConfigBuilder().
			WithWorkerCount(3).
			WithQueueSize(30).
			Build()
		if err != nil {
			t.Fatalf("Failed to create config: %v", err)
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}

		// 快速提交和完成任务
		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Add(1)
			task := &MemoryTestTask{
				id:       fmt.Sprintf("ch-p%d-t%d", i, j),
				duration: time.Microsecond * 100,
				onComplete: func() {
					wg.Done()
				},
			}
			pool.Submit(task)
		}

		wg.Wait()

		// 立即关闭
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		pool.Shutdown(ctx)
		cancel()
	}

	// 强制GC
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Millisecond * 100)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	memoryGrowth := finalMemStats.Alloc - initialMemStats.Alloc
	t.Logf("Channel leak detection:")
	t.Logf("  Initial memory: %d bytes", initialMemStats.Alloc)
	t.Logf("  Final memory: %d bytes", finalMemStats.Alloc)
	t.Logf("  Memory growth: %d bytes", memoryGrowth)

	// 检查内存增长是否合理（允许一定增长）
	if memoryGrowth > 1024*1024 { // 1MB
		t.Errorf("Excessive memory growth detected, possible channel leak: %d bytes", memoryGrowth)
	}
}

// testContextLeakDetection 测试上下文泄漏检测
func testContextLeakDetection(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(4).
		WithQueueSize(100).
		WithTaskTimeout(time.Millisecond * 100).
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

	var initialMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)

	// 提交大量带超时的任务
	rounds := 10
	tasksPerRound := 100

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup

		for i := 0; i < tasksPerRound; i++ {
			wg.Add(1)
			task := &MemoryTestTask{
				id: fmt.Sprintf("ctx-r%d-t%d", round, i),
				// 一些任务会超时，一些不会
				duration: time.Millisecond * time.Duration(50+i%100),
				onComplete: func() {
					wg.Done()
				},
				onError: func() {
					wg.Done()
				},
			}

			// 使用带超时的提交
			if err := pool.SubmitWithTimeout(task, time.Millisecond*80); err != nil {
				wg.Done()
			}
		}

		wg.Wait()

		// 每几轮检查一次内存
		if round%3 == 0 {
			runtime.GC()
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			t.Logf("Round %d: Memory = %d bytes", round, memStats.Alloc)
		}
	}

	// 最终内存检查
	runtime.GC()
	runtime.GC()
	time.Sleep(time.Millisecond * 100)

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	memoryGrowth := finalMemStats.Alloc - initialMemStats.Alloc
	t.Logf("Context leak detection:")
	t.Logf("  Initial memory: %d bytes", initialMemStats.Alloc)
	t.Logf("  Final memory: %d bytes", finalMemStats.Alloc)
	t.Logf("  Memory growth: %d bytes", memoryGrowth)

	// 检查内存增长
	growthRatio := float64(finalMemStats.Alloc) / float64(initialMemStats.Alloc)
	if growthRatio > 3.0 { // 允许3倍增长
		t.Errorf("Excessive memory growth in context test, possible leak. Growth ratio: %.2f", growthRatio)
	}
}

// calculateAverage 计算平均值
func calculateAverage(values []uint64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum uint64
	for _, v := range values {
		sum += v
	}

	return float64(sum) / float64(len(values))
}

// MemoryTestTask 内存测试任务
type MemoryTestTask struct {
	id         string
	duration   time.Duration
	result     any
	onComplete func()
	onError    func()
}

func (t *MemoryTestTask) Execute(ctx context.Context) (any, error) {
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

	if t.result != nil {
		return t.result, nil
	}

	return fmt.Sprintf("result-%s", t.id), nil
}

func (t *MemoryTestTask) Priority() int {
	return PriorityNormal
}
