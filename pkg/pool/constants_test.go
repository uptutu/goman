package pool

import (
	"testing"
	"time"
)

// Test priority constants
func TestPriorityConstants(t *testing.T) {
	// Test that priority constants are in correct order
	priorities := []int{PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical}

	for i := 1; i < len(priorities); i++ {
		if priorities[i] <= priorities[i-1] {
			t.Errorf("Priority constant at index %d (%d) should be greater than previous (%d)",
				i, priorities[i], priorities[i-1])
		}
	}

	// Test specific values
	if PriorityLow != 0 {
		t.Errorf("Expected PriorityLow to be 0, got %d", PriorityLow)
	}
	if PriorityNormal != 1 {
		t.Errorf("Expected PriorityNormal to be 1, got %d", PriorityNormal)
	}
	if PriorityHigh != 2 {
		t.Errorf("Expected PriorityHigh to be 2, got %d", PriorityHigh)
	}
	if PriorityCritical != 3 {
		t.Errorf("Expected PriorityCritical to be 3, got %d", PriorityCritical)
	}
}

// Test default configuration constants
func TestDefaultConfigConstants(t *testing.T) {
	// Test that default values are reasonable
	if DefaultWorkerCount <= 0 {
		t.Errorf("DefaultWorkerCount should be positive, got %d", DefaultWorkerCount)
	}
	if DefaultQueueSize <= 0 {
		t.Errorf("DefaultQueueSize should be positive, got %d", DefaultQueueSize)
	}
	if DefaultObjectPoolSize < 0 {
		t.Errorf("DefaultObjectPoolSize should be non-negative, got %d", DefaultObjectPoolSize)
	}
	if DefaultTaskTimeout <= 0 {
		t.Errorf("DefaultTaskTimeout should be positive, got %v", DefaultTaskTimeout)
	}
	if DefaultShutdownTimeout <= 0 {
		t.Errorf("DefaultShutdownTimeout should be positive, got %v", DefaultShutdownTimeout)
	}
	if DefaultMetricsInterval <= 0 {
		t.Errorf("DefaultMetricsInterval should be positive, got %v", DefaultMetricsInterval)
	}

	// Test specific expected values
	if DefaultWorkerCount != 10 {
		t.Errorf("Expected DefaultWorkerCount to be 10, got %d", DefaultWorkerCount)
	}
	if DefaultQueueSize != 1000 {
		t.Errorf("Expected DefaultQueueSize to be 1000, got %d", DefaultQueueSize)
	}
	if DefaultObjectPoolSize != 100 {
		t.Errorf("Expected DefaultObjectPoolSize to be 100, got %d", DefaultObjectPoolSize)
	}
	if DefaultTaskTimeout != 30*time.Second {
		t.Errorf("Expected DefaultTaskTimeout to be 30s, got %v", DefaultTaskTimeout)
	}
	if DefaultShutdownTimeout != 30*time.Second {
		t.Errorf("Expected DefaultShutdownTimeout to be 30s, got %v", DefaultShutdownTimeout)
	}
	if DefaultMetricsInterval != 10*time.Second {
		t.Errorf("Expected DefaultMetricsInterval to be 10s, got %v", DefaultMetricsInterval)
	}
}

// Test minimum and maximum constants
func TestMinMaxConstants(t *testing.T) {
	// Test minimum values
	if MinWorkerCount <= 0 {
		t.Errorf("MinWorkerCount should be positive, got %d", MinWorkerCount)
	}

	// Test maximum values
	if MaxWorkerCount <= MinWorkerCount {
		t.Errorf("MaxWorkerCount (%d) should be greater than MinWorkerCount (%d)",
			MaxWorkerCount, MinWorkerCount)
	}
	if MaxQueueSize <= 0 {
		t.Errorf("MaxQueueSize should be positive, got %d", MaxQueueSize)
	}

	// Test that defaults are within min/max ranges
	if DefaultWorkerCount < MinWorkerCount || DefaultWorkerCount > MaxWorkerCount {
		t.Errorf("DefaultWorkerCount (%d) should be between %d and %d",
			DefaultWorkerCount, MinWorkerCount, MaxWorkerCount)
	}
	if DefaultQueueSize <= 0 || DefaultQueueSize > MaxQueueSize {
		t.Errorf("DefaultQueueSize (%d) should be between 1 and %d",
			DefaultQueueSize, MaxQueueSize)
	}

	// Test specific expected values
	if MinWorkerCount != 1 {
		t.Errorf("Expected MinWorkerCount to be 1, got %d", MinWorkerCount)
	}
	if MaxWorkerCount != 10000 {
		t.Errorf("Expected MaxWorkerCount to be 10000, got %d", MaxWorkerCount)
	}
	if MaxQueueSize != 1000000 {
		t.Errorf("Expected MaxQueueSize to be 1000000, got %d", MaxQueueSize)
	}
}

