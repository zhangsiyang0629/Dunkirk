package main

import (
	"context"
	"dunkirk/internal/agent"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"dunkirk/internal/script"
	"dunkirk/internal/tts"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/redis/go-redis/v9"
)

//var log = logrus.WithField("component", "agent_demo")

func main() {
	ctx := context.Background()
	cfg := config.Load()

	file, _ := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	log.SetOutput(file)

	// 也可以同时输出到文件和终端
	log.SetOutput(io.MultiWriter(os.Stdout, file))

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

	var ttsProvider tts.TTSProvider
	switch cfg.TTSProvider {
	case "azure":
		ttsProvider = tts.NewAzureClient(cfg.AzureSpeechKey, cfg.AzureRegion, cfg.TTSVoice, cfg.AudioDir)
	default:
		ttsProvider = tts.NewWSClient(cfg.TTSVoice, cfg.AudioDir)
	}
	agt, err := agent.New(ctx, cfg, knowledgeBase, ttsProvider, scriptStore)
	if err != nil {
		log.Fatalf("init agent: %v", err)
	}

	input := "测试：你是一位资深三国博主，请生成一段官渡之战的音频，3分钟左右。"
	if len(os.Args) > 1 {
		input = os.Args[1]
	}

	fmt.Printf("用户: %s\n\n", input)
	iter := agt.Run(ctx, input, "你是一位资深三国博主")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			log.Fatalf("agent error: %v", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			mv := event.Output.MessageOutput
			if mv.IsStreaming {
				for {
					chunk, err := mv.MessageStream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Fatalf("stream error: %v", err)
					}
					fmt.Print(chunk.Content)
					log.Printf("[%s] %s\n", event.AgentName, chunk.Content)
				}
				fmt.Println()
			} else {
				if mv.Message != nil && mv.Message.Content != "" {
					fmt.Printf("[%s] %s\n", event.AgentName, mv.Message.Content)
					log.Printf("[%s] %s\n", event.AgentName, mv.Message.Content)
				}
			}
		}
	}
}
