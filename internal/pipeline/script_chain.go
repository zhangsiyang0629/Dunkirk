package pipeline

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func newScriptChain(ctx context.Context, cm model.BaseChatModel) *compose.Chain[map[string]any, *schema.Message] {
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一位专业的有声读物编剧，擅长将各类内容转化为生动、吸引人的音频脚本。"),
		schema.UserMessage("原文内容：{content}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标时长：{duration_min}分钟\n\n请生成纯叙述性朗读文本，禁止任何表格和标记符号。直接输出正文。"),
	)
	chain := compose.NewChain[map[string]any, *schema.Message]()
	return chain.AppendChatTemplate(tmpl).AppendChatModel(cm)
}
