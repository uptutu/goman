package pool

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Test error constants
func TestErrorConstants(t *testing.T) {
	// Test that all error constants are defined and not nil
	errorConstants := []error{
		ErrPoolClosed,
		ErrQueueFull,
		ErrTaskTimeout,
		ErrTaskCancelled,
		ErrInvalidConfig,
		ErrInvalidWorkerCount,
		ErrInvalidQueueSize,
		ErrShutdownTimeout,
		ErrShutdownInProgress,
		ErrForceShutdown,
		ErrCircuitBreakerOpen,
	}

	for i, err := range errorConstants {
		if err == nil {
			t.Errorf("Error constant at index %d is nil", i)
		}
		if err.Error() == "" {
			t.Errorf("Error constant at index %d has empty message", i)
		}
	}

	// Test specific error messages
	if ErrPoolClosed.Error() != "pool is closed" {
		t.Errorf("Expected 'pool is closed', got '%s'", ErrPoolClosed.Error())
	}
	if ErrQueueFull.Error() != "task queue is full" {
		t.Errorf("Expected 'task queue is full', got '%s'", ErrQueueFull.Error())
	}
	if ErrTaskTimeout.Error() != "task execution timeout" {
		t.Errorf("Expected 'task execution timeout', got '%s'", ErrTaskTimeout.Error())
	}
}

// Test PoolError
func TestPoolError(t *testing.T) {
	// Test NewPoolError
	originalErr := errors.New("test error")
	err := NewPoolError("test operation", originalErr)
	if err == nil {
		t.Fatal("NewPoolError should not return nil")
	}

	if err.Op != "test operation" {
		t.Errorf("Expected op 'test operation', got '%s'", err.Op)
	}
	if err.Err != originalErr {
		t.Errorf("Expected original error, got %v", err.Err)
	}

	// Test Error method
	expectedError := "test operation: test error"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Test Unwrap method
	if err.Unwrap() != originalErr {
		t.Error("PoolError should unwrap to original error")
	}

	// Test with empty operation
	emptyOpErr := NewPoolError("", originalErr)
	if emptyOpErr.Error() != "test error" {
		t.Errorf("Expected 'test error' for empty op, got '%s'", emptyOpErr.Error())
	}
}

// Test TaskError
func TestTaskError(t *testing.T) {
	originalErr := errors.New("task failed")

	// Test NewTaskError with task ID
	err := NewTaskError("task-123", "execute", originalErr)
	if err == nil {
		t.Fatal("NewTaskError should not return nil")
	}

	if err.TaskID != "task-123" {
		t.Errorf("Expected TaskID 'task-123', got '%s'", err.TaskID)
	}
	if err.Op != "execute" {
		t.Errorf("Expected Op 'execute', got '%s'", err.Op)
	}
	if err.Err != originalErr {
		t.Errorf("Expected original error, got %v", err.Err)
	}

	// Test Error method
	expectedError := "task task-123 execute: task failed"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}

	// Test Unwrap method
	if err.Unwrap() != originalErr {
		t.Error("TaskError should unwrap to original error")
	}

	// Test with empty task ID
	emptyIDErr := NewTaskError("", "execute", originalErr)
	expectedEmptyIDError := "execute: task failed"
	if emptyIDErr.Error() != expectedEmptyIDError {
		t.Errorf("Expected error '%s' for empty task ID, got '%s'", expectedEmptyIDError, emptyIDErr.Error())
	}
}

// Test ConfigError
func TestConfigError(t *testing.T) {
	originalErr := errors.New("invalid value")

	// Test NewConfigError
	configErr := NewConfigError("WorkerCount", 0, originalErr)
	if configErr == nil {
		t.Fatal("NewConfigError should not return nil")
	}

	if configErr.Field != "WorkerCount" {
		t.Errorf("Expected field 'WorkerCount', got '%s'", configErr.Field)
	}
	if configErr.Value != 0 {
		t.Errorf("Expected value 0, got %v", configErr.Value)
	}
	if configErr.Err != originalErr {
		t.Errorf("Expected original error, got %v", configErr.Err)
	}

	// Test Error method
	expectedError := "config field 'WorkerCount' with value '0': invalid value"
	if configErr.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, configErr.Error())
	}

	// Test Unwrap method
	if configErr.Unwrap() != originalErr {
		t.Error("ConfigError should unwrap to original error")
	}
}

// Test PanicError
func TestPanicError(t *testing.T) {
	panicValue := "test panic value"
	task := &mockTask{id: "test-task"}

	// Test NewPanicError
	panicErr := NewPanicError(panicValue, task)
	if panicErr == nil {
		t.Fatal("NewPanicError should not return nil")
	}

	if panicErr.Value != panicValue {
		t.Errorf("Expected panic value '%v', got '%v'", panicValue, panicErr.Value)
	}
	if panicErr.Task != task {
		t.Errorf("Expected task %v, got %v", task, panicErr.Task)
	}

	// Test Error method
	expectedError := "task panic: test panic value"
	if panicErr.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, panicErr.Error())
	}

	// Test with nil panic value
	nilPanicErr := NewPanicError(nil, task)
	expectedNilError := "task panic: <nil>"
	if nilPanicErr.Error() != expectedNilError {
		t.Errorf("Expected error '%s' for nil panic, got '%s'", expectedNilError, nilPanicErr.Error())
	}
}

