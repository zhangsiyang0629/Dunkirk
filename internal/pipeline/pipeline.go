package pipeline

import (
	"context"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"dunkirk/internal/tts"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ctxKeyType struct{}

var ctxKeyChapterTask = ctxKeyType{}

type Pipeline struct {
	runnable compose.Runnable[*ChapterTask, *ChapterTask]
	kb       *kb.KnowledgeBase
	tc       tts.TTSProvider
	outDir   string
}

func New(ctx context.Context,
	knowledgeBase *kb.KnowledgeBase,
	chatModel model.BaseChatModel,
	ttsClient tts.TTSProvider,
	audioDir string) (*Pipeline, error) {
	streamingMode := &streamingModel{BaseChatModel: chatModel}
	//ssmlRender := NewEdgeRenderer()
	// scriptRunnable := newScriptChain(ctx, streamingMode, ssmlRender)
	plainChain := newPlainScriptChain(ctx, streamingMode)
	ssmlChain := newSSMLScriptChain(ctx, streamingMode)
	searchNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
			userID, _ := ctx.Value("userID").(string)
			docs, err := knowledgeBase.Search(ctx, task.Topic, 5, userID, task.FileRefID)
			if err != nil {
				return nil, err
			}
			var parts []string
			for _, d := range docs {
				parts = append(parts, d.Content)
			}
			task.Content = strings.Join(parts, "\n\n")
			return task, nil
		})

	prepareNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (map[string]any, error) {
			duration := task.DurationMin
			if duration == 0 {
				duration = estimateDuration(len(task.Content), task.Style)
				task.DurationMin = duration
			}
			runeLen := duration2RuneLen(duration, task.Style)
			return map[string]any{
				"content":      task.Content,
				"topic":        task.Topic,
				"style":        task.Style,
				"duration_min": duration,
				"rune_len":     runeLen,
				"use_ssml":     task.UseSSML,
			}, nil
		})

	extractNode := compose.InvokableLambda(
		func(ctx context.Context, msg *schema.Message) (*ChapterTask, error) {
			task, ok := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
			if !ok {
				return nil, fmt.Errorf("task not found in context")
			}
			task.Script = msg.Content
			return task, nil
		})

	ttsNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
			//filename := fmt.Sprintf("chapter_%d", task.ChapterIdx)
			userID, _ := ctx.Value("userID").(string)
			path, err := ttsClient.TextToSpeech(ctx, task.Script, task.Topic, userID)
			if err != nil {
				return nil, err
			}
			task.AudioPath = path
			return task, nil
		})
	branch := compose.NewGraphBranch(func(ctx context.Context, input map[string]any) (string, error) {
		fmt.Printf("branch input use_ssml: %v\n", input["use_ssml"])
		if input["use_ssml"].(bool) {
			return "ssml_script", nil
		}
		return "plain_script", nil
	}, map[string]bool{"plain_script": true, "ssml_script": true})

	g := compose.NewGraph[*ChapterTask, *ChapterTask]()
	g.AddLambdaNode("search", searchNode, compose.WithNodeName("pipeline_search"))
	g.AddLambdaNode("prepare", prepareNode, compose.WithNodeName("pipeline_prepare"))
	g.AddGraphNode("plain_script", plainChain, compose.WithNodeName("pipeline_plain_script"))
	g.AddGraphNode("ssml_script", ssmlChain, compose.WithNodeName("pipeline_ssml_script"))
	//g.AddGraphNode("script", scriptRunnable, compose.WithNodeName("pipeline_script"))
	g.AddLambdaNode("extract", extractNode, compose.WithNodeName("pipeline_extract"))
	g.AddLambdaNode("tts", ttsNode, compose.WithNodeName("pipeline_tts"))

	g.AddEdge(compose.START, "search")
	g.AddEdge("search", "prepare")
	g.AddBranch("prepare", branch)
	g.AddEdge("plain_script", "extract")
	g.AddEdge("ssml_script", "extract")
	g.AddEdge("extract", "tts")
	g.AddEdge("tts", compose.END)

	runnable, err := g.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile graph: %w", err)
	}
	return &Pipeline{runnable: runnable, tc: ttsClient, outDir: audioDir, kb: knowledgeBase}, nil
}

// func (p *Pipeline) ProcessChapter(
// 	ctx context.Context,
// 	task *ChapterTask,
// 	bookName string,
// 	eventCh chan *adk.AgentEvent) (*ChapterTask, error) {

// 	ctx = context.WithValue(ctx, ctxKeyChapterTask, task)
// 	ctx = context.WithValue(ctx, "eventCh", eventCh)

