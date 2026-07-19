# AI 后端工程化・稳定性保障 - 开发文档

## 一、项目概述

本项目为第9周学习任务的实现，旨在为 AI 系统添加生产级高可用保障，打造区别于普通 AI 开发者的核心差异化优势。

### 核心目标

- 实现大模型接口故障场景的容错处理
- 构建重试、熔断、降级三级保障体系
- 实现全局限流与租户限流策略
- 搭建异步任务队列处理批量向量化
- 建立异常监控与日志埋点体系

---

## 二、新增文件清单

| 文件路径 | 功能说明 |
|---------|---------|
| `cmd/rag/stability/retry.go` | 指数退避重试机制 |
| `cmd/rag/stability/circuit_breaker.go` | 滑动窗口熔断器 |
| `cmd/rag/stability/fallback.go` | 降级兜底策略 |
| `cmd/rag/stability/rate_limit.go` | 多级限流管理器 |
| `cmd/rag/stability/task_queue.go` | 异步任务队列 |
| `cmd/rag/stability/logger.go` | 结构化日志与错误码体系 |
| `cmd/rag/stability/llm_wrapper.go` | LLM客户端包装器（集成三级保障） |
| `cmd/rag/stability/stability_test.go` | 单元测试 |
| `cmd/rag/stability/DEVELOPMENT_DOC.md` | 开发文档（本文档） |

---

## 三、核心功能模块

### 3.1 重试机制 (retry.go)

**设计思路**：基于指数退避策略实现，区分可重试错误与不可重试错误

**关键配置**：
- `MaxRetries`：最大重试次数
- `InitialDelay`：初始重试间隔
- `MaxDelay`：最大重试间隔（防止间隔无限增长）
- `Multiplier`：退避乘数

**可重试错误类型**：
- 超时 (timeout)
- 连接拒绝 (connection refused)
- 服务端错误 (500/502/503/504)
- 限流 (429/rate limit)

**核心流程**：
```
请求 → 执行 → 成功 → 返回结果
            ↓ 失败
        判断可重试?
            ↓ 是
        等待指数退避时间
            ↓
        重试（最多MaxRetries次）
            ↓ 否
        返回错误
```

### 3.2 熔断器 (circuit_breaker.go)

**设计思路**：基于滑动窗口实现，保护下游大模型服务不被持续请求拖垮

**三种状态**：
- **Closed（闭合）**：所有请求正常通过，收集成功率数据
- **Open（打开）**：错误率超过阈值，所有请求直接拒绝
- **HalfOpen（半开）**：打开状态持续一段时间后，允许少量请求探测

**关键配置**：
- `FailureThreshold`：错误率阈值（百分比）
- `WindowDuration`：滑动窗口时长
- `MinRequests`：窗口内最小请求数
- `SleepWindow`：打开状态持续时间
- `HalfOpenMaxRequests`：半开状态下最大请求数

**核心流程**：
```
Closed → 错误率>阈值 → Open → 等待SleepWindow → HalfOpen
   ↑                     ↓                          ↓
   └───────── 成功 ────────┘              失败→Open
                                          成功→Closed
```

### 3.3 降级策略 (fallback.go)

**设计思路**：主模型不可用时，自动切换到低成本备用模型

**触发条件**：
- 超时
- 5xx错误
- 连接拒绝
- 熔断器打开
- 限流

**核心流程**：
```
主模型调用 → 成功 → 返回结果
            ↓ 失败
        判断可降级?
            ↓ 是
        切换到备用模型
            ↓
        返回降级结果
```

### 3.4 多级限流 (rate_limit.go)

**设计思路**：基于令牌桶算法实现多级限流，保护系统稳定性

**限流层级**：
| 层级 | 限流方式 | 用途 |
|-----|---------|-----|
| 全局 | QPS + 并发数 | 保护下游大模型接口 |
| 用户 | QPS + 每日次数 | 防止单个用户滥用 |
| IP | QPS + 并发数 | 防止恶意高频请求 |

**关键配置**：
- `GlobalMaxQPS`：全局限流 QPS
- `GlobalMaxConcurrent`：全局最大并发数
- `PerUserMaxQPS`：单用户限流 QPS
- `PerUserMaxDaily`：单用户每日最大调用次数
- `PerIPMaxQPS`：单IP限流 QPS
- `PerIPMaxConcurrent`：单IP最大并发数

### 3.5 异步任务队列 (task_queue.go)

**设计思路**：处理文档批量向量化等耗时任务，支持任务状态查询、失败重试、死信队列

**任务状态**：
- `pending`：等待中
- `running`：运行中
- `completed`：已完成
- `failed`：失败
- `retrying`：重试中
- `dead_letter`：死信队列

**关键配置**：
- `WorkerCount`：工作线程数
- `MaxRetries`：最大重试次数
- `RetryDelay`：重试间隔
- `QueueCapacity`：队列容量
- `DeadLetterEnabled`：是否启用死信队列

