package task

import (
	"context"
	"dunkirk/internal/agent"
	"dunkirk/internal/pipeline"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type Task struct {
	ID           string
	Status       string // pending / running / completed / failed
	CreatedAt    time.Time
	Input        string
	Style        string
	PipelineMode bool
	FilePath     string
	Output       []string
	Error        string
	EventCh      chan *adk.AgentEvent
	done         chan struct{}
	Intent       *pipeline.IntentResult
}

type Manager struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	agent    *agent.Agent
	pipeline *pipeline.Pipeline
}

func NewManager(a *agent.Agent, p *pipeline.Pipeline) *Manager {
	return &Manager{
		tasks:    make(map[string]*Task),
		agent:    a,
		pipeline: p,
	}
}

func (m *Manager) CreateTask(input, style string, pipelineMode bool) *Task {
	task := &Task{
		ID:           uuid.New().String()[:8],
		Status:       "pending",
		CreatedAt:    time.Now(),
		Input:        input,
		Style:        style,
		EventCh:      make(chan *adk.AgentEvent, 100),
		done:         make(chan struct{}),
		PipelineMode: pipelineMode,
	}
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()
	return task
}

func (m *Manager) GetTask(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return t, ok
}

func (m *Manager) StartTask(task *Task) {
	task.Status = "running"
	go func() {
		defer close(task.done)
		ctx := context.Background()
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
	results, err := pipeline.ProcessBook(ctx, m.pipeline, task.FilePath, task.Style)
	if err != nil {
		task.Error = err.Error()
		task.Status = "failed"
		return
	}
	for _, ch := range results {
		task.Output = append(task.Output, ch.AudioPath)
		task.EventCh <- &adk.AgentEvent{
			AgentName: "pipeline",
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage(
						fmt.Sprintf("第%d章完成: %s", ch.ChapterIdx, ch.AudioPath), nil),
				},
			},
		}
	}
	task.Status = "completed"
	// close(task.done)
}

func (m *Manager) CreateTaskFromIntent(intent *pipeline.IntentResult, filePath string, pipelineMode bool) *Task {
	task := &Task{
		ID:           uuid.New().String()[:8],
		Status:       "pending",
		CreatedAt:    time.Now(),
		Style:        intent.Style,
		PipelineMode: pipelineMode,
		FilePath:     filePath,
		Intent:       intent,
		EventCh:      make(chan *adk.AgentEvent, 100),
		done:         make(chan struct{}),
	}
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()
	return task
}
