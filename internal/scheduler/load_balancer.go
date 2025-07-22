package scheduler

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// RoundRobinBalancer 轮询负载均衡器
type RoundRobinBalancer struct {
	counter uint64 // 计数器，使用原子操作
}

// NewRoundRobinBalancer 创建轮询负载均衡器
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

// SelectWorker 选择工作协程（轮询策略）
func (rb *RoundRobinBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 {
		return nil
	}

	// 过滤出运行中的工作协程
	runningWorkers := make([]pool.Worker, 0, len(workers))
	for _, worker := range workers {
		if worker.IsRunning() {
			runningWorkers = append(runningWorkers, worker)
		}
	}

	if len(runningWorkers) == 0 {
		return nil
	}

	// 原子递增计数器并取模
	index := atomic.AddUint64(&rb.counter, 1) % uint64(len(runningWorkers))
	return runningWorkers[index]
}

// LeastLoadBalancer 最少负载负载均衡器
type LeastLoadBalancer struct {
	// 可以添加一些配置参数
}

// NewLeastLoadBalancer 创建最少负载负载均衡器
func NewLeastLoadBalancer() *LeastLoadBalancer {
	return &LeastLoadBalancer{}
}

// SelectWorker 选择工作协程（最少负载策略）
func (lb *LeastLoadBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 {
		return nil
	}

	var selectedWorker pool.Worker
	minLoad := int64(^uint64(0) >> 1) // 最大int64值

	for _, worker := range workers {
		if !worker.IsRunning() {
			continue
		}

		// 计算工作协程的总负载（任务数 + 本地队列大小）
		taskCount := worker.TaskCount()
		localQueueSize := int64(worker.LocalQueueSize())
		totalLoad := taskCount + localQueueSize

		// 如果发现完全空闲的工作协程，直接选择
		if totalLoad == 0 {
			return worker
		}

		// 选择负载最小的工作协程
		if totalLoad < minLoad {
			minLoad = totalLoad
			selectedWorker = worker
		}
	}

	return selectedWorker
}

// RandomBalancer 随机负载均衡器
type RandomBalancer struct {
	rng   *rand.Rand // 随机数生成器
	mutex sync.Mutex // 保护随机数生成器的互斥锁
}

// NewRandomBalancer 创建随机负载均衡器
func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectWorker 选择工作协程（随机策略）
func (rb *RandomBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 {
		return nil
	}

	// 过滤出运行中的工作协程
	runningWorkers := make([]pool.Worker, 0, len(workers))
	for _, worker := range workers {
		if worker.IsRunning() {
			runningWorkers = append(runningWorkers, worker)
		}
	}

	if len(runningWorkers) == 0 {
		return nil
	}

	// 线程安全的随机选择
	rb.mutex.Lock()
	index := rb.rng.Intn(len(runningWorkers))
	rb.mutex.Unlock()

	return runningWorkers[index]
}

// WeightedRoundRobinBalancer 加权轮询负载均衡器
type WeightedRoundRobinBalancer struct {
	weights []int  // 权重数组
	counter uint64 // 计数器
}

// NewWeightedRoundRobinBalancer 创建加权轮询负载均衡器
func NewWeightedRoundRobinBalancer(weights []int) *WeightedRoundRobinBalancer {
	if len(weights) == 0 {
		weights = []int{1} // 默认权重
	}

	return &WeightedRoundRobinBalancer{
		weights: weights,
	}
}

