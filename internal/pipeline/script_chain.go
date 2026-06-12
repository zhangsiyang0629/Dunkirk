package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// 纯文本 Chain（无 SSML 指令）
func newPlainScriptChain(_ context.Context, cm model.BaseChatModel) *compose.Chain[map[string]any, *schema.Message] {
	tmpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage("{system_prompt}"),
		schema.UserMessage("原文内容：{content}\n\n上一回结尾：{prev_ending}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标字数：{rune_len}字\n\n请生成纯叙述性朗读文本，直接输出正文。开头请自然承接上一回内容。"),
		//schema.UserMessage("原文内容：{content}\n\n上一回结尾：{prev_ending}\n\n请生成纯叙述性朗读文本，直接输出正文。如果上一回结尾不为空则开头请自然承接上一回内容。"),
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
		schema.UserMessage("原文内容：{content}\n\n上一回结尾：{prev_ending}\n\n请重点讲述以下主题：{topic}\n\n风格要求：{style}\n目标字数：{rune_len}字\n\n请基于原文生成 SSML 格式的朗读脚本。开头请自然承接上一回内容。"),
	)
	chain := compose.NewChain[map[string]any, *schema.Message]()
	return chain.AppendChatTemplate(tmpl).AppendChatModel(cm,
		compose.WithNodeKey("sub_pipeline_ssml_chatMode"),
		compose.WithNodeName("sub_pipeline_ssml_gen_script"),
	)
}

// 文段plain文案生成
func newSegmentedScriptChain(_ context.Context, cm model.BaseChatModel) *compose.Chain[map[string]any, *schema.Message] {
	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendLambda(compose.InvokableLambda(
		func(ctx context.Context, input map[string]any) (*schema.Message, error) {
			eventCh, _ := ctx.Value("eventCh").(chan *adk.AgentEvent)
			pushEvent(eventCh, "由于用户没有指定时长，可能需要分段输出\n")
			return loopSegmentedScriptGenerate(ctx, input, cm)
		}),
		compose.WithNodeKey("sub_pipeline_seg_chatMode"),
		compose.WithNodeName("sub_pipeline_seg_chatMode"))
	return chain
}

func loopSegmentedScriptGenerate(
	ctx context.Context,
	input map[string]any,
	cm model.BaseChatModel) (*schema.Message, error) {

	content := input["content"].(string)
	prevEnding := input["prev_ending"].(string)
	style := input["style"].(string)
	systemPrompt := computeSystemPrompt(style)

	runeCount := len([]rune(content))
	parts := getPartNum(runeCount)
	eventCh, _ := ctx.Value("eventCh").(chan *adk.AgentEvent)
	pushEvent(eventCh, "原文长度：%d，分为%d段\n", runeCount, parts)

	firstUserMsgPromptFun := func(theContent, thePreEnding, ending string) string {
		return fmt.Sprintf(`
原文：%s
上一回结尾：%s

## 要求
- 请根据原文生成生动、吸引人的音频脚本。自由发挥，不限字数。如果上一回结尾不为空则开头请自然承接上一回内容。
- %s


## 输出格式：
[脚本正文]{结束语}

===摘要===
用小于300字概括本段生成的内容`, theContent, thePreEnding, ending)
	}

	sentences := splitBySentence(content)
	if parts <= 1 {
		log.Printf("[SEG PLAIN SCRIPT]no segment, generate whole chapter, runeCount:%d", runeCount)
		pushEvent(eventCh, "原文长度%d子，长度较小，无需分段，直接生成\n\n", runeCount)
		stream, err := cm.Stream(ctx, []*schema.Message{
			schema.SystemMessage(systemPrompt),
			schema.UserMessage(firstUserMsgPromptFun(content, prevEnding,
				"请在结尾加上章节结束语，例如：\"欲知后事如何，且听下回分解\"")),
		})
		if err != nil {
			return nil, err
		}
		res, err := collectStream(stream)
		if err != nil {
			return nil, err
		}
		return &schema.Message{Content: res}, nil
	}

	segs := distributeSentences(sentences, parts)
	var rewritten, prevSegEndings []string
	prevSummary := ""

	for i, seg := range segs {
		segText := strings.Join(seg, "")
		log.Printf("[SEG PLAIN SCRIPT START]should segment, generate segement %d/%d chapter, runeCount:%d/%d\n",
			i+1, parts, len([]rune(segText)), runeCount)
		pushEvent(eventCh, "开始生成第%d/%d段音频文案，原文长度：%d/%d\n\n",
			i+1, parts, len([]rune(segText)), runeCount)
		var opening string
		if i == 0 {
			opening = "请直接开始根据原文进行叙述。"
		} else {
			opening = fmt.Sprintf("上一集咱们讲了：%s\n\n上一集结尾：%s\n\n这一集我们接着讲。",
				prevSummary, strings.Join(prevSegEndings, ""))
		}

		var ending string
		if i == parts-1 {
			ending = "请在正文结尾（===摘要===之前）加上章节结束语，例如：\"欲知后事如何，且听下回分解\""
		} else {
			ending = "请在正文结尾（===摘要===之前）加上本集结束语，例如：\"后面又会发生什么呢？咱们下集接着说！\""
		}

		var prompt string
		if i == 0 {
			prompt = firstUserMsgPromptFun(segText, prevEnding, ending)
		} else {
			prompt = fmt.Sprintf(`原文（只基于以下内容创作，不要续写未提供的情节）：%s

要求（必须遵守）:
1. %s
2. %s
3. 根据以上信息，继续生成下一段的音频脚本，保持情节和语气连贯。自由发挥，不限字数。


输出格式：
正文

===摘要===
用一句话概括本段生成的内容`,
				segText, opening, ending)
		}

		stream, err := cm.Stream(ctx, []*schema.Message{
			schema.SystemMessage(systemPrompt),
			schema.UserMessage(prompt),
		})
		if err != nil {
			return nil, err
		}

		originScript, err := collectStream(stream)
		if err != nil {
			return nil, err
		}

		script, summary := parseSegmentOutput(originScript)
		script = strings.TrimPrefix(script, "[脚本正文]")
		script = strings.TrimPrefix(script, "【脚本正文】")
		rewritten = append(rewritten, script)
		prevSummary = summary
		if summary == "" {
			prevSummary = trunc(script, 100)
		}
		prevSegEndings = lastNSentences(script, 5)
		log.Printf("[SEG PLAIN SCRIPT END]segment %d/%d chapter summary:%s\n",
			i+1, parts, summary)
		pushEvent(eventCh, "开始完成第%d/%d段音频文案，脚本摘要：%s\n\n", i+1, parts, prevSummary)
	}
	log.Printf("[SEG PLAIN SCRIPT]all segment finished, total parts:%d\n", parts)
	return &schema.Message{Content: "===SEGMENT_BOUNDARY===\n" + strings.Join(rewritten, "\n===SEGMENT_BOUNDARY===\n")}, nil
}

func parseSegmentOutput(text string) (script, summary string) {
	parts := strings.Split(text, "===摘要===")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(text), ""
}

func collectStream(stream *schema.StreamReader[*schema.Message]) (string, error) {
	defer stream.Close()
	var chunks []string
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		chunks = append(chunks, chunk.Content)
	}
	return strings.Join(chunks, ""), nil
}

func getPartNum(runeCount int) int {
	parts := 1
	if runeCount > 6000 {
		parts = 4
	} else if runeCount > 4500 {
		parts = 3
	} else if runeCount > 3000 {
		parts = 2
	}
	return parts
}
