package queue

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// TestNewLockFreeQueue 测试队列创建
func TestNewLockFreeQueue(t *testing.T) {
	tests := []struct {
		name     string
		capacity uint64
		expected uint64
	}{
		{"Zero capacity", 0, 16},
		{"Power of two", 8, 8},
		{"Non-power of two", 10, 16},
		{"Large capacity", 100, 128},
		{"Already power of two", 64, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewLockFreeQueue(tt.capacity)
			if q.capacity != tt.expected {
				t.Errorf("Expected capacity %d, got %d", tt.expected, q.capacity)
			}
			if q.Capacity() != tt.expected-1 {
				t.Errorf("Expected usable capacity %d, got %d", tt.expected-1, q.Capacity())
			}
			if !q.IsEmpty() {
				t.Error("New queue should be empty")
			}
			if q.IsFull() {
				t.Error("New queue should not be full")
			}
		})
	}
}

// TestBasicEnqueueDequeue 测试基本的入队出队操作
func TestBasicEnqueueDequeue(t *testing.T) {
	q := NewLockFreeQueue(8)

	// 测试入队
	item1 := unsafe.Pointer(&struct{ value int }{1})
	item2 := unsafe.Pointer(&struct{ value int }{2})

	if !q.Enqueue(item1) {
		t.Error("Failed to enqueue item1")
	}
	if !q.Enqueue(item2) {
		t.Error("Failed to enqueue item2")
	}

	if q.IsEmpty() {
		t.Error("Queue should not be empty after enqueue")
	}
	if q.Size() != 2 {
		t.Errorf("Expected size 2, got %d", q.Size())
	}

	// 测试出队
	dequeued1 := q.Dequeue()
	if dequeued1 != item1 {
		t.Error("Dequeued item should be item1")
	}

	dequeued2 := q.Dequeue()
	if dequeued2 != item2 {
		t.Error("Dequeued item should be item2")
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after dequeuing all items")
	}
	if q.Size() != 0 {
		t.Errorf("Expected size 0, got %d", q.Size())
	}
}

// TestEnqueueNil 测试入队nil值
func TestEnqueueNil(t *testing.T) {
	q := NewLockFreeQueue(8)

	if q.Enqueue(nil) {
		t.Error("Should not be able to enqueue nil")
	}

	if !q.IsEmpty() {
		t.Error("Queue should remain empty after failed nil enqueue")
	}
}

// TestDequeueEmpty 测试从空队列出队
func TestDequeueEmpty(t *testing.T) {
	q := NewLockFreeQueue(8)

	item := q.Dequeue()
	if item != nil {
		t.Error("Dequeue from empty queue should return nil")
	}
}

// TestQueueFull 测试队列满的情况
func TestQueueFull(t *testing.T) {
	q := NewLockFreeQueue(4) // 实际可用容量为3

	// 填满队列
	items := make([]unsafe.Pointer, 3)
	for i := 0; i < 3; i++ {
		items[i] = unsafe.Pointer(&struct{ value int }{i})
		if !q.Enqueue(items[i]) {
			t.Errorf("Failed to enqueue item %d", i)
		}
	}

	if !q.IsFull() {
		t.Error("Queue should be full")
	}

	// 尝试再次入队应该失败
	extraItem := unsafe.Pointer(&struct{ value int }{999})
	if q.Enqueue(extraItem) {
		t.Error("Should not be able to enqueue to full queue")
	}

	// 出队一个元素后应该可以再次入队
	q.Dequeue()
	if q.IsFull() {
		t.Error("Queue should not be full after dequeue")
	}

	if !q.Enqueue(extraItem) {
		t.Error("Should be able to enqueue after dequeue")
	}
}