// Test local queue constants
func TestLocalQueueConstants(t *testing.T) {
	if LocalQueueSize <= 0 {
		t.Errorf("LocalQueueSize should be positive, got %d", LocalQueueSize)
	}

	// Test specific expected value
	if LocalQueueSize != 256 {
		t.Errorf("Expected LocalQueueSize to be 256, got %d", LocalQueueSize)
	}
}

// Test pool state constants
func TestPoolStateConstants(t *testing.T) {
	// Test that pool state constants are in correct order
	states := []int32{PoolStateRunning, PoolStateShutdown, PoolStateClosed}

	for i := 1; i < len(states); i++ {
		if states[i] <= states[i-1] {
			t.Errorf("Pool state constant at index %d (%d) should be greater than previous (%d)",
				i, states[i], states[i-1])
		}
	}

	// Test specific values
	if PoolStateRunning != 0 {
		t.Errorf("Expected PoolStateRunning to be 0, got %d", PoolStateRunning)
	}
	if PoolStateShutdown != 1 {
		t.Errorf("Expected PoolStateShutdown to be 1, got %d", PoolStateShutdown)
	}
	if PoolStateClosed != 2 {
		t.Errorf("Expected PoolStateClosed to be 2, got %d", PoolStateClosed)
	}
}

// Test circuit breaker constants
func TestCircuitBreakerConstants(t *testing.T) {
	// Test that circuit breaker constants are in correct order
	states := []int32{CircuitBreakerClosed, CircuitBreakerOpen, CircuitBreakerHalfOpen}

	for i := 1; i < len(states); i++ {
		if states[i] <= states[i-1] {
			t.Errorf("Circuit breaker constant at index %d (%d) should be greater than previous (%d)",
				i, states[i], states[i-1])
		}
	}

	// Test specific values
	if CircuitBreakerClosed != 0 {
		t.Errorf("Expected CircuitBreakerClosed to be 0, got %d", CircuitBreakerClosed)
	}
	if CircuitBreakerOpen != 1 {
		t.Errorf("Expected CircuitBreakerOpen to be 1, got %d", CircuitBreakerOpen)
	}
	if CircuitBreakerHalfOpen != 2 {
		t.Errorf("Expected CircuitBreakerHalfOpen to be 2, got %d", CircuitBreakerHalfOpen)
	}
}

// Test monitoring constants
func TestMonitoringConstants(t *testing.T) {
	if MetricsBufferSize <= 0 {
		t.Errorf("MetricsBufferSize should be positive, got %d", MetricsBufferSize)
	}
	if HealthCheckInterval <= 0 {
		t.Errorf("HealthCheckInterval should be positive, got %v", HealthCheckInterval)
	}
	if StatsUpdateInterval <= 0 {
		t.Errorf("StatsUpdateInterval should be positive, got %v", StatsUpdateInterval)
	}

	// Test specific expected values
	if MetricsBufferSize != 1000 {
		t.Errorf("Expected MetricsBufferSize to be 1000, got %d", MetricsBufferSize)
	}
	if HealthCheckInterval != 5*time.Second {
		t.Errorf("Expected HealthCheckInterval to be 5s, got %v", HealthCheckInterval)
	}
	if StatsUpdateInterval != 1*time.Second {
		t.Errorf("Expected StatsUpdateInterval to be 1s, got %v", StatsUpdateInterval)
	}
}

// Test queue type constants
func TestQueueTypeConstants(t *testing.T) {
	if QueueTypeLockFree != "lockfree" {
		t.Errorf("Expected QueueTypeLockFree to be 'lockfree', got '%s'", QueueTypeLockFree)
	}
	if QueueTypeChannel != "channel" {
		t.Errorf("Expected QueueTypeChannel to be 'channel', got '%s'", QueueTypeChannel)
	}
	if DefaultQueueType != QueueTypeLockFree {
		t.Errorf("Expected DefaultQueueType to be '%s', got '%s'", QueueTypeLockFree, DefaultQueueType)
	}
}

