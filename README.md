# 🎧 Dunkirk — AI Audio Book Generator

> 基于 [Eino](https://github.com/cloudwego/eino) 框架构建的多集有声读物智能生成系统。
>
> An intelligent multi-episode audiobook generation system built on the [Eino](https://github.com/cloudwego/eino) framework.

[中文](#中文) · [English](#english)

---

## 中文

### 项目简介

Dunkirk 是一个基于大语言模型的多集音频生成后端服务。用户上传 PDF/MD/DOCX 文档，系统自动解析、按章节拆分、语义搜索、生成朗读脚本，最后通过 TTS 转换为音频文件。

### 核心流程

```
用户上传文件 (PDF/MD/DOCX)
        ↓
文档解析 → 章节拆分 → 向量化入库 (Redis)
        ↓
自然语言交互 → 意图解析 → 启动 Agent/Pipeline
        ↓
逐章处理: 知识检索 → 脚本生成 (LLM) → TTS (Edge TTS)
        ↓
SSE 实时推送进度 + 音频文件可下载
```

### 技术栈

| 层面 | 技术 |
|------|------|
| 语言 | Go 1.24 |
| LLM 框架 | [Eino](https://github.com/cloudwego/eino) (CloudWeGo) |
| ChatModel | 火山引擎 Ark (豆包) |
| Embedding | 火山 Ark Embedding |
| 向量检索 | Redis Stack (FT.SEARCH) |
| TTS | Edge TTS (Microsoft) |
| 文档解析 | Markitdown / pdftotext |
| HTTP | Gin |
| 前端 | Vue 3 + Vite |

### 快速开始

#### 前置条件

- Go 1.24+
- Redis Stack (`docker run -d -p 6379:6379 redis/redis-stack`)
- 火山引擎 Ark API Key + ChatModel/Embedding 端点
- Python 3.10+ (用于 markitdown TTS)

```bash
# 安装依赖
pip install markitdown

# 配置环境变量
export ARK_API_KEY=your_ark_api_key
export ARK_CHAT_MODEL=your_chat_endpoint_id
export ARK_EMBEDDING_MODEL=your_embedding_endpoint_id
export TTS_VOICE=zh-CN-XiaoxiaoNeural

# 启动后端
go run cmd/server/main.go

# 启动前端（另一个终端）
cd frontend && npm run dev
```

访问 `http://localhost:5173` 即可使用。

#### API 示例

```bash
# 上传文件
curl -X POST localhost:8080/api/v1/upload \
  -H "X-User-ID: zsy" \
  -F "file=@三国演义.pdf"

# 对话式交互（SSE 流式）
curl -N -X POST localhost:8080/api/v1/chat \
  -H "X-User-ID: zsy" \
  -F "message=生成第1到5回的音频，适合7岁小朋友听"

# 中断选择后恢复
curl -X POST localhost:8080/api/v1/resume \
  -H "Content-Type: application/json" \
  -d '{"checkpoint_id":"xxx","interrupt_id":"yyy","choice":"三国演义"}'

# 下载音频
curl -O http://localhost:8080/api/v1/audio/download/zsy/chapter_1.mp3
```

### 项目结构

```
dunkirk/
├── cmd/server/               # HTTP 服务入口
├── internal/
│   ├── agent/                # ChatModelAgent (ReAct 模式)
│   ├── config/               # 配置管理
│   ├── docproc/              # 文档加载/解析/拆分
│   ├── handler/              # HTTP 路由 + SSE
│   ├── kb/                   # 知识库 (Embedding + Indexer + Retriever)
│   ├── pipeline/             # Graph 编排流水线
│   ├── task/                 # 异步任务管理
│   └── tts/                  # TTS 引擎 (Edge TTS)
├── frontend/                 # Vue 3 前端
│   └── src/
│       ├── composables/      # SSE 解析 + 状态管理
│       └── components/       # UI 组件
├── audio/                    # 音频输出目录
└── uploads/                  # 上传文件目录
```

### 架构要点

- **两套引擎**：Agent (话题场景) + Pipeline (全本批量处理)
- **中断恢复**：Eino Interrupt/Resume 实现书名模糊选择
- **多租户**：X-User-ID 隔离，公共/私有双索引
- **流式输出**：SSE 实时推送 token + 进度 + 中断事件
- **段落级语义**：文档按自然段落切分 + 1000 字聚合入库

### Eino 知识点覆盖

| 概念 | 使用位置 |
|------|---------|
| ChatModel | Ark 模型调用 |
| Embedder | 文本向量化 |
| Indexer | Redis 文档入库 |
| Retriever | 语义搜索 |
| ChatTemplate | prompt 模板组装 |
| Chain | 意图解析 / 脚本生成 |
| Graph | 章节处理流水线 |
| Lambda | 自定义处理节点 |
| ChatModelAgent | ReAct 循环 |
| Runner | Agent 执行 |
| Tool (InferTool) | 知识搜索 / 脚本生成 / TTS |
| Callback | 节点级进度推送 |
| Interrupt/Resume | 书名模糊选择 |
| SessionValues | 多轮上下文传递 |
| CheckPointStore | 中断状态持久化 |
| Middleware (Summarization) | 长对话压缩 |

---

## English

### Introduction

Dunkirk is a backend service for generating multi-episode audiobooks powered by LLM. Users upload PDF/MD/DOCX files, and the system automatically parses them, splits by chapters, performs semantic search, generates narration scripts, and converts them to audio via TTS.

### Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.24 |
| LLM Framework | [Eino](https://github.com/cloudwego/eino) |
| ChatModel | Volcengine Ark (Doubao) |
| Embedding | Volcengine Ark Embedding |
| Vector Search | Redis Stack (FT.SEARCH) |
| TTS | Edge TTS (Microsoft) |
| Doc Parser | Markitdown / pdftotext |
| HTTP | Gin |
| Frontend | Vue 3 + Vite |

### Quick Start

```bash
# Install dependencies
pip install markitdown

# Set environment variables
export ARK_API_KEY=your_ark_api_key
export ARK_CHAT_MODEL=your_chat_endpoint_id
export ARK_EMBEDDING_MODEL=your_embedding_endpoint_id
export TTS_VOICE=zh-CN-XiaoxiaoNeural

# Start backend
go run cmd/server/main.go

# Start frontend (separate terminal)
cd frontend && npm run dev
```

Visit `http://localhost:5173` to use the web UI.

### Project Structure

```
dunkirk/
├── cmd/server/               # HTTP server entry
├── internal/
│   ├── agent/                # ChatModelAgent (ReAct pattern)
│   ├── config/               # Configuration
│   ├── docproc/              # Document loading/parsing/splitting
│   ├── handler/              # HTTP routes + SSE
│   ├── kb/                   # Knowledge base (Embedding + Indexer + Retriever)
│   ├── pipeline/             # Graph orchestration pipeline
│   ├── task/                 # Async task management
│   └── tts/                  # TTS engine (Edge TTS)
├── frontend/                 # Vue 3 web app
├── audio/                    # Audio output
└── uploads/                  # Uploaded files
```

### API Reference

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/chat` | POST | Chat interface (SSE stream) |
| `/api/v1/upload` | POST | Upload file (PDF/MD/DOCX) |
| `/api/v1/resume` | POST | Resume interrupted selection |
| `/api/v1/audio/download/:userID/:filename` | GET | Download audio file |
| `/api/v1/audio/:task_id` | GET | Query task status |

### Key Features

- **Two Engines**: Agent (single topic) + Pipeline (full book batch)
- **Interrupt/Resume**: Eino native HITL for book selection
- **Multi-tenant**: X-User-ID isolation, public/private indexes
- **Streaming**: SSE real-time tokens + progress + interrupts
- **Paragraph-level RAG**: Natural paragraph splitting with 1000-char aggregation

### License

MIT