// SelectWorker 选择工作协程（加权轮询策略）
func (wrb *WeightedRoundRobinBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 {
		return nil
	}

	// 过滤出运行中的工作协程
	runningWorkers := make([]pool.Worker, 0, len(workers))
	for _, worker := range workers {
		if worker.IsRunning() {
			runningWorkers = append(runningWorkers, worker)
		}
	}

	if len(runningWorkers) == 0 {
		return nil
	}

	// 确保权重数组长度匹配
	if len(wrb.weights) != len(runningWorkers) {
		// 如果权重数组长度不匹配，使用轮询策略
		index := atomic.AddUint64(&wrb.counter, 1) % uint64(len(runningWorkers))
		return runningWorkers[index]
	}

	// 计算总权重
	totalWeight := 0
	for _, weight := range wrb.weights {
		totalWeight += weight
	}

	if totalWeight == 0 {
		// 所有权重都为0，使用轮询策略
		index := atomic.AddUint64(&wrb.counter, 1) % uint64(len(runningWorkers))
		return runningWorkers[index]
	}

	// 根据权重选择工作协程
	counter := atomic.AddUint64(&wrb.counter, 1)
	position := int(counter % uint64(totalWeight))

	currentWeight := 0
	for i, weight := range wrb.weights {
		currentWeight += weight
		if position < currentWeight {
			return runningWorkers[i]
		}
	}

	// 理论上不应该到达这里，但为了安全起见
	return runningWorkers[0]
}

// ConsistentHashBalancer 一致性哈希负载均衡器
type ConsistentHashBalancer struct {
	hashRing     map[uint32]pool.Worker // 哈希环
	sortedHashes []uint32               // 排序的哈希值
}

// NewConsistentHashBalancer 创建一致性哈希负载均衡器
func NewConsistentHashBalancer() *ConsistentHashBalancer {
	return &ConsistentHashBalancer{
		hashRing:     make(map[uint32]pool.Worker),
		sortedHashes: make([]uint32, 0),
	}
}

// AddWorker 添加工作协程到哈希环
func (chb *ConsistentHashBalancer) AddWorker(worker pool.Worker) {
	// 为每个工作协程创建多个虚拟节点
	virtualNodes := 150 // 虚拟节点数量

	for i := 0; i < virtualNodes; i++ {
		hash := chb.hash(worker.ID(), i)
		chb.hashRing[hash] = worker
		chb.sortedHashes = append(chb.sortedHashes, hash)
	}

	// 排序哈希值
	chb.sortHashes()
}

// RemoveWorker 从哈希环移除工作协程
func (chb *ConsistentHashBalancer) RemoveWorker(worker pool.Worker) {
	virtualNodes := 150

	for i := 0; i < virtualNodes; i++ {
		hash := chb.hash(worker.ID(), i)
		delete(chb.hashRing, hash)

		// 从排序数组中移除
		for j, h := range chb.sortedHashes {
			if h == hash {
				chb.sortedHashes = append(chb.sortedHashes[:j], chb.sortedHashes[j+1:]...)
				break
			}
		}
	}
}

// SelectWorker 选择工作协程（一致性哈希策略）
func (chb *ConsistentHashBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 || len(chb.sortedHashes) == 0 {
		return nil
	}

	// 使用当前时间作为键进行哈希
	key := time.Now().UnixNano()
	hash := chb.hashKey(key)

	// 在哈希环上找到第一个大于等于该哈希值的节点
	idx := chb.binarySearch(hash)
	if idx >= len(chb.sortedHashes) {
		idx = 0 // 环形结构，回到开头
	}

	selectedHash := chb.sortedHashes[idx]
	worker := chb.hashRing[selectedHash]

	// 检查工作协程是否运行中
	if worker != nil && worker.IsRunning() {
		return worker
	}

	// 如果选中的工作协程不可用，尝试下一个
	for i := 1; i < len(chb.sortedHashes); i++ {
		nextIdx := (idx + i) % len(chb.sortedHashes)
		nextHash := chb.sortedHashes[nextIdx]
		nextWorker := chb.hashRing[nextHash]

		if nextWorker != nil && nextWorker.IsRunning() {
			return nextWorker
		}
	}

	return nil
}

// hash 计算哈希值
func (chb *ConsistentHashBalancer) hash(workerID, virtualNode int) uint32 {
	// 简单的哈希函数实现
	key := workerID*1000 + virtualNode
	return uint32(key * 2654435761) // 使用黄金比例的近似值
}

// hashKey 计算键的哈希值
func (chb *ConsistentHashBalancer) hashKey(key int64) uint32 {
	return uint32(key * 2654435761)
}

