# Dunkirk

Go 1.24 project for building AI agents/applications with the [Eino](https://github.com/cloudwego/eino) framework (Go LLM framework by CloudWeGo).

## Quick start

```bash
go build ./...
go test ./...
```

## Eino skills

This repo has Eino skill files in `.opencode/skills/`. When working on an Eino task, load the matching skill first:

| Skill file | When to use |
|---|---|
| `eino-guide` | General Eino questions, architecture, getting started |
| `eino-compose` | Building graphs, chains, workflows (orchestration) |
| `eino-component` | Choosing/configuring ChatModel, Embedding, Retriever, Tool, etc. |
| `eino-agent` | Building ChatModelAgent, DeepAgents, ReAct patterns, middleware, runner |

## Project structure

```
├── go.mod                    # module dunkirk, Go 1.24
├── .opencode/
│   └── skills/               # Eino reference skills (loaded on demand)
└── AGENTS.md
```

No source tree yet — this is the starting scaffold.
