package pipeline

import (
	"context"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"dunkirk/internal/tts"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ctxKeyType struct{}

var ctxKeyChapterTask = ctxKeyType{}

type Pipeline struct {
	runnable compose.Runnable[*ChapterTask, *ChapterTask]
	kb       *kb.KnowledgeBase
	tc       *tts.Client
	outDir   string
}

func New(ctx context.Context,
	knowledgeBase *kb.KnowledgeBase,
	chatModel model.BaseChatModel,
	ttsClient *tts.Client,
	audioDir string) (*Pipeline, error) {
	scriptRunnable := newScriptChain(ctx, chatModel)
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
			return map[string]any{
				"content":      task.Content,
				"topic":        task.Topic,
				"style":        task.Style,
				"duration_min": task.DurationMin,
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
			filename := fmt.Sprintf("chapter_%d", task.ChapterIdx)
			path, err := ttsClient.TextToSpeech(ctx, task.Script, filename)
			if err != nil {
				return nil, err
			}
			task.AudioPath = path
			return task, nil
		})

	g := compose.NewGraph[*ChapterTask, *ChapterTask]()
	g.AddLambdaNode("search", searchNode, compose.WithNodeName("pipeline_search"))
	g.AddLambdaNode("prepare", prepareNode, compose.WithNodeName("pipeline_prepare"))
	g.AddGraphNode("script", scriptRunnable, compose.WithNodeName("pipeline_script"))
	g.AddLambdaNode("extract", extractNode, compose.WithNodeName("pipeline_extract"))
	g.AddLambdaNode("tts", ttsNode, compose.WithNodeName("pipeline_tts"))
	g.AddEdge(compose.START, "search")
	g.AddEdge("search", "prepare")
	g.AddEdge("prepare", "script")
	g.AddEdge("script", "extract")
	g.AddEdge("extract", "tts")
	g.AddEdge("tts", compose.END)

	runnable, err := g.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile graph: %w", err)
	}
	return &Pipeline{runnable: runnable, tc: ttsClient, outDir: audioDir, kb: knowledgeBase}, nil
}

func (p *Pipeline) ProcessChapter(ctx context.Context, task *ChapterTask) (*ChapterTask, error) {
	ctx = context.WithValue(ctx, ctxKeyChapterTask, task)
	return p.runnable.Invoke(ctx, task)
}

func ProcessBook(ctx context.Context,
	p *Pipeline,
	userID, fileRefID, bookName, style string,
	chapters []int) ([]*ChapterTask, error) {
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
			DurationMin:   0,
			ChapterIdx:    i + 1,
			FileRefID:     fileRefID,
			TotalChapters: len(chapters),
		}
		result, err := p.ProcessChapter(ctx, task)
		if err != nil {
			result = task
			result.Error = err.Error()
		}
		results = append(results, result)
		log.Printf("chapter %d/%d done: %s", i+1, len(chapters), result.AudioPath)
	}
	return results, nil
}
