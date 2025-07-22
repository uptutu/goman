package pool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config.WorkerCount != DefaultWorkerCount {
		t.Errorf("Expected WorkerCount %d, got %d", DefaultWorkerCount, config.WorkerCount)
	}

	if config.QueueSize != DefaultQueueSize {
		t.Errorf("Expected QueueSize %d, got %d", DefaultQueueSize, config.QueueSize)
	}

	if config.TaskTimeout != DefaultTaskTimeout {
		t.Errorf("Expected TaskTimeout %v, got %v", DefaultTaskTimeout, config.TaskTimeout)
	}
}

func TestConfigBuilder(t *testing.T) {
	config, err := NewConfigBuilder().
		WithWorkerCount(20).
		WithQueueSize(2000).
		WithTaskTimeout(60 * time.Second).
		WithMetrics(false).
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.WorkerCount != 20 {
		t.Errorf("Expected WorkerCount 20, got %d", config.WorkerCount)
	}

	if config.QueueSize != 2000 {
		t.Errorf("Expected QueueSize 2000, got %d", config.QueueSize)
	}

	if config.TaskTimeout != 60*time.Second {
		t.Errorf("Expected TaskTimeout 60s, got %v", config.TaskTimeout)
	}

	if config.EnableMetrics != false {
		t.Errorf("Expected EnableMetrics false, got %v", config.EnableMetrics)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorField  string
	}{
		{
			name:        "valid config",
			config:      NewConfig(),
			expectError: false,
		},
		{
			name: "invalid worker count - zero",
			config: &Config{
				WorkerCount:     0,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "WorkerCount",
		},
		{
			name: "invalid worker count - negative",
			config: &Config{
				WorkerCount:     -1,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "WorkerCount",
		},
		{
			name: "invalid worker count - exceeds maximum",
			config: &Config{
				WorkerCount:     MaxWorkerCount + 1,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "WorkerCount",
		},
		{
			name: "invalid queue size - zero",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       0,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "QueueSize",
		},
		{
			name: "invalid queue size - exceeds maximum",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       MaxQueueSize + 1,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "QueueSize",
		},
		{
			name: "invalid queue size - smaller than worker count",
			config: &Config{
				WorkerCount:     10,
				QueueSize:       5,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "QueueSize",
		},
		{
			name: "invalid object pool size - negative",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				ObjectPoolSize:  -1,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "ObjectPoolSize",
		},
		{
			name: "invalid task timeout - zero",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     0,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "TaskTimeout",
		},
		{
			name: "invalid task timeout - too large",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     25 * time.Hour,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "TaskTimeout",
		},
		{
			name: "invalid shutdown timeout - zero",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: 0,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "ShutdownTimeout",
		},
		{
			name: "invalid shutdown timeout - too large",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: 2 * time.Hour,
				MetricsInterval: DefaultMetricsInterval,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "ShutdownTimeout",
		},
		{
			name: "invalid metrics interval - zero when metrics enabled",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: 0,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "MetricsInterval",
		},
		{
			name: "valid metrics interval - zero when metrics disabled",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: 0,
				EnableMetrics:   false,
			},
			expectError: false,
		},
		{
			name: "invalid metrics interval - too large",
			config: &Config{
				WorkerCount:     DefaultWorkerCount,
				QueueSize:       DefaultQueueSize,
				TaskTimeout:     DefaultTaskTimeout,
				ShutdownTimeout: DefaultShutdownTimeout,
				MetricsInterval: 2 * time.Hour,
				EnableMetrics:   true,
			},
			expectError: true,
			errorField:  "MetricsInterval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if tt.expectError && err != nil && tt.errorField != "" {
				if configErr, ok := err.(*ConfigError); ok {
					if configErr.Field != tt.errorField {
						t.Errorf("Expected error field %s, got %s", tt.errorField, configErr.Field)
					}
				} else {
					t.Errorf("Expected ConfigError, got %T", err)
				}
			}
		})
	}
}
func TestConfigBuilderChaining(t *testing.T) {
	// Test all builder methods in a chain
	config, err := NewConfigBuilder().
		WithWorkerCount(5).
		WithQueueSize(100).
		WithObjectPoolSize(50).
		WithPreAlloc(true).
		WithTaskTimeout(10 * time.Second).
		WithShutdownTimeout(5 * time.Second).
		WithMetrics(true).
		WithMetricsInterval(2 * time.Second).
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify all values were set correctly
	if config.WorkerCount != 5 {
		t.Errorf("Expected WorkerCount 5, got %d", config.WorkerCount)
	}
	if config.QueueSize != 100 {
		t.Errorf("Expected QueueSize 100, got %d", config.QueueSize)
	}
	if config.ObjectPoolSize != 50 {
		t.Errorf("Expected ObjectPoolSize 50, got %d", config.ObjectPoolSize)
	}
	if !config.PreAlloc {
		t.Errorf("Expected PreAlloc true, got %v", config.PreAlloc)
	}
	if config.TaskTimeout != 10*time.Second {
		t.Errorf("Expected TaskTimeout 10s, got %v", config.TaskTimeout)
	}
	if config.ShutdownTimeout != 5*time.Second {
		t.Errorf("Expected ShutdownTimeout 5s, got %v", config.ShutdownTimeout)
	}
	if !config.EnableMetrics {
		t.Errorf("Expected EnableMetrics true, got %v", config.EnableMetrics)
	}
	if config.MetricsInterval != 2*time.Second {
		t.Errorf("Expected MetricsInterval 2s, got %v", config.MetricsInterval)
	}
}

func TestConfigBuilderWithPanicHandler(t *testing.T) {
	called := false
	panicHandler := func(v interface{}) {
		called = true
	}

	config, err := NewConfigBuilder().
		WithPanicHandler(panicHandler).
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.PanicHandler == nil {
		t.Error("Expected PanicHandler to be set")
	}

	// Test the panic handler
	config.PanicHandler("test panic")
	if !called {
		t.Error("Expected panic handler to be called")
	}
}

func TestConfigBuilderWithTaskInterceptor(t *testing.T) {
	interceptor := &mockTaskInterceptor{}

	config, err := NewConfigBuilder().
		WithTaskInterceptor(interceptor).
		Build()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.TaskInterceptor != interceptor {
		t.Error("Expected TaskInterceptor to be set")
	}
}

func TestConfigBuilderValidationFailure(t *testing.T) {
	// Test that Build() returns error for invalid config
	_, err := NewConfigBuilder().
		WithWorkerCount(-1).
		Build()

	if err == nil {
		t.Error("Expected error for invalid worker count")
	}

	if configErr, ok := err.(*ConfigError); ok {
		if configErr.Field != "WorkerCount" {
			t.Errorf("Expected error field WorkerCount, got %s", configErr.Field)
		}
	} else {
		t.Errorf("Expected ConfigError, got %T", err)
	}
}

func TestConfigBuilderBuildUnsafe(t *testing.T) {
	// Test BuildUnsafe doesn't validate
	config := NewConfigBuilder().
		WithWorkerCount(-1).
		BuildUnsafe()

	if config.WorkerCount != -1 {
		t.Errorf("Expected WorkerCount -1, got %d", config.WorkerCount)
	}

	// Verify it's actually invalid
	err := config.Validate()
	if err == nil {
		t.Error("Expected validation error")
	}
}

func TestConfigBuilderReset(t *testing.T) {
	builder := NewConfigBuilder().
		WithWorkerCount(100).
		WithQueueSize(5000)

	// Reset should restore defaults
	config, err := builder.Reset().Build()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.WorkerCount != DefaultWorkerCount {
		t.Errorf("Expected WorkerCount %d after reset, got %d", DefaultWorkerCount, config.WorkerCount)
	}
	if config.QueueSize != DefaultQueueSize {
		t.Errorf("Expected QueueSize %d after reset, got %d", DefaultQueueSize, config.QueueSize)
	}
}

func TestConfigBuilderClone(t *testing.T) {
	original := NewConfigBuilder().
		WithWorkerCount(20).
		WithQueueSize(2000)

	cloned := original.Clone()

	// Modify original
	originalConfig, err := original.WithWorkerCount(30).Build()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Clone should be unchanged
	clonedConfig, err := cloned.Build()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if originalConfig.WorkerCount == clonedConfig.WorkerCount {
		t.Error("Expected cloned config to be independent")
	}

	if clonedConfig.WorkerCount != 20 {
		t.Errorf("Expected cloned WorkerCount 20, got %d", clonedConfig.WorkerCount)
	}
	if originalConfig.WorkerCount != 30 {
		t.Errorf("Expected original WorkerCount 30, got %d", originalConfig.WorkerCount)
	}
}

func TestConfigDefaults(t *testing.T) {
	config := NewConfig()

	// Test all default values
	if config.WorkerCount != DefaultWorkerCount {
		t.Errorf("Expected default WorkerCount %d, got %d", DefaultWorkerCount, config.WorkerCount)
	}
	if config.QueueSize != DefaultQueueSize {
		t.Errorf("Expected default QueueSize %d, got %d", DefaultQueueSize, config.QueueSize)
	}
	if config.ObjectPoolSize != DefaultObjectPoolSize {
		t.Errorf("Expected default ObjectPoolSize %d, got %d", DefaultObjectPoolSize, config.ObjectPoolSize)
	}
	if config.PreAlloc != false {
		t.Errorf("Expected default PreAlloc false, got %v", config.PreAlloc)
	}
	if config.TaskTimeout != DefaultTaskTimeout {
		t.Errorf("Expected default TaskTimeout %v, got %v", DefaultTaskTimeout, config.TaskTimeout)
	}
	if config.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Expected default ShutdownTimeout %v, got %v", DefaultShutdownTimeout, config.ShutdownTimeout)
	}
	if config.EnableMetrics != true {
		t.Errorf("Expected default EnableMetrics true, got %v", config.EnableMetrics)
	}
	if config.MetricsInterval != DefaultMetricsInterval {
		t.Errorf("Expected default MetricsInterval %v, got %v", DefaultMetricsInterval, config.MetricsInterval)
	}
	if config.PanicHandler != nil {
		t.Error("Expected default PanicHandler to be nil")
	}
	if config.TaskInterceptor != nil {
		t.Error("Expected default TaskInterceptor to be nil")
	}
}

