package pool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestNewTaskWrapper tests the creation of task wrappers
func TestNewTaskWrapper(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "test", nil
	}, PriorityNormal)

	// Test without timeout
	wrapper := newTaskWrapper(task, 0, objectPool)
	if wrapper == nil {
		t.Fatal("Expected non-nil task wrapper")
	}
	if wrapper.task != task {
		t.Error("Expected task to be set correctly")
	}
	if wrapper.priority != PriorityNormal {
		t.Error("Expected priority to be PriorityNormal")
	}
	if wrapper.future == nil {
		t.Error("Expected future to be created")
	}
	if wrapper.ctx == nil {
		t.Error("Expected context to be created")
	}
	if wrapper.cancel == nil {
		t.Error("Expected cancel function to be created")
	}

	// Test with timeout
	timeout := 100 * time.Millisecond
	wrapper2 := newTaskWrapper(task, timeout, objectPool)
	if wrapper2.timeout != timeout {
		t.Error("Expected timeout to be set correctly")
	}

	// Clean up
	objectPool.PutTaskWrapper(wrapper)
	objectPool.PutTaskWrapper(wrapper2)
}

// TestTaskWrapper_Execute tests task execution
func TestTaskWrapper_Execute(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	t.Run("successful execution", func(t *testing.T) {
		expectedResult := "success"
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return expectedResult, nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Execute in goroutine
		go wrapper.Execute()

		// Wait for completion and check result
		result, err := wrapper.future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
		if !wrapper.future.IsDone() {
			t.Error("Expected future to be done")
		}
	})

	t.Run("execution with error", func(t *testing.T) {
		expectedError := errors.New("test error")
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return nil, expectedError
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.Get()
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})

	t.Run("execution with panic", func(t *testing.T) {
		panicValue := "test panic"
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			panic(panicValue)
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.Get()
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}

		panicErr, ok := err.(*PanicError)
		if !ok {
			t.Errorf("Expected PanicError, got %T", err)
		} else if panicErr.Value != panicValue {
			t.Errorf("Expected panic value %v, got %v", panicValue, panicErr.Value)
		}
	})

	t.Run("execution with cancellation", func(t *testing.T) {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Cancel before execution
		wrapper.cancel()

		go wrapper.Execute()

		result, err := wrapper.future.Get()
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})

	t.Run("execution with timeout", func(t *testing.T) {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return "result", nil
			}
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 50*time.Millisecond, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.Get()
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
	})
}

// TestTaskWrapper_Cancel tests task cancellation
func TestTaskWrapper_Cancel(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "result", nil
	}, PriorityNormal)

	wrapper := newTaskWrapper(task, 0, objectPool)
	defer objectPool.PutTaskWrapper(wrapper)

	// Cancel the task
	cancelled := wrapper.Cancel()
	if !cancelled {
		t.Error("Expected cancellation to succeed")
	}

	// Check if task is cancelled
	if !wrapper.IsCancelled() {
		t.Error("Expected task to be cancelled")
	}

	// Try to cancel again
	cancelled2 := wrapper.Cancel()
	if cancelled2 {
		t.Error("Expected second cancellation to fail")
	}
}

// TestTaskWrapper_IsTimeout tests timeout detection
func TestTaskWrapper_IsTimeout(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "result", nil
	}, PriorityNormal)

	// Test with timeout
	wrapper := newTaskWrapper(task, 50*time.Millisecond, objectPool)
	defer objectPool.PutTaskWrapper(wrapper)

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	if !wrapper.IsTimeout() {
		t.Error("Expected task to be timed out")
	}
}

