package pool

import (
	"context"
	"testing"
	"time"
)

// Test SimpleTask implementation
func TestSimpleTask(t *testing.T) {
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

// Test SimpleTask with context cancellation
func TestSimpleTask_ContextCancellation(t *testing.T) {
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
			return "should not reach here", nil
		}
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
}

// Test FuncTask implementation
func TestFuncTask(t *testing.T) {
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
}

// Test FuncTask with context cancellation
func TestFuncTask_ContextCancellation(t *testing.T) {
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
}

// Test FuncTask with error
func TestFuncTask_WithError(t *testing.T) {
	expectedError := ErrTaskTimeout

	task := NewFuncTask(func() (any, error) {
		return nil, expectedError
	}, PriorityNormal)

	result, err := task.Execute(context.Background())
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
	if err != expectedError {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}

// Test SimpleTask with panic
func TestSimpleTask_WithPanic(t *testing.T) {
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		panic("test panic")
	}, PriorityNormal)

	// This should panic, but we'll test it in a controlled way
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic, but none occurred")
		} else if r != "test panic" {
			t.Errorf("Expected panic 'test panic', got %v", r)
		}
	}()

	task.Execute(context.Background())
}

// Test FuncTask with panic
func TestFuncTask_WithPanic(t *testing.T) {
	task := NewFuncTask(func() (any, error) {
		panic("func panic")
	}, PriorityNormal)

	// This should panic, but we'll test it in a controlled way
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic, but none occurred")
		} else if r != "func panic" {
			t.Errorf("Expected panic 'func panic', got %v", r)
		}
	}()

	task.Execute(context.Background())
}

// Test task priority values
func TestTaskPriorities(t *testing.T) {
	tasks := []struct {
		priority int
		name     string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
	}

	for _, tc := range tasks {
		t.Run(tc.name, func(t *testing.T) {
			simpleTask := NewSimpleTask(func(ctx context.Context) (any, error) {
				return "result", nil
			}, tc.priority)

			if simpleTask.Priority() != tc.priority {
				t.Errorf("Expected SimpleTask priority %d, got %d", tc.priority, simpleTask.Priority())
			}

			funcTask := NewFuncTask(func() (any, error) {
				return "result", nil
			}, tc.priority)

			if funcTask.Priority() != tc.priority {
				t.Errorf("Expected FuncTask priority %d, got %d", tc.priority, funcTask.Priority())
			}
		})
	}
}

// Test task execution with different contexts
func TestTaskExecution_WithDifferentContexts(t *testing.T) {
	// Test with background context
	task1 := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "background", nil
	}, PriorityNormal)

	result1, err1 := task1.Execute(context.Background())
	if err1 != nil {
		t.Errorf("Expected no error with background context, got %v", err1)
	}
	if result1 != "background" {
		t.Errorf("Expected result 'background', got %v", result1)
	}

	// Test with timeout context
	task2 := NewSimpleTask(func(ctx context.Context) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond * 10):
			return "timeout", nil
		}
	}, PriorityNormal)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	result2, err2 := task2.Execute(ctx)
	if err2 != nil {
		t.Errorf("Expected no error with timeout context, got %v", err2)
	}
	if result2 != "timeout" {
		t.Errorf("Expected result 'timeout', got %v", result2)
	}

	// Test with already cancelled context
	task3 := NewFuncTask(func() (any, error) {
		return "cancelled", nil
	}, PriorityNormal)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	result3, err3 := task3.Execute(cancelledCtx)
	if result3 != nil {
		t.Errorf("Expected nil result with cancelled context, got %v", result3)
	}
	if err3 != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err3)
	}
}

// Benchmark SimpleTask execution
func BenchmarkSimpleTask_Execute(b *testing.B) {
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark", nil
	}, PriorityNormal)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task.Execute(ctx)
	}
}

// Benchmark FuncTask execution
func BenchmarkFuncTask_Execute(b *testing.B) {
	task := NewFuncTask(func() (any, error) {
		return "benchmark", nil
	}, PriorityNormal)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task.Execute(ctx)
	}
}

// Benchmark task creation
func BenchmarkSimpleTask_Creation(b *testing.B) {
	fn := func(ctx context.Context) (any, error) {
		return "benchmark", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewSimpleTask(fn, PriorityNormal)
	}
}

func BenchmarkFuncTask_Creation(b *testing.B) {
	fn := func() (any, error) {
		return "benchmark", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewFuncTask(fn, PriorityNormal)
	}
}
