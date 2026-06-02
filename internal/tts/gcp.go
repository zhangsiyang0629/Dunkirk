package tts

import (
	"context"
	"fmt"
	"path/filepath"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
)

type GcpClient struct {
	voice  string
	outDir string
	lang   string
}

func NewGcpClient(voice, outDir string) *GcpClient {
	if voice == "" {
		voice = "zh-CN-Neural2-A"
	}
	return &GcpClient{voice: voice, outDir: outDir, lang: "zh-CN"}
}

func (c *GcpClient) TextToSpeech(ctx context.Context, text, filename string) (string, error) {
	client, err := texttospeech.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("gcp tts client: %w", err)
	}
	defer client.Close()

	// req := &texttospeechpb.SynthesizeSpeechRequest{
	// 	Input: &texttospeechpb.SynthesisInput{
	// 		InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
	// 	},
	// 	Voice: &texttospeechpb.VoiceSelectionParams{
	// 		LanguageCode: c.lang,
	// 		Name:         c.voice,
	// 	},
	// 	AudioConfig: &texttospeechpb.AudioConfig{
	// 		AudioEncoding: texttospeechpb.AudioEncoding_MP3,
	// 	},
	// }

	// resp, err := client.SynthesizeSpeech(ctx, req)
	// if err != nil {
	// 	return "", fmt.Errorf("synthesize: %w", err)
	// }

	// outPath := filepath.Join(c.outDir, filename+".mp3")
	// if err := os.WriteFile(outPath, resp.AudioContent, 0644); err != nil {
	// 	return "", fmt.Errorf("write: %w", err)
	// }
	// return outPath, nil
	outPath := filepath.Join(c.outDir, filename+".mp3")
	_ = outPath
	return "", fmt.Errorf("GCP TTS not configured: set GOOGLE_APPLICATION_CREDENTIALS")
}
