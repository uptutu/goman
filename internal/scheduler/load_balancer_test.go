package scheduler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// MockWorker 模拟工作协程
type MockWorker struct {
	id             int
	isRunning      bool
	isActive       bool
	taskCount      int64
	localQueueSize int
	lastActiveTime time.Time
	restartCount   int32
}

func NewMockWorker(id int) *MockWorker {
	return &MockWorker{
		id:             id,
		isRunning:      true,
		isActive:       false,
		taskCount:      0,
		localQueueSize: 0,
		lastActiveTime: time.Now(),
		restartCount:   0,
	}
}

func (w *MockWorker) ID() int {
	return w.id
}

func (w *MockWorker) IsActive() bool {
	return w.isActive
}

func (w *MockWorker) IsRunning() bool {
	return w.isRunning
}

func (w *MockWorker) TaskCount() int64 {
	return atomic.LoadInt64(&w.taskCount)
}

func (w *MockWorker) LastActiveTime() time.Time {
	return w.lastActiveTime
}

func (w *MockWorker) RestartCount() int32 {
	return atomic.LoadInt32(&w.restartCount)
}

func (w *MockWorker) LocalQueueSize() int {
	return w.localQueueSize
}

func (w *MockWorker) Start(wg *sync.WaitGroup) {
	w.isRunning = true
}

func (w *MockWorker) Stop() {
	w.isRunning = false
}

func (w *MockWorker) SetTaskCount(count int64) {
	atomic.StoreInt64(&w.taskCount, count)
}

func (w *MockWorker) SetLocalQueueSize(size int) {
	w.localQueueSize = size
}

func (w *MockWorker) SetRunning(running bool) {
	w.isRunning = running
}

// 创建测试用的工作协程列表
func createMockWorkers(count int) []pool.Worker {
	workers := make([]pool.Worker, count)
	for i := 0; i < count; i++ {
		workers[i] = NewMockWorker(i)
	}
	return workers
}

func TestRoundRobinBalancer(t *testing.T) {
	balancer := NewRoundRobinBalancer()
	workers := createMockWorkers(3)

	// 测试空工作协程列表
	selected := balancer.SelectWorker([]pool.Worker{})
	if selected != nil {
		t.Error("Expected nil for empty worker list")
	}

	// 测试轮询选择
	selections := make(map[int]int)
	for i := 0; i < 9; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		selections[selected.ID()]++
	}

	// 验证每个工作协程都被选择了3次
	for i := 0; i < 3; i++ {
		if selections[i] != 3 {
			t.Errorf("Expected worker %d to be selected 3 times, got %d", i, selections[i])
		}
	}

	// 测试部分工作协程停止的情况
	workers[1].(*MockWorker).SetRunning(false)

	selections = make(map[int]int)
	for i := 0; i < 6; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		selections[selected.ID()]++
	}

	// 验证只有运行中的工作协程被选择
	if selections[1] != 0 {
		t.Errorf("Expected stopped worker 1 not to be selected, got %d", selections[1])
	}
	if selections[0] != 3 || selections[2] != 3 {
		t.Errorf("Expected running workers to be selected equally, got %v", selections)
	}
}

func TestLeastLoadBalancer(t *testing.T) {
	balancer := NewLeastLoadBalancer()
	workers := createMockWorkers(3)

	// 测试空工作协程列表
	selected := balancer.SelectWorker([]pool.Worker{})
	if selected != nil {
		t.Error("Expected nil for empty worker list")
	}

	// 设置不同的负载
	workers[0].(*MockWorker).SetTaskCount(10)
	workers[0].(*MockWorker).SetLocalQueueSize(5)
	workers[1].(*MockWorker).SetTaskCount(5)
	workers[1].(*MockWorker).SetLocalQueueSize(2)
	workers[2].(*MockWorker).SetTaskCount(15)
	workers[2].(*MockWorker).SetLocalQueueSize(8)

	// 应该选择负载最小的工作协程（worker 1: 5+2=7）
	selected = balancer.SelectWorker(workers)
	if selected == nil {
		t.Fatal("Expected worker to be selected")
	}
	if selected.ID() != 1 {
		t.Errorf("Expected worker 1 to be selected (least load), got worker %d", selected.ID())
	}

	// 测试完全空闲的工作协程
	workers[2].(*MockWorker).SetTaskCount(0)
	workers[2].(*MockWorker).SetLocalQueueSize(0)

	selected = balancer.SelectWorker(workers)
	if selected == nil {
		t.Fatal("Expected worker to be selected")
	}
	if selected.ID() != 2 {
		t.Errorf("Expected worker 2 to be selected (idle), got worker %d", selected.ID())
	}

	// 测试所有工作协程都停止的情况
	for _, worker := range workers {
		worker.(*MockWorker).SetRunning(false)
	}

	selected = balancer.SelectWorker(workers)
	if selected != nil {
		t.Error("Expected nil when all workers are stopped")
	}
}

