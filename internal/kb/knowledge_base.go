package kb

import (
	"context"
	"dunkirk/internal/config"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/embedding/ark"
	redisIndexer "github.com/cloudwego/eino-ext/components/indexer/redis"
	redisRetriever "github.com/cloudwego/eino-ext/components/retriever/redis"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

const (
	vectorDim        = 2048
	bookIndexPrefix  = "book_index:"
	publicPrefix     = "doc:public:"
	privatePrefix    = "doc:private:"
	publicIndex      = "doc_index_public"
	privateIndex     = "doc_index_private"
	bookNameRefIndex = "book_name_ref_index"
)

type BriefChapter struct {
	Title      string `json:"title"`
	Index      int    `json:"index"`
	ContentLen int    `json:"contentLen"`
}

func NewBriefChapter(title string, index int, contentLen int) BriefChapter {
	return BriefChapter{
		Title:      title,
		Index:      index,
		ContentLen: contentLen,
	}
}

type bookRef struct {
	BookName      string         `json:"book_name"`
	Visibility    string         `json:"visibility"`
	UserID        string         `json:"user_id"`
	FilePath      string         `json:"filePath"`
	BriefChapters []BriefChapter `json:"briefChapters"` // bref chapters
}

type KnowledgeBase struct {
	embedder         *ark.Embedder
	indexerPublic    *redisIndexer.Indexer
	indexerPrivate   *redisIndexer.Indexer
	retrieverPublic  *redisRetriever.Retriever
	retrieverPrivate *redisRetriever.Retriever
	redisClient      *redis.Client
}

func New(ctx context.Context, cfg *config.Config) (*KnowledgeBase, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping error: %w", err)
	}

	apiType := ark.APITypeMultiModal
	emb, err := ark.NewEmbedder(ctx, &ark.EmbeddingConfig{
		APIKey:  cfg.ArkAPIKey,
		Model:   cfg.ArkEmbeddingModel,
		APIType: &apiType,
	})
	if err != nil {
		return nil, fmt.Errorf("new embedder error: %w", err)
	}

	if err := createIndex(ctx, rdb, publicIndex, publicPrefix); err != nil {
		return nil, fmt.Errorf("create public index error: %w", err)
	}
	if err := createIndex(ctx, rdb, privateIndex, privatePrefix); err != nil {
		return nil, fmt.Errorf("create private index error: %w", err)
	}
	if err := createBookRefIndex(ctx, rdb, bookNameRefIndex, bookIndexPrefix); err != nil {
		return nil, fmt.Errorf("create book ref index error: %w", err)
	}

	idxPublic, err := redisIndexer.NewIndexer(ctx, &redisIndexer.IndexerConfig{
		Client:    rdb,
		KeyPrefix: publicPrefix,
		Embedding: emb,
	})
	if err != nil {
		return nil, fmt.Errorf("new public indexer error: %w", err)
	}
	idxPrivate, err := redisIndexer.NewIndexer(ctx, &redisIndexer.IndexerConfig{
		Client:    rdb,
		KeyPrefix: privatePrefix,
		Embedding: emb,
	})
	if err != nil {
		return nil, fmt.Errorf("new private indexer error: %w", err)
	}

	retPublic, err := redisRetriever.NewRetriever(ctx, &redisRetriever.RetrieverConfig{
		Client:    rdb,
		Index:     publicIndex,
		Embedding: emb,
		TopK:      5,
	})
	if err != nil {
		return nil, fmt.Errorf("new public retriever error: %w", err)
	}
	retPrivate, err := redisRetriever.NewRetriever(ctx, &redisRetriever.RetrieverConfig{
		Client:    rdb,
		Index:     privateIndex,
		Embedding: emb,
		TopK:      5,
	})
	if err != nil {
		return nil, fmt.Errorf("new private retriever error: %w", err)
	}

	return &KnowledgeBase{
		embedder:         emb,
		indexerPublic:    idxPublic,
		indexerPrivate:   idxPrivate,
		retrieverPublic:  retPublic,
		retrieverPrivate: retPrivate,
		redisClient:      rdb,
	}, nil
}

func createIndex(ctx context.Context, rdb *redis.Client, indexName, keyPrefix string) error {
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
		{FieldName: "user_id", FieldType: redis.SearchFieldTypeTag},
		{FieldName: "visibility", FieldType: redis.SearchFieldTypeTag},
		{FieldName: "book_ref", FieldType: redis.SearchFieldTypeTag},
	}

	_, err = rdb.FTCreate(ctx, indexName, &redis.FTCreateOptions{
		OnHash: true, Prefix: []any{keyPrefix},
	}, schemas...).Result()
	return err
}

func createBookRefIndex(ctx context.Context, rdb *redis.Client, indexName, keyPrefix string) error {
	_, err := rdb.FTInfo(ctx, indexName).Result()
	if err == nil {
		return nil
	}

	schemas := []*redis.FieldSchema{
		{FieldName: "userID", FieldType: redis.SearchFieldTypeTag},
		{FieldName: "visibility", FieldType: redis.SearchFieldTypeTag},
		{FieldName: "bookName", FieldType: redis.SearchFieldTypeText},
	}

	_, err = rdb.FTCreate(ctx, indexName, &redis.FTCreateOptions{
		OnHash: true, Prefix: []any{keyPrefix},
	}, schemas...).Result()
	return err
}

func (kb *KnowledgeBase) StoreDocuments(
	ctx context.Context,
	docs []*schema.Document,
	visibility string) ([]string, error) {
	switch visibility {
	case "private":
		return kb.indexerPrivate.Store(ctx, docs)
	default:
		return kb.indexerPublic.Store(ctx, docs)
	}
}