**核心流程**：
```
提交任务 → 入队 → 等待 → Worker执行 → 成功→completed
                                          ↓ 失败
                                      重试<MaxRetries?
                                          ↓ 是
                                      等待RetryDelay后重试
                                          ↓ 否
                                      DeadLetterEnabled?
                                          ↓ 是          ↓ 否
                                      入死信队列      failed
```

### 3.6 结构化日志与错误码 (logger.go)

**设计思路**：全链路结构化日志，包含请求ID、用户ID、耗时、Token消耗、错误码

**日志级别**：
- `debug`：调试信息
- `info`：一般运行时信息
- `warn`：警告信息
- `error`：错误信息

**错误码体系**：

| 分类 | 错误码 | HTTP状态码 | 说明 |
|-----|-------|-----------|------|
| 客户端 | `BAD_REQUEST` | 400 | 请求参数错误 |
| 客户端 | `UNAUTHORIZED` | 401 | 未授权 |
| 客户端 | `FORBIDDEN` | 403 | 禁止访问 |
| 客户端 | `NOT_FOUND` | 404 | 资源未找到 |
| 客户端 | `RATE_LIMITED` | 429 | 限流 |
| 客户端 | `VALIDATION_FAILED` | 400 | 校验失败 |
| 模型 | `MODEL_TIMEOUT` | 504 | 模型超时 |
| 模型 | `MODEL_ERROR` | 500 | 模型错误 |
| 模型 | `MODEL_RATE_LIMIT` | 429 | 模型限流 |
| 模型 | `MODEL_QUOTA_EXCEED` | 429 | 配额超限 |
| 模型 | `MODEL_CONTENT_BLOCK` | 400 | 内容审核拦截 |
| 系统 | `INTERNAL_ERROR` | 500 | 内部错误 |
| 系统 | `CIRCUIT_OPEN` | 503 | 熔断器打开 |
| 系统 | `SERVICE_UNAVAILABLE` | 503 | 服务不可用 |
| 系统 | `DATABASE_ERROR` | 500 | 数据库错误 |
| 系统 | `NETWORK_ERROR` | 500 | 网络错误 |

**指标记录**：
- 调用量计数器
- 延迟统计（平均/最小/最大）
- 错误率分布

### 3.7 LLM客户端包装器 (llm_wrapper.go)

**设计思路**：将重试、熔断、降级三级保障机制集成到LLM客户端调用中

**集成架构**：
```
用户请求
    ↓
┌───────────────────────────┐
│     Circuit Breaker       │ ← 第一层：熔断保护
│   (熔断器拦截异常流量)      │
└───────────┬───────────────┘
            ↓
┌───────────────────────────┐
│        Retryer            │ ← 第二层：重试机制
│   (指数退避重试可重试错误)   │
└───────────┬───────────────┘
            ↓
┌───────────────────────────┐
│       LLM Client          │ ← 第三层：原始调用
└───────────┬───────────────┘
            ↓ 失败
┌───────────────────────────┐
│       Fallback            │ ← 第四层：降级兜底
│   (切换到备用模型)          │
└───────────────────────────┘
```

---

## 四、配置示例

```go
// 重试配置
retryConfig := stability.RetryConfig{
    MaxRetries:   3,
    InitialDelay: 100 * time.Millisecond,
    MaxDelay:     5 * time.Second,
    Multiplier:   2,
}

// 熔断器配置
circuitConfig := stability.CircuitBreakerConfig{
    Enabled:             true,
    FailureThreshold:    50,
    WindowDuration:      30 * time.Second,
    MinRequests:         10,
    SleepWindow:         60 * time.Second,
    HalfOpenMaxRequests: 5,
}

// 降级配置
fallbackConfig := stability.FallbackConfig{
    Enabled:          true,
    FallbackProvider: "ollama",
    FallbackModel:    "qwen2:0.5b",
}

// 限流配置
rateLimitConfig := stability.RateLimitConfig{
    GlobalMaxQPS:        100,
    GlobalMaxConcurrent: 50,
    PerUserMaxQPS:       10,
    PerUserMaxDaily:     1000,
    PerIPMaxQPS:         20,
    PerIPMaxConcurrent:  10,
}

// 任务队列配置
taskQueueConfig := stability.TaskQueueConfig{
    Enabled:           true,
    WorkerCount:       4,
    MaxRetries:        3,
    RetryDelay:        5 * time.Second,
    QueueCapacity:     1000,
    DeadLetterEnabled: true,
}
```

---

## 五、错误处理流程图

