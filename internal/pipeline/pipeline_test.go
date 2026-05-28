package pipeline

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"fmt"
	"log"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/compose"
)

func TestIntentParse(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	knowledgeBase, err := kb.New(ctx, cfg)
	if err != nil {
		log.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	input := "生成1到3章的内容，适合7岁小朋友听"
	result, err := initParser.Invoke(ctx, input)
	if err != nil {
		t.Fatalf("invoke intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}

func TestInterrup(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	knowledgeBase, err := kb.New(ctx, cfg)
	if err != nil {
		log.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	input := "生成三国1到3章的内容，适合7岁小朋友听"
	checkPointID := "test-checkpoint-1"
	_, err = initParser.Invoke(ctx, input, compose.WithCheckPointID(checkPointID))
	fmt.Printf("first invoke error: %v\n", err)
	assert.Error(t, err)
	interruptInfo, isInterrupt := compose.ExtractInterruptInfo(err)
	assert.True(t, isInterrupt)
	assert.NotNil(t, interruptInfo)

	interruptContexts := interruptInfo.InterruptContexts
	nctx := compose.BatchResumeWithData(ctx, map[string]any{
		interruptContexts[0].ID: "三国演义",
	})
	result, err := initParser.Invoke(nctx, "", compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("resume intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}
