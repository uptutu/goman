package pool

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewObjectPool(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	if pool == nil {
		t.Fatal("Expected non-nil object pool")
	}

	if pool.config != config {
		t.Error("Expected config to be set")
	}

	if pool.stats == nil {
		t.Error("Expected stats to be initialized")
	}
}

func TestNewObjectPoolWithPreAlloc(t *testing.T) {
	config := NewConfig()
	config.PreAlloc = true
	config.ObjectPoolSize = 10

	pool := NewObjectPool(config)

	if !pool.preAllocated {
		t.Error("Expected preAllocated to be true")
	}

	// 验证预分配的对象可以被获取
	wrapper := pool.GetTaskWrapper()
	if wrapper == nil {
		t.Error("Expected to get pre-allocated taskWrapper")
	}
	pool.PutTaskWrapper(wrapper)

	future := pool.GetFuture()
	if future == nil {
		t.Error("Expected to get pre-allocated future")
	}
	pool.PutFuture(future)
}

func TestTaskWrapperPoolOperations(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 测试获取taskWrapper
	wrapper := pool.GetTaskWrapper()
	if wrapper == nil {
		t.Fatal("Expected non-nil taskWrapper")
	}

	// 验证初始状态
	if wrapper.task != nil {
		t.Error("Expected task to be nil")
	}
	if wrapper.future != nil {
		t.Error("Expected future to be nil")
	}
	if wrapper.priority != PriorityNormal {
		t.Error("Expected priority to be PriorityNormal")
	}

	// 设置一些值
	wrapper.priority = PriorityHigh
	wrapper.submitTime = time.Now()

	// 归还对象
	pool.PutTaskWrapper(wrapper)

	// 再次获取，应该是重置后的对象
	wrapper2 := pool.GetTaskWrapper()
	if wrapper2.priority != PriorityNormal {
		t.Error("Expected priority to be reset to PriorityNormal")
	}
	if !wrapper2.submitTime.IsZero() {
		t.Error("Expected submitTime to be reset")
	}

	pool.PutTaskWrapper(wrapper2)
}

func TestFuturePoolOperations(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 测试获取Future
	future := pool.GetFuture()
	if future == nil {
		t.Fatal("Expected non-nil future")
	}

	// 验证初始状态
	if future.IsDone() {
		t.Error("Expected future to not be done initially")
	}
	if future.result != nil {
		t.Error("Expected result to be nil")
	}
	if future.err != nil {
		t.Error("Expected err to be nil")
	}

	// 设置结果
	future.setResult("test", nil)
	if !future.IsDone() {
		t.Error("Expected future to be done after setting result")
	}

	result, err := future.Get()
	if result != "test" {
		t.Errorf("Expected result to be 'test', got %v", result)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 归还对象
	pool.PutFuture(future)

	// 再次获取，应该是重置后的对象
	future2 := pool.GetFuture()
	if future2.IsDone() {
		t.Error("Expected future to be reset and not done")
	}
	if future2.result != nil {
		t.Error("Expected result to be reset to nil")
	}

	pool.PutFuture(future2)
}

func TestContextPoolOperations(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 测试获取Context
	ctx := pool.GetContext()
	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	// 归还Context
	pool.PutContext(ctx)

	// 测试归还nil context
	pool.PutContext(nil) // 应该不会panic
}

func TestObjectPoolFutureInterface(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)
	future := pool.GetFuture()
	defer pool.PutFuture(future)

	// 测试Get方法（超时场景）
	go func() {
		time.Sleep(100 * time.Millisecond)
		future.setResult("delayed", nil)
	}()

	result, err := future.Get()
	if result != "delayed" {
		t.Errorf("Expected result to be 'delayed', got %v", result)
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestFutureGetWithTimeout(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)
	future := pool.GetFuture()
	defer pool.PutFuture(future)

	// 测试超时
	result, err := future.GetWithTimeout(50 * time.Millisecond)
	if err != ErrTaskTimeout {
		t.Errorf("Expected timeout error, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result on timeout, got %v", result)
	}

	// 测试正常获取
	future2 := pool.GetFuture()
	defer pool.PutFuture(future2)

	go func() {
		time.Sleep(10 * time.Millisecond)
		future2.setResult("quick", nil)
	}()

	result, err = future2.GetWithTimeout(100 * time.Millisecond)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "quick" {
		t.Errorf("Expected result to be 'quick', got %v", result)
	}
}

func TestFutureCancel(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)
	future := pool.GetFuture()
	defer pool.PutFuture(future)

	// 测试取消
	if !future.Cancel() {
		t.Error("Expected Cancel to return true")
	}

	// 再次取消应该返回false
	if future.Cancel() {
		t.Error("Expected second Cancel to return false")
	}

	// 获取结果应该返回取消错误
	result, err := future.Get()
	if err != ErrTaskCancelled {
		t.Errorf("Expected cancelled error, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result on cancel, got %v", result)
	}
}

