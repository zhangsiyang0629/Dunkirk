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
	"github.com/redis/go-redis/v9"
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

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	input := "生成1到3章的内容，适合7岁小朋友听"
	result, err := initParser.Invoke(ctx, map[string]any{"user_input": input})
	if err != nil {
		t.Fatalf("invoke intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}

func TestBookInterrup(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

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

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇侠传", "uid:sanguozhi")
	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇缘", "uid:sanguoyanyi")

	input := "生成仙剑1到3章的内容，适合7岁小朋友听"
	checkPointID := "test-checkpoint-1"
	ctx = context.WithValue(ctx, "userID", "anonymous")
	_, err = initParser.Invoke(ctx, map[string]any{"user_input": input}, compose.WithCheckPointID(checkPointID))
	fmt.Printf("first invoke error: %v\n", err)
	assert.Error(t, err)
	interruptInfo, isInterrupt := compose.ExtractInterruptInfo(err)
	assert.True(t, isInterrupt)
	assert.NotNil(t, interruptInfo)

	interruptContexts := interruptInfo.InterruptContexts
	nctx := compose.BatchResumeWithData(ctx, map[string]any{
		interruptContexts[0].ID: "仙剑奇侠传",
	})
	result, err := initParser.Invoke(nctx, map[string]any{}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("resume intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}

func TestGenerateGiveUpInterrup(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

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

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇侠传", "uid:sanguozhi")
	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇缘", "uid:sanguoyanyi")

	input := "生成圣斗士1到3章的内容，适合7岁小朋友听"
	checkPointID := "test-checkpoint-1"
	ctx = context.WithValue(ctx, "userID", "anonymous")
	_, err = initParser.Invoke(ctx, map[string]any{"user_input": input}, compose.WithCheckPointID(checkPointID))
	fmt.Printf("first invoke error: %v\n", err)
	assert.Error(t, err)
	interruptInfo, isInterrupt := compose.ExtractInterruptInfo(err)
	assert.True(t, isInterrupt)
	assert.NotNil(t, interruptInfo)

	interruptContexts := interruptInfo.InterruptContexts
	nctx := compose.BatchResumeWithData(ctx, map[string]any{
		interruptContexts[0].ID: GIVEUP,
	})
	result, err := initParser.Invoke(nctx, map[string]any{}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("resume intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}

func TestGenerateGenerateInterrup(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

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

	initParser, err := NewIntentParser(ctx, cm, knowledgeBase)
	if err != nil {
		t.Fatalf("new intent parser: %v", err)
	}

	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇侠传", "uid:sanguozhi")
	knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇缘", "uid:sanguoyanyi")

	input := "生成圣斗士1到3章的内容，适合7岁小朋友听"
	checkPointID := "test-checkpoint-1"
	ctx = context.WithValue(ctx, "userID", "anonymous")
	_, err = initParser.Invoke(ctx, map[string]any{"user_input": input}, compose.WithCheckPointID(checkPointID))
	fmt.Printf("first invoke error: %v\n", err)
	assert.Error(t, err)
	interruptInfo, isInterrupt := compose.ExtractInterruptInfo(err)
	assert.True(t, isInterrupt)
	assert.NotNil(t, interruptInfo)

	interruptContexts := interruptInfo.InterruptContexts
	nctx := compose.BatchResumeWithData(ctx, map[string]any{
		interruptContexts[0].ID: GENERATE_LLM,
	})
	result, err := initParser.Invoke(nctx, map[string]any{}, compose.WithCheckPointID(checkPointID))
	if err != nil {
		t.Fatalf("resume intent parser: %v", err)
	}
	t.Logf("result = %#v", result)
}
