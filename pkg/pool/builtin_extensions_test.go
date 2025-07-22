package pool

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Test LoggingMiddleware
func TestLoggingMiddleware(t *testing.T) {
	logger := &DefaultExtendedLogger{}
	middleware := NewLoggingMiddleware(logger)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "test result", nil
	}, PriorityNormal)

	// Test Before
	ctx := middleware.Before(context.Background(), task)
	if ctx == nil {
		t.Error("Before should return a context")
	}

	// Test After with success
	middleware.After(ctx, task, "test result", nil)

	// Test After with error
	testErr := errors.New("test error")
	middleware.After(ctx, task, nil, testErr)
}

// Test MetricsMiddleware
func TestMetricsMiddleware(t *testing.T) {
	middleware := NewMetricsMiddleware()
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		time.Sleep(time.Millisecond * 10)
		return "test result", nil
	}, PriorityNormal)

	// Test Before
	ctx := middleware.Before(context.Background(), task)
	if ctx == nil {
		t.Error("Before should return a context")
	}

	// Simulate some execution time
	time.Sleep(time.Millisecond * 5)

	// Test After
	middleware.After(ctx, task, "test result", nil)

	// Get metrics
	metrics := middleware.GetMetrics()
	if metrics.TaskCount != 1 {
		t.Errorf("Expected 1 total task, got %d", metrics.TaskCount)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("Expected 1 successful task, got %d", metrics.SuccessCount)
	}
	if metrics.FailureCount != 0 {
		t.Errorf("Expected 0 failed tasks, got %d", metrics.FailureCount)
	}
	if metrics.AvgDuration <= 0 {
		t.Error("Average duration should be positive")
	}
}

// Test MetricsMiddleware with errors
func TestMetricsMiddleware_WithErrors(t *testing.T) {
	middleware := NewMetricsMiddleware()
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return nil, errors.New("test error")
	}, PriorityNormal)

	ctx := middleware.Before(context.Background(), task)
	middleware.After(ctx, task, nil, errors.New("test error"))

	metrics := middleware.GetMetrics()
	if metrics.TaskCount != 1 {
		t.Errorf("Expected 1 total task, got %d", metrics.TaskCount)
	}
	if metrics.SuccessCount != 0 {
		t.Errorf("Expected 0 successful tasks, got %d", metrics.SuccessCount)
	}
	if metrics.FailureCount != 1 {
		t.Errorf("Expected 1 failed task, got %d", metrics.FailureCount)
	}
}

// Test TimeoutMiddleware
func TestTimeoutMiddleware(t *testing.T) {
	middleware := NewTimeoutMiddleware(time.Millisecond * 100)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond * 50):
			return "completed in time", nil
		}
	}, PriorityNormal)

	// Test successful execution within timeout
	ctx := middleware.Before(context.Background(), task)
	if ctx == nil {
		t.Error("Before should return a context")
	}

	// Check if context has timeout
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("Context should have a deadline")
	}
	if time.Until(deadline) > time.Millisecond*100 {
		t.Error("Deadline should be within timeout duration")
	}

	middleware.After(ctx, task, "completed in time", nil)
}

// Test TimeoutMiddleware with timeout
func TestTimeoutMiddleware_WithTimeout(t *testing.T) {
	middleware := NewTimeoutMiddleware(time.Millisecond * 50)

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond * 100):
			return "should not complete", nil
		}
	}, PriorityNormal)

	ctx := middleware.Before(context.Background(), task)

	// Simulate timeout
	time.Sleep(time.Millisecond * 60)
	middleware.After(ctx, task, nil, context.DeadlineExceeded)
}

// Test MonitoringPlugin
func TestMonitoringPlugin(t *testing.T) {
	logger := &DefaultExtendedLogger{}
	plugin := NewMonitoringPlugin("test-monitor", time.Millisecond*100, logger)

	if plugin.Name() != "test-monitor" {
		t.Errorf("Expected plugin name 'test-monitor', got '%s'", plugin.Name())
	}

	// Test initialization
	err := plugin.Init(nil) // Pass nil pool for testing
	if err != nil {
		t.Errorf("Expected no error on init, got %v", err)
	}

	// Test start
	err = plugin.Start()
	if err != nil {
		t.Errorf("Expected no error on start, got %v", err)
	}

	// Let it run briefly
	time.Sleep(time.Millisecond * 50)

	// Test stop
	err = plugin.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop, got %v", err)
	}
}

