package memory

import (
	"context"
	"dunkirk/internal/config"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestGetMsgs(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()

	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})

	m := NewConversationStore(rdb)
	res, err := m.GetMessages(ctx, "zsy", "c50e6085")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
}