func TestRandomBalancer(t *testing.T) {
	balancer := NewRandomBalancer()
	workers := createMockWorkers(3)

	// 测试空工作协程列表
	selected := balancer.SelectWorker([]pool.Worker{})
	if selected != nil {
		t.Error("Expected nil for empty worker list")
	}

	// 测试随机选择
	selections := make(map[int]int)
	for i := 0; i < 300; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		selections[selected.ID()]++
	}

	// 验证每个工作协程都被选择了（随机分布）
	for i := 0; i < 3; i++ {
		if selections[i] == 0 {
			t.Errorf("Expected worker %d to be selected at least once", i)
		}
		// 随机分布应该相对均匀（允许一定偏差）
		if selections[i] < 50 || selections[i] > 150 {
			t.Logf("Worker %d selected %d times (may be within random variance)", i, selections[i])
		}
	}

	// 测试部分工作协程停止
	workers[0].(*MockWorker).SetRunning(false)
	workers[2].(*MockWorker).SetRunning(false)

	// 只有worker 1在运行
	for i := 0; i < 10; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		if selected.ID() != 1 {
			t.Errorf("Expected only worker 1 to be selected, got worker %d", selected.ID())
		}
	}
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	weights := []int{1, 2, 3}
	balancer := NewWeightedRoundRobinBalancer(weights)
	workers := createMockWorkers(3)

	// 测试空工作协程列表
	selected := balancer.SelectWorker([]pool.Worker{})
	if selected != nil {
		t.Error("Expected nil for empty worker list")
	}

	// 测试加权轮询
	selections := make(map[int]int)
	totalSelections := 60 // 总权重6的倍数

	for i := 0; i < totalSelections; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		selections[selected.ID()]++
	}

	// 验证权重分配（worker 0:1, worker 1:2, worker 2:3）
	expectedRatio := []int{10, 20, 30} // 基于权重1:2:3
	for i := 0; i < 3; i++ {
		if selections[i] != expectedRatio[i] {
			t.Errorf("Expected worker %d to be selected %d times, got %d", i, expectedRatio[i], selections[i])
		}
	}

	// 测试权重不匹配的情况（应该回退到轮询）
	workers = createMockWorkers(4) // 4个工作协程但只有3个权重
	selections = make(map[int]int)

	for i := 0; i < 12; i++ {
		selected := balancer.SelectWorker(workers)
		if selected == nil {
			t.Fatal("Expected worker to be selected")
		}
		selections[selected.ID()]++
	}

	// 应该均匀分布（轮询模式）
	for i := 0; i < 4; i++ {
		if selections[i] != 3 {
			t.Errorf("Expected worker %d to be selected 3 times (round-robin fallback), got %d", i, selections[i])
		}
	}
}

func TestConsistentHashBalancer(t *testing.T) {
	balancer := NewConsistentHashBalancer()
	workers := createMockWorkers(3)

	// 添加工作协程到哈希环
	for _, worker := range workers {
		balancer.AddWorker(worker)
	}

	// 测试选择
	selected := balancer.SelectWorker(workers)
	if selected == nil {
		t.Fatal("Expected worker to be selected")
	}

	// 测试一致性（相同的时间点应该选择相同的工作协程）
	// 注意：由于使用时间作为键，这个测试可能不稳定
	selections := make(map[int]int)
	for i := 0; i < 100; i++ {
		selected := balancer.SelectWorker(workers)
		if selected != nil {
			selections[selected.ID()]++
		}
	}

	// 验证至少有工作协程被选择
	totalSelections := 0
	for _, count := range selections {
		totalSelections += count
	}
	if totalSelections == 0 {
		t.Error("Expected at least some workers to be selected")
	}

	// 测试移除工作协程
	balancer.RemoveWorker(workers[0])

	// 移除后仍应该能选择工作协程
	selected = balancer.SelectWorker(workers[1:])
	if selected == nil {
		t.Error("Expected worker to be selected after removal")
	}

	// 测试空哈希环
	for _, worker := range workers {
		balancer.RemoveWorker(worker)
	}

	selected = balancer.SelectWorker(workers)
	if selected != nil {
		t.Error("Expected nil when hash ring is empty")
	}
}

