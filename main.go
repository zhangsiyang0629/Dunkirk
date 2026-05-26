package main

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	knowledgeBase, err := kb.New(ctx, cfg)
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