// TestConcurrentEnqueue 测试并发入队
func TestConcurrentEnqueue(t *testing.T) {
	q := NewLockFreeQueue(1024)
	numGoroutines := 10
	itemsPerGoroutine := 100
	totalItems := numGoroutines * itemsPerGoroutine

	var wg sync.WaitGroup
	var successCount int64

	// 启动多个协程并发入队
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < itemsPerGoroutine; j++ {
				item := unsafe.Pointer(&struct {
					goroutineID int
					itemID      int
				}{goroutineID, j})

				if q.Enqueue(item) {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	if successCount != int64(totalItems) {
		t.Errorf("Expected %d successful enqueues, got %d", totalItems, successCount)
	}

	if q.Size() != uint64(totalItems) {
		t.Errorf("Expected queue size %d, got %d", totalItems, q.Size())
	}
}

// TestConcurrentDequeue 测试并发出队
func TestConcurrentDequeue(t *testing.T) {
	q := NewLockFreeQueue(1024)
	numItems := 1000

	// 先填充队列
	for i := 0; i < numItems; i++ {
		item := unsafe.Pointer(&struct{ value int }{i})
		q.Enqueue(item)
	}

	numGoroutines := 10
	var wg sync.WaitGroup
	var successCount int64
	dequeued := make([]unsafe.Pointer, numItems)
	var index int64

	// 启动多个协程并发出队
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				item := q.Dequeue()
				if item == nil {
					break
				}
				idx := atomic.AddInt64(&index, 1) - 1
				if idx < int64(len(dequeued)) {
					dequeued[idx] = item
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	if successCount != int64(numItems) {
		t.Errorf("Expected %d successful dequeues, got %d", numItems, successCount)
	}

	if !q.IsEmpty() {
		t.Error("Queue should be empty after all items dequeued")
	}
}

// TestConcurrentEnqueueDequeue 测试并发入队和出队
func TestConcurrentEnqueueDequeue(t *testing.T) {
	q := NewLockFreeQueue(256)
	duration := 100 * time.Millisecond
	numProducers := 5
	numConsumers := 5

	var wg sync.WaitGroup
	var enqueueCount, dequeueCount int64
	done := make(chan struct{})

	// 启动生产者协程
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-done:
					return
				default:
					item := unsafe.Pointer(&struct {
						producerID int
						counter    int
					}{producerID, counter})

					if q.Enqueue(item) {
						atomic.AddInt64(&enqueueCount, 1)
						counter++
					}
					runtime.Gosched() // 让出CPU时间
				}
			}
		}(i)
	}

	// 启动消费者协程
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					// 继续消费剩余的项目
					for q.Dequeue() != nil {
						atomic.AddInt64(&dequeueCount, 1)
					}
					return
				default:
					if item := q.Dequeue(); item != nil {
						atomic.AddInt64(&dequeueCount, 1)
					}
					runtime.Gosched() // 让出CPU时间
				}
			}
		}()
	}

	// 运行指定时间
	time.Sleep(duration)
	close(done)
	wg.Wait()

	t.Logf("Enqueued: %d, Dequeued: %d", enqueueCount, dequeueCount)

	// 验证没有数据丢失
	if enqueueCount < dequeueCount {
		t.Errorf("Dequeue count (%d) should not exceed enqueue count (%d)", dequeueCount, enqueueCount)
	}

	// 验证队列大小
	expectedSize := enqueueCount - dequeueCount
	actualSize := int64(q.Size())
	if actualSize != expectedSize {
		t.Errorf("Expected queue size %d, got %d", expectedSize, actualSize)
	}
}