// Test that constants are consistent with each other
func TestConstantConsistency(t *testing.T) {
	// Test that default queue size is at least as large as default worker count
	if DefaultQueueSize < DefaultWorkerCount {
		t.Errorf("DefaultQueueSize (%d) should be at least as large as DefaultWorkerCount (%d)",
			DefaultQueueSize, DefaultWorkerCount)
	}

	// Test that object pool size is reasonable relative to worker count
	if DefaultObjectPoolSize < DefaultWorkerCount {
		t.Logf("DefaultObjectPoolSize (%d) is smaller than DefaultWorkerCount (%d), this might be intentional",
			DefaultObjectPoolSize, DefaultWorkerCount)
	}

	// Test that local queue size is reasonable
	if LocalQueueSize < 10 {
		t.Errorf("LocalQueueSize (%d) seems too small for efficient operation", LocalQueueSize)
	}
	if LocalQueueSize > 10000 {
		t.Errorf("LocalQueueSize (%d) seems too large and might waste memory", LocalQueueSize)
	}
}

// Test priority range validation
func TestPriorityRange(t *testing.T) {
	// Test that all priorities are within a reasonable range
	priorities := []int{PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical}

	for _, priority := range priorities {
		if priority < 0 {
			t.Errorf("Priority %d should not be negative", priority)
		}
		if priority > 100 {
			t.Errorf("Priority %d seems too high", priority)
		}
	}

	// Test that priorities are distinct
	prioritySet := make(map[int]bool)
	for _, priority := range priorities {
		if prioritySet[priority] {
			t.Errorf("Priority %d is duplicated", priority)
		}
		prioritySet[priority] = true
	}
}

// Test configuration validation ranges
func TestConfigValidationRanges(t *testing.T) {
	// Test that min values are reasonable
	if MinWorkerCount < 1 {
		t.Errorf("MinWorkerCount (%d) should be at least 1", MinWorkerCount)
	}

	// Test that max values are not too restrictive
	if MaxWorkerCount < 100 {
		t.Errorf("MaxWorkerCount (%d) seems too restrictive", MaxWorkerCount)
	}
	if MaxQueueSize < 1000 {
		t.Errorf("MaxQueueSize (%d) seems too restrictive", MaxQueueSize)
	}

	// Test that max values are not unreasonably high
	if MaxWorkerCount > 100000 {
		t.Errorf("MaxWorkerCount (%d) seems unreasonably high", MaxWorkerCount)
	}
	if MaxQueueSize > 100000000 {
		t.Errorf("MaxQueueSize (%d) seems unreasonably high", MaxQueueSize)
	}
}

// Test timeout value reasonableness
func TestTimeoutReasonableness(t *testing.T) {
	// Test that default timeouts are reasonable for most use cases
	if DefaultTaskTimeout < time.Second {
		t.Errorf("DefaultTaskTimeout (%v) seems too short", DefaultTaskTimeout)
	}
	if DefaultTaskTimeout > time.Hour {
		t.Errorf("DefaultTaskTimeout (%v) seems too long", DefaultTaskTimeout)
	}

	if DefaultShutdownTimeout < time.Second {
		t.Errorf("DefaultShutdownTimeout (%v) seems too short", DefaultShutdownTimeout)
	}
	if DefaultShutdownTimeout > time.Minute*5 {
		t.Errorf("DefaultShutdownTimeout (%v) seems too long for default", DefaultShutdownTimeout)
	}

	if DefaultMetricsInterval < time.Second {
		t.Errorf("DefaultMetricsInterval (%v) seems too frequent", DefaultMetricsInterval)
	}
	if DefaultMetricsInterval > time.Minute {
		t.Errorf("DefaultMetricsInterval (%v) seems too infrequent", DefaultMetricsInterval)
	}
}

// Benchmark constant access (should be very fast)
func BenchmarkConstantAccess(b *testing.B) {
	var sum int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sum += DefaultWorkerCount + DefaultQueueSize + PriorityNormal
	}
	_ = sum // Prevent optimization
}
