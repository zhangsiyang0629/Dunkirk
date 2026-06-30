package main

import (
	"context"
	"dunkirk/internal/agent"
	"dunkirk/internal/config"
	"dunkirk/internal/handler"
	"dunkirk/internal/kb"
	"dunkirk/internal/memory"
	"dunkirk/internal/pipeline"
	"dunkirk/internal/script"
	"dunkirk/internal/task"
	"dunkirk/internal/tts"
	"log"
	"os"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	log.SetOutput(file)

	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})

	knowledgeBase, err := kb.New(ctx, cfg, rdb)
	if err != nil {
		log.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()

	scriptStore := script.NewStore(rdb)

	ttsProvider := tts.GetTTSProvider(cfg)
	maxTokens := 16384
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:    cfg.ArkAPIKey,
		Model:     cfg.ArkChatModel,
		MaxTokens: &maxTokens,
	})
	if err != nil {
		log.Fatalf("new chat model: %v", err)
	}

	agt, err := agent.NewWithChatMode(ctx, cfg, cm, knowledgeBase, ttsProvider, scriptStore)
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
			if info, ok := compose.ExtractInterruptInfo(err); ok && len(info.InterruptContexts) > 0 {
				log.Printf("[Global Interrupt] ID=%s, Addr=%s",
					info.InterruptContexts[0].ID, info.InterruptContexts[0].Address)
				return ctx
			}
			log.Printf("[Global Error] component=%s name=%s err=%v", info.Component, info.Name, err)
			return ctx
		}).
		Build()

	// Register as global callbacks (applies to all subsequent runs)
	callbacks.AppendGlobalHandlers(ghandler)

	p, err := pipeline.New(ctx, knowledgeBase, cm, ttsProvider, cfg.AudioDir, scriptStore)
	if err != nil {
		log.Fatalf("init pipeline: %v", err)
	}

	convStore := memory.NewConversationStore(rdb)
	profileStore := memory.NewProfileStore(rdb)
	taskMgr := task.NewManager(agt, p, convStore, profileStore)
	fileStatus := handler.NewFileStatus()
	h := handler.New(taskMgr, knowledgeBase, ttsProvider, cfg, cm, fileStatus, scriptStore)
	initParser, err := pipeline.NewIntentParser(ctx, cm, knowledgeBase)
	h.SetConvStore(convStore)
	h.SetProfileStore(profileStore)
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