// TestFuture_Get tests the Get method
func TestFuture_Get(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	t.Run("get result", func(t *testing.T) {
		expectedResult := "test result"
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return expectedResult, nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Start execution
		go wrapper.Execute()

		// Get result
		result, err := wrapper.future.Get()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("get error", func(t *testing.T) {
		expectedError := errors.New("test error")
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return nil, expectedError
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.Get()
		if err != expectedError {
			t.Errorf("Expected error %v, got %v", expectedError, err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
	})
}

// TestFuture_GetWithTimeout tests the GetWithTimeout method
func TestFuture_GetWithTimeout(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	t.Run("get result within timeout", func(t *testing.T) {
		expectedResult := "fast result"
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return expectedResult, nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.GetWithTimeout(100 * time.Millisecond)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("timeout before completion", func(t *testing.T) {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			time.Sleep(100 * time.Millisecond)
			return "slow result", nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		go wrapper.Execute()

		result, err := wrapper.future.GetWithTimeout(10 * time.Millisecond)
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}

		if err != ErrTaskTimeout {
			t.Errorf("Expected ErrTaskTimeout, got %v", err)
		}
	})
}

// TestFuture_IsDone tests the IsDone method
func TestFuture_IsDone(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		time.Sleep(50 * time.Millisecond)
		return "result", nil
	}, PriorityNormal)

	wrapper := newTaskWrapper(task, 0, objectPool)
	defer objectPool.PutTaskWrapper(wrapper)

	// Initially not done
	if wrapper.future.IsDone() {
		t.Error("Expected future to not be done initially")
	}

	// Start execution
	go wrapper.Execute()

	// Wait a bit and check again
	time.Sleep(10 * time.Millisecond)
	if wrapper.future.IsDone() {
		t.Error("Expected future to not be done yet")
	}

	// Wait for completion
	wrapper.future.Get()
	if !wrapper.future.IsDone() {
		t.Error("Expected future to be done after completion")
	}
}

// TestFuture_Cancel tests the Cancel method
func TestFuture_Cancel(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	t.Run("cancel before execution", func(t *testing.T) {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return "result", nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Cancel before execution
		cancelled := wrapper.future.Cancel()
		if !cancelled {
			t.Error("Expected cancellation to succeed")
		}

		// Check if cancelled
		if !wrapper.future.IsCancelled() {
			t.Error("Expected future to be cancelled")
		}

		// Try to cancel again
		cancelled2 := wrapper.future.Cancel()
		if cancelled2 {
			t.Error("Expected second cancellation to fail")
		}
	})

	t.Run("cancel during execution", func(t *testing.T) {
		started := make(chan struct{})
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Start execution
		go wrapper.Execute()
		<-started // Wait for task to start

		// Cancel during execution
		cancelled := wrapper.future.Cancel()
		if !cancelled {
			t.Error("Expected cancellation to succeed")
		}

		// Check result
		result, err := wrapper.future.Get()
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
		if err != ErrTaskCancelled {
			t.Errorf("Expected ErrTaskCancelled, got %v", err)
		}
	})

	t.Run("cancel after completion", func(t *testing.T) {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return "result", nil
		}, PriorityNormal)

		wrapper := newTaskWrapper(task, 0, objectPool)
		defer objectPool.PutTaskWrapper(wrapper)

		// Execute and wait for completion
		go wrapper.Execute()
		wrapper.future.Get()

		// Try to cancel after completion
		cancelled := wrapper.future.Cancel()
		if cancelled {
			t.Error("Expected cancellation to fail after completion")
		}
	})
}

// TestSimpleTaskImplementation tests the SimpleTask implementation
func TestSimpleTaskImplementation(t *testing.T) {
	expectedResult := "simple result"
	expectedPriority := PriorityHigh

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return expectedResult, nil
	}, expectedPriority)

	if task.Priority() != expectedPriority {
		t.Errorf("Expected priority %d, got %d", expectedPriority, task.Priority())
	}

	result, err := task.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != expectedResult {
		t.Errorf("Expected result %v, got %v", expectedResult, result)
	}
}

// TestFuncTaskImplementation tests the FuncTask implementation
func TestFuncTaskImplementation(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		expectedResult := "func result"
		expectedPriority := PriorityCritical

		task := NewFuncTask(func() (any, error) {
			return expectedResult, nil
		}, expectedPriority)

		if task.Priority() != expectedPriority {
			t.Errorf("Expected priority %d, got %d", expectedPriority, task.Priority())
		}

		result, err := task.Execute(context.Background())
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != expectedResult {
			t.Errorf("Expected result %v, got %v", expectedResult, result)
		}
	})

	t.Run("execution with cancelled context", func(t *testing.T) {
		task := NewFuncTask(func() (any, error) {
			return "result", nil
		}, PriorityNormal)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result, err := task.Execute(ctx)
		if result != nil {
			t.Errorf("Expected nil result, got %v", result)
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})
}

