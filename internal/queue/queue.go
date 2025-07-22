package queue

import (
	"sync/atomic"
	"unsafe"
)

// LockFreeQueue 无锁任务队列
// 使用环形缓冲区和原子操作实现高性能的并发队列
type LockFreeQueue struct {
	buffer   []unsafe.Pointer // 存储任务指针的环形缓冲区
	mask     uint64           // 用于快速取模的掩码 (capacity - 1)
	capacity uint64           // 队列容量，必须是2的幂
	head     uint64           // 队列头部位置，使用原子操作
	tail     uint64           // 队列尾部位置，使用原子操作
}

// NewLockFreeQueue 创建新的无锁队列
// capacity 必须是2的幂，如果不是会自动调整到最近的2的幂
func NewLockFreeQueue(capacity uint64) *LockFreeQueue {
	// 确保容量是2的幂
	if capacity == 0 {
		capacity = 16 // 默认容量
	}

	// 调整到最近的2的幂
	capacity = nextPowerOfTwo(capacity)

	return &LockFreeQueue{
		buffer:   make([]unsafe.Pointer, capacity),
		mask:     capacity - 1,
		capacity: capacity,
		head:     0,
		tail:     0,
	}
}

// Enqueue 入队操作
// 返回true表示成功，false表示队列已满
func (q *LockFreeQueue) Enqueue(item unsafe.Pointer) bool {
	if item == nil {
		return false
	}

	for {
		tail := atomic.LoadUint64(&q.tail)
		head := atomic.LoadUint64(&q.head)

		// 检查队列是否已满
		// 当tail追上head时队列满（保留一个空位以区分满和空）
		if (tail+1)&q.mask == head&q.mask {
			return false // 队列已满
		}

		// 尝试获取tail位置的所有权
		if atomic.CompareAndSwapUint64(&q.tail, tail, tail+1) {
			// 成功获取位置，写入数据
			index := tail & q.mask
			atomic.StorePointer(&q.buffer[index], item)
			return true
		}
		// CAS失败，重试
	}
}

// Dequeue 出队操作
// 返回队列中的元素，如果队列为空返回nil
func (q *LockFreeQueue) Dequeue() unsafe.Pointer {
	for {
		head := atomic.LoadUint64(&q.head)
		tail := atomic.LoadUint64(&q.tail)

		// 检查队列是否为空
		if head == tail {
			return nil // 队列为空
		}

		index := head & q.mask
		item := atomic.LoadPointer(&q.buffer[index])

		// 如果数据还没有被写入，重新检查队列状态
		if item == nil {
			// 重新检查head和tail，可能队列状态已经改变
			newHead := atomic.LoadUint64(&q.head)
			newTail := atomic.LoadUint64(&q.tail)
			if newHead == newTail {
				return nil // 队列确实为空
			}
			// 队列不为空但数据还没写入，继续重试
			continue
		}

		// 尝试移动head指针
		if atomic.CompareAndSwapUint64(&q.head, head, head+1) {
			// 成功移动head，清空该位置
			atomic.StorePointer(&q.buffer[index], nil)
			return item
		}
		// CAS失败，重试
	}
}

// IsEmpty 检查队列是否为空
func (q *LockFreeQueue) IsEmpty() bool {
	head := atomic.LoadUint64(&q.head)
	tail := atomic.LoadUint64(&q.tail)
	return head == tail
}

// IsFull 检查队列是否已满
func (q *LockFreeQueue) IsFull() bool {
	head := atomic.LoadUint64(&q.head)
	tail := atomic.LoadUint64(&q.tail)
	return (tail+1)&q.mask == head&q.mask
}

// Size 获取队列当前大小
func (q *LockFreeQueue) Size() uint64 {
	tail := atomic.LoadUint64(&q.tail)
	head := atomic.LoadUint64(&q.head)
	return (tail - head) & q.mask
}

// Capacity 获取队列容量
func (q *LockFreeQueue) Capacity() uint64 {
	return q.capacity - 1 // 实际可用容量（保留一个空位）
}

// nextPowerOfTwo 计算大于等于n的最小2的幂
func nextPowerOfTwo(n uint64) uint64 {
	if n == 0 {
		return 1
	}

	// 如果已经是2的幂，直接返回
	if n&(n-1) == 0 {
		return n
	}

	// 找到最高位
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++

	return n
}
