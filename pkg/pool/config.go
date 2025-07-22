package pool

import (
	"errors"
	"fmt"
	"time"
)

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		WorkerCount:     DefaultWorkerCount,
		QueueSize:       DefaultQueueSize,
		ObjectPoolSize:  DefaultObjectPoolSize,
		PreAlloc:        false,
		TaskTimeout:     DefaultTaskTimeout,
		ShutdownTimeout: DefaultShutdownTimeout,
		EnableMetrics:   true,
		MetricsInterval: DefaultMetricsInterval,
	}
}

// ConfigBuilder 配置构建器
type ConfigBuilder struct {
	config *Config
}

// NewConfigBuilder 创建配置构建器
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: NewConfig(),
	}
}

// WithWorkerCount 设置工作协程数量
func (b *ConfigBuilder) WithWorkerCount(count int) *ConfigBuilder {
	b.config.WorkerCount = count
	return b
}

// WithQueueSize 设置队列大小
func (b *ConfigBuilder) WithQueueSize(size int) *ConfigBuilder {
	b.config.QueueSize = size
	return b
}

// WithObjectPoolSize 设置对象池大小
func (b *ConfigBuilder) WithObjectPoolSize(size int) *ConfigBuilder {
	b.config.ObjectPoolSize = size
	return b
}

// WithPreAlloc 设置是否预分配内存
func (b *ConfigBuilder) WithPreAlloc(preAlloc bool) *ConfigBuilder {
	b.config.PreAlloc = preAlloc
	return b
}

// WithTaskTimeout 设置任务超时时间
func (b *ConfigBuilder) WithTaskTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.TaskTimeout = timeout
	return b
}

// WithShutdownTimeout 设置关闭超时时间
func (b *ConfigBuilder) WithShutdownTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.ShutdownTimeout = timeout
	return b
}

// WithMetrics 设置是否启用监控
func (b *ConfigBuilder) WithMetrics(enable bool) *ConfigBuilder {
	b.config.EnableMetrics = enable
	return b
}

// WithMetricsInterval 设置监控间隔
func (b *ConfigBuilder) WithMetricsInterval(interval time.Duration) *ConfigBuilder {
	b.config.MetricsInterval = interval
	return b
}

// WithPanicHandler 设置panic处理器
func (b *ConfigBuilder) WithPanicHandler(handler func(any)) *ConfigBuilder {
	b.config.PanicHandler = handler
	return b
}

// WithTaskInterceptor 设置任务拦截器
func (b *ConfigBuilder) WithTaskInterceptor(interceptor TaskInterceptor) *ConfigBuilder {
	b.config.TaskInterceptor = interceptor
	return b
}

// Build 构建配置并验证
func (b *ConfigBuilder) Build() (*Config, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}
	return b.config, nil
}

// BuildUnsafe 构建配置但不验证（用于测试或特殊场景）
func (b *ConfigBuilder) BuildUnsafe() *Config {
	return b.config
}

// Reset 重置配置为默认值
func (b *ConfigBuilder) Reset() *ConfigBuilder {
	b.config = NewConfig()
	return b
}

// Clone 克隆当前配置
func (b *ConfigBuilder) Clone() *ConfigBuilder {
	newConfig := *b.config
	return &ConfigBuilder{config: &newConfig}
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证工作协程数量
	if c.WorkerCount <= 0 {
		return NewConfigError("WorkerCount", c.WorkerCount, ErrInvalidWorkerCount)
	}
	if c.WorkerCount < MinWorkerCount {
		return NewConfigError("WorkerCount", c.WorkerCount, errors.New("worker count is below minimum limit"))
	}
	if c.WorkerCount > MaxWorkerCount {
		return NewConfigError("WorkerCount", c.WorkerCount, errors.New("worker count exceeds maximum limit"))
	}

	// 验证队列大小
	if c.QueueSize <= 0 {
		return NewConfigError("QueueSize", c.QueueSize, ErrInvalidQueueSize)
	}
	if c.QueueSize > MaxQueueSize {
		return NewConfigError("QueueSize", c.QueueSize, errors.New("queue size exceeds maximum limit"))
	}

	// 验证对象池大小
	if c.ObjectPoolSize < 0 {
		return NewConfigError("ObjectPoolSize", c.ObjectPoolSize, errors.New("object pool size cannot be negative"))
	}

	// 验证超时配置
	if c.TaskTimeout <= 0 {
		return NewConfigError("TaskTimeout", c.TaskTimeout, errors.New("task timeout must be positive"))
	}
	if c.TaskTimeout > 24*time.Hour {
		return NewConfigError("TaskTimeout", c.TaskTimeout, errors.New("task timeout is too large (max 24 hours)"))
	}

	if c.ShutdownTimeout <= 0 {
		return NewConfigError("ShutdownTimeout", c.ShutdownTimeout, errors.New("shutdown timeout must be positive"))
	}
	if c.ShutdownTimeout > time.Hour {
		return NewConfigError("ShutdownTimeout", c.ShutdownTimeout, errors.New("shutdown timeout is too large (max 1 hour)"))
	}

	// 验证监控配置
	if c.EnableMetrics && c.MetricsInterval <= 0 {
		return NewConfigError("MetricsInterval", c.MetricsInterval, errors.New("metrics interval must be positive when metrics are enabled"))
	}
	if c.MetricsInterval > time.Hour {
		return NewConfigError("MetricsInterval", c.MetricsInterval, errors.New("metrics interval is too large (max 1 hour)"))
	}

	// 验证配置合理性
	if c.QueueSize < c.WorkerCount {
		return NewConfigError("QueueSize", c.QueueSize, errors.New("queue size should be at least equal to worker count"))
	}

	return nil
}

// Copy 创建配置的深拷贝
func (c *Config) Copy() *Config {
	newConfig := *c
	return &newConfig
}

// String 返回配置的字符串表示
func (c *Config) String() string {
	return fmt.Sprintf("Config{WorkerCount:%d, QueueSize:%d, ObjectPoolSize:%d, PreAlloc:%v, TaskTimeout:%v, ShutdownTimeout:%v, EnableMetrics:%v, MetricsInterval:%v}",
		c.WorkerCount, c.QueueSize, c.ObjectPoolSize, c.PreAlloc, c.TaskTimeout, c.ShutdownTimeout, c.EnableMetrics, c.MetricsInterval)
}

// IsValid 检查配置是否有效
func (c *Config) IsValid() bool {
	return c.Validate() == nil
}

// WithDefaults 使用默认值填充未设置的字段
func (c *Config) WithDefaults() *Config {
	defaults := NewConfig()

	if c.WorkerCount <= 0 {
		c.WorkerCount = defaults.WorkerCount
	}
	if c.QueueSize <= 0 {
		c.QueueSize = defaults.QueueSize
	}
	if c.ObjectPoolSize < 0 {
		c.ObjectPoolSize = defaults.ObjectPoolSize
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = defaults.TaskTimeout
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = defaults.ShutdownTimeout
	}
	if c.MetricsInterval <= 0 {
		c.MetricsInterval = defaults.MetricsInterval
	}

	return c
}
