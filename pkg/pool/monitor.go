package pool

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// monitor 默认监控器实现
type monitor struct {
	// 基础指标 (使用atomic操作保证并发安全)
	activeWorkers  int64
	queuedTasks    int64
	completedTasks int64
	failedTasks    int64

	// 性能指标
	totalTaskDuration int64 // 总任务执行时间 (纳秒)
	lastStatsTime     int64 // 上次统计时间 (Unix纳秒)
	throughputTPS     int64 // 吞吐量 (使用atomic存储float64的bits)

	// 资源指标
	memoryUsage int64
	gcCount     int64

	// 配置
	metricsInterval time.Duration
	enableMetrics   bool

	// 内部状态
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	updateTicker *time.Ticker

	// 历史数据用于计算平均值和吞吐量
	taskDurations  []time.Duration
	taskTimes      []time.Time
	maxHistorySize int
}

// NewMonitor 创建新的监控器
func NewMonitor(config *Config) Monitor {
	m := &monitor{
		metricsInterval: config.MetricsInterval,
		enableMetrics:   config.EnableMetrics,
		stopCh:          make(chan struct{}),
		maxHistorySize:  1000, // 保留最近1000个任务的历史数据
		taskDurations:   make([]time.Duration, 0, 1000),
		taskTimes:       make([]time.Time, 0, 1000),
	}

	// 设置默认监控间隔
	if m.metricsInterval <= 0 {
		m.metricsInterval = 5 * time.Second
	}

	// 初始化统计时间
	atomic.StoreInt64(&m.lastStatsTime, time.Now().UnixNano())

	return m
}

// Start 启动监控器
func (m *monitor) Start() {
	if !m.enableMetrics {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.running = true
	m.updateTicker = time.NewTicker(m.metricsInterval)

	go m.updateLoop()
}

// Stop 停止监控器
func (m *monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	close(m.stopCh)

	if m.updateTicker != nil {
		m.updateTicker.Stop()
	}
}

// updateLoop 定期更新监控指标
func (m *monitor) updateLoop() {
	for {
		select {
		case <-m.updateTicker.C:
			m.updateResourceMetrics()
			m.updateThroughput()
		case <-m.stopCh:
			return
		}
	}
}

// updateResourceMetrics 更新资源指标
func (m *monitor) updateResourceMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 更新内存使用量 (使用堆内存大小)
	atomic.StoreInt64(&m.memoryUsage, int64(memStats.Alloc))

	// 更新GC次数
	atomic.StoreInt64(&m.gcCount, int64(memStats.NumGC))
}

// updateThroughput 更新吞吐量
func (m *monitor) updateThroughput() {
	now := time.Now()
	currentTime := now.UnixNano()
	lastTime := atomic.LoadInt64(&m.lastStatsTime)

	if lastTime == 0 {
		atomic.StoreInt64(&m.lastStatsTime, currentTime)
		return
	}

	// 计算时间间隔
	duration := time.Duration(currentTime - lastTime)
	if duration <= 0 {
		return
	}

	// 计算在这个时间间隔内完成的任务数
	m.mu.RLock()
	completedInInterval := int64(0)
	cutoffTime := now.Add(-duration)

	for i := len(m.taskTimes) - 1; i >= 0; i-- {
		if m.taskTimes[i].After(cutoffTime) {
			completedInInterval++
		} else {
			break
		}
	}
	m.mu.RUnlock()

	// 计算吞吐量 (任务数/秒)
	tps := float64(completedInInterval) / duration.Seconds()
	atomic.StoreInt64(&m.throughputTPS, int64(tps*1000)) // 存储为千分之一精度

	// 更新统计时间
	atomic.StoreInt64(&m.lastStatsTime, currentTime)
}

// RecordTaskSubmit 记录任务提交
func (m *monitor) RecordTaskSubmit(task Task) {
	if !m.enableMetrics {
		return
	}

	atomic.AddInt64(&m.queuedTasks, 1)
}

// RecordTaskComplete 记录任务完成
func (m *monitor) RecordTaskComplete(task Task, duration time.Duration) {
	if !m.enableMetrics {
		return
	}

	// 更新基础指标
	atomic.AddInt64(&m.queuedTasks, -1)
	atomic.AddInt64(&m.completedTasks, 1)

	// 更新任务执行时间
	atomic.AddInt64(&m.totalTaskDuration, int64(duration))

	// 更新历史数据
	m.mu.Lock()
	now := time.Now()

	// 添加新的任务时间和持续时间
	m.taskTimes = append(m.taskTimes, now)
	m.taskDurations = append(m.taskDurations, duration)

	// 保持历史数据大小在限制范围内
	if len(m.taskTimes) > m.maxHistorySize {
		// 移除最旧的数据
		copy(m.taskTimes, m.taskTimes[1:])
		copy(m.taskDurations, m.taskDurations[1:])
		m.taskTimes = m.taskTimes[:len(m.taskTimes)-1]
		m.taskDurations = m.taskDurations[:len(m.taskDurations)-1]
	}
	m.mu.Unlock()
}