// 	optPrepareNode := compose.WithCallbacks(
// 		callbacks.NewHandlerBuilder().
// 			OnStartFn(onPrepareNodeStart(eventCh)).
// 			OnEndFn(onPrepareNodeEnd(eventCh)).
// 			Build(),
// 	).DesignateNode("prepare")

// 	optSerchaeNode := compose.WithCallbacks(
// 		callbacks.NewHandlerBuilder().
// 			OnStartFn(onSearchNodeStart(bookName, eventCh)).
// 			OnEndFn(onSearchNodeEnd(bookName, eventCh)).
// 			Build(),
// 	).DesignateNode("search")

// 	optNestedChatMode := compose.WithCallbacks(
// 		callbacks.NewHandlerBuilder().
// 			OnStartFn(onSubChatModeNodeStart(eventCh)).
// 			OnEndFn(onSubChatModeNodeEnd(eventCh)).
// 			Build(),
// 	).DesignateNodeWithPath(compose.NewNodePath("script", "sub_pipeline_chatMode"))
// 	return p.runnable.Invoke(ctx, task, optPrepareNode, optSerchaeNode, optNestedChatMode)
// }

func (p *Pipeline) ProcessChapterStream(
	ctx context.Context,
	task *ChapterTask,
	bookName string,
	eventCh chan *adk.AgentEvent) (*schema.StreamReader[*ChapterTask], error) {

	ctx = context.WithValue(ctx, ctxKeyChapterTask, task)
	ctx = context.WithValue(ctx, "eventCh", eventCh)
	ctx = context.WithValue(ctx, "style", task.Style)

	optPrepareNode := compose.WithCallbacks(
		callbacks.NewHandlerBuilder().
			OnStartFn(onPrepareNodeStart(eventCh)).
			OnEndFn(onPrepareNodeEnd(eventCh)).
			Build(),
	).DesignateNode("prepare")

	optSerchaeNode := compose.WithCallbacks(
		callbacks.NewHandlerBuilder().
			OnStartFn(onSearchNodeStart(bookName, eventCh)).
			OnEndFn(onSearchNodeEnd(bookName, eventCh)).
			Build(),
	).DesignateNode("search")

	optNestedChatMode1 := compose.WithCallbacks(
		callbacks.NewHandlerBuilder().
			OnStartFn(onSubChatModeNodeStart(eventCh)).
			OnEndFn(onSubChatModeNodeEnd(eventCh)).
			Build(),
	).DesignateNodeWithPath(compose.NewNodePath("plain_script", "sub_pipeline_plain_chatMode"))

	optNestedChatMode2 := compose.WithCallbacks(
		callbacks.NewHandlerBuilder().
			OnStartFn(onSubChatModeNodeStart(eventCh)).
			OnEndFn(onSubChatModeNodeEnd(eventCh)).
			Build(),
	).DesignateNodeWithPath(compose.NewNodePath("ssml_script", "sub_pipeline_ssml_chatMode"))
	return p.runnable.Stream(ctx, task, optPrepareNode, optSerchaeNode, optNestedChatMode1, optNestedChatMode2)
}

func ProcessBook(ctx context.Context,
	p *Pipeline,
	userID, fileRefID, bookName, style string,
	durationMin int,
	chapters []int,
	eventCh chan *adk.AgentEvent,
	useSSML bool) ([]*ChapterTask, error) {

	allChapters, err := docproc.GetBriefChapters(ctx, p.kb, fileRefID)
	if err != nil {
		return nil, fmt.Errorf("process: %w", err)
	}
	var targets []kb.BriefChapter
	if len(chapters) == 0 {
		targets = allChapters
	} else {
		for _, idx := range chapters {
			if idx > 0 && idx <= len(allChapters) {
				allChapters[idx-1].ChapterInt = idx
				targets = append(targets, allChapters[idx-1])
			}
		}
	}
	log.Printf("total chapters: %d, target chapters: %v %v", len(allChapters), chapters, targets)

	var results []*ChapterTask
	for i, ch := range targets {
		task := &ChapterTask{
			Topic:         ch.Title,
			Style:         style,
			DurationMin:   durationMin,
			ChapterIdx:    i + 1,
			FileRefID:     fileRefID,
			UseSSML:       useSSML,
			TotalChapters: len(chapters),
		}
		var result *ChapterTask
		reader, err := p.ProcessChapterStream(ctx, task, bookName, eventCh)
		if err != nil {
			result = task
			result.Error = err.Error()
			pushEvent(eventCh, "第%d章失败: %s", result.ChapterIdx, err.Error())
			continue
		}
		defer reader.Close()
		for {
			frame, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, err)
				log.Printf("[ERROR]ProcessChapterStream error:%v", err)

			}
			if frame != nil {
				result = frame
			}
		}
		//result, err := p.ProcessChapter(ctx, task, bookName, eventCh)
		//var msg string
		// if err != nil {
		// 	result = task
		// 	result.Error = err.Error()
		// 	msg = fmt.Sprintf("第%d章失败: %s", result.ChapterIdx, err.Error())
		// }
		if result != nil {
			pushEvent(eventCh, "\n第%d章完成: %s\n", ch.ChapterInt, result.AudioPath)
			results = append(results, result)
			log.Printf("chapter %d/%d done: %s", ch.ChapterInt, len(chapters), result.AudioPath)
		} else {
			pushEvent(eventCh, "\n第%d章失败\n", ch.ChapterInt)
			log.Printf("[ERROR]chapter %d/%d failed", ch.ChapterInt, len(chapters))
		}

	}
	return results, nil
}

