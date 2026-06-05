package tts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const defaultInstruction = "你正在以一个故事机的身份说话，你说话的情感是neutral。"

type cosyvoiceEvent struct {
	Output struct {
		FinishReason string `json:"finish_reason"`
		Type         string `json:"type"`
		OriginalText string `json:"original_text"`
		Audio        struct {
			Data string `json:"data"`
			URL  string `json:"url"`
		} `json:"audio"`
	} `json:"output"`
}

type CosyVoiceClient struct {
	apiKey      string
	voice       string
	outDir      string
	instruction string
}

func NewCosyVoiceClient(apiKey, voice, outDir string) *CosyVoiceClient {
	if voice == "" {
		voice = "longanyang"
	}
	return &CosyVoiceClient{
		apiKey:      apiKey,
		voice:       voice,
		outDir:      outDir,
		instruction: defaultInstruction,
	}
}

func (c *CosyVoiceClient) TextToSpeech(ctx context.Context, text, filename, userID string) (string, error) {
	userDir := filepath.Join(c.outDir, userID)
	os.MkdirAll(userDir, 0755)
	outPath := filepath.Join(userDir, filename+".wav")

	input := map[string]any{
		"text":        text,
		"voice":       c.voice,
		"format":      "wav",
		"sample_rate": 24000,
	}
	if c.instruction != "" {
		input["instruction"] = c.instruction
	}

	body := map[string]any{
		"model": "cosyvoice-v3-flash",
		"input": input,
	}
	jsonData, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://dashscope.aliyuncs.com/api/v1/services/audio/tts/SpeechSynthesizer",
		bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-SSE", "enable")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("cosyvoice error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	eventCh, _ := ctx.Value("eventCh").(chan *adk.AgentEvent)
	pushEvent := func(msg string) {
		if eventCh == nil {
			return
		}
		eventCh <- &adk.AgentEvent{
			AgentName: "tts",
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: schema.AssistantMessage(msg, nil),
				},
			},
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event cosyvoiceEvent
		if err := json.Unmarshal([]byte(line[5:]), &event); err != nil {
			continue
		}

		switch event.Output.Type {
		case "sentence-begin":
			pushEvent(fmt.Sprintf("🎤 正在生成音频: %s...", trunc(event.Output.OriginalText, 20)))
		case "sentence-synthesis":
			// audio data chunk, 不需要处理
		default:
			if event.Output.FinishReason == "stop" && event.Output.Audio.URL != "" {
				pushEvent("✅ AI音频生成完成")
				return downloadFile(event.Output.Audio.URL, outPath)
			}
		}
	}
	return "", fmt.Errorf("cosyvoice: unexpected end of stream")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func downloadFile(url, path string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	return path, nil
}
