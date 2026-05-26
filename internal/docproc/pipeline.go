package docproc

import (
	"context"
	"crypto/sha256"
	"dunkirk/internal/kb"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

func ProcessAndStore(ctx context.Context, knowledgeBase *kb.KnowledgeBase, filePath string) ([]Chapter, error) {
	// 计算文件哈希
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	// 检测是否已处理过
	exists, err := knowledgeBase.HasFile(ctx, hash)
	if err != nil {
		return nil, err
	}
	if exists {
		raw, err := knowledgeBase.LoadFileIndexRaw(ctx, hash)
		if err != nil {
			return nil, err
		}
		var chapters []Chapter
		if err := json.Unmarshal(raw, &chapters); err != nil {
			return nil, err
		}
		log.Printf("file already processed, cached chapters: %d", len(chapters))
		return chapters, nil
	}

	doc, err := LoadDocument(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	chapters := SplitByChapters(CleanMarkitdownOutput(doc.Content))
	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapters found")
	}
	var docs []*schema.Document
	for _, ch := range chapters {
		segs := SplitByParagraphs(ch.Content, 1000)
		for _, seg := range segs {
			docs = append(docs, &schema.Document{
				ID:      uuid.New().String(),
				Content: ch.Title + "\n" + seg.Content, // ← 标题进向量
				MetaData: map[string]any{
					"title":          ch.Title,
					"index":          ch.Index,
					"segment_index":  seg.Index,
					"total_segments": len(segs),
					"source":         filePath,
				},
			})
		}
	}
	ids, err := knowledgeBase.StoreDocuments(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	for i := range chapters {
		chapters[i].Index = i + 1
	}
	_ = ids

	chaptersJSON, _ := json.Marshal(chapters)
	if err := knowledgeBase.SaveFileIndex(ctx, hash, chaptersJSON); err != nil {
		return nil, fmt.Errorf("save file index: %w", err)
	}

	return chapters, nil
}
