package pipeline

import (
	"context"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"dunkirk/internal/script"
	"dunkirk/internal/tts"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func init() {
	schema.Register[*ChapterTask]()
}

type ctxKeyType struct{}

var ctxKeyChapterTask = ctxKeyType{}

type Pipeline struct {
	runnable    compose.Runnable[*ChapterTask, *ChapterTask]
	kb          *kb.KnowledgeBase
	tc          tts.TTSProvider
	outDir      string
	scriptStore *script.Store
}

func New(ctx context.Context,
	knowledgeBase *kb.KnowledgeBase,
	chatModel model.BaseChatModel,
	ttsClient tts.TTSProvider,
	audioDir string,
	scriptStore *script.Store) (*Pipeline, error) {
	streamingMode := &streamingModel{BaseChatModel: chatModel}

	plainChain := newPlainScriptChain(ctx, streamingMode)
	ssmlChain := newSSMLScriptChain(ctx, streamingMode)
	segPlainChain := newSegmentedScriptChain(ctx, streamingMode)

	searchNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
			return searchNodeFunc(ctx, task, knowledgeBase)
		})

	prepareNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (map[string]any, error) {
			return prepareNodeFunc(ctx, knowledgeBase, task)
		})

	extractNode := compose.InvokableLambda(extractNodeFunc)

	ttsNode := compose.InvokableLambda(
		func(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
			return ttsNodeFunc(ctx, task, ttsClient)
		})

	branch := compose.NewGraphBranch(func(ctx context.Context, input map[string]any) (string, error) {
		if input["duration_min"].(int) == 0 {
			return "seg_plain_script", nil
		}
		if input["use_ssml"].(bool) {
			return "ssml_script", nil
		}
		return "plain_script", nil
	}, map[string]bool{"plain_script": true, "ssml_script": true, "seg_plain_script": true})

	saveEndingNode := compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		return saveEndingNodeFunc(ctx, msg, knowledgeBase)
	})

	approvalNode := compose.InvokableLambda(func(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
		return approvalNodeFunc(ctx, task, scriptStore)
	})

	g := compose.NewGraph[*ChapterTask, *ChapterTask]()
	g.AddLambdaNode("search", searchNode, compose.WithNodeName("pipeline_search"))
	g.AddLambdaNode("prepare", prepareNode, compose.WithNodeName("pipeline_prepare"))
	g.AddGraphNode("plain_script", plainChain, compose.WithNodeName("pipeline_plain_script"))
	g.AddGraphNode("seg_plain_script", segPlainChain, compose.WithNodeName("pipeline_seg_plain_script"))
	g.AddGraphNode("ssml_script", ssmlChain, compose.WithNodeName("pipeline_ssml_script"))
	g.AddLambdaNode("extract", extractNode, compose.WithNodeName("pipeline_extract"))
	g.AddLambdaNode("tts", ttsNode, compose.WithNodeName("pipeline_tts"))
	g.AddLambdaNode("save_ending", saveEndingNode, compose.WithNodeName("pipeline_save_ending"))
	g.AddLambdaNode("approval", approvalNode, compose.WithNodeName("pipeline_approval"))

	g.AddEdge(compose.START, "search")
	g.AddEdge("search", "prepare")
	g.AddBranch("prepare", branch)
	g.AddEdge("plain_script", "save_ending") // script → save_ending
	g.AddEdge("ssml_script", "save_ending")
	g.AddEdge("seg_plain_script", "save_ending")
	g.AddEdge("save_ending", "extract") // save_ending → extract
	g.AddEdge("extract", "approval")
	g.AddEdge("approval", "tts")
	g.AddEdge("tts", compose.END)

	runnable, err := g.Compile(ctx, compose.WithCheckPointStore(newInMemoryStore()))
	if err != nil {
		return nil, fmt.Errorf("compile graph: %w", err)
	}
	return &Pipeline{runnable: runnable, tc: ttsClient, outDir: audioDir,
		kb: knowledgeBase, scriptStore: scriptStore}, nil
}

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

	return p.runnable.Stream(
		ctx,
		task,
		optPrepareNode,
		optSerchaeNode,
		optNestedChatMode1,
		optNestedChatMode2,
		compose.WithCheckPointID(task.CheckpointID))
}

