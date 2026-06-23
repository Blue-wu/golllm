# Go 语言学习笔记

## 1. 基础概念

### 1.1 Goroutine

Goroutine 是 Go 语言中的轻量级线程，由 Go 运行时管理。与传统线程相比，Goroutine 的创建和切换开销非常小，可以同时运行成千上万个 Goroutine。

创建 Goroutine 非常简单，只需在函数调用前加上 go 关键字：

```go
go func() {
    fmt.Println("Hello from goroutine")
}()
```

### 1.2 Channel

Channel 是 Goroutine 之间的通信机制。通过 Channel，Goroutine 可以安全地传递数据，避免共享内存带来的并发问题。

```go
ch := make(chan int)
go func() {
    ch <- 42
}()
val := <-ch
```

## 2. 并发模式

### 2.1 Worker Pool

Worker Pool 模式用于控制并发任务的数量，避免创建过多的 Goroutine。

```go
func worker(id int, jobs <-chan int, results chan<- int) {
    for j := range jobs {
        fmt.Printf("worker %d processing job %d\n", id, j)
        results <- j * 2
    }
}
```

### 2.2 Fan-Out Fan-In

Fan-Out 模式是指一个任务分成多个子任务并行处理，Fan-In 模式是指将多个子任务的结果汇总。

## 3. 错误处理

Go 语言采用显式错误处理方式，通过返回值传递错误。

```go
func doSomething() error {
    // 执行操作
    if err != nil {
        return fmt.Errorf("failed to do something: %w", err)
    }
    return nil
}
```

## 4. 模块化

Go 语言使用包（package）来组织代码，每个包可以包含多个源文件。

```go
package main

import (
    "fmt"
    "github.com/example/utils"
)
```

## 5. 测试

Go 语言内置了测试框架，测试文件以 `_test.go` 结尾。

```go
func TestAdd(t *testing.T) {
    result := Add(2, 3)
    if result != 5 {
        t.Errorf("Expected 5, got %d", result)
    }
}
```