# RAG 知识库系统测试指南

## 系统概述

本系统是一个基于本地大模型的 RAG（检索增强生成）知识库问答系统，支持：
- 文档上传与自动切片
- 向量化存储与检索
- 多轮对话上下文管理
- 本地模型完全离线运行

## 前置条件

### 1. 安装 Ollama

```bash
# Windows: 从 https://ollama.ai 下载安装
# 或使用 winget
winget install Ollama.Ollama
```

### 2. 拉取模型

```bash
# Embedding 模型（用于文档向量化）
ollama pull nomic-embed-text

# LLM 模型（用于问答生成）
ollama pull qwen2.5:7b
```

### 3. 配置文件

复制 `config.example.toml` 为 `config.toml`：

```toml
# 服务器配置
[server]
host = "0.0.0.0"
port = 8080

# 向量数据库配置
[vector]
type = "memory"           # 使用内存存储（可改为 milvus）
collection = "documents"
dim = 768                 # nomic-embed-text 向量维度

# Embedding 模型配置
[embedding]
provider = "ollama"
model = "nomic-embed-text"

# LLM 模型配置
[llm]
provider = "ollama"
model = "qwen2.5:7b"

# RAG 配置
[rag]
topk = 5                  # 检索返回的文档数量
min_score = 0.6           # 最小相似度阈值
```

## 启动服务

```bash
cd cmd/rag
go build -o rag-server.exe .
./rag-server.exe
```

启动成功日志：
```
RAG 知识库系统启动中...
Embedding: ollama (nomic-embed-text)
LLM: ollama (qwen2.5:7b)
服务器启动成功: http://0.0.0.0:8080
```

## API 接口文档

### 1. 健康检查

**GET** `/api/v1/health`

检查服务状态。

**响应示例**：
```json
{
  "status": "ok",
  "version": "1.1",
  "model": "qwen2.5:7b"
}
```

**测试命令**：
```bash
curl http://localhost:8080/api/v1/health
```

---

### 2. 文档上传

**POST** `/api/v1/documents/upload`

上传文档并自动切片、向量化。

**请求参数**：
- `file`: 文档文件（支持 .md, .txt）
- `title`: 文档标题（可选）

**响应示例**：
```json
{
  "code": 0,
  "data": {
    "document_id": 1,
    "title": "test_rag",
    "chunks": 0,
    "message": "文档上传成功，正在后台处理向量化..."
  },
  "msg": "上传成功"
}
```

**测试命令**：
```bash
curl -X POST http://localhost:8080/api/v1/documents/upload \
  -F "file=@test.md"
```

---

### 3. 文档列表

**GET** `/api/v1/documents/list`

获取已上传的文档列表。

**响应示例**：
```json
{
  "code": 0,
  "data": {
    "total": 1,
    "page": 1,
    "page_size": 10,
    "documents": [
      {
        "id": 1,
        "title": "test_rag",
        "file_name": "test.md",
        "chunk_count": 9,
        "status": "completed"
      }
    ]
  }
}
```

---

### 4. 文档详情

**GET** `/api/v1/documents/:id`

获取文档详情及切片信息。

---

### 5. 问答接口

**POST** `/api/v1/chat/ask`

进行 RAG 问答，支持多轮对话。

**请求参数**：
```json
{
  "question": "Go语言的特性有哪些？",
  "session_id": "可选，用于多轮对话"
}
```

**响应示例**：
```json
{
  "code": 0,
  "data": {
    "session_id": "session_1782470256425279100",
    "question": "Go语言的特性有哪些？",
    "answer": "Go是一种编程语言，它具有goroutine和channel等特性...",
    "references": [
      {
        "doc_id": 1,
        "content": "Go 语言原生支持并发编程...",
        "score": 0.58,
        "file_name": "test.md"
      }
    ],
    "cost": 0
  }
}
```

---

### 6. 会话历史

**GET** `/api/v1/chat/history/:session_id`

获取指定会话的对话历史。

---

## 测试流程

### Step 1: 准备测试文档

创建测试文档 `test_rag.md`：

```markdown
# Go 语言入门指南

## 1. Go 语言简介
Go 是一个开源的编程语言，能让构造简单、可靠且高效的软件变得容易。

## 2. Go 语言特性

### 2.1 简洁性
Go 语言简洁明了，语法简单，易于学习和使用。

### 2.2 并发性
Go 语言原生支持并发编程，通过 goroutine 和 channel 实现轻量级并发。

### 2.3 高性能
Go 语言编译成机器码，执行效率接近 C/C++。
```

### Step 2: 上传文档

```bash
curl -X POST http://localhost:8080/api/v1/documents/upload \
  -F "file=@test_rag.md"
```

等待后台处理完成（查看日志确认）。

### Step 3: 单轮问答测试

```bash
curl -X POST http://localhost:8080/api/v1/chat/ask \
  -H "Content-Type: application/json" \
  -d '{"question": "Go语言的特性有哪些？"}'
```

### Step 4: 多轮对话测试

保存第一次问答返回的 `session_id`，然后追问：

```bash
curl -X POST http://localhost:8080/api/v1/chat/ask \
  -H "Content-Type: application/json" \
  -d '{"session_id": "session_xxx", "question": "详细说说并发性"}'
```

### Step 5: 查看历史记录

```bash
curl http://localhost:8080/api/v1/chat/history/session_xxx
```

---

## 验收标准

| 功能 | 验收标准 |
|------|---------|
| 文档上传 | 返回 document_id，后台日志显示切片完成 |
| 文档切片 | Markdown 按标题层级正确切分 |
| 向量化 | 日志显示"共入库 N 个切片" |
| RAG 问答 | 回答包含检索到的文档片段引用 |
| 多轮对话 | 使用相同 session_id 能正确理解上下文 |
| 离线运行 | 断网状态下仍可正常问答 |

---

## 常见问题

### Q1: Embedding 维度不匹配

错误：`vector dim 0 not match collection definition`

解决：确保 `config.toml` 中 `vector.dim` 与模型维度一致：
- nomic-embed-text: 768
- bge-m3: 1024

### Q2: 模型未拉取

错误：`ollama request failed`

解决：先拉取模型
```bash
ollama pull nomic-embed-text
ollama pull qwen2.5:7b
```

### Q3: 端口占用

错误：`bind: Only one usage of each socket address`

解决：停止占用进程或修改配置端口
```bash
# Windows
netstat -ano | findstr :8080
taskkill /PID <进程ID> /F
```

---

## 架构说明

```
┌─────────────────────────────────────────────┐
│                   用户请求                    │
└─────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────┐
│              HTTP API (Gin)                  │
└─────────────────────────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│ 文档服务  │   │ RAG 服务  │   │ 会话管理 │
└──────────┘   └──────────┘   └──────────┘
        │             │             │
        ▼             ▼             ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│ 文本切片  │   │ 向量检索  │   │   SQLite │
│ Markdown │   │  Memory  │   │  历史存储│
└──────────┘   └──────────┘   └──────────┘
        │             │
        ▼             ▼
┌──────────┐   ┌──────────┐
│Embedding │   │   LLM    │
│  Ollama  │   │  Ollama  │
│nomic-embed│   │qwen2.5:7b│
└──────────┘   └──────────┘
```

---

## 下一步优化

1. **流式输出** - 添加 SSE 接口，实时返回生成内容
2. **Milvus 集成** - 替换内存存储，支持大规模数据
3. **更多文档格式** - 支持 PDF、Word 等
4. **Web UI** - 添加前端界面
5. **混合检索** - 关键词 + 向量混合检索