package handler

import (
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"dunkirk/internal/pipeline"
	"dunkirk/internal/task"
	"dunkirk/internal/tts"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	tm           *task.Manager
	kb           *kb.KnowledgeBase
	tts          *tts.Client
	cfg          *config.Config
	cm           model.BaseChatModel
	intentParser compose.Runnable[string, *pipeline.IntentResult]
}

func New(tm *task.Manager,
	kb *kb.KnowledgeBase,
	ttsClient *tts.Client,
	cfg *config.Config,
	cm model.BaseChatModel) *Handler {
	return &Handler{tm: tm, kb: kb, tts: ttsClient, cfg: cfg, cm: cm}
}

func Register(r *gin.Engine, h *Handler) {
	v1 := r.Group("/api/v1")
	{
		v1.POST("/audio/generate", h.Generate)
		v1.POST("/chat", h.Chat)
		v1.GET("/audio/:task_id/stream", h.StreamTask)
		v1.GET("/audio/:task_id", h.GetTask)
	}
	r.GET("/api/audio/:filename", h.DownloadAudio)
}

type generateReq struct {
	Topic       string `json:"topic"`
	Style       string `json:"style"`
	DurationMin int    `json:"duration_min,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
}

// type chatReq struct {
// 	Message string            `json:"message"`
// 	History []*schema.Message `json:"history,omitempty"`
// }

func (h *Handler) Generate(c *gin.Context) {
	var req generateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userInput := fmt.Sprintf("用户话题：%s\n风格要求：%s", req.Topic, req.Style)
	if req.FilePath != "" {
		userInput = fmt.Sprintf("用户上传文件：%s\n%s", req.FilePath, userInput)
	}
	if req.FilePath != "" && req.Topic == "" {
		t := h.tm.CreateTask(userInput, req.Style, true)
		t.FilePath = req.FilePath
		h.tm.StartTask(t)
		c.JSON(202, gin.H{"task_id": t.ID})
	} else {
		t := h.tm.CreateTask(userInput, req.Style, false)
		h.tm.StartTask(t)
		c.JSON(202, gin.H{"task_id": t.ID})
	}
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

func (h *Handler) StreamTask(c *gin.Context) {
	taskID := c.Param("task_id")
	t, ok := h.tm.GetTask(taskID)
	if !ok {
		c.JSON(404, gin.H{"error": "task not found"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-t.EventCh:
			if !ok {
				return false
			}
			if event.Err != nil {
				data, _ := json.Marshal(gin.H{"type": "error", "message": event.Err.Error()})
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
				return false
			}
			if event.Output != nil && event.Output.MessageOutput != nil {
				msg, err := event.Output.MessageOutput.GetMessage()
				if err == nil && msg.Content != "" {
					data, _ := json.Marshal(gin.H{
						"type":    "output",
						"agent":   event.AgentName,
						"content": msg.Content,
					})
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				}
			}
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
	if t.Status == "completed" || t.Status == "failed" {
		data, _ := json.Marshal(gin.H{"type": "done", "status": t.Status, "files": t.Output})
		fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", data)
	}
}

func (h *Handler) DownloadAudio(c *gin.Context) {
	filename := c.Param("filename")
	c.File(filepath.Join(h.cfg.AudioDir, filename))
}

func (h *Handler) Chat(c *gin.Context) {
	message := c.PostForm("message")
	historyRaw := c.PostForm("history")
	var history []*schema.Message
	if historyRaw != "" {
		json.Unmarshal([]byte(historyRaw), &history)
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil && err != http.ErrMissingFile {
		c.JSON(400, gin.H{"error": "file error: " + err.Error()})
		return
	}

	var filePath string
	if file != nil {
		ext := filepath.Ext(header.Filename)
		tmpFile := filepath.Join(h.cfg.UploadDir, uuid.New().String()+ext)
		os.MkdirAll(h.cfg.UploadDir, 0755)
		out, _ := os.Create(tmpFile)
		io.Copy(out, file)
		out.Close()
		filePath = tmpFile
	}

	input := message
	if filePath != "" {
		if input == "" {
			input = "请处理这个文件"
		}
		input = fmt.Sprintf("用户上传了文件：%s\n%s", filePath, input)
	}

	result, err := h.intentParser.Invoke(c.Request.Context(), input)
	if err != nil {
		c.JSON(500, gin.H{"error": "parse intent: " + err.Error()})
		return
	}

	if !result.IsAudioRequest {
		c.JSON(200, gin.H{"reply": result.ChatReply, "intent": result})
		return
	}

	var t *task.Task
	switch result.Mode {
	case "book":
		t = h.tm.CreateTask("全本生成", result.Style, true)
		t.FilePath = filePath
	default:
		userInput := fmt.Sprintf("用户话题：%s\n风格要求：%s", result.Topic, result.Style)
		if filePath != "" {
			userInput = fmt.Sprintf("用户上传文件：%s\n%s", filePath, userInput)
		}
		t = h.tm.CreateTask(userInput, result.Style, false)
	}
	h.tm.StartTask(t)
	c.JSON(202, gin.H{"task_id": t.ID, "intent": result})
}

func (h *Handler) SetIntentParser(p compose.Runnable[string, *pipeline.IntentResult]) {
	h.intentParser = p
}
