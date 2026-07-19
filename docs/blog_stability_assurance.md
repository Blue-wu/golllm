# 【实战】给 AI 系统加上生产级高可用保障

> 本文是 AI 后端工程化系列的第 9 周学习总结，记录了如何从零开始构建一套完整的大模型服务稳定性保障体系。

---

## 一、引言：为什么稳定性保障如此重要？

在 AI 应用开发中，我们常常会遇到这样的场景：

> "代码本地调试一切正常，部署到生产环境后却频繁报错"
> "大模型接口偶尔超时，导致整个服务雪崩"
> "用户并发量一上来，系统就变得不可用"

这些问题的根源在于：**我们的系统缺乏生产级的稳定性保障机制**。

作为一名 AI 后端工程师，区别于普通 AI 开发者的核心竞争力之一，就是能否为 AI 系统构建一套完整的高可用保障体系。

---

## 二、痛点分析：大模型接口的 "坑"

在接入大模型 API 的过程中，我们会遇到各种各样的故障场景，我将其总结为 **5 大类**：

| 故障类型 | 典型表现 | 是否可重试 |
|---------|---------|-----------|
| **超时无响应** | 请求长时间挂起，最终超时 | ✅ |
| **限流 429** | 接口返回 "Too Many Requests" | ✅（等待后重试） |
| **服务端 5xx** | 500/502/503/504 错误 | ✅ |
| **Token 超限** | 提示 "max tokens exceeded" | ❌（需要调整参数） |
| **内容审核拦截** | 返回 "content policy violation" | ❌ |

这些故障如果不妥善处理，会导致：
- 用户体验下降
- 系统资源浪费
- 服务雪崩效应

---

## 三、方案设计：三级保障体系

针对上述问题，我设计了一套 **重试 → 熔断 → 降级** 的三级保障体系：

```
┌─────────────────────────────────────────────────────────────┐
│                     用户请求入口                              │
└─────────────────────────────┬───────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  第一层：限流保护                                             │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐                  │
│  │ 全局限流   │ │ 用户限流   │ │ IP限流    │                  │
│  └───────────┘ └───────────┘ └───────────┘                  │
│     令牌桶算法，防止流量洪峰                                   │
└─────────────────────────────┬───────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  第二层：熔断保护                                             │
│  ┌─────────────────────────────────────────────┐            │
│  │   Closed → Open → HalfOpen → Closed          │            │
│  │   (正常)   (熔断)   (探测)    (恢复)         │            │
│  └─────────────────────────────────────────────┘            │
│     滑动窗口计算错误率，自动熔断保护下游服务                      │
└─────────────────────────────┬───────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  第三层：重试机制                                             │
│  ┌─────────────────────────────────────────────┐            │
│  │   指数退避策略：100ms → 200ms → 400ms → ...  │            │
│  │   区分可重试错误与不可重试错误                      │            │
│  └─────────────────────────────────────────────┘            │
│     自动重试可恢复的错误，提高成功率                            │
└─────────────────────────────┬───────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  第四层：降级兜底                                             │
│  ┌─────────────────────────────────────────────┐            │
│  │   主模型失败 → 自动切换到低成本备用模型            │            │
│  │   如：GPT-4 → Qwen2:0.5B                      │            │
│  └─────────────────────────────────────────────┘            │
│     保证服务可用性的最后一道防线                                │
└─────────────────────────────┬───────────────────────────────┘
                              ↓
                    ┌─────────────────┐
                    │   返回结果       │
                    └─────────────────┘
```

---

## 四、核心实现：关键代码解析

### 4.1 指数退避重试

```go
func (r *Retryer) Execute(ctx context.Context, fn RetryableFunc) (interface{}, error) {
    var lastErr error
    delay := r.config.InitialDelay

    for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
        result, err := fn(ctx)
        if err == nil {
            return result, nil
        }

        lastErr = err
        if !r.isRetryable(err) {
            return nil, err  // 不可重试错误，直接返回
        }

        if attempt >= r.config.MaxRetries {
            break
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()  // 上下文取消，立即退出
        case <-time.After(delay):
        }

        // 指数退避：delay = min(delay * multiplier, maxDelay)
        delay = time.Duration(math.Min(
            float64(delay)*float64(r.config.Multiplier), 
            float64(r.config.MaxDelay)))
    }

    return nil, lastErr
}
```

