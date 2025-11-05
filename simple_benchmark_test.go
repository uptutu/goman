package main

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// SimpleTask for benchmarking
type SimpleTask struct {
	id       int64
	workload time.Duration
}

func (t *SimpleTask) Execute(ctx context.Context) (any, error) {
	if t.workload > 0 {
		time.Sleep(t.workload)
	}
	return fmt.Sprintf("task-%d-result", t.id), nil
}

func (t *SimpleTask) Priority() int {
	return pool.PriorityNormal
}

// BenchmarkPoolSubmit tests basic task submission performance
func BenchmarkPoolSubmit(b *testing.B) {
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(1000).
		WithMetrics(false).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	p, err := pool.NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		p.Shutdown(ctx)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var taskID int64
		for pb.Next() {
			id := atomic.AddInt64(&taskID, 1)
			task := &SimpleTask{id: id, workload: 0}
			p.Submit(task)
		}
	})
}

// BenchmarkPoolSubmitAsync tests async task submission performance
func BenchmarkPoolSubmitAsync(b *testing.B) {
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(8).
		WithQueueSize(1000).
		WithMetrics(false).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	p, err := pool.NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		p.Shutdown(ctx)
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var taskID int64
		for pb.Next() {
			id := atomic.AddInt64(&taskID, 1)
			task := &SimpleTask{id: id, workload: 0}
			future := p.SubmitAsync(task)
			future.Get() // Wait for completion
		}
	})
}

// BenchmarkPoolHighConcurrency tests high concurrency scenarios
func BenchmarkPoolHighConcurrency(b *testing.B) {
	config, err := pool.NewConfigBuilder().
		WithWorkerCount(runtime.NumCPU() * 2).
		WithQueueSize(10000).
		WithMetrics(false).
		Build()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	p, err := pool.NewPool(config)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		p.Shutdown(ctx)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		const numTasks = 1000
		for j := 0; j < numTasks; j++ {
			task := &SimpleTask{id: int64(j), workload: time.Microsecond}
			p.Submit(task)
		}
	}
}