package pool

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MiddlewareChain 中间件链
type MiddlewareChain struct {
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewMiddlewareChain 创建新的中间件链
func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]Middleware, 0),
	}
}

// Add 添加中间件
func (mc *MiddlewareChain) Add(middleware Middleware) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.middlewares = append(mc.middlewares, middleware)
}

// Remove 移除中间件
func (mc *MiddlewareChain) Remove(middleware Middleware) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	for i, m := range mc.middlewares {
		if m == middleware {
			mc.middlewares = append(mc.middlewares[:i], mc.middlewares[i+1:]...)
			break
		}
	}
}

// ExecuteBefore 执行所有中间件的Before方法
func (mc *MiddlewareChain) ExecuteBefore(ctx context.Context, task Task) context.Context {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	currentCtx := ctx
	for _, middleware := range mc.middlewares {
		currentCtx = middleware.Before(currentCtx, task)
	}
	return currentCtx
}

// ExecuteAfter 执行所有中间件的After方法（逆序执行）
func (mc *MiddlewareChain) ExecuteAfter(ctx context.Context, task Task, result any, err error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// 逆序执行After方法
	for i := len(mc.middlewares) - 1; i >= 0; i-- {
		mc.middlewares[i].After(ctx, task, result, err)
	}
}

// PluginManager 插件管理器
type PluginManager struct {
	plugins map[string]Plugin
	mu      sync.RWMutex
}

// NewPluginManager 创建新的插件管理器
func NewPluginManager() *PluginManager {
	return &PluginManager{
		plugins: make(map[string]Plugin),
	}
}

// Register 注册插件
func (pm *PluginManager) Register(plugin Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	name := plugin.Name()
	if _, exists := pm.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	pm.plugins[name] = plugin
	return nil
}

// Unregister 注销插件
func (pm *PluginManager) Unregister(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plugin, exists := pm.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// 停止插件
	if err := plugin.Stop(); err != nil {
		return fmt.Errorf("failed to stop plugin %s: %w", name, err)
	}

	delete(pm.plugins, name)
	return nil
}

// Get 获取插件
func (pm *PluginManager) Get(name string) (Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugin, exists := pm.plugins[name]
	return plugin, exists
}

// List 列出所有插件
func (pm *PluginManager) List() []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugins := make([]Plugin, 0, len(pm.plugins))
	for _, plugin := range pm.plugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// StartAll 启动所有插件
func (pm *PluginManager) StartAll() error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for name, plugin := range pm.plugins {
		if err := plugin.Start(); err != nil {
			return fmt.Errorf("failed to start plugin %s: %w", name, err)
		}
	}
	return nil
}

// StopAll 停止所有插件
func (pm *PluginManager) StopAll() error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var lastErr error
	for name, plugin := range pm.plugins {
		if err := plugin.Stop(); err != nil {
			lastErr = fmt.Errorf("failed to stop plugin %s: %w", name, err)
		}
	}
	return lastErr
}

// EventManager 事件管理器
type EventManager struct {
	listeners []EventListener
	mu        sync.RWMutex
}

// NewEventManager 创建新的事件管理器
func NewEventManager() *EventManager {
	return &EventManager{
		listeners: make([]EventListener, 0),
	}
}

// AddListener 添加事件监听器
func (em *EventManager) AddListener(listener EventListener) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.listeners = append(em.listeners, listener)
}

// RemoveListener 移除事件监听器
func (em *EventManager) RemoveListener(listener EventListener) {
	em.mu.Lock()
	defer em.mu.Unlock()
	for i, l := range em.listeners {
		if l == listener {
			em.listeners = append(em.listeners[:i], em.listeners[i+1:]...)
			break
		}
	}
}

// EmitPoolStart 触发协程池启动事件
func (em *EventManager) EmitPoolStart(pool Pool) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, listener := range em.listeners {
		go listener.OnPoolStart(pool)
	}
}

// EmitPoolShutdown 触发协程池关闭事件
func (em *EventManager) EmitPoolShutdown(pool Pool) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, listener := range em.listeners {
		go listener.OnPoolShutdown(pool)
	}
}

// EmitTaskSubmit 触发任务提交事件
func (em *EventManager) EmitTaskSubmit(task Task) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, listener := range em.listeners {
		go listener.OnTaskSubmit(task)
	}
}

// EmitTaskComplete 触发任务完成事件
func (em *EventManager) EmitTaskComplete(task Task, result any) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, listener := range em.listeners {
		go listener.OnTaskComplete(task, result)
	}
}

// EmitWorkerPanic 触发工作协程panic事件
func (em *EventManager) EmitWorkerPanic(workerID int, panicValue any) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, listener := range em.listeners {
		go listener.OnWorkerPanic(workerID, panicValue)
	}
}

// SchedulerPluginManager 调度器插件管理器
type SchedulerPluginManager struct {
	plugins []SchedulerPlugin
	current SchedulerPlugin
	mu      sync.RWMutex
}

// NewSchedulerPluginManager 创建新的调度器插件管理器
func NewSchedulerPluginManager() *SchedulerPluginManager {
	return &SchedulerPluginManager{
		plugins: make([]SchedulerPlugin, 0),
	}
}

// Register 注册调度器插件
func (spm *SchedulerPluginManager) Register(plugin SchedulerPlugin) {
	spm.mu.Lock()
	defer spm.mu.Unlock()

	spm.plugins = append(spm.plugins, plugin)
	// 按优先级排序，优先级高的在前
	sort.Slice(spm.plugins, func(i, j int) bool {
		return spm.plugins[i].Priority() > spm.plugins[j].Priority()
	})
}