// Test LoggingEventListener
func TestLoggingEventListener(t *testing.T) {
	logger := &DefaultExtendedLogger{}
	listener := NewLoggingEventListener(logger)

	// Test pool events
	listener.OnPoolStart(nil)
	listener.OnPoolShutdown(nil)

	// Test task events
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "test", nil
	}, PriorityNormal)

	listener.OnTaskSubmit(task)
	listener.OnTaskComplete(task, "result")

	// Test worker panic event
	listener.OnWorkerPanic(1, "test panic")
}

// Test RoundRobinSchedulerPlugin
func TestRoundRobinSchedulerPlugin(t *testing.T) {
	plugin := NewRoundRobinSchedulerPlugin(1)

	if plugin.Name() != "round-robin-scheduler" {
		t.Errorf("Expected plugin name 'round-robin-scheduler', got '%s'", plugin.Name())
	}

	if plugin.Priority() != 1 {
		t.Errorf("Expected priority 1, got %d", plugin.Priority())
	}

	// Test scheduler creation
	config := NewConfig()
	scheduler, err := plugin.CreateScheduler(config)
	if err != nil {
		t.Errorf("Expected no error creating scheduler, got %v", err)
	}

	if scheduler == nil {
		t.Error("Expected scheduler to be created")
	}

	// Test scheduler operations
	err = scheduler.Start()
	if err != nil {
		t.Errorf("Expected no error starting scheduler, got %v", err)
	}

	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "test", nil
	}, PriorityNormal)

	err = scheduler.Schedule(task)
	if err != nil {
		t.Errorf("Expected no error scheduling task, got %v", err)
	}

	err = scheduler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping scheduler, got %v", err)
	}
}

// Test DefaultExtendedLogger
func TestDefaultExtendedLogger(t *testing.T) {
	logger := &DefaultExtendedLogger{}

	// Test all log methods (they should not panic)
	logger.Printf("test printf %s", "arg")
	logger.Println("test println")
	logger.Debug("test debug")
	logger.Debugf("test debugf %s", "arg")
	logger.Info("test info")
	logger.Infof("test infof %s", "arg")
	logger.Warn("test warn")
	logger.Warnf("test warnf %s", "arg")
	logger.Error("test error")
	logger.Errorf("test errorf %s", "arg")
}

// Test MetricsData
func TestMetricsData(t *testing.T) {
	data := MetricsData{
		TaskCount:    10,
		SuccessCount: 8,
		FailureCount: 2,
		AvgDuration:  time.Millisecond * 100,
		SuccessRate:  0.8,
	}

	if data.TaskCount != 10 {
		t.Errorf("Expected TaskCount 10, got %d", data.TaskCount)
	}
	if data.SuccessCount != 8 {
		t.Errorf("Expected SuccessCount 8, got %d", data.SuccessCount)
	}
	if data.FailureCount != 2 {
		t.Errorf("Expected FailureCount 2, got %d", data.FailureCount)
	}
	if data.AvgDuration != time.Millisecond*100 {
		t.Errorf("Expected AvgDuration 100ms, got %v", data.AvgDuration)
	}
	if data.SuccessRate != 0.8 {
		t.Errorf("Expected SuccessRate 0.8, got %f", data.SuccessRate)
	}
}

// Benchmark middleware overhead
func BenchmarkLoggingMiddleware(b *testing.B) {
	logger := &DefaultExtendedLogger{}
	middleware := NewLoggingMiddleware(logger)
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark", nil
	}, PriorityNormal)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := middleware.Before(context.Background(), task)
		middleware.After(ctx, task, "benchmark", nil)
	}
}

func BenchmarkMetricsMiddleware(b *testing.B) {
	middleware := NewMetricsMiddleware()
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark", nil
	}, PriorityNormal)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := middleware.Before(context.Background(), task)
		middleware.After(ctx, task, "benchmark", nil)
	}
}

func BenchmarkTimeoutMiddleware(b *testing.B) {
	middleware := NewTimeoutMiddleware(time.Second)
	task := NewSimpleTask(func(ctx context.Context) (any, error) {
		return "benchmark", nil
	}, PriorityNormal)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := middleware.Before(context.Background(), task)
		middleware.After(ctx, task, "benchmark", nil)
	}
}
