package docproc

import (
	"context"
	"dunkirk/internal/kb"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

func GetBriefChapters(ctx context.Context, knowledgeBase *kb.KnowledgeBase, refID string) ([]kb.BriefChapter, error) {
	ref, err := knowledgeBase.ResolveBookRef(ctx, refID)
	if err != nil {
		return nil, err
	}
	if ref != nil {
		return ref.BriefChapters, nil
	}
	return []kb.BriefChapter{}, nil
}

/*
是否重复向量化应该在该函数调用之前判定
*/
func ProcessAndStore(
	ctx context.Context,
	knowledgeBase *kb.KnowledgeBase,
	bookName, filePath, refID, userID, visibility string) ([]kb.BriefChapter, error) {

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
					"book_ref":       refID,
					"user_id":        userID,
					"visibility":     visibility,
				},
			})
		}
	}
	ids, err := knowledgeBase.StoreDocuments(ctx, docs, visibility)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	briefChapers := make([]kb.BriefChapter, len(chapters))
	for i := range chapters {
		chapters[i].Index = i + 1
		briefChapers[i] = kb.NewBriefChapter(chapters[i].Title,
			chapters[i].Index, len(chapters[i].Content))
	}
	_ = ids

	if err := knowledgeBase.SaveBookRef(ctx, bookName, refID,
		userID, visibility, briefChapers); err != nil {
		return nil, fmt.Errorf("save file index: %w", err)
	}
	return briefChapers, nil
}