// SetCurrent 设置当前使用的调度器插件
func (spm *SchedulerPluginManager) SetCurrent(plugin SchedulerPlugin) {
	spm.mu.Lock()
	defer spm.mu.Unlock()
	spm.current = plugin
}

// GetCurrent 获取当前使用的调度器插件
func (spm *SchedulerPluginManager) GetCurrent() SchedulerPlugin {
	spm.mu.RLock()
	defer spm.mu.RUnlock()
	return spm.current
}

// GetByName 根据名称获取调度器插件
func (spm *SchedulerPluginManager) GetByName(name string) (SchedulerPlugin, bool) {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	for _, plugin := range spm.plugins {
		if plugin.Name() == name {
			return plugin, true
		}
	}
	return nil, false
}

// List 列出所有调度器插件
func (spm *SchedulerPluginManager) List() []SchedulerPlugin {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	plugins := make([]SchedulerPlugin, len(spm.plugins))
	copy(plugins, spm.plugins)
	return plugins
}

// GetHighestPriority 获取优先级最高的调度器插件
func (spm *SchedulerPluginManager) GetHighestPriority() (SchedulerPlugin, bool) {
	spm.mu.RLock()
	defer spm.mu.RUnlock()

	if len(spm.plugins) == 0 {
		return nil, false
	}
	return spm.plugins[0], true
}

// ExtensionManager 扩展管理器，整合所有扩展功能
type ExtensionManager struct {
	middlewareChain        *MiddlewareChain
	pluginManager          *PluginManager
	eventManager           *EventManager
	schedulerPluginManager *SchedulerPluginManager
	pool                   Pool
}

// NewExtensionManager 创建新的扩展管理器
func NewExtensionManager() *ExtensionManager {
	return &ExtensionManager{
		middlewareChain:        NewMiddlewareChain(),
		pluginManager:          NewPluginManager(),
		eventManager:           NewEventManager(),
		schedulerPluginManager: NewSchedulerPluginManager(),
	}
}

// SetPool 设置协程池引用
func (em *ExtensionManager) SetPool(pool Pool) {
	em.pool = pool
}

// AddMiddleware 添加中间件
func (em *ExtensionManager) AddMiddleware(middleware Middleware) {
	em.middlewareChain.Add(middleware)
}

// RemoveMiddleware 移除中间件
func (em *ExtensionManager) RemoveMiddleware(middleware Middleware) {
	em.middlewareChain.Remove(middleware)
}

// RegisterPlugin 注册插件
func (em *ExtensionManager) RegisterPlugin(plugin Plugin) error {
	if err := em.pluginManager.Register(plugin); err != nil {
		return err
	}

	// 初始化插件
	if em.pool != nil {
		if err := plugin.Init(em.pool); err != nil {
			em.pluginManager.Unregister(plugin.Name())
			return fmt.Errorf("failed to initialize plugin %s: %w", plugin.Name(), err)
		}
	}

	return nil
}

// UnregisterPlugin 注销插件
func (em *ExtensionManager) UnregisterPlugin(name string) error {
	return em.pluginManager.Unregister(name)
}

// GetPlugin 获取插件
func (em *ExtensionManager) GetPlugin(name string) (Plugin, bool) {
	return em.pluginManager.Get(name)
}

// AddEventListener 添加事件监听器
func (em *ExtensionManager) AddEventListener(listener EventListener) {
	em.eventManager.AddListener(listener)
}

// RemoveEventListener 移除事件监听器
func (em *ExtensionManager) RemoveEventListener(listener EventListener) {
	em.eventManager.RemoveListener(listener)
}

// SetSchedulerPlugin 设置调度器插件
func (em *ExtensionManager) SetSchedulerPlugin(plugin SchedulerPlugin) {
	em.schedulerPluginManager.SetCurrent(plugin)
}

// GetSchedulerPlugin 获取调度器插件
func (em *ExtensionManager) GetSchedulerPlugin() SchedulerPlugin {
	return em.schedulerPluginManager.GetCurrent()
}

// ExecuteMiddlewareBefore 执行中间件Before方法
func (em *ExtensionManager) ExecuteMiddlewareBefore(ctx context.Context, task Task) context.Context {
	return em.middlewareChain.ExecuteBefore(ctx, task)
}

// ExecuteMiddlewareAfter 执行中间件After方法
func (em *ExtensionManager) ExecuteMiddlewareAfter(ctx context.Context, task Task, result any, err error) {
	em.middlewareChain.ExecuteAfter(ctx, task, result, err)
}

// EmitPoolStart 触发协程池启动事件
func (em *ExtensionManager) EmitPoolStart() {
	if em.pool != nil {
		em.eventManager.EmitPoolStart(em.pool)
	}
}

// EmitPoolShutdown 触发协程池关闭事件
func (em *ExtensionManager) EmitPoolShutdown() {
	if em.pool != nil {
		em.eventManager.EmitPoolShutdown(em.pool)
	}
}

// EmitTaskSubmit 触发任务提交事件
func (em *ExtensionManager) EmitTaskSubmit(task Task) {
	em.eventManager.EmitTaskSubmit(task)
}

// EmitTaskComplete 触发任务完成事件
func (em *ExtensionManager) EmitTaskComplete(task Task, result any) {
	em.eventManager.EmitTaskComplete(task, result)
}

// EmitWorkerPanic 触发工作协程panic事件
func (em *ExtensionManager) EmitWorkerPanic(workerID int, panicValue any) {
	em.eventManager.EmitWorkerPanic(workerID, panicValue)
}

// StartAllPlugins 启动所有插件
func (em *ExtensionManager) StartAllPlugins() error {
	return em.pluginManager.StartAll()
}

// StopAllPlugins 停止所有插件
func (em *ExtensionManager) StopAllPlugins() error {
	return em.pluginManager.StopAll()
}
