package script

import (
	"context"
	"dunkirk/internal/config"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestList(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()

	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})
	st := NewStore(rdb)
	res, err := st.List(ctx, "zsy", 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
}
