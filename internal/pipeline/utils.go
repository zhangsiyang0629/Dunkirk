package pipeline

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func pushEvent(eventCh chan *adk.AgentEvent, customerAction any, format string, args ...any) {
	if eventCh == nil {
		return
	}
	output := &adk.AgentOutput{
		MessageOutput: &adk.MessageVariant{
			Message: schema.AssistantMessage(fmt.Sprintf(format, args...), nil),
		},
	}
	eventCh <- &adk.AgentEvent{
		AgentName: "pipeline",
		Output:    output,
		Action:    &adk.AgentAction{CustomizedAction: customerAction},
	}
}

func pushInterrupt(eventCh chan *adk.AgentEvent, ctxs []*compose.InterruptCtx) {
	if eventCh == nil {
		return
	}
	eventCh <- &adk.AgentEvent{
		AgentName: "pipeline",
		Action:    &adk.AgentAction{Interrupted: &adk.InterruptInfo{InterruptContexts: ctxs}},
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func splitBySentence(text string) []string {
	var result []string
	runes := []rune(text)
	start := 0
	for i, r := range runes {
		if strings.ContainsRune("。！？!?\n", r) {
			if i+1-start > 20 || i == len(runes)-1 {
				result = append(result, string(runes[start:i+1]))
				start = i + 1
			}
		}
	}
	if start < len(runes) {
		result = append(result, string(runes[start:]))
	}
	if len(result) == 0 {
		result = []string{text}
	}
	return result
}

func distributeSentences(sentences []string, n int) [][]string {
	total := len(sentences)
	if n <= 0 {
		n = 1
	}
	if n > total {
		n = total
	}
	base := total / n
	rem := total % n
	var result [][]string
	start := 0
	for i := 0; i < n; i++ {
		size := base
		if i < rem {
			size++
		}
		result = append(result, sentences[start:start+size])
		start += size
	}
	return result
}

func lastNSentences(text string, n int) []string {
	sentences := splitBySentence(text)
	if len(sentences) <= n {
		return sentences
	}
	return sentences[len(sentences)-n:]
}
