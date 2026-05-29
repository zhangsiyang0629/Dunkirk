package pipeline

import (
	"context"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type streamingModel struct {
	model.BaseChatModel
}

func (m *streamingModel) Stream(
	ctx context.Context,
	input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, err := m.BaseChatModel.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	eventCh, _ := ctx.Value("eventCh").(chan *adk.AgentEvent)
	pr, pw := schema.Pipe[*schema.Message](10)
	go func() {
		defer pw.Close()
		defer reader.Close()
		for {
			chunk, err := reader.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			eventCh <- &adk.AgentEvent{
				AgentName: "pipeline",
				Output: &adk.AgentOutput{
					MessageOutput: &adk.MessageVariant{
						IsStreaming:   true,
						MessageStream: schema.StreamReaderFromArray([]*schema.Message{chunk}),
					},
				},
			}
			pw.Send(chunk, nil)
		}
	}()
	return pr, nil
}