// Test TimeoutError
func TestTimeoutError(t *testing.T) {
	duration := time.Second * 5
	task := &mockTask{id: "timeout-task"}

	// Test NewTimeoutError
	timeoutErr := NewTimeoutError(duration, task)
	if timeoutErr == nil {
		t.Fatal("NewTimeoutError should not return nil")
	}

	if timeoutErr.Duration != duration {
		t.Errorf("Expected duration %v, got %v", duration, timeoutErr.Duration)
	}
	if timeoutErr.Task != task {
		t.Errorf("Expected task %v, got %v", task, timeoutErr.Task)
	}

	// Test Error method
	expectedError := "task timeout after 5s"
	if timeoutErr.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, timeoutErr.Error())
	}

	// Test with zero duration
	zeroTimeoutErr := NewTimeoutError(0, task)
	expectedZeroError := "task timeout after 0s"
	if zeroTimeoutErr.Error() != expectedZeroError {
		t.Errorf("Expected error '%s' for zero duration, got '%s'", expectedZeroError, zeroTimeoutErr.Error())
	}
}

// Test error wrapping and unwrapping
func TestErrorWrapping(t *testing.T) {
	originalErr := errors.New("original error")

	// Test PoolError wrapping
	poolErr := NewPoolError("test-op", originalErr)
	if !errors.Is(poolErr, originalErr) {
		t.Error("PoolError should wrap original error")
	}

	// Test ConfigError wrapping
	configErr := NewConfigError("TestField", "value", originalErr)
	if !errors.Is(configErr, originalErr) {
		t.Error("ConfigError should wrap original error")
	}

	// Test TaskError wrapping
	taskErr := NewTaskError("task-1", "execute", originalErr)
	if !errors.Is(taskErr, originalErr) {
		t.Error("TaskError should wrap original error")
	}

	// Test errors.As
	var poolErrPtr *PoolError
	if !errors.As(poolErr, &poolErrPtr) {
		t.Error("Should be able to extract PoolError with errors.As")
	}
	if poolErrPtr.Op != "test-op" {
		t.Error("Extracted PoolError should have correct op")
	}

	var configErrPtr *ConfigError
	if !errors.As(configErr, &configErrPtr) {
		t.Error("Should be able to extract ConfigError with errors.As")
	}
	if configErrPtr.Field != "TestField" {
		t.Error("Extracted ConfigError should have correct field")
	}

	// Test with non-matching types
	if errors.As(poolErr, &configErrPtr) {
		t.Error("Should not be able to extract ConfigError from PoolError")
	}
}

// Test error creation edge cases
func TestErrorCreationEdgeCases(t *testing.T) {
	// Test PoolError with empty values
	emptyOpErr := NewPoolError("", errors.New("message"))
	if emptyOpErr.Error() != "message" {
		t.Error("PoolError should handle empty op")
	}

	nilErr := NewPoolError("op", nil)
	// This might panic, but let's test it gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Logf("PoolError with nil error panicked as expected: %v", r)
		}
	}()
	_ = nilErr.Error()

	// Test ConfigError with different value types
	intConfigErr := NewConfigError("IntField", 42, errors.New("int error"))
	if !contains(intConfigErr.Error(), "42") {
		t.Error("ConfigError should handle int values")
	}

	stringConfigErr := NewConfigError("StringField", "test", errors.New("string error"))
	if !contains(stringConfigErr.Error(), "test") {
		t.Error("ConfigError should handle string values")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsAt(s, substr))))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Mock task for testing
type mockTask struct {
	id string
}

func (t *mockTask) Execute(ctx context.Context) (any, error) {
	return "result", nil
}

func (t *mockTask) Priority() int {
	return PriorityNormal
}

// Benchmark error creation
func BenchmarkPoolErrorCreation(b *testing.B) {
	originalErr := errors.New("benchmark error")
	for i := 0; i < b.N; i++ {
		_ = NewPoolError("bench-op", originalErr)
	}
}

func BenchmarkConfigErrorCreation(b *testing.B) {
	originalErr := errors.New("benchmark error")
	for i := 0; i < b.N; i++ {
		_ = NewConfigError("BenchField", "value", originalErr)
	}
}

func BenchmarkTimeoutErrorCreation(b *testing.B) {
	duration := time.Second * 5
	task := &mockTask{id: "bench-task"}
	for i := 0; i < b.N; i++ {
		_ = NewTimeoutError(duration, task)
	}
}

func BenchmarkPanicErrorCreation(b *testing.B) {
	panicValue := "benchmark panic"
	task := &mockTask{id: "bench-task"}
	for i := 0; i < b.N; i++ {
		_ = NewPanicError(panicValue, task)
	}
}

// Benchmark error methods
func BenchmarkPoolErrorError(b *testing.B) {
	err := NewPoolError("bench-op", errors.New("benchmark error"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkErrorUnwrap(b *testing.B) {
	originalErr := errors.New("benchmark error")
	err := NewPoolError("bench-op", originalErr)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Unwrap()
	}
}
