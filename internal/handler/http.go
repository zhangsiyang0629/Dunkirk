package handler

import (
	"context"
	"crypto/sha256"
	"dunkirk/internal/agent"
	"dunkirk/internal/config"
	"dunkirk/internal/docproc"
	"dunkirk/internal/global"
	"dunkirk/internal/kb"
	"dunkirk/internal/memory"
	"dunkirk/internal/pipeline"
	"dunkirk/internal/script"
	"dunkirk/internal/task"
	"dunkirk/internal/tts"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxFileSize = 100 * 1024 * 1024

type Handler struct {
	tm           *task.Manager
	kb           *kb.KnowledgeBase
	tts          tts.TTSProvider
	cfg          *config.Config
	cm           model.BaseChatModel
	intentParser compose.Runnable[map[string]any, *pipeline.IntentResult]
	agent        *agent.Agent
	fileStatus   *FileStatus
	scriptStore  *script.Store
	convStore    *memory.ConversationStore
	profileStore *memory.ProfileStore
}

func (h *Handler) SetConvStore(cs *memory.ConversationStore) {
	h.convStore = cs
}

func (h *Handler) SetIntentParser(p compose.Runnable[map[string]any, *pipeline.IntentResult]) {
	h.intentParser = p
}

func (h *Handler) SetProfileStore(ps *memory.ProfileStore) {
	h.profileStore = ps
}

func New(tm *task.Manager,
	kb *kb.KnowledgeBase,
	ttsClient tts.TTSProvider,
	cfg *config.Config,
	cm model.BaseChatModel,
	fs *FileStatus,
	scriptStore *script.Store) *Handler {
	return &Handler{tm: tm, kb: kb, tts: ttsClient, cfg: cfg, cm: cm, fileStatus: fs, scriptStore: scriptStore}
}

func LoggerMiddleware(c *gin.Context) {
	start := time.Now()
	userID := c.GetHeader("X-User-ID")
	c.Next()
	log.Printf("[HTTP] %s %s %d %s user=%s",
		c.Request.Method,
		c.Request.URL.Path,
		c.Writer.Status(),
		time.Since(start),
		userID,
	)
}

func Register(r *gin.Engine, h *Handler) {
	v1 := r.Group("/api/v1")
	r.Use(LoggerMiddleware)
	{
		v1.POST("/chat", h.Chat)
		v1.GET("/audio/:task_id", h.GetTask)
		v1.POST("/upload", h.UploadFile)
		v1.POST("/resume", h.Resume)
		v1.DELETE("/upload/:file_ref_id", h.DeleteFile)
		v1.GET("/audio/download/:userID/:filename", h.DownloadUserAudio)
		v1.GET("/scripts", h.ListScripts)
		v1.GET("/scripts/:hash", h.GetScript)
		v1.DELETE("/scripts/:hash", h.DeleteScript)
		v1.GET("/conversations", h.ListConversations)
		v1.GET("/conversations/:id/messages", h.GetConversationMessages)
	}
}

type resumeReq struct {
	CheckpointID string `json:"checkpoint_id"`
	InterruptID  string `json:"interrupt_id"`
	Choice       string `json:"choice"`
}

func (h *Handler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	t, ok := h.tm.GetTask(taskID)
	if !ok {
		c.JSON(404, gin.H{"error": "task not found"})
		return
	}
	c.JSON(200, gin.H{
		"task_id": t.ID,
		"status":  t.Status,
		"output":  t.Output,
		"error":   t.Error,
	})
}

func (h *Handler) DownloadAudio(c *gin.Context) {
	filename := c.Param("filename")
	c.File(filepath.Join(h.cfg.AudioDir, filename))
}

func (h *Handler) Chat(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	message := c.PostForm("message")
	convID := c.PostForm("conversation_id")
	ctx := context.WithValue(c.Request.Context(), "userID", userID)

	if convID == "" {
		conv, err := h.convStore.CreateConversation(c.Request.Context(), userID)
		if err != nil {
			c.JSON(500, gin.H{"error": "create conversation failed"})
			return
		}
		convID = conv.ID
	} else {
		_, err := h.convStore.GetOrCreateConversation(c.Request.Context(), userID, convID)
		if err != nil {
			c.JSON(500, gin.H{"error": "get or create conversation failed"})
			return
		}
	}

	h.convStore.AppendMessage(ctx, userID, convID, &memory.Message{
		Role:      memory.RoleUser,
		Content:   message,
		CreatedAt: time.Now(),
	})

	const historyTokenBudget = 60000
	h.trySummarize(ctx, userID, convID, historyTokenBudget)

	// 加载记忆上下文并注入到用户输入前
	summaryEntries, _ := h.convStore.GetSummaries(ctx, userID, convID)
	summaryTexts := make([]string, 0, len(summaryEntries))
	lastEnd := 0
	for _, e := range summaryEntries {
		summaryTexts = append(summaryTexts, e.Summary)
		lastEnd = e.EndIdx
	}
	recentGens, _ := h.convStore.GetRecentGenerations(c.Request.Context(), userID, convID, 5)
	memCtx := &memory.MemoryContext{
		Summaries:         summaryTexts,
		RecentGenerations: recentGens,
	}
	contextStr := memory.BuildContextPrompt(memCtx)

	profile, _ := h.profileStore.Get(ctx, userID)
	profileStr := ""
	if profile.PreferredStyle != "" {
		profileStr = fmt.Sprintf("用户偏好：上次使用的风格「%s」", profile.PreferredStyle)
		if profile.LastBookName != "" {
			profileStr += fmt.Sprintf("，上次制作的书籍「%s」", profile.LastBookName)
		}
	}
	if profileStr != "" {
		contextStr = profileStr + "\n\n" + contextStr
	}

	recentMsgs, _ := h.convStore.GetRecentMessagesFrom(ctx, userID, convID, int64(lastEnd))
	var history []*schema.Message
	for _, m := range recentMsgs {
		role := schema.User
		if m.Role == memory.RoleAgent {
			role = schema.Assistant
		}
		history = append(history, &schema.Message{Role: role, Content: m.Content})
	}

	input := map[string]any{
		"context":    contextStr,
		"history":    history,
		"user_input": message,
		"conv_id":    convID,
	}

	ctx = context.WithValue(ctx, "file_status", h.fileStatus)
	nr := c.Request.WithContext(ctx)
	c.Request = nr

	sessionID := uuid.New().String()
	result, err := h.intentParser.Invoke(ctx, input, compose.WithCheckPointID(sessionID))
	if err != nil {
		if info, ok := compose.ExtractInterruptInfo(err); ok {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			contexts := info.InterruptContexts
			if len(contexts) > 0 {
				data, _ := json.Marshal(gin.H{
					"interrupt_id":  contexts[0].ID,
					"checkpoint_id": sessionID,
					"question":      contexts[0].Info.(map[string]any)["question"],
					"options":       contexts[0].Info.(map[string]any)["options"],
				})
				fmt.Fprintf(c.Writer, "event: interrupt\ndata: %s\n\n", data)
				c.Writer.Flush()
			}
			return
		}
		c.JSON(500, gin.H{"error": "parse intent: " + err.Error()})
		return
	}

	// 统一 SSE 头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	convReadyData, _ := json.Marshal(convID)
	fmt.Fprintf(c.Writer, "event: conv_ready\ndata: %s\n\n", convReadyData)
	c.Writer.Flush()

	intentDat, _ := json.Marshal(gin.H{"type": "intent_result", "data": result})
	fmt.Fprintf(c.Writer, "data: %s\n\n", intentDat)
	c.Writer.Flush()

	if !result.IsAudioRequest {
		h.chatSSE(c, input, userID, result)
		return
	}
	if result.Book == "" {
		h.chatSSE(c, input, userID, result)
		return
	}
	h.audioSSE(c, result, userID)
}

func (h *Handler) chatSSE(c *gin.Context,
	input map[string]any,
	userID string,
	result *pipeline.IntentResult) {

	if result.Style != "" {
		h.profileStore.SaveField(c, userID, "preferred_style", result.Style)
	}

	intentJSON, _ := json.Marshal(result)
	fmt.Fprintf(c.Writer, "event: intent\ndata: %s\n\n", intentJSON)
	c.Writer.Flush()

	msgs := []*schema.Message{
		schema.SystemMessage("你是一个友好的有声读物制作助手，帮助用户生成音频作品。当用户闲聊时友好回应，并引导到音频制作话题。"),
		schema.SystemMessage(input["context"].(string)),
	}
	if history, ok := input["history"].([]*schema.Message); ok {
		msgs = append(msgs, history...)
	}
	msgs = append(msgs, schema.UserMessage(input["user_input"].(string)))
	ctx := context.WithValue(c.Request.Context(), "userID", userID)
	stream, err := h.cm.Stream(ctx, msgs)
	if err != nil {
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"%s\"}\n\n", err.Error())
		c.Writer.Flush()
		return
	}
	defer stream.Close()

	var fullReply string
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		fullReply += chunk.Content
		data, _ := json.Marshal(gin.H{"content": chunk.Content})
		fmt.Fprintf(c.Writer, "event: token\ndata: %s\n\n", data)
		c.Writer.Flush()
	}
	fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
	c.Writer.Flush()

	if convID, ok := input["conv_id"].(string); ok && convID != "" && fullReply != "" {
		h.convStore.AppendMessage(ctx, userID, convID, &memory.Message{
			Role:      memory.RoleAgent,
			Content:   fullReply,
			CreatedAt: time.Now(),
		})
	}
}