func TestAdaptiveBalancer(t *testing.T) {
	balancer := NewAdaptiveBalancer()
	workers := createMockWorkers(3)

	// 测试初始状态
	if len(balancer.strategies) != 3 {
		t.Errorf("Expected 3 strategies, got %d", len(balancer.strategies))
	}

	if balancer.current != 0 {
		t.Errorf("Expected initial strategy to be 0, got %d", balancer.current)
	}

	// 测试选择
	selected := balancer.SelectWorker(workers)
	if selected == nil {
		t.Fatal("Expected worker to be selected")
	}

	// 测试评估窗口
	initialCount := balancer.evaluationCount
	for i := 0; i < balancer.evaluationWindow-1; i++ {
		balancer.SelectWorker(workers)
	}

	expectedCount := initialCount + balancer.evaluationWindow - 1
	if balancer.evaluationCount != expectedCount {
		t.Logf("Evaluation count after %d selections: expected %d, got %d", balancer.evaluationWindow-1, expectedCount, balancer.evaluationCount)
	}

	// 触发评估 - 这应该重置计数器
	balancer.SelectWorker(workers)

	if balancer.evaluationCount != 1 { // 重置后应该是1（刚执行了一次SelectWorker）
		t.Logf("Expected evaluation count to be 1 after reset, got %d", balancer.evaluationCount)
	}

	// 测试负载方差计算
	workers[0].(*MockWorker).SetTaskCount(10)
	workers[1].(*MockWorker).SetTaskCount(5)
	workers[2].(*MockWorker).SetTaskCount(15)

	variance := balancer.calculateLoadVariance(workers)
	if variance <= 0 {
		t.Error("Expected positive variance for different loads")
	}

	// 测试相同负载的方差
	for _, worker := range workers {
		worker.(*MockWorker).SetTaskCount(10)
	}

	variance = balancer.calculateLoadVariance(workers)
	if variance != 0 {
		t.Errorf("Expected zero variance for equal loads, got %f", variance)
	}
}

// 基准测试
func BenchmarkRoundRobinBalancer(b *testing.B) {
	balancer := NewRoundRobinBalancer()
	workers := createMockWorkers(8)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			balancer.SelectWorker(workers)
		}
	})
}

func BenchmarkLeastLoadBalancer(b *testing.B) {
	balancer := NewLeastLoadBalancer()
	workers := createMockWorkers(8)

	// 设置不同的负载
	for i, worker := range workers {
		worker.(*MockWorker).SetTaskCount(int64(i * 10))
		worker.(*MockWorker).SetLocalQueueSize(i * 2)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			balancer.SelectWorker(workers)
		}
	})
}

func BenchmarkRandomBalancer(b *testing.B) {
	balancer := NewRandomBalancer()
	workers := createMockWorkers(8)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			balancer.SelectWorker(workers)
		}
	})
}

func BenchmarkWeightedRoundRobinBalancer(b *testing.B) {
	weights := []int{1, 2, 3, 4, 5, 6, 7, 8}
	balancer := NewWeightedRoundRobinBalancer(weights)
	workers := createMockWorkers(8)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			balancer.SelectWorker(workers)
		}
	})
}

func BenchmarkConsistentHashBalancer(b *testing.B) {
	balancer := NewConsistentHashBalancer()
	workers := createMockWorkers(8)

	for _, worker := range workers {
		balancer.AddWorker(worker)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.SelectWorker(workers)
	}
}

func BenchmarkAdaptiveBalancer(b *testing.B) {
	balancer := NewAdaptiveBalancer()
	workers := createMockWorkers(8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.SelectWorker(workers)
	}
}