func TestFutureCancelAfterDone(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)
	future := pool.GetFuture()
	defer pool.PutFuture(future)

	// 先设置结果
	future.setResult("done", nil)

	// 然后尝试取消，应该失败
	if future.Cancel() {
		t.Error("Expected Cancel to return false after done")
	}
}

func TestObjectPoolStats(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 初始统计应该为0
	stats := pool.GetStats()
	if stats.TaskWrapperGets != 0 {
		t.Error("Expected initial TaskWrapperGets to be 0")
	}

	// 执行一些操作
	wrapper := pool.GetTaskWrapper()
	pool.PutTaskWrapper(wrapper)

	future := pool.GetFuture()
	pool.PutFuture(future)

	ctx := pool.GetContext()
	pool.PutContext(ctx)

	// 检查统计
	stats = pool.GetStats()
	if stats.TaskWrapperGets != 1 {
		t.Errorf("Expected TaskWrapperGets to be 1, got %d", stats.TaskWrapperGets)
	}
	if stats.TaskWrapperPuts != 1 {
		t.Errorf("Expected TaskWrapperPuts to be 1, got %d", stats.TaskWrapperPuts)
	}
	if stats.FutureGets != 1 {
		t.Errorf("Expected FutureGets to be 1, got %d", stats.FutureGets)
	}
	if stats.FuturePuts != 1 {
		t.Errorf("Expected FuturePuts to be 1, got %d", stats.FuturePuts)
	}
	if stats.ContextGets != 1 {
		t.Errorf("Expected ContextGets to be 1, got %d", stats.ContextGets)
	}
	if stats.ContextPuts != 1 {
		t.Errorf("Expected ContextPuts to be 1, got %d", stats.ContextPuts)
	}

	// 检查命中率
	if stats.TaskWrapperHitRate != 1.0 {
		t.Errorf("Expected TaskWrapperHitRate to be 1.0, got %f", stats.TaskWrapperHitRate)
	}
}

func TestObjectPoolReset(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 执行一些操作
	wrapper := pool.GetTaskWrapper()
	pool.PutTaskWrapper(wrapper)

	// 重置统计
	pool.Reset()

	// 检查统计被重置
	stats := pool.GetStats()
	if stats.TaskWrapperGets != 0 {
		t.Error("Expected TaskWrapperGets to be 0 after reset")
	}
	if stats.TaskWrapperPuts != 0 {
		t.Error("Expected TaskWrapperPuts to be 0 after reset")
	}
}

func TestObjectPoolWarm(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 预热对象池
	pool.Warm(5)

	// 验证统计信息
	stats := pool.GetStats()
	if stats.TaskWrapperGets < 5 {
		t.Errorf("Expected at least 5 TaskWrapperGets, got %d", stats.TaskWrapperGets)
	}
	if stats.FutureGets < 5 {
		t.Errorf("Expected at least 5 FutureGets, got %d", stats.FutureGets)
	}
}

func TestObjectPoolConcurrency(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	const numGoroutines = 100
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发获取和归还对象
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				// taskWrapper操作
				wrapper := pool.GetTaskWrapper()
				wrapper.priority = PriorityHigh
				pool.PutTaskWrapper(wrapper)

				// Future操作
				future := pool.GetFuture()
				future.setResult("test", nil)
				pool.PutFuture(future)

				// Context操作
				ctx := pool.GetContext()
				pool.PutContext(ctx)
			}
		}()
	}

	wg.Wait()

	// 验证统计信息
	stats := pool.GetStats()
	expectedOps := int64(numGoroutines * numOperations)

	if stats.TaskWrapperGets != expectedOps {
		t.Errorf("Expected %d TaskWrapperGets, got %d", expectedOps, stats.TaskWrapperGets)
	}
	if stats.TaskWrapperPuts != expectedOps {
		t.Errorf("Expected %d TaskWrapperPuts, got %d", expectedOps, stats.TaskWrapperPuts)
	}
}

func TestPutNilObjects(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 测试归还nil对象不会panic
	pool.PutTaskWrapper(nil)
	pool.PutFuture(nil)
	pool.PutContext(nil)

	// 验证统计信息没有被错误更新
	stats := pool.GetStats()
	if stats.TaskWrapperPuts != 0 {
		t.Error("Expected TaskWrapperPuts to be 0 when putting nil")
	}
	if stats.FuturePuts != 0 {
		t.Error("Expected FuturePuts to be 0 when putting nil")
	}
	if stats.ContextPuts != 0 {
		t.Error("Expected ContextPuts to be 0 when putting nil")
	}
}