func (h *Handler) audioSSE(
	c *gin.Context,
	result *pipeline.IntentResult, userID string) {

	var filePath string
	var bookRef string

	convID := c.PostForm("conversation_id")

	if result.Book != "" {
		uuid, err := h.kb.ResolveBookName(c.Request.Context(), userID, result.Book)
		if err == nil && uuid != "" {
			bookRef = uuid
		}
	}

	h.profileStore.Save(c, userID, &memory.UserProfile{
		PreferredStyle: result.Style,
		LastBookName:   result.Book,
		LastBookRef:    bookRef,
	})

	var input string
	if bookRef != "" {
		input = fmt.Sprintf("用户上传了文件(uuid=%s)：请处理%s\n风格要求：%s",
			bookRef, result.Topic, result.Style)
		if len(result.Chapters) > 0 {
			input += fmt.Sprintf("\n指定章节：%v", result.Chapters)
		}
	} else {
		input = fmt.Sprintf("用户话题：%s\n风格要求：%s", result.Topic, result.Style)
	}

	var t *task.Task
	if result.Mode == "book" && !result.SkipFile {
		t = h.tm.CreateTaskFromIntent("全本生成", result, userID, bookRef, result.Book, true)
	} else if result.Mode == "chapter" && len(result.Chapters) > 0 && !result.SkipFile {
		t = h.tm.CreateTaskFromIntent("部分章节生成", result, userID, bookRef, result.Book, true)
	} else {
		userInput := fmt.Sprintf("用户话题：%s\n风格要求：%s", result.Topic, result.Style)
		if filePath != "" {
			userInput = fmt.Sprintf("用户上传文件：%s\n%s", filePath, userInput)
		}
		t = h.tm.CreateTaskFromIntent(userInput, result, userID, bookRef, result.Book, false)
	}
	sessionID := uuid.New().String()
	t.UseSSML = h.cfg.TTSProvider == "azure"
	t.CheckpointID = sessionID
	t.ConvID = convID
	fmt.Printf("tts provider: %v\n", h.cfg.TTSProvider)
	log.Printf("[audio task create] %#v", *t)

	h.tm.StartTask(c.Request.Context(), t)

	data, _ := json.Marshal(gin.H{"task_id": t.ID})
	fmt.Fprintf(c.Writer, "event: task_created\ndata: %s\n\n", data)
	c.Writer.Flush()

	appendMessageFunc := func(content string, action global.EventAction) {
		if action == global.ACTION_PIPELINE_SCRIPT_GEN {
			return
		}
		if convID == "" {
			return
		}
		h.convStore.AppendMessage(c.Request.Context(), userID, convID, &memory.Message{
			Role:      memory.RoleAgent,
			Content:   content,
			CreatedAt: time.Now(),
		})
	}

	scriptAcc := ""
	for {
		event, ok := t.NextEvent()
		if !ok {
			break
		}
		if event.Err != nil {
			errData, _ := json.Marshal(gin.H{"message": event.Err.Error()})
			log.Printf("[ERROR]audioSSE event err:%v", event.Err.Error())
			fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errData)
			c.Writer.Flush()
			break
		}

		if event.Output != nil && event.Output.MessageOutput != nil {
			mv := event.Output.MessageOutput
			if mv.IsStreaming {
				var chunkContent string
				for {
					chunk, err := mv.MessageStream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Fatalf("stream error: %v", err)
					}
					chunkContent += chunk.Content
				}
				if chunkContent != "" {
					eventData, _ := json.Marshal(gin.H{
						"agent":   event.AgentName,
						"content": chunkContent,
					})
					fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", eventData)
					c.Writer.Flush()
					if event.Action.CustomizedAction == global.ACTION_PIPELINE_SCRIPT_GEN &&
						len(scriptAcc) < 250 {
						scriptAcc += chunkContent
					}
				}
			} else {
				msg, _ := event.Output.MessageOutput.GetMessage()
				if msg != nil && msg.Content != "" {
					eventData, _ := json.Marshal(gin.H{
						"agent":   event.AgentName,
						"content": msg.Content,
					})
					fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", eventData)
					c.Writer.Flush()
					var evtAct global.EventAction
					var eOk bool
					if event.Action != nil {
						evtAct, eOk = event.Action.CustomizedAction.(global.EventAction)
						if !eOk {
							evtAct = global.ACTION_DEF
						}
					}
					appendMessageFunc(msg.Content, evtAct)
				}
			}
		} else if event.Action != nil && event.Action.Interrupted != nil {
			contexts := event.Action.Interrupted.InterruptContexts
			data, _ := json.Marshal(gin.H{
				"interrupt_id":  contexts[0].ID,
				"checkpoint_id": sessionID,
				"question":      contexts[0].Info.(map[string]any)["question"],
				"options":       contexts[0].Info.(map[string]any)["options"],
			})
			fmt.Fprintf(c.Writer, "event: interrupt\ndata: %s\n\n", data)
			c.Writer.Flush()
		}
	}

	if scriptAcc != "" && len(scriptAcc) > 200 {
		scriptAcc += "..."
	}
	if scriptAcc != "" {
		appendMessageFunc(scriptAcc, global.ACTION_DEF)
	}

	c.Writer.Flush()
}