func ProcessBook(ctx context.Context,
	p *Pipeline,
	userID, fileRefID, bookName, style string,
	durationMin int,
	chapters []int,
	eventCh chan *adk.AgentEvent,
	useSSML bool,
	checkpointID string,
	resumeCh chan string) ([]*ChapterTask, error) {

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
			ChapterInt:    ch.ChapterInt,
			CheckpointID:  checkpointID,
		}
		log.Printf("[ProcessBook] processing chapter %d/%d", i+1, len(targets))
		result, err := readChapter(ctx, p, task, bookName, eventCh, resumeCh)
		if err != nil {
			pushEvent(eventCh, "第%d章失败: %v", ch.ChapterInt, err)
			log.Printf("[ERROR]chapter %d/%d failed: %s", task.ChapterIdx, len(chapters), err)
			continue
		}

		if result.Error != "" {
			pushEvent(eventCh, "第%d章已跳过: %s", ch.ChapterInt, result.Error)
			log.Printf("[ERROR]chapter %d/%d failed: %s", result.ChapterIdx, len(chapters), result.Error)
			continue
		}

		if len(result.AudioPaths) > 1 {
			pushEvent(eventCh, "第%d章完成，共%d集", ch.ChapterInt, len(result.AudioPaths))
			for i, path := range result.AudioPaths {
				pushEvent(eventCh, "  第%d集: %s", i+1, path)
			}
		} else {
			pushEvent(eventCh, "\n第%d章完成: %s\n", ch.ChapterInt, result.AudioPath)
		}
		log.Printf("chapter %d/%d done: %s", ch.ChapterInt, len(chapters), result.AudioPath)
		results = append(results, result)
	}
	return results, nil
}

func readChapter(ctx context.Context, p *Pipeline, task *ChapterTask, bookName string,
	eventCh chan *adk.AgentEvent, resumeCh chan string) (*ChapterTask, error) {

	log.Printf("[readChapter] ENTER: task.CheckpointID=%s", task.CheckpointID)
	reader, err := p.ProcessChapterStream(ctx, task, bookName, eventCh)
	if err != nil {
		if info, ok := compose.ExtractInterruptInfo(err); ok && len(info.InterruptContexts) > 0 {
			pushInterrupt(eventCh, info.InterruptContexts)
			if reader != nil {
				reader.Close()
			}

			var choice string
			select {
			case choice = <-resumeCh:
			case <-time.After(10 * time.Minute):
				choice = "审核超时"
			}
			ctx = compose.BatchResumeWithData(ctx, map[string]any{
				info.InterruptContexts[0].ID: choice,
			})
			return readChapter(ctx, p, task, bookName, eventCh, resumeCh)
		}
		return task, err
	}
	defer reader.Close()

	for {
		frame, rErr := reader.Recv()
		if errors.Is(rErr, io.EOF) {
			break
		}
		if rErr != nil {
			return nil, rErr
		}
		if frame != nil {
			task = frame
		}
	}
	return task, nil
}

func searchNodeFunc(
	ctx context.Context,
	task *ChapterTask,
	knowledgeBase *kb.KnowledgeBase) (*ChapterTask, error) {

	userID, _ := ctx.Value("userID").(string)
	// 优先按题目精确查询
	if task.FileRefID != "" && isChapterTitle(task.Topic) {
		content, err := knowledgeBase.GetChapterSegments(ctx, task.FileRefID, task.Topic)
		if err != nil {
			return nil, err
		}
		task.Content = content
		task.IsExactSerach = true
		return task, nil
	}

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
}

func prepareNodeFunc(ctx context.Context, knowledgeBase *kb.KnowledgeBase, task *ChapterTask) (map[string]any, error) {
	duration := task.DurationMin
	// if duration == 0 {
	// 	duration = estimateDuration(len(task.Content), task.Style)
	// 	task.DurationMin = duration
	// }
	runeLen := duration2RuneLen(duration, task.Style)

	userID, _ := ctx.Value("userID").(string)
	if task.FileRefID != "" && task.ChapterInt > 1 {
		ending, _ := knowledgeBase.GetChapterEnding(ctx, userID, task.FileRefID, task.ChapterInt)
		task.PrevEnding = ending
	}

	return map[string]any{
		"content":       task.Content,
		"topic":         task.Topic,
		"style":         task.Style,
		"duration_min":  duration,
		"rune_len":      runeLen,
		"use_ssml":      task.UseSSML,
		"prev_ending":   task.PrevEnding,
		"system_prompt": computeSystemPrompt(task.Style),
	}, nil
}

