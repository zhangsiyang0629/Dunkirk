package agent

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"dunkirk/internal/script"
	"dunkirk/internal/tts"
	"fmt"
	"log"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	runner *adk.Runner
}

func New(ctx context.Context,
	cfg *config.Config,
	knowledgeBase *kb.KnowledgeBase,
	ttsClient tts.TTSProvider,
	scriptStore *script.Store) (*Agent, error) {
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		return nil, fmt.Errorf("new chat model: %w", err)
	}
	return NewWithChatMode(ctx, cfg, cm, knowledgeBase, ttsClient, scriptStore)
}

func NewWithChatMode(ctx context.Context,
	cfg *config.Config,
	cm *ark.ChatModel,
	knowledgeBase *kb.KnowledgeBase,
	ttsClient tts.TTSProvider,
	scriptStore *script.Store) (*Agent, error) {
	tools := newTools(knowledgeBase, cm, ttsClient, scriptStore)

	summMW, err := summarization.New(ctx, &summarization.Config{
		Model: cm, // 复用同一个 Ark ChatModel
		PreserveUserMessages: &summarization.PreserveUserMessages{
			Enabled: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("new summarization: %w", err)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "audio_book_maker",
		Description: "专业有声读物制作人，可处理文档并生成多集音频",
		Handlers:    []adk.ChatModelAgentMiddleware{summMW},
		Instruction: `你是{Style}。
工作流程：
1. 如果用户指定了话题，先调用 search_knowledge_base 了解相关内容，如果在search_knowledge_base中找不到相关内容，则自行推理
2. 再调用 generate_script 基于搜索到的内容生成音频脚本
3. 最后调用 text_to_speech 转为音频文件
4. 每完成一步向用户报告进度
5. 如果用户明确说了时长，严格按指定时长控制
6. 调用 generate_script 生成脚本后，必须调用 approve_script 让用户审核
7. approve_script 返回"同意"才调用 text_to_speech
8. approve_script 返回"拒绝"或"拒绝但保留脚本"或"审核超时"则跳过本章
9. 如果用户没提时长，generate_script 的 duration_min 参数传 0，工具会自动估算
10. **重要：所有数字、人名、地名、数量必须严格与原文一致，不得修改**
11. **如果记不清原文中的某个具体数字或细节，直接从原文引用该句，不要自己猜测**`,
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               tools,
				ToolCallMiddlewares: []compose.ToolMiddleware{{Invokable: loggingMW}},
			},
		},
		MaxIterations: 300,
	})
	if err != nil {
		return nil, fmt.Errorf("new agent: %w", err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})
	return &Agent{runner: runner}, nil
}

func (a *Agent) Run(ctx context.Context, userInput, style string) *adk.AsyncIterator[*adk.AgentEvent] {
	sessionVals := map[string]any{"Style": style}
	if style == "" {
		sessionVals["Style"] = "专业的有声读物制作人"
	}
	return a.runner.Run(ctx, []adk.Message{
		schema.UserMessage(userInput),
	}, adk.WithSessionValues(sessionVals))
}

func loggingMW(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
	return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
		trunc := func(s string, n int) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "..."
		}
		log.Printf("[Tool] name=%s args=%s", input.Name, trunc(input.Arguments, 200))
		output, err := next(ctx, input)
		if err != nil {
			log.Printf("[Tool] name=%s error=%v", input.Name, err)
		} else {
			log.Printf("[Tool] name=%s done result=%s", input.Name, trunc(output.Result, 100))
		}
		return output, err
	}
}