func (h *Handler) trySummarize(ctx context.Context, userID, convID string, budget int) {
	total, _ := h.convStore.MessageCount(ctx, userID, convID)
	if total == 0 {
		return
	}

	summaries, _ := h.convStore.GetSummaries(ctx, userID, convID)
	lastEnd := 0
	if len(summaries) > 0 {
		lastEnd = summaries[len(summaries)-1].EndIdx
	}

	if int(total)-1-lastEnd <= 0 {
		return
	}

	msgs, _ := h.convStore.GetMessagesInRange(ctx, userID, convID, lastEnd, int(total)-1)
	if len(msgs) == 0 {
		return
	}

	acc := 0
	splitIdx := 0 // 默认全部保留
	for i, m := range msgs {
		acc += estimateTokens(m.Content)
		if acc > budget {
			splitIdx = i
			break
		}
	}

	if splitIdx == 0 {
		return
	}

	summarizeMsgs := msgs[:splitIdx]
	var parts []string
	for _, m := range summarizeMsgs {
		role := "用户"
		if m.Role == memory.RoleAgent {
			role = "助手"
		}
		parts = append(parts, role+": "+m.Content)
	}

	resp, err := h.cm.Generate(ctx, []*schema.Message{
		schema.SystemMessage("你是一个对话摘要助手。将对话内容压缩为一段简洁的摘要（200字以内），保留关键信息：用户需求、书籍、章节、审核结果。"),
		schema.UserMessage(strings.Join(parts, "\n")),
	})
	if err != nil {
		log.Printf("[summarize] failed: %v", err)
		return
	}

	newEndIdx := lastEnd + splitIdx - 1
	h.convStore.AppendSummary(ctx, userID, convID, &memory.SummaryEntry{
		EndIdx:  newEndIdx,
		Summary: resp.Content,
	})
}

