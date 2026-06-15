package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func (c *ConversationStore) GetOrCreateConversation(ctx context.Context, userID, convID string) (*Conversation, error) {
	key := fmt.Sprintf("conv:%s:%s", userID, convID)
	exists, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if exists > 0 {
		return c.GetConversation(ctx, userID, convID)
	}
	now := time.Now()
	conv := &Conversation{
		ID:        convID,
		UserID:    userID,
		Title:     fmt.Sprintf("对话 %s", now.Format("01-02 15:04")),
		CreatedAt: now,
		UpdatedAt: now,
	}
	data, _ := json.Marshal(conv)
	if err := c.rdb.Set(ctx, key, data, 0).Err(); err != nil {
		return nil, err
	}
	listKey := fmt.Sprintf("user:%s:convs", userID)
	c.rdb.ZAdd(ctx, listKey, redis.Z{Score: float64(now.Unix()), Member: convID})
	return conv, nil
}

type ConversationStore struct {
	rdb redis.UniversalClient
}

func NewConversationStore(rdb redis.UniversalClient) *ConversationStore {
	return &ConversationStore{rdb: rdb}
}

func (c *ConversationStore) CreateConversation(ctx context.Context, userID string) (*Conversation, error) {
	convID := uuid.New().String()[:8]
	now := time.Now()
	conv := &Conversation{
		ID:        convID,
		UserID:    userID,
		Title:     fmt.Sprintf("对话 %s", now.Format("01-02 15:04")),
		CreatedAt: now,
		UpdatedAt: now,
	}
	key := fmt.Sprintf("conv:%s:%s", userID, convID)
	data, _ := json.Marshal(conv)
	if err := c.rdb.Set(ctx, key, data, 0).Err(); err != nil {
		return nil, err
	}
	listKey := fmt.Sprintf("user:%s:convs", userID)
	c.rdb.ZAdd(ctx, listKey, redis.Z{Score: float64(now.Unix()), Member: convID})
	return conv, nil
}

func (c *ConversationStore) GetConversation(ctx context.Context, userID, convID string) (*Conversation, error) {
	key := fmt.Sprintf("conv:%s:%s", userID, convID)
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, err
	}
	return &conv, nil
}

func (c *ConversationStore) ListConversations(ctx context.Context, userID string) ([]*Conversation, error) {
	listKey := fmt.Sprintf("user:%s:convs", userID)
	ids, err := c.rdb.ZRevRange(ctx, listKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	convs := make([]*Conversation, 0, len(ids))
	for _, id := range ids {
		conv, err := c.GetConversation(ctx, userID, id)
		if err == nil {
			convs = append(convs, conv)
		}
	}
	return convs, nil
}

func (c *ConversationStore) AppendMessage(ctx context.Context, userID, convID string, msg *Message) error {
	msgKey := fmt.Sprintf("conv:%s:%s:msgs", userID, convID)
	data, _ := json.Marshal(msg)
	return c.rdb.RPush(ctx, msgKey, data).Err()
}

func (c *ConversationStore) GetMessages(ctx context.Context, userID, convID string) ([]*Message, error) {
	key := fmt.Sprintf("conv:%s:%s:msgs", userID, convID)
	data, err := c.rdb.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	msgs := make([]*Message, 0, len(data))
	for _, d := range data {
		var msg Message
		if json.Unmarshal([]byte(d), &msg) == nil {
			msgs = append(msgs, &msg)
		}
	}
	return msgs, nil
}

func (c *ConversationStore) GetRecentMessages(ctx context.Context, userID, convID string, n int) ([]*Message, error) {
	key := fmt.Sprintf("conv:%s:%s:msgs", userID, convID)
	data, err := c.rdb.LRange(ctx, key, int64(-n), -1).Result()
	if err != nil {
		return nil, err
	}
	msgs := make([]*Message, 0, len(data))
	for _, d := range data {
		var msg Message
		if json.Unmarshal([]byte(d), &msg) == nil {
			msgs = append(msgs, &msg)
		}
	}
	return msgs, nil
}

func (c *ConversationStore) GetRecentMessagesWithBudget(ctx context.Context, userID, convID string, budget int) ([]*Message, error) {
	key := fmt.Sprintf("conv:%s:%s:msgs", userID, convID)
	total, err := c.rdb.LLen(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var msgs []*Message
	used := 0

	for i := total - 1; i >= 0; i-- {
		data, err := c.rdb.LIndex(ctx, key, i).Bytes()
		if err != nil {
			continue
		}
		var msg Message
		if json.Unmarshal(data, &msg) != nil {
			continue
		}

		tok := estimateTokens(msg.Content)
		if used+tok > budget {
			break
		}
		used += tok
		msgs = append([]*Message{&msg}, msgs...)
	}
	return msgs, nil
}

func (c *ConversationStore) AppendGeneration(ctx context.Context, userID, convID string, record *GenerationRecord) error {
	key := fmt.Sprintf("conv:%s:%s:gens", userID, convID)
	data, _ := json.Marshal(record)
	return c.rdb.RPush(ctx, key, data).Err()
}

func (c *ConversationStore) GetRecentGenerations(ctx context.Context, userID, convID string, n int) ([]*GenerationRecord, error) {
	key := fmt.Sprintf("conv:%s:%s:gens", userID, convID)
	data, err := c.rdb.LRange(ctx, key, int64(-n), -1).Result()
	if err != nil {
		return nil, err
	}
	records := make([]*GenerationRecord, 0, len(data))
	for _, d := range data {
		var r GenerationRecord
		if json.Unmarshal([]byte(d), &r) == nil {
			records = append(records, &r)
		}
	}
	return records, nil
}

func (c *ConversationStore) SaveSummary(ctx context.Context, userID, convID, summary string) error {
	key := fmt.Sprintf("conv:%s:%s:summary", userID, convID)
	return c.rdb.Set(ctx, key, summary, 0).Err()
}

func (c *ConversationStore) GetSummary(ctx context.Context, userID, convID string) (string, error) {
	key := fmt.Sprintf("conv:%s:%s:summary", userID, convID)
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func estimateTokens(s string) int {
	runes := []rune(s)
	ascii, nonAscii := 0, 0
	for _, r := range runes {
		if r < 128 {
			ascii++
		} else {
			nonAscii++
		}
	}
	return nonAscii/2 + ascii/4
}
