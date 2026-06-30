package memory

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type ProfileStore struct {
	rdb redis.UniversalClient
}

func NewProfileStore(rdb redis.UniversalClient) *ProfileStore {
	return &ProfileStore{rdb: rdb}
}

func (p *ProfileStore) Save(ctx context.Context, userID string, profile *UserProfile) error {
	key := fmt.Sprintf("user:%s:profile", userID)
	return p.rdb.HSet(ctx, key, map[string]any{
		"preferred_style": profile.PreferredStyle,
		"last_book_name":  profile.LastBookName,
		"last_book_ref":   profile.LastBookRef,
	}).Err()
}

func (p *ProfileStore) Get(ctx context.Context, userID string) (*UserProfile, error) {
	key := fmt.Sprintf("user:%s:profile", userID)
	data, err := p.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &UserProfile{}, nil
	}
	return &UserProfile{
		PreferredStyle: data["preferred_style"],
		LastBookName:   data["last_book_name"],
		LastBookRef:    data["last_book_ref"],
	}, nil
}

func (p *ProfileStore) SaveField(ctx context.Context, userID, field, value string) error {
	key := fmt.Sprintf("user:%s:profile", userID)
	return p.rdb.HSet(ctx, key, field, value).Err()
}