func (h *Handler) Resume(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	var req resumeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx := compose.BatchResumeWithData(c.Request.Context(), map[string]any{
		req.InterruptID: req.Choice,
	})

	if t, ok := h.tm.GetTaskByCheckpointID(req.CheckpointID); ok {
		// Pipeline 审核中断
		if t.ConvID != "" {
			h.convStore.AppendMessage(c.Request.Context(), t.UserID, t.ConvID, &memory.Message{
				Role:      memory.RoleUser,
				Content:   "审核选择: " + req.Choice,
				CreatedAt: time.Now(),
			})
		}
		t.ResumeCh <- req.Choice
		return
	}

	result, err := h.intentParser.Invoke(ctx, map[string]any{}, compose.WithCheckPointID(req.CheckpointID))
	if err != nil {
		// 检查是否又中断（理论上不会，但做防御）
		if info, ok := compose.ExtractInterruptInfo(err); ok {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			if len(info.InterruptContexts) > 0 {
				ctx2 := info.InterruptContexts[0]
				data, _ := json.Marshal(gin.H{
					"interrupt_id":  ctx2.ID,
					"checkpoint_id": req.CheckpointID,
					"question":      ctx2.Info.(map[string]any)["question"],
					"options":       ctx2.Info.(map[string]any)["options"],
				})
				fmt.Fprintf(c.Writer, "event: interrupt\ndata: %s\n\n", data)
				c.Writer.Flush()
			}
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if !result.IsAudioRequest {
		c.JSON(200, gin.H{"reply": result.ChatReply})
		return
	}
	// 正常处理音频请求
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	h.audioSSE(c, result, userID)
}

func (h *Handler) UploadFile(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}
	visibility := c.PostForm("visibility")
	if visibility != "private" {
		visibility = "public"
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileSize)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "file required or too large"})
		return
	}

	hasher := sha256.New()
	ext := filepath.Ext(header.Filename)
	tmpFile := filepath.Join(h.cfg.UploadDir, uuid.New().String()+ext)
	os.MkdirAll(h.cfg.UploadDir, 0755)
	out, _ := os.Create(tmpFile)
	tee := io.TeeReader(file, hasher)
	io.Copy(out, tee)
	out.Close()
	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	if existing := h.fileStatus.FindByHash(hash); existing != nil {
		os.Remove(tmpFile)
		switch existing.Status {
		case "ready":
			c.JSON(200, gin.H{"file_ref_id": existing.RefID, "cached": true})
			return
		case "processing":
			c.JSON(409, gin.H{"error": "file already being processed", "file_ref_id": existing.RefID})
			return
		case "failed":
			h.fileStatus.Remove(existing.RefID)
		}
	}

	hashPath := filepath.Join(h.cfg.UploadDir, hash+ext)
	os.Rename(tmpFile, hashPath)
	bookName := strings.TrimSuffix(header.Filename, ext)
	refID := uuid.New().String()[:8]

	kbCtx := context.Background()
	if existingUUID, _ := h.kb.ResolveBookName(kbCtx, userID, bookName); existingUUID != "" {
		c.JSON(409, gin.H{"error": fmt.Sprintf("书籍「%s」refID：%s 已存在，请删除后重新上传", bookName, existingUUID)})
		os.Remove(hashPath)
		h.kb.UpdateBookNameRefFilePath(kbCtx, userID, bookName, hashPath)
		return
	} else {
		// 同名public的书名也得去重

	}

	h.kb.SaveBookNameRef(kbCtx, userID, visibility, bookName, refID)
	h.fileStatus.Add(&FileInfo{
		RefID:      refID,
		FilePath:   hashPath,
		FileName:   header.Filename,
		BookName:   bookName,
		Hash:       hash,
		Status:     "processing",
		UserID:     userID,
		Visibility: visibility,
	})

	go func() {
		_, err := docproc.ProcessAndStore(context.Background(), h.kb, bookName, hashPath, refID, userID, visibility)
		if err != nil {
			h.fileStatus.SetStatus(refID, "failed")
			return
		}
		h.fileStatus.SetStatus(refID, "ready")
	}()
	c.JSON(200, gin.H{
		"file_ref_id": refID,
		"file_name":   header.Filename,
		"book_name":   bookName,
		"uuid":        refID,
	})
}

