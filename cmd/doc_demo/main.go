package main

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: go run cmd/doc_demo/main.go <file_path>")
	}
	filePath := os.Args[1]
	ctx := context.Background()
	cfg := config.Load()
	knowledgeBase, err := kb.New(ctx, cfg)
	if err != nil {
		log.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()
	chapters, err := docproc.ProcessAndStore(ctx, knowledgeBase, filePath)
	if err != nil {
		log.Fatalf("process: %v", err)
	}
	fmt.Printf("共拆出 %d 章：\n", len(chapters))
	for _, ch := range chapters[:3] {
		fmt.Printf("  %s (%d 字)\n", ch.Title, len([]rune(ch.Content)))
	}
	if len(chapters) > 3 {
		fmt.Printf("  ... 共 %d 章\n", len(chapters))
	}
	fmt.Println("\n搜索测试：")
	docs, err := knowledgeBase.Search(ctx, "曹操", 3)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	for _, d := range docs {
		title, _ := d.MetaData["title"].(string)
		fmt.Printf("  找到: %s\n", title)
	}
}