func TestConfigValidationEdgeCases(t *testing.T) {
	// Test minimum valid values
	config := &Config{
		WorkerCount:     MinWorkerCount,
		QueueSize:       MinWorkerCount, // Queue size should be at least equal to worker count
		ObjectPoolSize:  0,              // Zero is valid for object pool
		TaskTimeout:     1 * time.Nanosecond,
		ShutdownTimeout: 1 * time.Nanosecond,
		MetricsInterval: 1 * time.Nanosecond,
		EnableMetrics:   true,
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Expected no error for minimum valid config, got %v", err)
	}

	// Test maximum valid values
	config = &Config{
		WorkerCount:     MaxWorkerCount,
		QueueSize:       MaxQueueSize,
		ObjectPoolSize:  1000000,
		TaskTimeout:     24 * time.Hour,
		ShutdownTimeout: time.Hour,
		MetricsInterval: time.Hour,
		EnableMetrics:   true,
	}

	err = config.Validate()
	if err != nil {
		t.Errorf("Expected no error for maximum valid config, got %v", err)
	}
}

// Mock implementation for testing
type mockTaskInterceptor struct{}

func (m *mockTaskInterceptor) Before(ctx context.Context, task Task) context.Context {
	return ctx
}

func (m *mockTaskInterceptor) After(ctx context.Context, task Task, result interface{}, err error) {
	// Mock implementation
}
func TestConfigCopy(t *testing.T) {
	original := NewConfig()
	original.WorkerCount = 20
	original.QueueSize = 2000

	copy := original.Copy()

	// Modify original
	original.WorkerCount = 30

	// Copy should be unchanged
	if copy.WorkerCount != 20 {
		t.Errorf("Expected copied WorkerCount 20, got %d", copy.WorkerCount)
	}
	if copy.QueueSize != 2000 {
		t.Errorf("Expected copied QueueSize 2000, got %d", copy.QueueSize)
	}
}

