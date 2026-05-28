package main

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
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

	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	bookName := strings.TrimSuffix(base, ext)
	if existingUUID, _ := knowledgeBase.ResolveBookName(ctx, "anonymous", bookName); existingUUID != "" {
		fmt.Printf("Book '%s' already exists with UUID: %s, skipping processing.",
			bookName, existingUUID)
		return
	}

	refID := uuid.New().String()[:8]
	if err := knowledgeBase.SaveBookNameRef(ctx, "anonymous", bookName, "public", refID); err != nil {
		log.Fatalf("save book name ref: %v", err)
	}

	chapters, err := docproc.ProcessAndStore(ctx, knowledgeBase, filePath, refID, bookName, "anonymous", "private")
	if err != nil {
		log.Fatalf("process: %v", err)
	}
	fmt.Printf("共拆出 %d 章：\n", len(chapters))
	for _, ch := range chapters[:3] {
		fmt.Printf("  %s (%d 字)\n", ch.Title, ch.ContentLen)
	}
	if len(chapters) > 3 {
		fmt.Printf("  ... 共 %d 章\n", len(chapters))
	}
	fmt.Println("\n搜索测试：")
	docs, err := knowledgeBase.Search(ctx, "曹操", 3, "anonymous", "")
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	for _, d := range docs {
		title, _ := d.MetaData["title"].(string)
		fmt.Printf("  找到: %s\n", title)
	}
}
