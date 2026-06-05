package pipeline

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type SSMLConfig struct {
	Mode string // "none" / "paragraph"
}

func newScriptChain(_ context.Context, cm model.BaseChatModel, renderer SSMLRenderer) *compose.Chain[map[string]any, *schema.Message] {
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(`你是一位专业的有声读物编剧。
输出规则（必须遵守）：
1. 段落间必须加 [pause:300ms]
2. 每个段落至少使用一次 [em]...[/em] 标记关键人物或地名
3. 每段最后一个句子如果是感叹句、疑问句或对话，用 [slow]...[/slow] 包裹
4. 严禁输出 XML 或 SSML 标签
示例：
话说天下大势。[pause:300ms]
[em]曹操[/em]率大军南下。[slow]一场决战，即将开始！[/slow]`),
		schema.UserMessage("原文内容：{content}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标字数：{rune_len}字\n\n请使用 SSML 格式生成朗读脚本，加入适当的停顿和强调。"),
	)
	chain := compose.NewChain[map[string]any, *schema.Message]()
	return chain.AppendChatTemplate(tmpl).AppendChatModel(cm,
		compose.WithNodeKey("sub_pipeline_chatMode"),
		compose.WithNodeName("sub_pipeline_gen_script"),
	).AppendLambda(compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		msg.Content = renderer.Render(msg.Content)
		return msg, nil
	}))
}

// 纯文本 Chain（无 SSML 指令）
func newPlainScriptChain(_ context.Context, cm model.BaseChatModel) *compose.Chain[map[string]any, *schema.Message] {
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一位专业的有声读物编剧，擅长将各类内容转化为生动、吸引人的音频脚本。"),
		schema.UserMessage("原文内容：{content}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标字数：{rune_len}字\n\n请生成纯叙述性朗读文本，直接输出正文。"),
	)
	chain := compose.NewChain[map[string]any, *schema.Message]()
	return chain.AppendChatTemplate(tmpl).AppendChatModel(cm,
		compose.WithNodeKey("sub_pipeline_plain_chatMode"),
		compose.WithNodeName("sub_pipeline_plain_gen_script"),
	)
}

func newSSMLScriptChain(_ context.Context, cm model.BaseChatModel) *compose.Chain[map[string]any, *schema.Message] {
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage("你是一位专业的有声读物编剧。\n\n输出 SSML 格式的朗读脚本，使用以下标签增强表达：\n- <break time=\"300ms\"/> 停顿\n- <emphasis level=\"strong\">...</emphasis> 强调\n- <prosody rate=\"slow\" pitch=\"+5%%\">...</prosody> 慢速/高潮\n- <prosody rate=\"fast\">...</prosody> 紧张/快速\n\n直接输出 SSML，不要额外说明。"),
		schema.UserMessage("原文内容：{content}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标字数：{rune_len}字\n\n请基于原文生成 SSML 格式的朗读脚本。"),
	)
	chain := compose.NewChain[map[string]any, *schema.Message]()
	return chain.AppendChatTemplate(tmpl).AppendChatModel(cm,
		compose.WithNodeKey("sub_pipeline_ssml_chatMode"),
		compose.WithNodeName("sub_pipeline_ssml_gen_script"),
	)
}
