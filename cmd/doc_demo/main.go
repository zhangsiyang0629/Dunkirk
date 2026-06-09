package main

import (
	"context"
	"crypto/sha256"
	"dunkirk/internal/config"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: go run cmd/doc_demo/main.go <file_path> <query>")
	}
	filePath := os.Args[1]
	query := os.Args[2]
	ctx := context.Background()
	cfg := config.Load()

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("open file err: %v", err)
	}
	defer file.Close()

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

	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	bookName := strings.TrimSuffix(base, ext)

	duplicated := false
	refID := uuid.New().String()[:8]
	if existingUUID, _ := knowledgeBase.ResolveBookName(ctx, "anonymous", bookName); existingUUID != "" {
		fmt.Printf("Book '%s' already exists with UUID: %s, skipping processing.",
			bookName, existingUUID)
		duplicated = true
		refID = existingUUID
	}

	if err := knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", bookName, refID); err != nil {
		log.Fatalf("save book name ref: %v", err)
		return
	}

	if !duplicated {
		hasher := sha256.New()
		ext := filepath.Ext(filePath)
		tmpFile := filepath.Join(cfg.UploadDir, uuid.New().String()+ext)
		os.MkdirAll(cfg.UploadDir, 0755)
		out, _ := os.Create(tmpFile)
		tee := io.TeeReader(file, hasher)
		io.Copy(out, tee)
		out.Close()
		hash := fmt.Sprintf("%x", hasher.Sum(nil))
		hashPath := filepath.Join(cfg.UploadDir, hash+ext)
		os.Rename(tmpFile, hashPath)
		fmt.Println("\n开始解析文件并向量化")
		chapters, err := docproc.ProcessAndStore(ctx, knowledgeBase, bookName, filePath, refID, "anonymous", "private")
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
	}

	fmt.Println("\n搜索测试：")
	docs, err := knowledgeBase.Search(ctx, query, 3, "anonymous", refID)
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	for _, d := range docs {
		title, _ := d.MetaData["title"].(string)
		fmt.Printf("  找到: %s，内容: %s\n", title, d.Content)
	}
}
