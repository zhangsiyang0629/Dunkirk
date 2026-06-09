package script

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) Save(ctx context.Context, userID, bookRef, topic, content string) error {
	key := scriptKey(userID, bookRef, topic)
	return s.rdb.Set(ctx, key, content, 0).Err()
}

func (s *Store) Load(ctx context.Context, userID, bookRef, topic string) (string, error) {
	key := scriptKey(userID, bookRef, topic)
	data, err := s.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) Exists(ctx context.Context, userID, bookRef, topic string) (bool, error) {
	key := scriptKey(userID, bookRef, topic)
	n, err := s.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (s *Store) Delete(ctx context.Context, userID, bookRef, topic string) error {
	key := scriptKey(userID, bookRef, topic)
	return s.rdb.Del(ctx, key).Err()
}

func scriptKey(userID, bookRef, topic string) string {
	hash := sha256.Sum256([]byte(topic))
	return fmt.Sprintf("script:%s:%s:%x", userID, bookRef, hash)
}