// TestTaskWrapper_Reset tests the Reset method for object pool reuse
func TestTaskWrapper_Reset(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "result", nil
	}, PriorityNormal)

	wrapper := newTaskWrapper(task, 100*time.Millisecond, objectPool)

	// Verify initial state
	if wrapper.task == nil {
		t.Error("Expected task to be set")
	}
	if wrapper.future == nil {
		t.Error("Expected future to be set")
	}

	// Reset the wrapper
	wrapper.Reset()

	// Verify reset state
	if wrapper.task != nil {
		t.Error("Expected task to be nil after reset")
	}
	if wrapper.future != nil {
		t.Error("Expected future to be nil after reset")
	}
	if !wrapper.submitTime.IsZero() {
		t.Error("Expected submitTime to be zero after reset")
	}
	if wrapper.priority != 0 {
		t.Error("Expected priority to be 0 after reset")
	}
	if wrapper.timeout != 0 {
		t.Error("Expected timeout to be 0 after reset")
	}
	if wrapper.ctx != nil {
		t.Error("Expected ctx to be nil after reset")
	}
	if wrapper.cancel != nil {
		t.Error("Expected cancel to be nil after reset")
	}
	if wrapper.next != nil {
		t.Error("Expected next to be nil after reset")
	}
}

// TestConcurrentFutureOperations tests concurrent operations on Future
func TestConcurrentFutureOperations(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		time.Sleep(10 * time.Millisecond) // Reduced sleep time
		return "concurrent result", nil
	}, PriorityNormal)

	wrapper := newTaskWrapper(task, 0, objectPool)
	defer objectPool.PutTaskWrapper(wrapper)

	// Start execution
	go wrapper.Execute()

	// Concurrent operations
	var wg sync.WaitGroup
	results := make([]any, 5) // Reduced number of goroutines
	errors := make([]error, 5)

	// Multiple goroutines trying to get the result
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errors[index] = wrapper.future.GetWithTimeout(1 * time.Second) // Add timeout
		}(i)
	}

	// Add timeout for the wait
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for concurrent operations")
	}

	// All should get the same result
	expectedResult := "concurrent result"
	for i := 0; i < 5; i++ {
		if errors[i] != nil {
			t.Errorf("Expected no error at index %d, got %v", i, errors[i])
		}
		if results[i] != expectedResult {
			t.Errorf("Expected result %v at index %d, got %v", expectedResult, i, results[i])
		}
	}
}

// TestTaskPriority tests task priority handling
func TestTaskPriority(t *testing.T) {
	config := &Config{
		ObjectPoolSize: 10,
		PreAlloc:       false,
	}
	objectPool := NewObjectPool(config)

	priorities := []int{PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical}

	for _, priority := range priorities {
		task := NewSimpleTask(func(ctx context.Context) (any, error) {
			return "result", nil
		}, priority)

		wrapper := newTaskWrapper(task, 0, objectPool)

		if wrapper.priority != priority {
			t.Errorf("Expected priority %d, got %d", priority, wrapper.priority)
		}

		objectPool.PutTaskWrapper(wrapper)
	}
}

// BenchmarkTaskWrapper_Execute benchmarks task execution
func BenchmarkTaskWrapper_Execute(b *testing.B) {
	config := &Config{
		ObjectPoolSize: 100,
		PreAlloc:       true,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark result", nil
	}, PriorityNormal)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper := newTaskWrapper(task, 0, objectPool)
		go wrapper.Execute()
		wrapper.future.Get()
		objectPool.PutTaskWrapper(wrapper)
	}
}

// BenchmarkFuture_Get benchmarks Future.Get operation
func BenchmarkFuture_Get(b *testing.B) {
	config := &Config{
		ObjectPoolSize: 100,
		PreAlloc:       true,
	}
	objectPool := NewObjectPool(config)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark result", nil
	}, PriorityNormal)

	wrappers := make([]*taskWrapper, b.N)
	for i := 0; i < b.N; i++ {
		wrappers[i] = newTaskWrapper(task, 0, objectPool)
		go wrappers[i].Execute()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrappers[i].future.Get()
		objectPool.PutTaskWrapper(wrappers[i])
	}
}

// BenchmarkObjectPool_GetPut benchmarks object pool operations
func BenchmarkObjectPool_GetPut(b *testing.B) {
	config := &Config{
		ObjectPoolSize: 100,
		PreAlloc:       true,
	}
	objectPool := NewObjectPool(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper := objectPool.GetTaskWrapper()
		future := objectPool.GetFuture()
		objectPool.PutTaskWrapper(wrapper)
		objectPool.PutFuture(future)
	}
}
