package tts

import "context"

type TTSProvider interface {
	TextToSpeech(ctx context.Context, text, filename string) (string, error)
}
