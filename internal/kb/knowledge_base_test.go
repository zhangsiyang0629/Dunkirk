package kb

import (
	"context"
	"dunkirk/internal/config"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestSearch(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	knowledgeBase, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()
	relatedDocs, err := knowledgeBase.Search(ctx, "草船借箭", 2, "anonymous", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range relatedDocs {
		title, _ := d.MetaData["title"].(string)
		t.Logf("找到相关文档: %s, %s", title, d.Content)
	}
}

func TestFindBook(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	knowledgeBase, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()
	err = knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇侠传", "uid:sanguozhi")
	err = knowledgeBase.SaveBookNameRef(ctx, "anonymous", "public", "仙剑奇缘", "uid:sanguoyanyi")
	if err != nil {
		t.Fatalf("save book index: %v", err)
	}

	res, err := knowledgeBase.FindBooks(ctx, "anonymous", "仙剑")
	if err != nil {
		t.Fatalf("find books: %v", err)
	}
	t.Logf("res = %v", res)

	res, err = knowledgeBase.FindBooks(ctx, "anonymous", "三国")
	t.Logf("res:%v, err:%v", res, err)
}

func TestGetIndex(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})

	res, err := rdb.FTInfo(ctx, "book_name_ref_index").Result()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
}

func TestSearchDelete(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	kb, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}

	userID := "anonymous_test"
	for i := range 250 {
		kb.SaveBookNameRef(ctx, userID, "public", fmt.Sprintf("bookName_%d", i),
			fmt.Sprintf("uuid:test_%d", i))
	}

	query := fmt.Sprintf("@userID:{%s}", userID)
	res, err := kb.redisClient.FTSearchWithArgs(ctx, "book_name_ref_index", query,
		&redis.FTSearchOptions{
			Limit: 10,
		}).Result()
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	t.Logf("pubDocs len:%d, total: %d", len(res.Docs), res.Total)

	pipe := kb.redisClient.Pipeline()
	var cursor int
	for {
		res, err := kb.redisClient.FTSearchWithArgs(ctx, "book_name_ref_index",
			fmt.Sprintf("@userID:{%s}", userID),
			&redis.FTSearchOptions{
				LimitOffset: cursor,
				Limit:       100,
			}).Result()
		fmt.Println(res.Total)
		if err != nil {
			// 索引不存在或错误则跳过
			break
		}
		if len(res.Docs) == 0 {
			break
		}
		for _, doc := range res.Docs {
			pipe.Del(ctx, doc.ID)
		}
		if len(res.Docs) < 100 {
			break
		}
		cursor += len(res.Docs)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
}