func TestConfigString(t *testing.T) {
	config := NewConfig()
	str := config.String()

	// Should contain key configuration values
	if !strings.Contains(str, "WorkerCount:10") {
		t.Errorf("Expected string to contain WorkerCount:10, got %s", str)
	}
	if !strings.Contains(str, "QueueSize:1000") {
		t.Errorf("Expected string to contain QueueSize:1000, got %s", str)
	}
}

func TestConfigIsValid(t *testing.T) {
	validConfig := NewConfig()
	if !validConfig.IsValid() {
		t.Error("Expected valid config to return true for IsValid()")
	}

	invalidConfig := &Config{WorkerCount: -1}
	if invalidConfig.IsValid() {
		t.Error("Expected invalid config to return false for IsValid()")
	}
}

func TestConfigWithDefaults(t *testing.T) {
	// Test config with some invalid values
	config := &Config{
		WorkerCount:     -1, // Invalid
		QueueSize:       0,  // Invalid
		ObjectPoolSize:  -5, // Invalid
		TaskTimeout:     0,  // Invalid
		ShutdownTimeout: -1, // Invalid
		MetricsInterval: 0,  // Invalid
		EnableMetrics:   true,
	}

	config.WithDefaults()

	// Should now have default values for invalid fields
	if config.WorkerCount != DefaultWorkerCount {
		t.Errorf("Expected WorkerCount %d, got %d", DefaultWorkerCount, config.WorkerCount)
	}
	if config.QueueSize != DefaultQueueSize {
		t.Errorf("Expected QueueSize %d, got %d", DefaultQueueSize, config.QueueSize)
	}
	if config.ObjectPoolSize != DefaultObjectPoolSize {
		t.Errorf("Expected ObjectPoolSize %d, got %d", DefaultObjectPoolSize, config.ObjectPoolSize)
	}
	if config.TaskTimeout != DefaultTaskTimeout {
		t.Errorf("Expected TaskTimeout %v, got %v", DefaultTaskTimeout, config.TaskTimeout)
	}
	if config.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Expected ShutdownTimeout %v, got %v", DefaultShutdownTimeout, config.ShutdownTimeout)
	}
	if config.MetricsInterval != DefaultMetricsInterval {
		t.Errorf("Expected MetricsInterval %v, got %v", DefaultMetricsInterval, config.MetricsInterval)
	}

	// Test that valid values are preserved
	config2 := &Config{
		WorkerCount:     5,
		QueueSize:       100,
		ObjectPoolSize:  50,
		TaskTimeout:     10 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		MetricsInterval: 2 * time.Second,
		EnableMetrics:   true,
	}

	config2.WithDefaults()

	// Should preserve valid values
	if config2.WorkerCount != 5 {
		t.Errorf("Expected WorkerCount 5, got %d", config2.WorkerCount)
	}
	if config2.QueueSize != 100 {
		t.Errorf("Expected QueueSize 100, got %d", config2.QueueSize)
	}
}