func extractNodeFunc(ctx context.Context, msg *schema.Message) (*ChapterTask, error) {
	task, ok := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
	if !ok {
		return nil, fmt.Errorf("task not found in context")
	}
	task.Script = msg.Content

	parts := strings.Split(msg.Content, "===SEGMENT_BOUNDARY===")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			if i == 0 {
				task.Script = p
			}
			task.ScriptSegments = append(task.ScriptSegments, p)
		}
	}
	return task, nil
}

func ttsNodeFunc(ctx context.Context, task *ChapterTask, ttsClient tts.TTSProvider) (*ChapterTask, error) {
	if task.Error != "" {
		return task, nil // 审核未通过，跳过 TTS
	}
	userID, _ := ctx.Value("userID").(string)
	eventCh, _ := ctx.Value("eventCh").(chan *adk.AgentEvent)

	if len(task.ScriptSegments) <= 1 {
		path, err := ttsClient.TextToSpeech(ctx, task.Script, task.Topic, userID)
		if err != nil {
			return nil, err
		}
		task.AudioPath = path
		return task, nil
	}

	var paths []string
	for i, seg := range task.ScriptSegments {
		filename := fmt.Sprintf("%s_第%d集", task.Topic, i+1)
		path, err := ttsClient.TextToSpeech(ctx, seg, filename, userID)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
		pushEvent(eventCh, "第%d章第%d集生成完成", task.ChapterInt, i+1)
	}
	task.AudioPaths = paths
	task.AudioPath = paths[0]
	return task, nil
}

func saveEndingNodeFunc(ctx context.Context, msg *schema.Message, knowledgeBase *kb.KnowledgeBase) (*schema.Message, error) {
	task, _ := ctx.Value(ctxKeyChapterTask).(*ChapterTask)
	if task == nil || task.FileRefID == "" {
		return msg, nil
	}

	paragraphs := strings.Split(msg.Content, "\n\n")
	var lastTwo []string
	for i := len(paragraphs) - 1; i >= 0 && len(lastTwo) < 2; i-- {
		p := strings.TrimSpace(paragraphs[i])
		if p != "" {
			lastTwo = append([]string{p}, lastTwo...)
		}
	}
	if len(lastTwo) > 0 {
		userID, _ := ctx.Value("userID").(string)
		ending := strings.Join(lastTwo, "\n\n")
		knowledgeBase.SaveChapterEnding(ctx, userID, task.FileRefID, task.ChapterInt, ending)
	}
	return msg, nil
}

