package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/uptutu/goman/pkg/pool"
)

// ExampleTask 示例任务实现
type ExampleTask struct {
	id   int
	data string
}

// Execute 实现Task接口
func (t *ExampleTask) Execute(ctx context.Context) (interface{}, error) {
	// 模拟任务执行
	time.Sleep(100 * time.Millisecond)
	return fmt.Sprintf("Task %d processed: %s", t.id, t.data), nil
}

// Priority 返回任务优先级
func (t *ExampleTask) Priority() int {
	return pool.PriorityNormal
}

func main() {
	// 这是一个基础使用示例，实际的Pool实现将在后续任务中完成
	fmt.Println("Goroutine Pool SDK - Basic Usage Example")
	fmt.Println("This example demonstrates the basic structure and interfaces.")

	// 创建示例任务
	task := &ExampleTask{
		id:   1,
		data: "example data",
	}

	// 执行任务（直接调用，因为Pool实现还未完成）
	ctx := context.Background()
	result, err := task.Execute(ctx)
	if err != nil {
		log.Printf("Task execution failed: %v", err)
		return
	}

	fmt.Printf("Task result: %v\n", result)
	fmt.Printf("Task priority: %d\n", task.Priority())
}