// bookRef是可以为空的
func (kb *KnowledgeBase) Search(
	ctx context.Context,
	query string,
	topK int,
	userID string,
	bookRef string) ([]*schema.Document, error) {
	privFilter := fmt.Sprintf("@user_id:{%s}", userID)
	if bookRef != "" {
		privFilter += fmt.Sprintf(" @book_ref:{%s}", bookRef)
	}
	docs, _ := kb.retrieverPrivate.Retrieve(ctx, query,
		retriever.WithTopK(topK), redisRetriever.WithFilterQuery(privFilter))
	if len(docs) == 0 {
		var pubFilter string
		if bookRef != "" {
			pubFilter = fmt.Sprintf("@book_ref:{%s}", bookRef)
		}
		fmt.Println(pubFilter)
		pubDocs, _ := kb.retrieverPublic.Retrieve(ctx, query,
			retriever.WithTopK(topK), redisRetriever.WithFilterQuery(pubFilter))
		docs = append(docs, pubDocs...)
	}
	return docs, nil
}

/*
data应该是chapter的摘要信息
*/
func (kb *KnowledgeBase) SaveBookRef(
	ctx context.Context,
	bookName, uuid, userId, visibility string,
	briefChapters []BriefChapter) error {
	data, _ := json.Marshal(bookRef{
		BookName:      bookName,
		UserID:        userId,
		Visibility:    visibility,
		BriefChapters: briefChapters,
	})
	return kb.redisClient.Set(ctx, "book_ref:"+uuid, data, 0).Err()
}

func (kb *KnowledgeBase) ResolveBookRef(ctx context.Context, uuid string) (*bookRef, error) {
	data, err := kb.redisClient.Get(ctx, "book_ref:"+uuid).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var result bookRef
	json.Unmarshal(data, &result)
	return &result, nil
}

func (kb *KnowledgeBase) DeleteBook(ctx context.Context, refID string, ref *bookRef) error {

	pipe := kb.redisClient.Pipeline()
	bookKey := fmt.Sprintf("%s%s:%s", bookIndexPrefix, ref.BookName, ref.UserID)
	pipe.Del(ctx, bookKey)
	pipe.Del(ctx, "book_ref:"+refID)
	indexName := publicIndex
	if ref.Visibility != "public" {
		indexName = privateIndex
	}

	var cursor int
	for {
		res, err := kb.redisClient.FTSearchWithArgs(ctx, indexName,
			fmt.Sprintf("@book_ref:{%s}", refID), &redis.FTSearchOptions{
				LimitOffset: cursor,
				Limit:       100,
			}).Result()
		if err != nil {
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

	_, err := pipe.Exec(ctx)
	return err
}

func (kb *KnowledgeBase) Close() error {
	return kb.redisClient.Close()
}

func (kb *KnowledgeBase) FindBooks(ctx context.Context,
	userID, query string) ([]string, error) {
	if strings.TrimSpace(query) == "" {
		query = fmt.Sprintf("@userID:{%s}|@visibility:{public}", userID)
	} else {
		query = fmt.Sprintf("((@userID:{%s})|(@visibility:{public})) @bookName:*%s*",
			userID, escapeText(query))
	}

	fmt.Println(query)
	res, err := kb.redisClient.FTSearch(ctx, bookNameRefIndex, query).Result()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	if res.Total == 0 {
		return []string{}, nil
	}
	books := make([]string, 0, res.Total)
	for _, doc := range res.Docs {
		books = append(books, doc.Fields["bookName"])
	}
	return books, nil
}

func (kb *KnowledgeBase) SaveBookNameRef(ctx context.Context,
	userID, visibility, bookName, refID string) error {
	key := fmt.Sprintf("%s%s:%s", bookIndexPrefix, bookName, userID)
	return kb.redisClient.HSet(ctx, key,
		"userID", userID,
		"visibility", visibility,
		"bookName", bookName,
		"refID", refID,
	).Err()
}

func (kb *KnowledgeBase) UpdateBookNameRefFilePath(ctx context.Context,
	userID, bookName, filePath string) error {
	key := fmt.Sprintf("%s%s:%s", bookIndexPrefix, bookName, userID)
	return kb.redisClient.HSet(ctx, key,
		"filePath", filePath,
	).Err()
}

func (kb *KnowledgeBase) ResolveBookName(ctx context.Context, userID, bookName string) (string, error) {
	key := fmt.Sprintf("%s%s:%s", bookIndexPrefix, bookName, userID)
	exists, err := kb.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("check exists failed: %w", err)
	}
	if exists == 0 {
		return "", nil
	}

	fields, err := kb.redisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("HGetAll failed: %w", err)
	}
	return fields["refID"], nil
}

func escapeText(s string) string {
	// 需要转义的字符：, . < > { } [ ] " ' : ; ! @ # $ % ^ & * ( ) - + = ~ | \ / `
	replacer := strings.NewReplacer(
		",", "\\,",
		".", "\\.",
		"<", "\\<",
		">", "\\>",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"\"", "\\\"",
		"'", "\\'",
		":", "\\:",
		";", "\\;",
		"!", "\\!",
		"@", "\\@",
		"#", "\\#",
		"$", "\\$",
		"%", "\\%",
		"^", "\\^",
		"&", "\\&",
		"*", "\\*",
		"(", "\\(",
		")", "\\)",
		"-", "\\-",
		"+", "\\+",
		"=", "\\=",
		"~", "\\~",
		"|", "\\|",
		"\\", "\\\\",
		"/", "\\/",
		"`", "\\`",
	)
	return replacer.Replace(s)
}
