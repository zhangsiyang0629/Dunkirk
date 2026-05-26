package kb

import (
	"context"
	"dunkirk/internal/config"
	"testing"
)

func TestSearch(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	knowledgeBase, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()
	relatedDocs, _ := knowledgeBase.Search(ctx, "草船借箭", 2)
	for _, d := range relatedDocs {
		title, _ := d.MetaData["title"].(string)
		t.Logf("找到相关文档: %s, %s", title, d.Content)
	}
}
