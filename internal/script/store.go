package script

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) Save(ctx context.Context, userID, bookRef, topic, content string, chapterIdx int, segs []string) (ScriptInfo, error) {

	now := time.Now().UTC().Format(time.RFC3339)
	pipe := s.rdb.Pipeline()
	for idx, seg := range segs {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(seg)))
		pipe.Set(ctx, fmt.Sprintf("script:%s:%s:%s", userID, bookRef, hash), seg, 0)
		pipe.HSet(ctx, fmt.Sprintf("script_info:%s:%s:%s", userID, bookRef, hash),
			"topic", topic,
			"chapter_idx", chapterIdx,
			"created_at", now,
			"segment_idx", idx,
		)
		pipe.SAdd(ctx, fmt.Sprintf("script_set:%s:%s", userID, bookRef), hash)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return ScriptInfo{}, err
	}

	runes := []rune(content)
	preview := content
	if len(runes) > 100 {
		preview = string(runes[:100]) + "..."
	}

	return ScriptInfo{
		Topic:      topic,
		ChapterIdx: chapterIdx,
		CreatedAt:  now,
		Preview:    preview,
	}, nil
}

func (s *Store) List(ctx context.Context, userID string, offset, limit int) ([]ScriptInfo, error) {
	type entry struct {
		hash, bookRef, createdAt string
		chapterIdx, segIdx       int
	}
	var entries []entry

	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, fmt.Sprintf("script_info:%s:*", userID), 200).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			// key = "script_info:{userID}:{bookRef}:{hash}"
			parts := strings.Split(key, ":")
			if len(parts) < 4 {
				continue
			}
			hash := parts[len(parts)-1]
			// 读取 meta，去重（相同 hash 跳过）
			meta, _ := s.rdb.HGetAll(ctx, key).Result()
			if len(meta) == 0 {
				continue
			}
			cidx, _ := strconv.Atoi(meta["chapter_idx"])
			sidx, _ := strconv.Atoi(meta["segment_idx"])
			bookRef := strings.Join(parts[2:len(parts)-1], ":")
			entries = append(entries, entry{hash, bookRef, meta["created_at"], cidx, sidx})
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].bookRef != entries[j].bookRef {
			return entries[i].bookRef < entries[j].bookRef
		}
		if entries[i].chapterIdx != entries[j].chapterIdx {
			return entries[i].chapterIdx < entries[j].chapterIdx
		}
		if entries[i].segIdx != entries[j].segIdx {
			return entries[i].segIdx < entries[j].segIdx
		}
		return entries[i].createdAt > entries[j].createdAt
	})

	start := offset
	if start > len(entries) {
		start = len(entries)
	}
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}

	var result []ScriptInfo
	for _, e := range entries[start:end] {
		info, err := s.Get(ctx, userID, e.bookRef, e.hash)
		if err != nil || info.Topic == "" {
			continue
		}
		info.BookRef = e.bookRef
		result = append(result, info)
	}
	return result, nil
}

func (s *Store) Get(ctx context.Context, userID, bookRef, hash string) (ScriptInfo, error) {
	meta, err := s.rdb.HGetAll(ctx, fmt.Sprintf("script_info:%s:%s:%s", userID, bookRef, hash)).Result()
	if err != nil || len(meta) == 0 {
		return ScriptInfo{}, fmt.Errorf("script not found")
	}

	content, err := s.rdb.Get(ctx, fmt.Sprintf("script:%s:%s:%s", userID, bookRef, hash)).Bytes()
	if err != nil {
		return ScriptInfo{}, err
	}

	contentStr := string(content)
	runes := []rune(contentStr)
	preview := contentStr
	if len(runes) > 100 {
		preview = string(runes[:100]) + "..."
	}

	cidx, _ := strconv.Atoi(meta["chapter_idx"])
	sidx, _ := strconv.Atoi(meta["segment_idx"])

	return ScriptInfo{
		Hash:        hash,
		Topic:       meta["topic"],
		CreatedAt:   meta["created_at"],
		ChapterIdx:  cidx,
		SegmentIndx: sidx,
		Preview:     preview,
		Content:     contentStr,
	}, nil
}

func (s *Store) DeleteByHash(ctx context.Context, userID, bookRef, hash string) error {
	pipe := s.rdb.Pipeline()
	pipe.SRem(ctx, fmt.Sprintf("script_set:%s:%s", userID, bookRef), hash)
	pipe.Del(ctx, fmt.Sprintf("script:%s:%s:%s", userID, bookRef, hash))
	pipe.Del(ctx, fmt.Sprintf("script_info:%s:%s:%s", userID, bookRef, hash))
	_, err := pipe.Exec(ctx)
	return err
}