func approvalNodeFunc(ctx context.Context, task *ChapterTask, scriptStore *script.Store) (*ChapterTask, error) {
	wasInterrupted, _, interruptedState := compose.GetInterruptState[*ChapterTask](ctx)
	if !wasInterrupted {
		return nil, compose.StatefulInterrupt(ctx, map[string]any{
			"question": "请审核以下脚本",
			"options":  []string{"同意", "拒绝", "拒绝但保留脚本"},
			"type":     "script_review",
		}, task)
	}
	userID, _ := ctx.Value("userID").(string)
	isTarget, hasData, data := compose.GetResumeContext[string](ctx)
	task = interruptedState
	if isTarget && hasData {
		switch data {
		case "同意":
			scriptStore.Save(ctx, userID, task.FileRefID, task.Topic, task.Script,
				task.ChapterInt, task.ScriptSegments)
			return task, nil
		case "拒绝":
			task.Error = "脚本审核未通过"
			return task, nil
		case "拒绝但保留脚本":
			scriptStore.Save(ctx, userID, task.FileRefID, task.Topic, task.Script,
				task.ChapterInt, task.ScriptSegments)
			task.Error = "脚本审核未通过（脚本文案已保留）"
			return task, nil
		case "审核超时":
			scriptStore.Save(ctx, userID, task.FileRefID, task.Topic, task.Script,
				task.ChapterInt, task.ScriptSegments)
			task.Error = "审核超时，拒绝生成（脚本文案已保留）"
			return task, nil
		}
	}
	return nil, compose.StatefulInterrupt(ctx, map[string]any{
		"question": "请审核以下脚本",
		"options":  []string{"同意", "拒绝", "拒绝但保留脚本"},
		"type":     "script_review",
		//"script_preview": task.Script,
	}, *task)
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
		searchType := "模糊查询"
		if ct.IsExactSerach {
			searchType = "精确查询"
		}
		pushEvent(eventCh, "完成第%d章的知识库搜索\n搜索关键:%s\nfileRefID:%s\n书名:%s\n搜索类型:%s\n搜索结果:%s\n",
			task.ChapterIdx, ct.Topic, ct.FileRefID, bookName, searchType, trunc(ct.Content, 100))
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

var chapterTitleRegex = regexp.MustCompile(`第[一二三四五六七八九十百零\d]+[回章节部集话]`)

func isChapterTitle(s string) bool {
	return chapterTitleRegex.MatchString(s)
}

func computeSystemPrompt(style string) string {
	if strings.Contains(style, "小朋友") || strings.Contains(style, "儿童") || strings.Contains(style, "小孩") {
		return `你是专业的有声读物编剧，擅长将枯燥的历史故事改编成7岁小朋友爱听的生动故事。

## 改编原则（必须遵守）
1. **拒绝复读机**：不要简单换一种说法复述原文。请在不违背历史情节的前提下，
   大胆补充人物的对话、心理活动、动作细节。
2. **丰富感官描写**：加入声音（啊呀呀！叮叮当当！咚咚咚！）、表情（瞪圆了眼睛、咧开嘴笑）、
   动作细节（握紧了拳头、擦了擦汗）。
3. **扩展关键场景**：原文一句话带过的战斗、冲突，你要展开成3-5句。
4. **口语化表达**：用"哇塞"、"哎呀"、"太厉害了"等口语。多用拟声词和感叹词。
5. **用词规范**：小孩听不懂的词要解释。比如"檄文"改成"一封很厉害的信"。
6. **大胆扩展**：你的输出长度**至少**是原文的 **1.5倍**。
7. **上文承接**：当用户没有提供上一回结尾时，不用机械强行进行衔接
8. 禁止任何剧本格式、列表、序号

## 节奏感原则（非常重要）
1. **拟声词只用在关键时刻**：只在战斗、冲突、情绪爆发等高潮场景使用拟声词。平淡的叙述（走路、说话、思考）不要加。
2. **高潮场景详细写，过渡场景简单写**：
   - 高潮场景（战斗、对决、重要对话）：进行适当的扩写。
   - 过渡场景（行军、铺垫、日常）：简单带过即可，不要过度渲染。
3. **避免每句都加感叹词**："啊"、"呀"、"哇塞" 这类感叹词，只在人物情绪波动时使用，不要句句都加。

## 必须在每个场景中做到的三件事（逐句检查）
1. **每 100 字至少 1 个拟声词**：战斗用“咚咚咚”“嗖嗖嗖”“轰隆隆”“噼里啪啦”等等，
   人物动作用“啪嗒啪嗒”“咕噜咕噜”“呼哧呼哧”等等。
2. **每个关键人物至少 1 次内心独白**：被骗时想什么？中计后想什么？
   用引号把心里话写出来，比如“完了完了，上当了！”
3. **每个高潮瞬间至少 1 个特写镜头**：不要只写“他冲过去”，要写
   “他咬紧了牙关，眼睛瞪得像铜铃，汗珠从额头滚下来，大吼一声冲了过去”。

## 改写示例
原文："张飞大喝一声，冲入敌阵。"
合格改写："张飞深吸一口气，用足全身力气大喝一声：'啊呀呀呀呀呀呀！'
   他瞪着铜铃大的眼睛，胡子一根根竖起来，手提丈八蛇矛，像一头下山猛虎，
   冲入敌阵！"
`
	}
	// 默认 prompt
	return "你是一位专业的有声读物编剧，擅长将各类内容转化为生动、吸引人的音频脚本。"
}