// RecordTaskFail 记录任务失败
func (m *monitor) RecordTaskFail(task Task, err error) {
	if !m.enableMetrics {
		return
	}

	atomic.AddInt64(&m.queuedTasks, -1)
	atomic.AddInt64(&m.failedTasks, 1)
}

// SetActiveWorkers 设置活跃工作协程数
func (m *monitor) SetActiveWorkers(count int64) {
	atomic.StoreInt64(&m.activeWorkers, count)
}

// SetQueuedTasks 设置排队任务数
func (m *monitor) SetQueuedTasks(count int64) {
	atomic.StoreInt64(&m.queuedTasks, count)
}

// GetStats 获取统计信息
func (m *monitor) GetStats() PoolStats {
	// 读取原子变量
	activeWorkers := atomic.LoadInt64(&m.activeWorkers)
	queuedTasks := atomic.LoadInt64(&m.queuedTasks)
	completedTasks := atomic.LoadInt64(&m.completedTasks)
	failedTasks := atomic.LoadInt64(&m.failedTasks)
	totalDuration := atomic.LoadInt64(&m.totalTaskDuration)
	memoryUsage := atomic.LoadInt64(&m.memoryUsage)
	gcCount := atomic.LoadInt64(&m.gcCount)
	throughputBits := atomic.LoadInt64(&m.throughputTPS)

	// 计算平均任务执行时间
	var avgDuration time.Duration
	if completedTasks > 0 {
		avgDuration = time.Duration(totalDuration / completedTasks)
	}

	// 转换吞吐量
	throughputTPS := float64(throughputBits) / 1000.0

	return PoolStats{
		ActiveWorkers:   activeWorkers,
		QueuedTasks:     queuedTasks,
		CompletedTasks:  completedTasks,
		FailedTasks:     failedTasks,
		AvgTaskDuration: avgDuration,
		ThroughputTPS:   throughputTPS,
		MemoryUsage:     memoryUsage,
		GCCount:         gcCount,
	}
}

// GetDetailedStats 获取详细统计信息 (包含历史数据分析)
func (m *monitor) GetDetailedStats() DetailedStats {
	stats := m.GetStats()

	m.mu.RLock()
	defer m.mu.RUnlock()

	detailed := DetailedStats{
		PoolStats: stats,
	}

	if len(m.taskDurations) > 0 {
		// 计算任务执行时间的统计信息
		detailed.TaskDurationStats = m.calculateDurationStats()
	}

	if len(m.taskTimes) > 0 {
		// 计算最近的吞吐量趋势
		detailed.ThroughputTrend = m.calculateThroughputTrend()
	}

	return detailed
}

// calculateDurationStats 计算任务执行时间统计
func (m *monitor) calculateDurationStats() DurationStats {
	if len(m.taskDurations) == 0 {
		return DurationStats{}
	}

	// 复制并排序持续时间数据
	durations := make([]time.Duration, len(m.taskDurations))
	copy(durations, m.taskDurations)

	// 简单的冒泡排序 (对于小数据集足够)
	for i := 0; i < len(durations)-1; i++ {
		for j := 0; j < len(durations)-i-1; j++ {
			if durations[j] > durations[j+1] {
				durations[j], durations[j+1] = durations[j+1], durations[j]
			}
		}
	}

	stats := DurationStats{
		Min: durations[0],
		Max: durations[len(durations)-1],
	}

	// 计算中位数
	mid := len(durations) / 2
	if len(durations)%2 == 0 {
		stats.Median = (durations[mid-1] + durations[mid]) / 2
	} else {
		stats.Median = durations[mid]
	}

	// 计算P95和P99
	p95Index := int(float64(len(durations)) * 0.95)
	p99Index := int(float64(len(durations)) * 0.99)

	if p95Index >= len(durations) {
		p95Index = len(durations) - 1
	}
	if p99Index >= len(durations) {
		p99Index = len(durations) - 1
	}

	stats.P95 = durations[p95Index]
	stats.P99 = durations[p99Index]

	return stats
}

// calculateThroughputTrend 计算吞吐量趋势
func (m *monitor) calculateThroughputTrend() []ThroughputPoint {
	if len(m.taskTimes) < 2 {
		return nil
	}

	// 计算每分钟的吞吐量
	trend := make([]ThroughputPoint, 0)
	now := time.Now()

	// 按分钟分组计算吞吐量
	for i := 0; i < 10; i++ { // 最近10分钟
		minuteStart := now.Add(-time.Duration(i+1) * time.Minute)
		minuteEnd := now.Add(-time.Duration(i) * time.Minute)

		count := 0
		for _, taskTime := range m.taskTimes {
			if taskTime.After(minuteStart) && taskTime.Before(minuteEnd) {
				count++
			}
		}

		throughput := float64(count) / 60.0 // 每秒任务数
		trend = append([]ThroughputPoint{{
			Time:       minuteStart,
			Throughput: throughput,
		}}, trend...)
	}

	return trend
}
