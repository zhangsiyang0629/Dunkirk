package pipeline

import (
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func pushEvent(eventCh chan *adk.AgentEvent, format string, args ...any) {
	eventCh <- &adk.AgentEvent{
		AgentName: "pipeline",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: schema.AssistantMessage(fmt.Sprintf(format, args...), nil),
			},
		},
	}
}

func pushInterrupt(eventCh chan *adk.AgentEvent, ctxs []*compose.InterruptCtx) {
	eventCh <- &adk.AgentEvent{
		AgentName: "pipeline",
		Action: &adk.AgentAction{
			Interrupted: &adk.InterruptInfo{
				InterruptContexts: ctxs,
			},
		},
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
