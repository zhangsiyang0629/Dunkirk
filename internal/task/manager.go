package task

import (
	"context"
	"dunkirk/internal/agent"
	"dunkirk/internal/memory"
	"dunkirk/internal/pipeline"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/google/uuid"
)

type Task struct {
	ID           string
	Status       string // pending / running / completed / failed
	CreatedAt    time.Time
	Input        string
	Style        string
	PipelineMode bool
	FileRefID    string
	BookName     string
	Output       []string
	Error        string
	EventCh      chan *adk.AgentEvent
	done         chan struct{}
	Intent       *pipeline.IntentResult
	UserID       string
	UseSSML      bool
	ResumeCh     chan string // pipeline 阻塞等待用户审核结果
	CheckpointID string      // 用于 Resume 端点的 sessionID
	ConvID       string
}

func (t *Task) NextEvent() (*adk.AgentEvent, bool) {
	event, ok := <-t.EventCh
	return event, ok
}

type Manager struct {
	mu        sync.RWMutex
	tasks     map[string]*Task
	agent     *agent.Agent
	pipeline  *pipeline.Pipeline
	convStore *memory.ConversationStore
}

func NewManager(a *agent.Agent, p *pipeline.Pipeline, cs *memory.ConversationStore) *Manager {
	return &Manager{
		tasks:     make(map[string]*Task),
		agent:     a,
		pipeline:  p,
		convStore: cs,
	}
}

func (m *Manager) GetTask(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return t, ok
}

func (m *Manager) StartTask(ctx context.Context, task *Task) {
	task.Status = "running"
	go func() {
		defer close(task.done)
		defer close(task.EventCh)
		log.Printf("[start task]pipelineMode:%v", task.PipelineMode)
		if task.PipelineMode {
			m.runPipeline(ctx, task)
			return
		}
		iter := m.agent.Run(ctx, task.Input, task.Style)
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				task.Error = event.Err.Error()
				task.Status = "failed"
				task.EventCh <- event
				return
			}
			task.EventCh <- event
			if event.Output != nil && event.Output.MessageOutput != nil {
				if msg, err := event.Output.MessageOutput.GetMessage(); err == nil && msg.Content != "" {
					if strings.Contains(msg.Content, ".mp3") {
						task.Output = append(task.Output, msg.Content)
					}
				}
			}
		}
		task.Status = "completed"
	}()
}

func (m *Manager) Subscribe(id string) *Task {
	t, ok := m.GetTask(id)
	if !ok {
		return nil
	}
	return t
}

func (m *Manager) runPipeline(ctx context.Context, task *Task) {
	results, err := pipeline.ProcessBook(
		ctx,
		m.pipeline,
		task.UserID,
		task.FileRefID,
		task.BookName,
		task.Style,
		task.Intent.DurationMin,
		task.Intent.Chapters,
		task.EventCh,
		task.UseSSML,
		task.CheckpointID,
		task.ResumeCh)
	if err != nil {
		task.Error = err.Error()
		task.Status = "failed"
		log.Printf("[pipeline error] %v", err)
		return
	}

	// 保存 GenerationRecord
	if m.convStore != nil && task.ConvID != "" {
		record := &memory.GenerationRecord{
			ID:        task.ID,
			UserID:    task.UserID,
			BookRef:   task.FileRefID,
			BookName:  task.BookName,
			CreatedAt: time.Now(),
			Chapters:  buildChapterStates(results),
		}
		m.convStore.AppendGeneration(ctx, task.UserID, task.ConvID, record)
	}

	task.Status = "completed"
	log.Printf("task %s done, userID: %s, fileRefID: %s, topic: %s, resutlLen: %d",
		task.ID, task.UserID, task.FileRefID, task.Intent.Topic, len(results))
	// close(task.done)
}

func (m *Manager) CreateTaskFromIntent(
	input string,
	intent *pipeline.IntentResult,
	userID, refID, bookName string,
	pipelineMode bool) *Task {
	task := &Task{
		ID:           uuid.New().String()[:8],
		Status:       "pending",
		CreatedAt:    time.Now(),
		UserID:       userID,
		Input:        input,
		Style:        intent.Style,
		PipelineMode: pipelineMode,
		FileRefID:    refID,
		BookName:     bookName,
		Intent:       intent,
		EventCh:      make(chan *adk.AgentEvent, 100),
		done:         make(chan struct{}),
		ResumeCh:     make(chan string, 1),
	}
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()
	return task
}

func (m *Manager) GetTaskByCheckpointID(checkpointID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.tasks {
		if t.CheckpointID == checkpointID {
			return t, true
		}
	}
	return nil, false
}

func buildChapterStates(results []*pipeline.ChapterTask) []memory.ChapterState {
	states := make([]memory.ChapterState, 0, len(results))
	for _, r := range results {
		ch := memory.ChapterState{
			ChapterIdx: r.ChapterIdx,
			ChapterInt: r.ChapterInt,
			Topic:      r.Topic,
			Status:     memory.ChapterStatusDone,
		}
		if r.Error != "" {
			ch.Status = memory.ChapterStatusSkipped
			ch.Error = r.Error
		}
		for i, seg := range r.ScriptSegments {
			status := memory.SegmentStatusApproved
			if r.Error != "" {
				status = memory.SegmentStatusRejected
			}
			runes := []rune(seg)
			preview := string(runes)
			if len(runes) > 100 {
				preview = string(runes[:100]) + "..."
			}
			segState := memory.SegmentState{
				SegmentIdx: i,
				Preview:    preview,
				Status:     status,
			}
			if i < len(r.AudioPaths) {
				segState.AudioPath = r.AudioPaths[i]
			}
			ch.Segments = append(ch.Segments, segState)
		}
		states = append(states, ch)
	}
	return states
}
