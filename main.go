package main

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})
	knowledgeBase, err := kb.New(ctx, cfg, rdb)
	if err != nil {
		log.Fatalf("init knowledge base: %v", err)
	}
	defer knowledgeBase.Close()
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "kb_ready": true})
	})
	log.Printf("server starting on :%s", cfg.Port)
	r.Run(":" + cfg.Port)
}
