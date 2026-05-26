package kb

import (
	"context"
	"dunkirk/internal/config"
	"fmt"

	"github.com/cloudwego/eino-ext/components/embedding/ark"
	redisIndexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisRetriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

const (
	vectorDim = 2048
	keyPrefix = "doc:"
	indexName = "doc_index"
)

const fileIndexPrefix = "file_index:"

type KnowledgeBase struct {
	embedder    *ark.Embedder
	indexer     *redisIndexer.Indexer
	retriever   *redisRetriever.Retriever
	redisClient *redis.Client
}

func New(ctx context.Context, cfg *config.Config) (*KnowledgeBase, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	apiType := ark.APITypeMultiModal
	emb, err := ark.NewEmbedder(ctx, &ark.EmbeddingConfig{
		APIKey:  cfg.ArkAPIKey,
		Model:   cfg.ArkEmbeddingModel,
		APIType: &apiType,
	})
	if err != nil {
		return nil, fmt.Errorf("new embedder: %w", err)
	}
	if err := createIndex(ctx, rdb); err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}
	idx, err := redisIndexer.NewIndexer(ctx, &redisIndexer.IndexerConfig{
		Client:    rdb,
		KeyPrefix: keyPrefix,
		Embedding: emb,
	})
	if err != nil {
		return nil, fmt.Errorf("new indexer: %w", err)
	}
	ret, err := redisRetriever.NewRetriever(ctx, &redisRetriever.RetrieverConfig{
		Client:       rdb,
		Index:        indexName,
		Embedding:    emb,
		TopK:         5,
		ReturnFields: []string{"content", "vector_content", "title", "source"},
	})
	if err != nil {
		return nil, fmt.Errorf("new retriever: %w", err)
	}

	return &KnowledgeBase{
		embedder:    emb,
		indexer:     idx,
		retriever:   ret,
		redisClient: rdb,
	}, nil
}

func (kb *KnowledgeBase) StoreDocuments(ctx context.Context, docs []*schema.Document) ([]string, error) {
	return kb.indexer.Store(ctx, docs)
}
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]*schema.Document, error) {
	return kb.retriever.Retrieve(ctx, query, retriever.WithTopK(topK))
}
func (kb *KnowledgeBase) Close() error {
	return kb.redisClient.Close()
}
func createIndex(ctx context.Context, rdb *redis.Client) error {
	_, err := rdb.FTInfo(ctx, indexName).Result()
	if err == nil {
		return nil
	}
	schemas := []*redis.FieldSchema{
		{FieldName: "content", FieldType: redis.SearchFieldTypeText, Weight: 1},
		{FieldName: "vector_content", FieldType: redis.SearchFieldTypeVector,
			VectorArgs: &redis.FTVectorArgs{
				FlatOptions: &redis.FTFlatOptions{
					Type: "FLOAT32", Dim: vectorDim, DistanceMetric: "COSINE",
				},
			},
		},
	}
	_, err = rdb.FTCreate(ctx, indexName, &redis.FTCreateOptions{
		OnHash: true, Prefix: []any{keyPrefix},
	}, schemas...).Result()
	return err
}

func (kb *KnowledgeBase) HasFile(ctx context.Context, hash string) (bool, error) {
	n, err := kb.redisClient.Exists(ctx, fileIndexPrefix+hash).Result()
	return n > 0, err
}
func (kb *KnowledgeBase) SaveFileIndex(ctx context.Context, hash string, data []byte) error {
	return kb.redisClient.Set(ctx, fileIndexPrefix+hash, data, 0).Err()
}
func (kb *KnowledgeBase) LoadFileIndexRaw(ctx context.Context, hash string) ([]byte, error) {
	return kb.redisClient.Get(ctx, fileIndexPrefix+hash).Bytes()
}