func onSearchNodeStart(
	bookName string,
	eventCh chan *adk.AgentEvent) func(context.Context, *callbacks.RunInfo, callbacks.CallbackInput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		if info.Name != "pipeline_search" {
			return ctx
		}
		ct := input.(*ChapterTask)
		task, _ := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
		pushEvent(eventCh, "开始第%d章的知识库搜索, 搜索关键:%s\nfileRefID:%s\n书名:%s\n",
			task.ChapterIdx, ct.Topic, ct.FileRefID, bookName)
		return ctx
	}
}

func onSearchNodeEnd(
	bookName string,
	eventCh chan *adk.AgentEvent) func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		if info.Name != "pipeline_search" {
			return ctx
		}
		ct := output.(*ChapterTask)
		task, _ := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
		pushEvent(eventCh, "完成第%d章的知识库搜索\n搜索关键:%s\nfileRefID:%s\n书名:%s\n搜索结果:%s\n",
			task.ChapterIdx, ct.Topic, ct.FileRefID, bookName, trunc(ct.Content, 100))
		return ctx
	}
}

func onSubChatModeNodeStart(
	eventCh chan *adk.AgentEvent) func(context.Context, *callbacks.RunInfo, callbacks.CallbackInput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		if info.Name != "sub_pipeline_plain_gen_script" && info.Name != "sub_pipeline_ssml_gen_script" {
			return ctx
		}
		msgs := input.([]*schema.Message)
		task, _ := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
		var parts []string
		for _, m := range msgs {
			if m.Content != "" {
				parts = append(parts, fmt.Sprintf("[%s] %s", m.Role, m.Content))
			}
		}
		pushEvent(eventCh, "开始第%d章的音频文本生成\n输入:\n%s\n风格:%s\n时长:%d\n",
			task.ChapterIdx, trunc(strings.Join(parts, "\n"), 100), task.Style, task.DurationMin)
		// pushEvent(eventCh, "开始第%d章的音频文本生成\n输入:\n%s\n风格:%s\n时长:%d\n",
		// 	task.ChapterIdx, strings.Join(parts, "\n"), task.Style, task.DurationMin)
		return ctx
	}
}

func onSubChatModeNodeEnd(
	eventCh chan *adk.AgentEvent) func(context.Context, *callbacks.RunInfo, callbacks.CallbackOutput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		if info.Name != "sub_pipeline_plain_gen_script" && info.Name != "sub_pipeline_ssml_gen_script" {
			return ctx
		}
		cbOuput := output.(*schema.Message)
		task, _ := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
		pushEvent(eventCh, "完成第%d章的音频文本生成:%s\n", task.ChapterIdx, trunc(cbOuput.Content, 100))
		return ctx
	}
}

func onPrepareNodeStart(
	eventCh chan *adk.AgentEvent) func(context.Context, *callbacks.RunInfo, callbacks.CallbackInput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		if info.Name != "pipeline_prepare" {
			return ctx
		}
		ct := input.(*ChapterTask)
		if ct.DurationMin != 0 {
			return ctx
		}
		pushEvent(eventCh, "由于用户没有指定音频时长，需要根据原文长度%d, 和讲述风格%s, 进行时长预估\n", len(ct.Content), ct.Style)
		return ctx
	}
}

func onPrepareNodeEnd(
	eventCh chan *adk.AgentEvent) func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
	return func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		if info.Name != "pipeline_prepare" {
			return ctx
		}
		ct := output.(map[string]any)
		pushEvent(eventCh, "时长预估结果为%d分钟左右, 字数%d\n", ct["duration_min"], ct["rune_len"])
		return ctx
	}
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
		m = 3
	} else if m > 15 {
		m = 15
	}
	return m
}

func duration2RuneLen(duration int, style string) int {
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
	return speed * duration
}
