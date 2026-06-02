package agent

import (
	"context"
	"dunkirk/internal/kb"
	"dunkirk/internal/tts"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type ReadDocInput struct {
	FilePath string `json:"file_path" jsonschema_description:"文档文件路径"`
}

type SearchInput struct {
	Query    string `json:"query" jsonschema_description:"搜索关键词"`
	BookName string `json:"book_name" jsonschema_description:"书籍名称"`
	TopK     int    `json:"top_k" jsonschema_description:"返回结果数量"`
}

type ScriptInput struct {
	Topic       string `json:"topic" jsonschema_description:"主题、章节标题或原文内容"`
	Query       string `json:"query" jsonschema_description:"用于搜索原文的关键词，必须是用户原始输入的话题，不要替换为章节标题"`
	Style       string `json:"style" jsonschema_description:"风格要求，如'适合小学生'或'资深三国博主'"`
	DurationMin int    `json:"duration_min" jsonschema_description:"目标音频时长(分钟)"`
}

type TTSInput struct {
	Text     string `json:"text" jsonschema_description:"脚本文本"`
	Filename string `json:"filename" jsonschema_description:"输出文件名(不含扩展名)"`
}

func newTools(kb *kb.KnowledgeBase, cm *ark.ChatModel, tc tts.TTSProvider) []tool.BaseTool {
	search, _ := utils.InferTool("search_knowledge_base", "搜索知识库中相关的章节，返回章节标题列表。",
		func(ctx context.Context, input *SearchInput) (string, error) {
			if input.TopK <= 0 {
				input.TopK = 3
			}
			userID, _ := ctx.Value("userID").(string)
			uuid, err := kb.ResolveBookName(ctx, userID, input.BookName)
			refId := ""
			if err == nil && uuid != "" {
				refId = uuid
			}
			if refId == "" {
				return "", nil
			}

			docs, err := kb.Search(ctx, input.Query, input.TopK, userID, refId)
			if err != nil {
				return "", err
			}
			type brief struct {
				Title   string `json:"title"`
				Summary string `json:"summary"`
			}
			var list []brief
			for _, d := range docs {
				title, _ := d.MetaData["title"].(string)
				runes := []rune(d.Content)
				summary := string(runes)
				if len(runes) > 200 {
					summary = string(runes[:200]) + "..."
				}
				list = append(list, brief{Title: title, Summary: summary})
			}
			b, _ := json.Marshal(list)
			return string(b), nil
		})

	genScript, _ := utils.InferTool("generate_script", "根据原始话题从知识库检索相关原文，生成叙述性音频脚本。注意：query参数必须使用用户的原始输入。",
		func(ctx context.Context, input *ScriptInput) (string, error) {
			trunc := func(s string, n int) string {
				r := []rune(s)
				if len(r) <= n {
					return s
				}
				return string(r[:n]) + "..."
			}

			userID, _ := ctx.Value("userID").(string)
			relatedDocs, _ := kb.Search(ctx, input.Query, 2, userID, "")
			contextMsg := input.Topic
			if len(relatedDocs) > 0 {
				var parts []string
				for _, d := range relatedDocs {
					parts = append(parts, d.Content)
				}
				contextMsg = strings.Join(parts, "\n\n")
			}
			if input.DurationMin <= 0 {
				input.DurationMin = estimateDuration(len([]rune(contextMsg)), input.Style)
			}
			msg := fmt.Sprintf(`原文内容：%s
请重点讲述以下主题，不要偏离到原文的其他部分：
主题：%s
风格要求：%s
目标时长：%d 分钟
请基于原文内容，生成一份完整的、适合朗读的音频脚本。注意：
1. 严格基于原文，不要随意编造事实
2. 语言生动自然，符合%s的风格要求
3. 时长控制在%d分钟左右（约%d字）
4. 输出的文本应当是**纯叙述性文字**，适合直接朗读
5. **禁止使用任何表格、序号列表、星号、竖线、时间戳标记**
6. **直接输出正文本身，不要加"脚本如下"之类的引导语**
7. query 参数：传入用户的原始输入话题（如"苦肉计"、"草船借箭"），不要替换为章节标题
8. topic 参数：传入需要重点讲述的主题描述或章节标题`,
				contextMsg, input.Topic, input.Style, input.DurationMin,
				input.Style, input.DurationMin, input.DurationMin*200)
			log.Printf("Generating script with context: %s", trunc(contextMsg, 500))
			resp, err := cm.Generate(ctx, []*schema.Message{
				schema.SystemMessage("你是一位专业的有声读物编剧，擅长将各类内容转化为生动、吸引人的音频脚本。"),
				schema.UserMessage(msg),
			})
			if err != nil {
				return "", fmt.Errorf("generate script: %w", err)
			}
			return resp.Content, nil
		})

	ttsTool, _ := utils.InferTool("text_to_speech", "将文本转为音频文件，返回文件路径。",
		func(ctx context.Context, input *TTSInput) (string, error) {
			log.Printf("TTS called: filename=%s, text_len=%d", input.Filename, len(input.Text))
			path, err := tc.TextToSpeech(ctx, input.Text, input.Filename)
			if err != nil {
				return "", fmt.Errorf("tts: %w", err)
			}
			return path, nil
		})
	return []tool.BaseTool{search, genScript, ttsTool}
}

func estimateDuration(contentLen int, style string) int {
	speed := 200
	s := strings.ToLower(style)
	switch {
	case strings.Contains(s, "小朋友"), strings.Contains(s, "儿童"),
		strings.Contains(s, "小孩"), strings.Contains(s, "慢"):
		speed = 150
	case strings.Contains(s, "说书"), strings.Contains(s, "博主"),
		strings.Contains(s, "快"):
		speed = 250
	}
	m := contentLen / speed
	if m < 3 {
		return 3
	}
	if m > 20 {
		return 20
	}
	return m
}
