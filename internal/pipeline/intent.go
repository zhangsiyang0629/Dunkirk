package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewIntentParser(ctx context.Context,
	cm model.BaseChatModel) (compose.Runnable[string, *IntentResult], error) {
	sys := `你是音频制作助手的意图识别器和助手，当用户闲聊时友好回应，并引导到音频制作话题。

【场景一：用户闲聊】
直接友好回复，不要输出JSON，不要说你想生成音频，引导到音频制作话题。

【场景二：用户请求生成音频】
仅输出以下JSON，不要其他内容：
将用户输入转为 JSON，直接输出 JSON，不要任何其他内容。
{{
    "topic": "用户指定的话题或章节标题",
    "style": "用户指定的风格要求，未指定则空字符串",
    "duration_min": 用户指定的时长，未指定则为0,
    "mode": "chat"/"chapter"/"book",
    "is_audio_request": true/false,
    "reasoning": "简要推断说明"
}}

规则：
- "mode":"book" → 用户只上传了文件，没说具体话题，需要全本处理
- "mode":"chapter" → 用户指定了具体话题或章节
- 用户说"生成音频"、"做成音频"等，但没有上传文件，也没有指定主题 → 则为闲聊，需引导用户生成什么样的音频
- 用户说"生成音频"、"做成音频"等，且上传文件或指定主题 → is_audio_request=true
- 用户既上传文件又指定话题 → mode="chapter"`

	userTmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(sys),
		schema.UserMessage("用户输入：{user_input}"),
	)

	chain := compose.NewChain[string, *IntentResult]()
	chain.
		AppendLambda(compose.InvokableLambda(
			func(ctx context.Context, input string) (map[string]any, error) {
				return map[string]any{"user_input": input}, nil
			}), compose.WithNodeName("intent_parser")).
		AppendChatTemplate(userTmpl, compose.WithNodeName("intent_template")).
		AppendChatModel(cm, compose.WithNodeName("intent_model")).
		AppendLambda(compose.InvokableLambda(
			func(ctx context.Context, msg *schema.Message) (*IntentResult, error) {
				content := strings.TrimSpace(msg.Content)
				if strings.HasPrefix(content, "{") {
					result, err := parseIntentResponse(content)
					if err != nil {
						return nil, err
					}
					return result, nil
				}
				return &IntentResult{
					IsAudioRequest: false,
					ChatReply:      content,
				}, nil
			}), compose.WithNodeName("intent_parser_response"))

	r, err := chain.Compile(ctx)
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
