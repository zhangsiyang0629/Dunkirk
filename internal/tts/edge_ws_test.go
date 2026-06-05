package tts

import (
	"context"
	"dunkirk/internal/config"
	"testing"
)

func TestWsEdge(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()

	ttsProvider := NewWSClient(cfg.TTSVoice, cfg.AudioDir)
	msg := "<prosody pitch=\"10%\">李傕</prosody><prosody pitch=\"10%\">郭汜</prosody>"
	res, err := ttsProvider.TextToSpeech(ctx, msg, "test.wav", "user123")
	if err != nil {
		t.Fatalf("TextToSpeech error: %v", err)
	}
	t.Logf("Audio file generated at: %s", res)
}
