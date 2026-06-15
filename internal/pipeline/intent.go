package pipeline

import (
	"context"
	"dunkirk/internal/kb"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func init() {
	schema.Register[IntentResult]()
}

type inMemoryStore struct {
	m map[string][]byte
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		m: make(map[string][]byte),
	}
}

func (i *inMemoryStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	v, ok := i.m[checkPointID]
	return v, ok, nil
}

func (i *inMemoryStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	i.m[checkPointID] = checkPoint
	return nil
}

func NewIntentParser(ctx context.Context,
	cm model.BaseChatModel,
	kb *kb.KnowledgeBase) (compose.Runnable[map[string]any, *IntentResult], error) {
	sys := `你是音频制作助手的意图识别器和助手，当用户闲聊时友好回应，并引导到音频制作话题。

【场景一：用户闲聊】
直接友好回复，不要输出JSON，不要说你想生成音频，引导到音频制作话题。

【场景二：用户请求生成音频】
仅输出以下JSON，不要其他内容：
将用户输入转为 JSON，直接输出 JSON，不要任何其他内容。
{{
    "topic": "用户指定的话题或章节标题",
    "style": "用户指定的风格要求，未指定则空字符串",
    "duration_min": 0, // 分钟，用户指定的音频时长, 如"5分钟左右"→5, "10分钟"→10, 未指定则为0,
	"book": "用户提到的书籍名称",
    "mode": "chat"/"chapter"/"book",
    "is_audio_request": true/false,
    "reasoning": "简要推断说明",
	"chapters": 章节数组，如[1,2,3,4,5]或[1,3]，全本则不填或null
}}

规则：
- "mode":"book" → 用户只上传了文件，没说具体话题，需要全本处理
- "mode":"chapter" → 用户指定了具体话题或章节
- 用户说"生成音频"、"做成音频"等，但没有上传文件，也没有指定主题 → 则为闲聊，需引导用户生成什么样的音频
- 用户说"生成音频"、"做成音频"等，且上传文件或指定主题 → is_audio_request=true
- 用户既上传文件又指定话题 → mode="chapter"
- 用户说"第1到5回" → chapters: [1,2,3,4,5]
- 用户说"第一章和第三章" → chapters: [1,3]
- 用户说"全本"或没提章节 → 不填 chapters
- 用户明确说了书名，例如"三国演义" → book="三国演义"
- 用户的书名说得不明确，例如"三国" → book="三国"，不要自行推断完整书名
- 书名推断由后续系统逻辑处理，意图解析只需输出用户原文
- 用户没提到书 → book=""`

	userTmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(sys),
		schema.SystemMessage("{context}"),
		schema.MessagesPlaceholder("history", false),
		schema.UserMessage("用户输入：{user_input}"),
	)

	chain := compose.NewChain[map[string]any, *IntentResult]()
	chain.
		AppendChatTemplate(userTmpl, compose.WithNodeName("intent_template")).
		AppendChatModel(cm, compose.WithNodeName("intent_model")).
		AppendLambda(compose.InvokableLambda(
			func(ctx context.Context, msg *schema.Message) (*IntentResult, error) {
				return lambdaParseIntentResponse(ctx, msg, kb)
			}), compose.WithNodeName("intent_parser_response"))

	r, err := chain.Compile(ctx, compose.WithCheckPointStore(newInMemoryStore()))
	if err != nil {
		return nil, fmt.Errorf("compile intent parser: %w", err)
	}
	return r, nil
}

func parseIntentResponse(content string) (*IntentResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var result IntentResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse intent: %w", err)
	}
	return &result, nil
}

const (
	GENERATE_LLM = "依赖模型推理继续生成"
	GIVEUP       = "放弃生成"
)

func lambdaParseIntentResponse(
	ctx context.Context,
	msg *schema.Message,
	kb *kb.KnowledgeBase) (*IntentResult, error) {
	wasInterrupted, hasState, state := compose.GetInterruptState[IntentResult](ctx)
	lastRes := state
	if !wasInterrupted {
		content := strings.TrimSpace(msg.Content)
		if !strings.HasPrefix(content, "{") {
			return &IntentResult{IsAudioRequest: false, ChatReply: content}, nil
		}
		result, err := parseIntentResponse(content)
		if err != nil {
			return nil, err
		}
		if !result.IsAudioRequest {
			return result, nil
		}
		if result.Book == "" {
			return result, nil
		}

		userID, _ := ctx.Value("userID").(string)
		books, findErr := kb.FindBooks(ctx, userID, result.Book)
		if findErr != nil {
			return nil, findErr
		}
		if len(books) > 1 {
			result.InterruptType = INTERRUPT_BOOK_SELECT
			result.InterruptOpions = books
			info := result.interruptInfo()
			return nil, compose.StatefulInterrupt(ctx, info, *result)
		} else if len(books) == 1 {
			result.Book = books[0]
			return result, nil
		} else {
			result.InterruptType = INTERRUPT_GEN_SELECT
			result.InterruptOpions = []string{GENERATE_LLM, GIVEUP}
			info := result.interruptInfo()
			return nil, compose.StatefulInterrupt(ctx, info, *result)
		}
	}

	isTarget, hasData, data := compose.GetResumeContext[string](ctx)
	if isTarget && hasData {
		switch data {
		case GIVEUP:
			return &IntentResult{IsAudioRequest: false, ChatReply: "已取消"}, nil
		case GENERATE_LLM:
			lastRes.SkipFile = true
			lastRes.Book = ""
			return &lastRes, nil
		default:
			lastRes.Book = data
			return &lastRes, nil
		}
	}

	// 不是目标中断点 → 用保存的状态重新中断
	if hasState {
		return nil, compose.StatefulInterrupt(ctx, lastRes.interruptInfo(), lastRes)
	}
	return &lastRes, nil
}
