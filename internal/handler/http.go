package handler

import (
	"context"
	"crypto/sha256"
	"dunkirk/internal/agent"
	"dunkirk/internal/config"
	"dunkirk/internal/docproc"
	"dunkirk/internal/kb"
	"dunkirk/internal/pipeline"
	"dunkirk/internal/task"
	"dunkirk/internal/tts"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	tts          *tts.Client
	cfg          *config.Config
	cm           model.BaseChatModel
	intentParser compose.Runnable[string, *pipeline.IntentResult]
	agent        *agent.Agent
	fileStatus   *FileStatus
}

func New(tm *task.Manager,
	kb *kb.KnowledgeBase,
	ttsClient *tts.Client,
	cfg *config.Config,
	cm model.BaseChatModel,
	fs *FileStatus) *Handler {
	return &Handler{tm: tm, kb: kb, tts: ttsClient, cfg: cfg, cm: cm, fileStatus: fs}
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
	}
	r.GET("/api/audio/:filename", h.DownloadAudio)
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
	historyRaw := c.PostForm("history")
	fileRefID := c.PostForm("file_ref_id")
	var history []*schema.Message
	if historyRaw != "" {
		json.Unmarshal([]byte(historyRaw), &history)
	}

	input := message
	if fileRefID != "" {
		info, ok := h.fileStatus.Get(fileRefID)
		if !ok {
			c.JSON(400, gin.H{"error": "file not found"})
			return
		}
		input = fmt.Sprintf("用户上传了文件：%s\n%s", info.FilePath, message)
	}
	if message == "" {
		if fileRefID != "" {
			input = "请处理这个文件"
		} else {
			c.JSON(400, gin.H{"error": "message required"})
			return
		}
	}

	ctx := context.WithValue(c.Request.Context(), "userID", userID)
	ctx = context.WithValue(ctx, "file_ref_id", fileRefID)
	ctx = context.WithValue(ctx, "file_status", h.fileStatus)
	if fileRefID != "" {
		ctx = context.WithValue(ctx, "book_ref", fileRefID)
	}
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

	intentDat, _ := json.Marshal(gin.H{"type": "intent_result", "data": result})
	fmt.Fprintf(c.Writer, "data: %s\n\n", intentDat)
	c.Writer.Flush()

	if !result.IsAudioRequest {
		h.chatSSE(c, message, history, userID, result)
		return
	}
	if fileRefID == "" && result.Book == "" {
		h.chatSSE(c, message, history, userID, result)
		return
	}
	h.audioSSE(c, result, fileRefID, userID)
}

func (h *Handler) SetIntentParser(p compose.Runnable[string, *pipeline.IntentResult]) {
	h.intentParser = p
}

func (h *Handler) chatSSE(c *gin.Context,
	message string,
	history []*schema.Message,
	userID string,
	result *pipeline.IntentResult) {
	intentJSON, _ := json.Marshal(result)
	fmt.Fprintf(c.Writer, "event: intent\ndata: %s\n\n", intentJSON)
	c.Writer.Flush()

	// 流式输出
	msgs := []*schema.Message{
		schema.SystemMessage("你是一个友好的有声读物制作助手，帮助用户生成音频作品。当用户闲聊时友好回应，并引导到音频制作话题。"),
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, schema.UserMessage(message))
	ctx := context.WithValue(c.Request.Context(), "userID", userID)
	stream, err := h.cm.Stream(ctx, msgs)
	if err != nil {
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"%s\"}\n\n", err.Error())
		c.Writer.Flush()
		return
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		data, _ := json.Marshal(gin.H{"content": chunk.Content})
		fmt.Fprintf(c.Writer, "event: token\ndata: %s\n\n", data)
		c.Writer.Flush()
	}
	fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
	c.Writer.Flush()
}

func (h *Handler) audioSSE(
	c *gin.Context,
	result *pipeline.IntentResult,
	fileRefID, userID string) {

	var filePath string
	var bookRef string

	if fileRefID != "" {
		if info, ok := h.fileStatus.Get(fileRefID); ok {
			filePath = info.FilePath
		}
	}

	/*
		result.Book理论上一定是存在的书籍名称，因为在意图解析阶段，解析得到的书名会在数据库中进行模糊查询，
		如果找到多条会进行中断让用户选择，最终得到的结果一定是数据库中存在的书籍名称。
		如果没有查到，就应该让用户选择是放弃生成还是让大模型自己推理，不管是放弃生成还是大模型自己推理，都
		不会再走到代码这里了。
		所以result.Book理论上一定是存在的书籍名称
	*/
	if filePath == "" && result.Book != "" {
		uuid, err := h.kb.ResolveBookName(c.Request.Context(), userID, result.Book)
		if err == nil && uuid != "" {
			bookRef = uuid
		}
	}

	ref := fileRefID
	var input string
	if fileRefID != "" || bookRef != "" {
		if ref == "" {
			ref = bookRef
		}
		input = fmt.Sprintf("用户上传了文件(uuid=%s)：请处理%s\n风格要求：%s",
			ref, result.Topic, result.Style)
		if len(result.Chapters) > 0 {
			input += fmt.Sprintf("\n指定章节：%v", result.Chapters)
		}
	} else {
		input = fmt.Sprintf("用户话题：%s\n风格要求：%s", result.Topic, result.Style)
	}

	var t *task.Task
	if result.Mode == "book" && !result.SkipFile {
		t = h.tm.CreateTaskFromIntent("全本生成", result, userID, ref, result.Book, true)
	} else if result.Mode == "chapter" && len(result.Chapters) > 0 && !result.SkipFile {
		t = h.tm.CreateTaskFromIntent("部分章节生成", result, userID, ref, result.Book, true)
	} else {
		userInput := fmt.Sprintf("用户话题：%s\n风格要求：%s", result.Topic, result.Style)
		if filePath != "" {
			userInput = fmt.Sprintf("用户上传文件：%s\n%s", filePath, userInput)
		}
		t = h.tm.CreateTaskFromIntent(userInput, result, userID, ref, result.Book, false)
	}
	log.Printf("[audio task craete] %#v", *t)
	h.tm.StartTask(c.Request.Context(), t)

	data, _ := json.Marshal(gin.H{"task_id": t.ID})
	fmt.Fprintf(c.Writer, "event: task_created\ndata: %s\n\n", data)
	c.Writer.Flush()

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
				for {
					chunk, err := mv.MessageStream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						log.Fatalf("stream error: %v", err)
					}
					eventData, _ := json.Marshal(gin.H{
						"agent":   event.AgentName,
						"content": chunk.Content,
					})
					fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", eventData)
					c.Writer.Flush()
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
				}
			}
		}
	}
	c.Writer.Flush()
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

	result, err := h.intentParser.Invoke(ctx, "", compose.WithCheckPointID(req.CheckpointID))
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
	var fileRefID string
	// 如果只有书名没有 fileRefID，反查
	if result.Book != "" {
		uuid, err := h.kb.ResolveBookName(c.Request.Context(), userID, result.Book)
		if err == nil && uuid != "" {
			fileRefID = uuid
		}
	}
	h.audioSSE(c, result, fileRefID, userID)
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