```
                    ┌──────────────────────┐
                    │    用户发起请求       │
                    └──────────┬───────────┘
                               ↓
                    ┌──────────────────────┐
                    │    多级限流检查       │
                    │ (全局/用户/IP)        │
                    └──────────┬───────────┘
                               ↓
                      限流通过? ──否──→ 返回429
                               ↓是
                    ┌──────────────────────┐
                    │     熔断器检查        │
                    └──────────┬───────────┘
                               ↓
                      熔断器打开? ──是──→ 返回503
                               ↓否
                    ┌──────────────────────┐
                    │    LLM请求执行        │
                    │  (含指数退避重试)      │
                    └──────────┬───────────┘
                               ↓
                         请求成功?
                    ┌──────────┴──────────┐
                    ↓是                   ↓否
            ┌──────────────┐    ┌──────────────────┐
            │ 记录成功日志  │    │  判断可降级?      │
            │ 更新指标     │    └────────┬─────────┘
            └──────────────┘             ↓
                                    可降级?
                              ┌────────┴────────┐
                              ↓是               ↓否
                    ┌────────────────┐   ┌─────────────────┐
                    │ 切换降级模型    │   │ 记录错误日志     │
                    │ 执行降级请求    │   │ 更新错误指标     │
                    └────────┬────────┘   └─────────────────┘
                             ↓
                    ┌────────────────┐
                    │ 降级请求成功?   │
              ┌─────┴─────┐           │
              ↓是         ↓否          │
        ┌──────────┐ ┌──────────┐    │
        │返回结果  │ │返回错误  │    │
        └──────────┘ └──────────┘    │
                                      │
                    ┌─────────────────┘
                    ↓
              ┌──────────────┐
              │ 记录请求日志  │
              │ (耗时/Token)  │
              └──────────────┘
```

---

## 六、API 端点

### 6.1 健康检查

```
GET /health

响应：
{
  "status": "healthy",
  "circuit_state": "closed",
  "in_fallback": false
}
```

### 6.2 指标查询

```
GET /metrics

响应：
{
  "llm.call.success": 1000,
  "llm.call.avg": 500.5,
  "llm.call.min": 100,
  "llm.call.max": 2000,
  "circuit.requests": 1000,
  "circuit.failures": 10,
  "circuit.state": "closed",
  "fallback.active": false
}
```

### 6.3 任务状态查询

```
GET /tasks/{taskID}

响应：
{
  "id": "uuid-string",
  "type": "vectorize",
  "status": "completed",
  "progress": 100,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:01:00Z"
}
```

---

## 七、测试说明

### 7.1 单元测试

运行测试命令：
```bash
cd cmd/rag/stability
go test -v ./...
```

### 7.2 测试覆盖范围

| 测试文件 | 测试内容 |
|---------|---------|
| `stability_test.go` | 重试器、熔断器、限流器、任务队列、降级处理器的单元测试 |

### 7.3 测试用例

- **重试器测试**：验证指数退避重试逻辑、可重试错误判断
- **熔断器测试**：验证三种状态切换、滑动窗口计算、半开探测
- **限流器测试**：验证令牌桶算法、多级限流策略
- **任务队列测试**：验证任务提交、执行、重试、死信队列
- **降级测试**：验证主模型失败时自动切换到备用模型

---

## 八、运行要求

### 8.1 依赖

- Go 1.21+
- 依赖包：`github.com/google/uuid`

### 8.2 启动方式

```bash
cd cmd/rag
go run main.go
```

### 8.3 配置文件

配置文件路径：`config/config.yaml`

```yaml
stability:
  retry:
    max_retries: 3
    initial_delay_ms: 100
    max_delay_ms: 5000
    multiplier: 2
  
  circuit_breaker:
    enabled: true
    failure_threshold: 50
    window_duration_s: 30
    min_requests: 10
    sleep_window_s: 60
    half_open_max_requests: 5
  
  fallback:
    enabled: true
    fallback_provider: ollama
    fallback_model: qwen2:0.5b
  
  rate_limit:
    global_max_qps: 100
    global_max_concurrent: 50
    per_user_max_qps: 10
    per_user_max_daily: 1000
    per_ip_max_qps: 20
    per_ip_max_concurrent: 10
  
  task_queue:
    enabled: true
    worker_count: 4
    max_retries: 3
    retry_delay_ms: 5000
    queue_capacity: 1000
    dead_letter_enabled: true
```

---

## 九、验收标准

1. **容错机制验证**：模拟大模型接口超时/报错时，服务能自动重试或降级返回，不会整体崩溃
2. **限流验证**：超出限流阈值的请求会被平滑拦截，不会穿透到下游模型服务
3. **并发支持**：单实例稳定支持 500 并发对话请求，无内存泄漏与连接耗尽问题
4. **异步任务**：批量文档向量化任务能正确入队、执行、重试
5. **监控可观测**：全链路日志包含请求ID、用户ID、耗时、Token消耗等关键信息

---

## 十、技术亮点

1. **三级保障体系**：重试 → 熔断 → 降级，层层递进的容错策略
2. **多级限流**：全局、用户、IP三级限流，全面保护系统稳定性
3. **异步解耦**：批量向量化任务异步处理，提高系统吞吐量
4. **结构化日志**：统一的日志格式和错误码体系，便于问题排查
5. **指标监控**：关键指标埋点，支持实时监控和告警
6. **优雅关闭**：HTTP服务器和任务队列协同关闭，确保正在处理的请求完成