// 简单的Task实现用于测试
type objectPoolTestTask struct {
	value    string
	priority int
}

func (t *objectPoolTestTask) Execute(ctx context.Context) (any, error) {
	return t.value, nil
}

func (t *objectPoolTestTask) Priority() int {
	return t.priority
}

func TestTaskWrapperWithRealTask(t *testing.T) {
	config := NewConfig()
	pool := NewObjectPool(config)

	wrapper := pool.GetTaskWrapper()
	defer pool.PutTaskWrapper(wrapper)

	task := &objectPoolTestTask{value: "test", priority: PriorityHigh}
	wrapper.task = task
	wrapper.priority = task.Priority()
	wrapper.submitTime = time.Now()

	// 验证设置的值
	if wrapper.task != task {
		t.Error("Expected task to be set")
	}
	if wrapper.priority != PriorityHigh {
		t.Error("Expected priority to be PriorityHigh")
	}
	if wrapper.submitTime.IsZero() {
		t.Error("Expected submitTime to be set")
	}
}

// Benchmark tests for object pool performance

func BenchmarkObjectPool_TaskWrapper(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wrapper := pool.GetTaskWrapper()
			wrapper.priority = PriorityHigh
			pool.PutTaskWrapper(wrapper)
		}
	})
}

func BenchmarkObjectPool_Future(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			future := pool.GetFuture()
			future.setResult("test", nil)
			pool.PutFuture(future)
		}
	})
}

func BenchmarkObjectPool_Context(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := pool.GetContext()
			pool.PutContext(ctx)
		}
	})
}

func BenchmarkObjectPool_Mixed(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 混合操作
			wrapper := pool.GetTaskWrapper()
			future := pool.GetFuture()
			ctx := pool.GetContext()

			wrapper.priority = PriorityNormal
			future.setResult("mixed", nil)

			pool.PutTaskWrapper(wrapper)
			pool.PutFuture(future)
			pool.PutContext(ctx)
		}
	})
}

func BenchmarkObjectPool_WithPreAlloc(b *testing.B) {
	config := NewConfig()
	config.PreAlloc = true
	config.ObjectPoolSize = 1000
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wrapper := pool.GetTaskWrapper()
			wrapper.priority = PriorityHigh
			pool.PutTaskWrapper(wrapper)
		}
	})
}

func BenchmarkObjectPool_WithoutPreAlloc(b *testing.B) {
	config := NewConfig()
	config.PreAlloc = false
	pool := NewObjectPool(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wrapper := pool.GetTaskWrapper()
			wrapper.priority = PriorityHigh
			pool.PutTaskWrapper(wrapper)
		}
	})
}

func BenchmarkObjectPool_CompareWithDirect(b *testing.B) {
	b.Run("WithPool", func(b *testing.B) {
		config := NewConfig()
		pool := NewObjectPool(config)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				wrapper := pool.GetTaskWrapper()
				wrapper.priority = PriorityHigh
				pool.PutTaskWrapper(wrapper)
			}
		})
	})

	b.Run("DirectAllocation", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				wrapper := &taskWrapper{}
				wrapper.priority = PriorityHigh
				// 直接分配，不使用对象池
				_ = wrapper
			}
		})
	})
}

func BenchmarkFuture_Operations(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	b.Run("SetAndGet", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			future := pool.GetFuture()
			future.setResult("test", nil)
			result, _ := future.Get()
			_ = result
			pool.PutFuture(future)
		}
	})

	b.Run("Cancel", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			future := pool.GetFuture()
			future.Cancel()
			pool.PutFuture(future)
		}
	})

	b.Run("IsDone", func(b *testing.B) {
		future := pool.GetFuture()
		future.setResult("test", nil)
		defer pool.PutFuture(future)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = future.IsDone()
		}
	})
}

func BenchmarkObjectPool_Stats(b *testing.B) {
	config := NewConfig()
	pool := NewObjectPool(config)

	// 执行一些操作以产生统计数据
	for i := 0; i < 100; i++ {
		wrapper := pool.GetTaskWrapper()
		pool.PutTaskWrapper(wrapper)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.GetStats()
	}
}

func BenchmarkObjectPool_Warm(b *testing.B) {
	config := NewConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool := NewObjectPool(config)
		pool.Warm(100)
	}
}