// binarySearch 二分查找
func (chb *ConsistentHashBalancer) binarySearch(hash uint32) int {
	left, right := 0, len(chb.sortedHashes)

	for left < right {
		mid := (left + right) / 2
		if chb.sortedHashes[mid] < hash {
			left = mid + 1
		} else {
			right = mid
		}
	}

	return left
}

// sortHashes 排序哈希值
func (chb *ConsistentHashBalancer) sortHashes() {
	// 简单的冒泡排序（对于小数组足够了）
	n := len(chb.sortedHashes)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if chb.sortedHashes[j] > chb.sortedHashes[j+1] {
				chb.sortedHashes[j], chb.sortedHashes[j+1] = chb.sortedHashes[j+1], chb.sortedHashes[j]
			}
		}
	}
}

// AdaptiveBalancer 自适应负载均衡器
type AdaptiveBalancer struct {
	strategies       []pool.LoadBalancer // 多种策略
	current          int                 // 当前使用的策略索引
	metrics          []float64           // 各策略的性能指标
	switchThreshold  float64             // 切换阈值
	evaluationWindow int                 // 评估窗口大小
	evaluationCount  int                 // 当前评估计数
}

// NewAdaptiveBalancer 创建自适应负载均衡器
func NewAdaptiveBalancer() *AdaptiveBalancer {
	strategies := []pool.LoadBalancer{
		NewRoundRobinBalancer(),
		NewLeastLoadBalancer(),
		NewRandomBalancer(),
	}

	return &AdaptiveBalancer{
		strategies:       strategies,
		current:          0,
		metrics:          make([]float64, len(strategies)),
		switchThreshold:  0.1, // 10%的性能差异触发切换
		evaluationWindow: 100, // 每100次调用评估一次
	}
}

// SelectWorker 选择工作协程（自适应策略）
func (ab *AdaptiveBalancer) SelectWorker(workers []pool.Worker) pool.Worker {
	if len(workers) == 0 || len(ab.strategies) == 0 {
		return nil
	}

	// 使用当前策略选择工作协程
	currentStrategy := ab.strategies[ab.current]
	selectedWorker := currentStrategy.SelectWorker(workers)

	// 增加评估计数
	ab.evaluationCount++

	// 定期评估和切换策略
	if ab.evaluationCount >= ab.evaluationWindow {
		ab.evaluateAndSwitch(workers)
		ab.evaluationCount = 0
	}

	return selectedWorker
}

// evaluateAndSwitch 评估并切换策略
func (ab *AdaptiveBalancer) evaluateAndSwitch(workers []pool.Worker) {
	if len(workers) == 0 {
		return
	}

	// 计算当前负载分布的标准差（作为负载均衡效果的指标）
	currentVariance := ab.calculateLoadVariance(workers)

	// 更新当前策略的指标
	ab.metrics[ab.current] = currentVariance

	// 找到最佳策略
	bestStrategy := 0
	bestMetric := ab.metrics[0]

	for i := 1; i < len(ab.metrics); i++ {
		if ab.metrics[i] < bestMetric {
			bestMetric = ab.metrics[i]
			bestStrategy = i
		}
	}

	// 如果找到更好的策略，且改进超过阈值，则切换
	if bestStrategy != ab.current &&
		(ab.metrics[ab.current]-bestMetric) > ab.switchThreshold*ab.metrics[ab.current] {
		ab.current = bestStrategy
	}
}

// calculateLoadVariance 计算负载方差
func (ab *AdaptiveBalancer) calculateLoadVariance(workers []pool.Worker) float64 {
	if len(workers) == 0 {
		return 0
	}

	// 计算平均负载
	totalLoad := int64(0)
	runningCount := 0

	for _, worker := range workers {
		if worker.IsRunning() {
			totalLoad += worker.TaskCount() + int64(worker.LocalQueueSize())
			runningCount++
		}
	}

	if runningCount == 0 {
		return 0
	}

	avgLoad := float64(totalLoad) / float64(runningCount)

	// 计算方差
	variance := 0.0
	for _, worker := range workers {
		if worker.IsRunning() {
			load := float64(worker.TaskCount() + int64(worker.LocalQueueSize()))
			diff := load - avgLoad
			variance += diff * diff
		}
	}

	return variance / float64(runningCount)
}