func (h *Handler) DeleteFile(c *gin.Context) {
	refID := c.Param("file_ref_id")
	if refID == "" {
		c.JSON(403, gin.H{"error": "ref id required"})
		return
	}
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(403, gin.H{"error": "user id required"})
		return
	}

	if info, ok := h.fileStatus.Get(refID); ok {
		if info.Status == "processing" {
			c.JSON(403, gin.H{"error": fmt.Sprintf("file:%s is processing, can't delete file", refID)})
			return
		}
	}

	ctx := context.Background()
	ref, err := h.kb.ResolveBookRef(ctx, refID)
	if err != nil {
		c.JSON(500, gin.H{"error": err})
		return
	}
	if ref == nil {
		c.JSON(403, gin.H{"error": "book not exist"})
		return
	}
	if ref.UserID != userID {
		c.JSON(403, gin.H{"error": "permission denied"})
		return
	}

	log.Printf("http endpoint: delete file, userID:%s, fileName:%s, refID:%s, filePath:%s",
		userID, ref.BookName, refID, ref.FilePath)
	// 删文件
	err = os.Remove(ref.FilePath)
	if err != nil {
		log.Printf("[HTTP ERROR]--%v", err)
	}
	// 删 Redis 索引
	h.kb.DeleteBook(context.Background(), refID, ref)
	// 删内存记录
	h.fileStatus.Remove(refID)
	c.JSON(200, gin.H{"status": "deleted"})
}

