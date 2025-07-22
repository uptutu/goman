package pool

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Test CircuitBreakerState String method
func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{StateClosed, "CLOSED"},
		{StateHalfOpen, "HALF_OPEN"},
		{StateOpen, "OPEN"},
		{CircuitBreakerState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("CircuitBreakerState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test PoolStatsStruct struct
func TestPoolStatsStruct(t *testing.T) {
	stats := PoolStats{
		ActiveWorkers:   5,
		QueuedTasks:     10,
		CompletedTasks:  100,
		FailedTasks:     2,
		AvgTaskDuration: time.Millisecond * 50,
		ThroughputTPS:   20.5,
		MemoryUsage:     1024 * 1024,
		GCCount:         5,
	}

	if stats.ActiveWorkers != 5 {
		t.Errorf("Expected ActiveWorkers 5, got %d", stats.ActiveWorkers)
	}
	if stats.QueuedTasks != 10 {
		t.Errorf("Expected QueuedTasks 10, got %d", stats.QueuedTasks)
	}
	if stats.CompletedTasks != 100 {
		t.Errorf("Expected CompletedTasks 100, got %d", stats.CompletedTasks)
	}
	if stats.FailedTasks != 2 {
		t.Errorf("Expected FailedTasks 2, got %d", stats.FailedTasks)
	}
	if stats.AvgTaskDuration != time.Millisecond*50 {
		t.Errorf("Expected AvgTaskDuration 50ms, got %v", stats.AvgTaskDuration)
	}
	if stats.ThroughputTPS != 20.5 {
		t.Errorf("Expected ThroughputTPS 20.5, got %f", stats.ThroughputTPS)
	}
	if stats.MemoryUsage != 1024*1024 {
		t.Errorf("Expected MemoryUsage 1MB, got %d", stats.MemoryUsage)
	}
	if stats.GCCount != 5 {
		t.Errorf("Expected GCCount 5, got %d", stats.GCCount)
	}
}

// Test ErrorStats struct
func TestErrorStats(t *testing.T) {
	stats := ErrorStats{
		PanicCount:          3,
		TimeoutCount:        5,
		QueueFullCount:      2,
		CircuitBreakerState: StateOpen,
	}

	if stats.PanicCount != 3 {
		t.Errorf("Expected PanicCount 3, got %d", stats.PanicCount)
	}
	if stats.TimeoutCount != 5 {
		t.Errorf("Expected TimeoutCount 5, got %d", stats.TimeoutCount)
	}
	if stats.QueueFullCount != 2 {
		t.Errorf("Expected QueueFullCount 2, got %d", stats.QueueFullCount)
	}
	if stats.CircuitBreakerState != StateOpen {
		t.Errorf("Expected CircuitBreakerState OPEN, got %v", stats.CircuitBreakerState)
	}
}

// Test DetailedStats struct
func TestDetailedStats(t *testing.T) {
	stats := DetailedStats{
		PoolStats: PoolStats{
			ActiveWorkers:  5,
			CompletedTasks: 100,
		},
		TaskDurationStats: DurationStats{
			Min:    time.Millisecond * 10,
			Max:    time.Millisecond * 100,
			Median: time.Millisecond * 50,
			P95:    time.Millisecond * 90,
			P99:    time.Millisecond * 95,
		},
		ThroughputTrend: []ThroughputPoint{
			{Time: time.Now(), Throughput: 10.0},
			{Time: time.Now().Add(time.Second), Throughput: 15.0},
		},
	}

	if stats.ActiveWorkers != 5 {
		t.Errorf("Expected ActiveWorkers 5, got %d", stats.ActiveWorkers)
	}
	if stats.TaskDurationStats.Min != time.Millisecond*10 {
		t.Errorf("Expected Min duration 10ms, got %v", stats.TaskDurationStats.Min)
	}
	if len(stats.ThroughputTrend) != 2 {
		t.Errorf("Expected 2 throughput points, got %d", len(stats.ThroughputTrend))
	}
}

// Test DurationStats struct
func TestDurationStats(t *testing.T) {
	stats := DurationStats{
		Min:    time.Millisecond * 5,
		Max:    time.Millisecond * 200,
		Median: time.Millisecond * 50,
		P95:    time.Millisecond * 150,
		P99:    time.Millisecond * 180,
	}

	if stats.Min != time.Millisecond*5 {
		t.Errorf("Expected Min 5ms, got %v", stats.Min)
	}
	if stats.Max != time.Millisecond*200 {
		t.Errorf("Expected Max 200ms, got %v", stats.Max)
	}
	if stats.Median != time.Millisecond*50 {
		t.Errorf("Expected Median 50ms, got %v", stats.Median)
	}
	if stats.P95 != time.Millisecond*150 {
		t.Errorf("Expected P95 150ms, got %v", stats.P95)
	}
	if stats.P99 != time.Millisecond*180 {
		t.Errorf("Expected P99 180ms, got %v", stats.P99)
	}
}

// Test ThroughputPoint struct
func TestThroughputPoint(t *testing.T) {
	now := time.Now()
	point := ThroughputPoint{
		Time:       now,
		Throughput: 25.5,
	}

	if !point.Time.Equal(now) {
		t.Errorf("Expected Time %v, got %v", now, point.Time)
	}
	if point.Throughput != 25.5 {
		t.Errorf("Expected Throughput 25.5, got %f", point.Throughput)
	}
}

// Mock implementations for interface testing

// MockTask for testing Task interface
type MockTask struct {
	id       string
	priority int
	result   any
	err      error
	executed bool
}

func NewMockTask(id string, priority int) *MockTask {
	return &MockTask{
		id:       id,
		priority: priority,
		result:   "result-" + id,
	}
}

func (t *MockTask) Execute(ctx context.Context) (any, error) {
	t.executed = true
	return t.result, t.err
}

func (t *MockTask) Priority() int {
	return t.priority
}

func (t *MockTask) SetResult(result any) {
	t.result = result
}

func (t *MockTask) SetError(err error) {
	t.err = err
}

func (t *MockTask) IsExecuted() bool {
	return t.executed
}

// Test Task interface
func TestTaskInterface(t *testing.T) {
	task := NewMockTask("test", PriorityHigh)

	// Test Priority
	if task.Priority() != PriorityHigh {
		t.Errorf("Expected priority %d, got %d", PriorityHigh, task.Priority())
	}

	// Test Execute
	result, err := task.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "result-test" {
		t.Errorf("Expected result 'result-test', got %v", result)
	}
	if !task.IsExecuted() {
		t.Error("Task should be marked as executed")
	}

	// Test Execute with error
	task2 := NewMockTask("error", PriorityNormal)
	task2.SetError(ErrTaskCancelled)
	result2, err2 := task2.Execute(context.Background())
	if err2 != ErrTaskCancelled {
		t.Errorf("Expected ErrTaskCancelled, got %v", err2)
	}
	if result2 != "result-error" {
		t.Errorf("Expected result 'result-error', got %v", result2)
	}
}

// MockFuture for testing Future interface
type MockFuture struct {
	result    any
	err       error
	done      bool
	cancelled bool
}

func NewMockFuture() *MockFuture {
	return &MockFuture{}
}

func (f *MockFuture) Get() (any, error) {
	if f.cancelled {
		return nil, ErrTaskCancelled
	}
	return f.result, f.err
}

func (f *MockFuture) GetWithTimeout(timeout time.Duration) (any, error) {
	if f.cancelled {
		return nil, ErrTaskCancelled
	}
	if !f.done {
		return nil, ErrTaskTimeout
	}
	return f.result, f.err
}

func (f *MockFuture) IsDone() bool {
	return f.done || f.cancelled
}

func (f *MockFuture) Cancel() bool {
	if f.done || f.cancelled {
		return false
	}
	f.cancelled = true
	return true
}

func (f *MockFuture) SetResult(result any, err error) {
	if f.cancelled {
		return
	}
	f.result = result
	f.err = err
	f.done = true
}

// Test Future interface
func TestFutureInterface(t *testing.T) {
	future := NewMockFuture()

	// Test initial state
	if future.IsDone() {
		t.Error("New future should not be done")
	}

	// Test Cancel
	if !future.Cancel() {
		t.Error("Should be able to cancel future")
	}
	if !future.IsDone() {
		t.Error("Cancelled future should be done")
	}

	// Test Get after cancel
	result, err := future.Get()
	if result != nil {
		t.Error("Cancelled future should return nil result")
	}
	if err != ErrTaskCancelled {
		t.Errorf("Expected ErrTaskCancelled, got %v", err)
	}

	// Test second cancel
	if future.Cancel() {
		t.Error("Should not be able to cancel already cancelled future")
	}

	// Test new future with result
	future2 := NewMockFuture()
	future2.SetResult("test-result", nil)

	if !future2.IsDone() {
		t.Error("Future with result should be done")
	}

	result2, err2 := future2.Get()
	if err2 != nil {
		t.Errorf("Expected no error, got %v", err2)
	}
	if result2 != "test-result" {
		t.Errorf("Expected 'test-result', got %v", result2)
	}

	// Test GetWithTimeout on done future
	result3, err3 := future2.GetWithTimeout(time.Millisecond * 10)
	if err3 != nil {
		t.Errorf("Expected no error, got %v", err3)
	}
	if result3 != "test-result" {
		t.Errorf("Expected 'test-result', got %v", result3)
	}

	// Test GetWithTimeout on not done future
	future3 := NewMockFuture()
	result4, err4 := future3.GetWithTimeout(time.Millisecond * 10)
	if result4 != nil {
		t.Error("Timeout should return nil result")
	}
	if err4 != ErrTaskTimeout {
		t.Errorf("Expected ErrTaskTimeout, got %v", err4)
	}
}

// MockTaskInterceptor for testing TaskInterceptor interface
type MockTaskInterceptor struct {
	beforeCalled bool
	afterCalled  bool
	beforeCtx    context.Context
	afterTask    Task
	afterResult  any
	afterErr     error
}

func (i *MockTaskInterceptor) Before(ctx context.Context, task Task) context.Context {
	i.beforeCalled = true
	i.beforeCtx = ctx
	return ctx
}

func (i *MockTaskInterceptor) After(ctx context.Context, task Task, result any, err error) {
	i.afterCalled = true
	i.afterTask = task
	i.afterResult = result
	i.afterErr = err
}

// Test TaskInterceptor interface
func TestTaskInterceptorInterface(t *testing.T) {
	interceptor := &MockTaskInterceptor{}
	task := NewMockTask("interceptor-test", PriorityNormal)
	ctx := context.Background()

	// Test Before
	newCtx := interceptor.Before(ctx, task)
	if !interceptor.beforeCalled {
		t.Error("Before should be called")
	}
	if newCtx != ctx {
		t.Error("Before should return the same context")
	}
	if interceptor.beforeCtx != ctx {
		t.Error("Before should receive the correct context")
	}

	// Test After
	result := "test-result"
	err := ErrTaskTimeout
	interceptor.After(ctx, task, result, err)

	if !interceptor.afterCalled {
		t.Error("After should be called")
	}
	if interceptor.afterTask != task {
		t.Error("After should receive the correct task")
	}
	if interceptor.afterResult != result {
		t.Error("After should receive the correct result")
	}
	if interceptor.afterErr != err {
		t.Error("After should receive the correct error")
	}
}

// MockErrorHandler for testing ErrorHandler interface
type MockErrorHandler struct {
	panicCalls     int
	timeoutCalls   int
	queueFullCalls int
	lastPanic      any
	lastTask       Task
	lastDuration   time.Duration
}

func (h *MockErrorHandler) HandlePanic(workerID int, task Task, panicValue any) {
	h.panicCalls++
	h.lastPanic = panicValue
	h.lastTask = task
}

func (h *MockErrorHandler) HandleTimeout(task Task, duration time.Duration) {
	h.timeoutCalls++
	h.lastTask = task
	h.lastDuration = duration
}

func (h *MockErrorHandler) HandleQueueFull(task Task) error {
	h.queueFullCalls++
	h.lastTask = task
	return ErrQueueFull
}

func (h *MockErrorHandler) GetStats() ErrorStats {
	return ErrorStats{
		PanicCount:     int64(h.panicCalls),
		TimeoutCount:   int64(h.timeoutCalls),
		QueueFullCount: int64(h.queueFullCalls),
	}
}

// Test ErrorHandler interface
func TestErrorHandlerInterface(t *testing.T) {
	handler := &MockErrorHandler{}
	task := NewMockTask("error-test", PriorityNormal)

	// Test HandlePanic
	panicValue := "test panic"
	handler.HandlePanic(1, task, panicValue)

	if handler.panicCalls != 1 {
		t.Errorf("Expected 1 panic call, got %d", handler.panicCalls)
	}
	if handler.lastPanic != panicValue {
		t.Errorf("Expected panic value '%v', got '%v'", panicValue, handler.lastPanic)
	}
	if handler.lastTask != task {
		t.Error("Expected correct task in panic handler")
	}

	// Test HandleTimeout
	duration := time.Second * 5
	handler.HandleTimeout(task, duration)

	if handler.timeoutCalls != 1 {
		t.Errorf("Expected 1 timeout call, got %d", handler.timeoutCalls)
	}
	if handler.lastDuration != duration {
		t.Errorf("Expected duration %v, got %v", duration, handler.lastDuration)
	}

	// Test HandleQueueFull
	err := handler.HandleQueueFull(task)

	if handler.queueFullCalls != 1 {
		t.Errorf("Expected 1 queue full call, got %d", handler.queueFullCalls)
	}
	if err != ErrQueueFull {
		t.Errorf("Expected ErrQueueFull, got %v", err)
	}

	// Test GetStats
	stats := handler.GetStats()
	if stats.PanicCount != 1 {
		t.Errorf("Expected panic count 1, got %d", stats.PanicCount)
	}
	if stats.TimeoutCount != 1 {
		t.Errorf("Expected timeout count 1, got %d", stats.TimeoutCount)
	}
	if stats.QueueFullCount != 1 {
		t.Errorf("Expected queue full count 1, got %d", stats.QueueFullCount)
	}
}

// MockMonitor for testing Monitor interface
type MockMonitor struct {
	submitCalls   int
	completeCalls int
	failCalls     int
	started       bool
	stopped       bool
	activeWorkers int64
	queuedTasks   int64
}

func (m *MockMonitor) RecordTaskSubmit(task Task) {
	m.submitCalls++
}

func (m *MockMonitor) RecordTaskComplete(task Task, duration time.Duration) {
	m.completeCalls++
}

func (m *MockMonitor) RecordTaskFail(task Task, err error) {
	m.failCalls++
}

func (m *MockMonitor) GetStats() PoolStats {
	return PoolStats{
		ActiveWorkers:  m.activeWorkers,
		QueuedTasks:    m.queuedTasks,
		CompletedTasks: int64(m.completeCalls),
		FailedTasks:    int64(m.failCalls),
	}
}

func (m *MockMonitor) Start() {
	m.started = true
}

func (m *MockMonitor) Stop() {
	m.stopped = true
}

func (m *MockMonitor) SetActiveWorkers(count int64) {
	m.activeWorkers = count
}

func (m *MockMonitor) SetQueuedTasks(count int64) {
	m.queuedTasks = count
}

// Test Monitor interface
func TestMonitorInterface(t *testing.T) {
	monitor := &MockMonitor{}
	task := NewMockTask("monitor-test", PriorityNormal)

	// Test RecordTaskSubmit
	monitor.RecordTaskSubmit(task)
	if monitor.submitCalls != 1 {
		t.Errorf("Expected 1 submit call, got %d", monitor.submitCalls)
	}

	// Test RecordTaskComplete
	monitor.RecordTaskComplete(task, time.Millisecond*100)
	if monitor.completeCalls != 1 {
		t.Errorf("Expected 1 complete call, got %d", monitor.completeCalls)
	}

	// Test RecordTaskFail
	monitor.RecordTaskFail(task, ErrTaskTimeout)
	if monitor.failCalls != 1 {
		t.Errorf("Expected 1 fail call, got %d", monitor.failCalls)
	}

	// Test SetActiveWorkers
	monitor.SetActiveWorkers(5)
	if monitor.activeWorkers != 5 {
		t.Errorf("Expected active workers 5, got %d", monitor.activeWorkers)
	}

	// Test SetQueuedTasks
	monitor.SetQueuedTasks(10)
	if monitor.queuedTasks != 10 {
		t.Errorf("Expected queued tasks 10, got %d", monitor.queuedTasks)
	}

	// Test GetStats
	stats := monitor.GetStats()
	if stats.ActiveWorkers != 5 {
		t.Errorf("Expected active workers 5, got %d", stats.ActiveWorkers)
	}
	if stats.QueuedTasks != 10 {
		t.Errorf("Expected queued tasks 10, got %d", stats.QueuedTasks)
	}
	if stats.CompletedTasks != 1 {
		t.Errorf("Expected completed tasks 1, got %d", stats.CompletedTasks)
	}
	if stats.FailedTasks != 1 {
		t.Errorf("Expected failed tasks 1, got %d", stats.FailedTasks)
	}

	// Test Start/Stop
	monitor.Start()
	if !monitor.started {
		t.Error("Monitor should be started")
	}

	monitor.Stop()
	if !monitor.stopped {
		t.Error("Monitor should be stopped")
	}
}

// MockWorker for testing Worker interface
type MockWorker struct {
	id             int
	active         bool
	running        bool
	taskCount      int64
	lastActiveTime time.Time
	restartCount   int32
	localQueueSize int
	wg             *sync.WaitGroup
}

func NewMockWorker(id int) *MockWorker {
	return &MockWorker{
		id:             id,
		lastActiveTime: time.Now(),
	}
}

func (w *MockWorker) ID() int {
	return w.id
}

func (w *MockWorker) IsActive() bool {
	return w.active
}

func (w *MockWorker) IsRunning() bool {
	return w.running
}

func (w *MockWorker) TaskCount() int64 {
	return w.taskCount
}

func (w *MockWorker) LastActiveTime() time.Time {
	return w.lastActiveTime
}

func (w *MockWorker) RestartCount() int32 {
	return w.restartCount
}

func (w *MockWorker) LocalQueueSize() int {
	return w.localQueueSize
}

func (w *MockWorker) Start(wg *sync.WaitGroup) {
	w.running = true
	w.wg = wg
}

func (w *MockWorker) Stop() {
	w.running = false
}

// Test Worker interface
func TestWorkerInterface(t *testing.T) {
	worker := NewMockWorker(1)

	// Test ID
	if worker.ID() != 1 {
		t.Errorf("Expected ID 1, got %d", worker.ID())
	}

	// Test initial state
	if worker.IsActive() {
		t.Error("New worker should not be active")
	}
	if worker.IsRunning() {
		t.Error("New worker should not be running")
	}
	if worker.TaskCount() != 0 {
		t.Errorf("Expected task count 0, got %d", worker.TaskCount())
	}
	if worker.RestartCount() != 0 {
		t.Errorf("Expected restart count 0, got %d", worker.RestartCount())
	}
	if worker.LocalQueueSize() != 0 {
		t.Errorf("Expected local queue size 0, got %d", worker.LocalQueueSize())
	}

	// Test Start
	var wg sync.WaitGroup
	worker.Start(&wg)
	if !worker.IsRunning() {
		t.Error("Worker should be running after start")
	}

	// Test Stop
	worker.Stop()
	if worker.IsRunning() {
		t.Error("Worker should not be running after stop")
	}

	// Test LastActiveTime
	lastActive := worker.LastActiveTime()
	if lastActive.IsZero() {
		t.Error("LastActiveTime should not be zero")
	}
}

// MockScheduler for testing Scheduler interface
type MockScheduler struct {
	running bool
	tasks   []Task
}

func (s *MockScheduler) Schedule(task Task) error {
	if !s.running {
		return ErrPoolClosed
	}
	s.tasks = append(s.tasks, task)
	return nil
}

func (s *MockScheduler) Start() error {
	if s.running {
		return ErrPoolAlreadyRunning
	}
	s.running = true
	return nil
}

func (s *MockScheduler) Stop() error {
	if !s.running {
		return ErrPoolNotRunning
	}
	s.running = false
	return nil
}

// Test Scheduler interface
func TestSchedulerInterface(t *testing.T) {
	scheduler := &MockScheduler{}
	task := NewMockTask("scheduler-test", PriorityNormal)

	// Test initial state
	if scheduler.running {
		t.Error("New scheduler should not be running")
	}

	// Test Schedule before start
	err := scheduler.Schedule(task)
	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}

	// Test Start
	err = scheduler.Start()
	if err != nil {
		t.Errorf("Expected no error on start, got %v", err)
	}
	if !scheduler.running {
		t.Error("Scheduler should be running after start")
	}

	// Test double start
	err = scheduler.Start()
	if err == nil {
		t.Error("Expected error on double start")
	}

	// Test Schedule after start
	err = scheduler.Schedule(task)
	if err != nil {
		t.Errorf("Expected no error on schedule, got %v", err)
	}
	if len(scheduler.tasks) != 1 {
		t.Errorf("Expected 1 scheduled task, got %d", len(scheduler.tasks))
	}

	// Test Stop
	err = scheduler.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop, got %v", err)
	}
	if scheduler.running {
		t.Error("Scheduler should not be running after stop")
	}

	// Test double stop
	err = scheduler.Stop()
	if err != ErrPoolNotRunning {
		t.Errorf("Expected ErrPoolNotRunning, got %v", err)
	}
}