**设计要点**：
- 使用 `time.After` 实现非阻塞等待，同时监听 `ctx.Done()` 实现快速退出
- 指数退避公式：`delay = min(delay * multiplier, maxDelay)`，防止间隔无限增长
- 区分可重试错误与不可重试错误，避免无效重试

### 4.2 滑动窗口熔断器

```go
func (cb *CircuitBreaker) checkThreshold() {
    requestCount := len(cb.window)
    if requestCount < cb.config.MinRequests {
        return  // 请求数不足，不计算错误率
    }

    failureCount := 0
    for _, record := range cb.window {
        if record.failed {
            failureCount++
        }
    }

    failureRate := (failureCount * 100) / requestCount
    if failureRate >= cb.config.FailureThreshold {
        cb.state = StateOpen
        cb.lastStateChange = time.Now()
    }
}
```

**设计要点**：
- 滑动窗口自动清理过期记录，保证统计数据的时效性
- 设置最小请求数阈值，避免小样本导致误判
- 半开状态允许少量探测请求，验证下游服务是否恢复

### 4.3 LLM 客户端包装器

```go
func (w *LLMClientWrapper) Chat(ctx context.Context, messages []llm.Message) (string, error) {
    startTime := time.Now()
    var result string
    var err error

    // 第一层：熔断器保护
    _, err = w.circuitBreaker.Execute(ctx, func(ctx context.Context) (interface{}, error) {
        // 第二层：重试机制
        innerResult, innerErr := w.retryer.Execute(ctx, func(ctx context.Context) (interface{}, error) {
            // 第三层：原始调用
            return w.client.Chat(ctx, messages)
        })
        if innerErr != nil {
            return nil, innerErr
        }
        result = innerResult.(string)
        return result, nil
    })

    // 第四层：降级兜底
    if err != nil {
        if errors.Is(err, ErrCircuitOpen) {
            w.logger.Warn(ctx, "circuit breaker is open, attempting fallback")
        }
        result, err = w.fallback.Execute(ctx, messages, func(ctx context.Context) (string, error) {
            return w.client.Chat(ctx, messages)
        })
    }

    // 记录指标
    latency := time.Since(startTime).Milliseconds()
    w.metrics.RecordLatency("llm.call", float64(latency))

    return result, err
}
```

**设计要点**：
- 采用装饰器模式，将三级保障机制透明地集成到 LLM 调用中
- 统一的指标记录和日志输出，便于监控和排查问题
- 实现 `llm.Client` 接口，可以无缝替换原有客户端

---

## 五、踩坑经验：那些让我头秃的问题

### 5.1 重试次数的边界问题

**问题**：设置 `MaxRetries=2`，预期有 3 次尝试（1次初始 + 2次重试），实际只有 2 次。

**原因**：判断条件写成了 `>=` 而不是 `>`：

```go
// 错误写法
if task.RetryCount >= tq.config.MaxRetries {
    return
}

// 正确写法
if task.RetryCount > tq.config.MaxRetries {
    return
}
```

### 5.2 接口实现的陷阱

**问题**：`LLMClientWrapper` 无法作为 `llm.Client` 使用，编译报错。

**原因**：没有实现接口的所有方法。`llm.Client` 接口包含三个方法：

```go
type Client interface {
    Chat(ctx context.Context, messages []Message) (string, error)
    ChatStream(ctx context.Context, messages []Message, callback func(string)) error
    GetModel() string
}
```

我只实现了 `Chat` 方法，漏掉了 `ChatStream` 和 `GetModel`。

**解决方案**：补充实现所有接口方法。

### 5.3 并发安全问题

**问题**：多个 goroutine 同时修改共享状态，导致数据竞争。

**原因**：没有正确使用锁保护共享资源。

**解决方案**：为所有共享状态添加读写锁：

```go
type CircuitBreaker struct {
    state           CircuitState
    window          []requestRecord
    mu              sync.RWMutex  // 读写锁
}
```

---

## 六、扩展：异步任务队列

除了实时请求的保障，我们还需要处理批量任务（如文档向量化）。为此，我实现了一个异步任务队列：

