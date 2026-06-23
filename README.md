# 向量数据库 + 文本向量化全流程

这是一个用 Go 实现的向量数据库和文本向量化项目，包含完整的 RAG（检索增强生成）基础链路。

## 📁 项目结构

```
golllm/
├── pkg/
│   ├── vector/          # Milvus 向量数据库客户端封装
│   │   └── client.go    # 向量增删改查、索引管理
│   ├── text/            # 文本切片策略实现
│   │   └── splitter.go  # 固定长度、段落、Markdown 标题切片
│   └── embedding/       # Embedding 模型调用封装
│       └── client.go    # OpenAI/DeepSeek 文本向量化
├── cmd/
│   ├── vector/          # 向量数据库管理命令行工具
│   │   └── main.go
│   ├── search/          # 命令行检索工具
│   │   └── main.go
│   └── import/          # 文档导入工具
│       └── main.go
├── docs/
│   └── go_notes.md      # 测试文档
├── go.mod
└── go.sum
```

## 🚀 快速开始

### 1. 部署 Milvus

使用 Docker 部署 Milvus 单机版：

```bash
docker run -d --name milvus-standalone \
  -p 19530:19530 \
  -p 9091:9091 \
  milvusdb/milvus:latest
```

### 2. 编译项目

```bash
go build -o vector-cli.exe ./cmd/vector/main.go
go build -o search-cli.exe ./cmd/search/main.go
go build -o import-cli.exe ./cmd/import/main.go
```

### 3. 使用示例

#### 创建集合

```bash
./vector-cli.exe --action create --collection test --dim 1536
```

#### 创建索引

```bash
./vector-cli.exe --action create-index --collection test --index-type HNSW
```

#### 导入文档

```bash
./import-cli.exe \
  --file docs/go_notes.md \
  --splitter markdown \
  --api-key your-api-key \
  --provider deepseek
```

#### 检索

```bash
./search-cli.exe \
  --query "向量检索原理" \
  --api-key your-api-key \
  --provider deepseek
```

## 📚 核心模块说明

### 1. 向量数据库客户端 (`pkg/vector/client.go`)

封装了 Milvus 的核心操作：

| 方法 | 说明 |
|-----|------|
| `NewClient()` | 创建客户端连接 |
| `CreateCollection()` | 创建集合（包含 id、vector、text、metadata 字段） |
| `DropCollection()` | 删除集合 |
| `HasCollection()` | 检查集合是否存在 |
| `ListCollections()` | 列出所有集合 |
| `CreateIndex()` | 创建索引（HNSW / IVF_FLAT） |
| `Insert()` | 插入单条向量数据 |
| `BatchInsert()` | 批量插入向量数据 |
| `GetByID()` | 根据 ID 查询 |
| `Search()` | 向量相似度检索 |
| `DeleteByID()` | 根据 ID 删除 |
| `Flush()` | 强制持久化 |

**索引类型对比**：

| 索引类型 | 适用场景 | 参数 | 特点 |
|---------|---------|------|------|
| **HNSW** | 小规模数据、高召回率 | M=16, efConstruction=200 | 基于图结构，查询速度快 |
| **IVF_FLAT** | 大规模数据、追求速度 | nlist=128 | 基于聚类，内存占用低 |

### 2. 文本切片策略 (`pkg/text/splitter.go`)

提供三种文本切片方式：

| 切片器 | 说明 | 适用场景 |
|-------|------|---------|
| `FixedLengthSplitter` | 固定长度切片，支持重叠 | 长文本、需要控制切片大小 |
| `ParagraphSplitter` | 按段落切片 | 结构化文档 |
| `MarkdownSplitter` | 按标题层级切片 | Markdown 文档，保留结构 |

**使用示例**：

```go
// 固定长度切片（500 字符，重叠 50 字符）
splitter := text.NewFixedLengthSplitter(500, 50)
chunks := splitter.Split(text)

// 段落切片
splitter := text.NewParagraphSplitter()
chunks := splitter.Split(text)

// Markdown 标题切片
splitter := text.NewMarkdownSplitter()
chunks := splitter.Split(text)
```

### 3. Embedding 客户端 (`pkg/embedding/client.go`)

支持调用 OpenAI 和 DeepSeek 的 Embedding API：

| 方法 | 说明 |
|-----|------|
| `NewClient()` | 创建客户端 |
| `Embeddings()` | 批量生成向量 |
| `Embedding()` | 生成单个向量 |
| `GetDimension()` | 获取向量维度 |

**支持的模型**：

| 模型 | 维度 | 提供商 |
|-----|------|-------|
| text-embedding-3-small | 1536 | OpenAI |
| text-embedding-3-large | 3072 | OpenAI |
| text-embedding-ada-002 | 1536 | OpenAI |
| deepseek-embedding | 1024 | DeepSeek |

## 🔄 完整流程

```
文档文件 → 文本切片 → Embedding → 向量数据库 → 相似度检索
```

1. **读取文档**：读取本地 TXT/Markdown 文件
2. **文本切片**：根据策略将文档切分成小块
3. **生成向量**：调用 Embedding API 将文本转换为向量
4. **写入数据库**：将向量、文本、元数据写入 Milvus
5. **相似度检索**：输入查询词，生成向量后检索最相似的文本片段

## 📖 学习路径

### Day1：向量数据库核心原理
- 学习向量检索的核心逻辑（余弦距离、欧氏距离）
- 了解主流向量数据库选型（Milvus、Pinecone、Chroma）
- 学习 Milvus 基础架构（Collection、Partition、Field、Index）

### Day2：Milvus 部署 + Go SDK 基础操作
- Docker 部署 Milvus 单机版
- 使用 Go SDK 创建/删除 Collection
- 实现单条向量插入、查询、检索

### Day3：批量向量操作 + 索引配置
- 实现批量向量插入、删除
- 学习 HNSW、IVF_FLAT 索引的区别
- 测试不同索引参数的检索速度差异

### Day4：文本切片策略实现
- 学习固定长度切片、按段落切片的优缺点
- 实现 Markdown 标题层级切片

### Day5：文档向量化链路打通
- 串起完整链路：读取文件 → 切片 → Embedding → 写入 Milvus
- 调试链路，解决批量写入超时、向量维度不匹配等问题

## 🔧 配置说明

### Milvus 配置

| 参数 | 默认值 | 说明 |
|-----|-------|------|
| Addr | localhost:19530 | Milvus 服务器地址 |
| Collection | documents | 集合名称 |
| Dim | 1536 | 向量维度 |

### Embedding 配置

| 参数 | 默认值 | 说明 |
|-----|-------|------|
| Provider | deepseek | 提供商：openai/deepseek |
| Model | deepseek-embedding | 模型名称 |
| APIKey | - | API 密钥 |

## ⚠️ 注意事项

1. **向量维度匹配**：确保 Milvus 集合的维度与 Embedding 模型的维度一致
2. **索引创建**：插入数据前需要先创建索引，否则检索会很慢
3. **数据持久化**：插入数据后建议调用 `Flush()` 确保数据持久化
4. **API 密钥**：使用前需要配置正确的 API 密钥

## 📚 参考资源

- [Milvus 官方文档](https://milvus.io/docs)
- [Milvus Go SDK](https://github.com/milvus-io/milvus-sdk-go)
- [OpenAI Embedding API](https://platform.openai.com/docs/guides/embeddings)
- [向量检索原理 - B站 BV1ByVg6FE9q](https://www.bilibili.com/video/BV1ByVg6FE9q)