// TestQueueStress 压力测试
func TestQueueStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := NewLockFreeQueue(1024)
	numGoroutines := 20
	operationsPerGoroutine := 10000

	var wg sync.WaitGroup
	var totalEnqueues, totalDequeues int64

	// 启动多个协程进行混合操作
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// 随机选择操作类型
				if j%2 == 0 {
					// 入队操作
					item := unsafe.Pointer(&struct {
						goroutineID int
						operation   int
					}{goroutineID, j})

					if q.Enqueue(item) {
						atomic.AddInt64(&totalEnqueues, 1)
					}
				} else {
					// 出队操作
					if q.Dequeue() != nil {
						atomic.AddInt64(&totalDequeues, 1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Total enqueues: %d, Total dequeues: %d", totalEnqueues, totalDequeues)
	t.Logf("Final queue size: %d", q.Size())

	// 验证队列状态一致性
	expectedSize := totalEnqueues - totalDequeues
	actualSize := int64(q.Size())
	if actualSize != expectedSize {
		t.Errorf("Expected queue size %d, got %d", expectedSize, actualSize)
	}
}

// TestNextPowerOfTwo 测试nextPowerOfTwo函数
func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		input    uint64
		expected uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{8, 8},
		{9, 16},
		{15, 16},
		{16, 16},
		{17, 32},
		{63, 64},
		{64, 64},
		{65, 128},
		{1023, 1024},
		{1024, 1024},
		{1025, 2048},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := nextPowerOfTwo(tt.input)
			if result != tt.expected {
				t.Errorf("nextPowerOfTwo(%d) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

// BenchmarkEnqueue 入队性能基准测试
func BenchmarkEnqueue(b *testing.B) {
	q := NewLockFreeQueue(uint64(b.N))
	items := make([]unsafe.Pointer, b.N)
	for i := 0; i < b.N; i++ {
		items[i] = unsafe.Pointer(&struct{ value int }{i})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			q.Enqueue(items[i%len(items)])
			i++
		}
	})
}

// BenchmarkDequeue 出队性能基准测试
func BenchmarkDequeue(b *testing.B) {
	q := NewLockFreeQueue(uint64(b.N))

	// 预填充队列
	for i := 0; i < b.N; i++ {
		item := unsafe.Pointer(&struct{ value int }{i})
		q.Enqueue(item)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Dequeue()
		}
	})
}

// TestQueueFullEmptyHandling 测试队列满和空的并发处理
func TestQueueFullEmptyHandling(t *testing.T) {
	q := NewLockFreeQueue(8) // 小容量便于测试
	numGoroutines := 4
	duration := 10 * time.Millisecond

	var wg sync.WaitGroup
	var fullCount, emptyCount int64
	done := make(chan struct{})

	// 启动生产者协程 - 尝试填满队列
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-done:
					return
				default:
					item := unsafe.Pointer(&struct {
						producerID int
						counter    int
					}{producerID, counter})

					if !q.Enqueue(item) {
						atomic.AddInt64(&fullCount, 1)
					}
					counter++
					runtime.Gosched()
				}
			}
		}(i)
	}

	// 启动消费者协程 - 尝试清空队列
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					if q.Dequeue() == nil {
						atomic.AddInt64(&emptyCount, 1)
					}
					runtime.Gosched()
				}
			}
		}()
	}

	time.Sleep(duration)
	close(done)
	wg.Wait()

	t.Logf("Queue full attempts: %d, Queue empty attempts: %d", fullCount, emptyCount)
	t.Logf("Final queue size: %d, Is full: %v, Is empty: %v", q.Size(), q.IsFull(), q.IsEmpty())

	// 验证队列状态的一致性
	if q.IsFull() && q.IsEmpty() {
		t.Error("Queue cannot be both full and empty")
	}

	// 验证队列大小与状态的一致性
	if q.IsEmpty() && q.Size() != 0 {
		t.Errorf("Empty queue should have size 0, got %d", q.Size())
	}

	if q.IsFull() && q.Size() != q.Capacity() {
		t.Errorf("Full queue should have size %d, got %d", q.Capacity(), q.Size())
	}
}

// BenchmarkMixedOperations 混合操作性能基准测试
func BenchmarkMixedOperations(b *testing.B) {
	q := NewLockFreeQueue(1024)

	// 预填充队列到一半容量，确保有足够空间进行入队和出队
	halfCapacity := int(q.Capacity() / 2)
	for i := 0; i < halfCapacity; i++ {
		item := unsafe.Pointer(&struct{ value int }{i})
		q.Enqueue(item)
	}

	b.ResetTimer()
	// 使用单线程测试避免复杂的并发问题
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			// 尝试入队
			item := unsafe.Pointer(&struct{ value int }{i})
			q.Enqueue(item)
		} else {
			// 尝试出队
			q.Dequeue()
		}
	}
}