```go
func (tq *TaskQueue) executeTask(task *Task) {
    tq.updateStatus(task.ID, StatusRunning)
    handler, ok := tq.handlers[task.Type]
    if !ok {
        tq.updateStatus(task.ID, StatusFailed)
        return
    }

    for {
        err := handler(context.Background(), task)
        if err == nil {
            tq.updateStatus(task.ID, StatusCompleted)
            tq.setProgress(task.ID, 100)
            return
        }

        task.RetryCount++
        if task.RetryCount > tq.config.MaxRetries {
            if tq.config.DeadLetterEnabled {
                tq.updateStatus(task.ID, StatusDeadLetter)
                tq.deadLetter <- task  // 进入死信队列
            } else {
                tq.updateStatus(task.ID, StatusFailed)
            }
            return
        }

        tq.updateStatus(task.ID, StatusRetrying)
        time.Sleep(tq.config.RetryDelay)
    }
}
```

**特性**：
- 支持任务状态查询（pending/running/completed/failed/retrying/dead_letter）
- 失败自动重试，超过阈值进入死信队列
- 可配置工作线程数，控制并发度

---

## 七、监控体系：看得见的稳定性

### 7.1 结构化日志

```json
{
    "timestamp": "2024-01-01T00:00:00Z",
    "level": "info",
    "request_id": "req-12345",
    "user_id": "user-001",
    "latency_ms": 500.5,
    "token_usage": {
        "prompt_tokens": 100,
        "completion_tokens": 50,
        "total_tokens": 150
    },
    "message": "LLM call successful"
}
```

### 7.2 错误码体系

| 分类 | 错误码 | 说明 |
|-----|-------|------|
| 客户端 | `BAD_REQUEST` | 请求参数错误 |
| 客户端 | `RATE_LIMITED` | 限流 |
| 模型 | `MODEL_TIMEOUT` | 模型超时 |
| 模型 | `MODEL_RATE_LIMIT` | 模型限流 |
| 系统 | `CIRCUIT_OPEN` | 熔断器打开 |
| 系统 | `INTERNAL_ERROR` | 内部错误 |

---

## 八、总结与展望

### 8.1 已完成的功能

✅ **重试机制**：指数退避策略，区分可重试/不可重试错误  
✅ **熔断器**：滑动窗口实现，三种状态自动切换  
✅ **降级策略**：主模型失败自动切换备用模型  
✅ **多级限流**：全局/用户/IP 三级限流保护  
✅ **异步任务队列**：支持任务状态查询、失败重试、死信队列  
✅ **结构化日志**：统一的日志格式和错误码体系  
✅ **指标监控**：关键指标埋点，支持实时监控  

### 8.2 未来改进方向

1. **分布式限流**：当前实现为单机限流，后续可引入 Redis 实现分布式限流
2. **熔断状态持久化**：当前熔断器状态仅存于内存，重启后丢失，后续可持久化到 Redis
3. **动态配置**：支持运行时动态调整配置参数，无需重启服务
4. **全链路追踪**：集成 OpenTelemetry，实现端到端的请求追踪
5. **告警机制**：基于指标配置告警规则，及时发现问题

### 8.3 核心收获

通过这一周的学习和实践，我深刻体会到：

> **稳定性不是靠测试出来的，而是靠设计出来的。**

在 AI 系统中，由于大模型接口的不确定性，稳定性保障尤为重要。一套完善的容错体系，可以让系统在面对各种异常情况时依然保持可用，这才是生产级 AI 系统的核心竞争力。

---

## 附录：项目结构

```
cmd/rag/stability/
├── retry.go           # 重试机制
├── circuit_breaker.go # 熔断器
├── fallback.go        # 降级策略
├── rate_limit.go      # 多级限流
├── task_queue.go      # 异步任务队列
├── logger.go          # 结构化日志与错误码
├── llm_wrapper.go     # LLM客户端包装器
└── stability_test.go  # 单元测试
```

---

**参考链接**：
- [开发文档](file:///C:/Users/blue/Documents/trae_projects/golllm/cmd/rag/stability/DEVELOPMENT_DOC.md)
- [学习路线](file:///C:/Users/blue/Documents/trae_projects/golllm/学习路线.md)

---

*本文是 AI 后端工程化系列的第 9 周学习总结，如果你有任何问题或建议，欢迎留言讨论！*

---

**标签**：#AI后端 #稳定性保障 #重试 #熔断 #降级 #Go语言
