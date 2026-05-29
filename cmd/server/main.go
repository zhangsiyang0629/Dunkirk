package main

import (
	"context"
	"dunkirk/internal/agent"
	"dunkirk/internal/config"
	"dunkirk/internal/handler"
	"dunkirk/internal/kb"
	"dunkirk/internal/pipeline"
	"dunkirk/internal/task"
	"dunkirk/internal/tts"
	"log"
	"os"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/callbacks"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	log.SetOutput(file)

	knowledgeBase, err := kb.New(ctx, cfg)
	if err != nil {
		log.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()
	ttsClient := tts.NewClient(cfg.TTSVoice, cfg.AudioDir)

	maxTokens := 16384
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:    cfg.ArkAPIKey,
		Model:     cfg.ArkChatModel,
		MaxTokens: &maxTokens,
	})
	if err != nil {
		log.Fatalf("new chat model: %v", err)
	}

	agt, err := agent.NewWithChatMode(ctx, cfg, cm, knowledgeBase, ttsClient)
	if err != nil {
		log.Fatalf("init agent: %v", err)
	}

	ghandler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			log.Printf("[Global Start] component=%s name=%s input=%T", info.Component, info.Name, input)
			return ctx
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			log.Printf("[Global End] component=%s name=%s output=%T", info.Component, info.Name, output)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			log.Printf("[Global Error] component=%s name=%s err=%v", info.Component, info.Name, err)
			return ctx
		}).
		Build()

	// Register as global callbacks (applies to all subsequent runs)
	callbacks.AppendGlobalHandlers(ghandler)

	p, err := pipeline.New(ctx, knowledgeBase, cm, ttsClient, cfg.AudioDir)
	if err != nil {
		log.Fatalf("init pipeline: %v", err)
	}

	taskMgr := task.NewManager(agt, p)
	fileStatus := handler.NewFileStatus()
	h := handler.New(taskMgr, knowledgeBase, ttsClient, cfg, cm, fileStatus)
	initParser, err := pipeline.NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		log.Fatalf("init intent parser: %v", err)
	}
	h.SetIntentParser(initParser)
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "kb_ready": true})
	})
	handler.Register(r, h)
	log.Printf("server starting on :%s", cfg.Port)
	r.Run(":" + cfg.Port)
}