func (h *Handler) DownloadUserAudio(c *gin.Context) {
	userID := c.Param("userID")
	filename := c.Param("filename")
	if strings.Contains(filename, "..") || strings.Contains(userID, "..") {
		c.JSON(400, gin.H{"error": "invalid path"})
		return
	}
	c.File(filepath.Join(h.cfg.AudioDir, userID, filename))
}

func (h *Handler) ListScripts(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if userID == "" {
		c.JSON(400, gin.H{"error": "user_id required"})
		return
	}
	scripts, err := h.scriptStore.List(c.Request.Context(), userID, offset, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"scripts": scripts})
}

func (h *Handler) GetScript(c *gin.Context) {
	hash := c.Param("hash")
	userID := c.GetHeader("X-User-ID")
	bookRef := c.Query("book_ref")
	if userID == "" || bookRef == "" {
		c.JSON(400, gin.H{"error": "user_id and book_ref required"})
		return
	}
	info, err := h.scriptStore.Get(c.Request.Context(), userID, bookRef, hash)
	if err != nil {
		c.JSON(404, gin.H{"error": "script not found"})
		return
	}
	c.JSON(200, info)
}

func (h *Handler) DeleteScript(c *gin.Context) {
	hash := c.Param("hash")
	userID := c.GetHeader("X-User-ID")
	bookRef := c.Query("book_ref")
	if userID == "" || bookRef == "" {
		c.JSON(400, gin.H{"error": "user_id and book_ref required"})
		return
	}
	h.scriptStore.DeleteByHash(c.Request.Context(), userID, bookRef, hash)
	c.JSON(200, gin.H{"status": "deleted"})
}

func (h *Handler) ListConversations(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(400, gin.H{"error": "user_id required"})
		return
	}
	convs, err := h.convStore.ListConversations(c.Request.Context(), userID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"conversations": convs})
}

func (h *Handler) GetConversationMessages(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(400, gin.H{"error": "user_id required"})
		return
	}
	convID := c.Param("id")
	msgs, err := h.convStore.GetMessages(c.Request.Context(), userID, convID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"messages": msgs})
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
