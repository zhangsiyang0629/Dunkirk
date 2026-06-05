package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type AzureClient struct {
	speechKey string
	region    string
	voice     string
	outDir    string
}

func NewAzureClient(speechKey, region, voice, outDir string) *AzureClient {
	if voice == "" {
		voice = "zh-CN-XiaoxiaoNeural"
	}
	return &AzureClient{
		speechKey: speechKey,
		region:    region,
		voice:     voice,
		outDir:    outDir,
	}
}

func (c *AzureClient) TextToSpeech(ctx context.Context, text, filename, userID string) (string, error) {
	userDir := filepath.Join(c.outDir, userID)
	os.MkdirAll(userDir, 0755)
	outPath := filepath.Join(userDir, filename+".mp3")

	// 1. 获取 access token
	token, err := c.getToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	// 2. 构建 SSML
	ssml := c.buildSSML(text)

	// 3. 调用 TTS API
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", c.region)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(ssml))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-24khz-96kbitrate-mono-mp3")
	req.Header.Set("User-Agent", "Dunkirk")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure tts error: status=%d body=%s", resp.StatusCode, string(body))
	}

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return outPath, nil
}

func (c *AzureClient) getToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://%s.api.cognitive.microsoft.com/sts/v1.0/issueToken", c.region)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	req.Header.Set("Ocp-Apim-Subscription-Key", c.speechKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token error: status=%d body=%s", resp.StatusCode, string(body))
	}

	token, _ := io.ReadAll(resp.Body)
	return string(token), nil
}

func (c *AzureClient) buildSSML(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "<speak") {
		start := strings.Index(trimmed, ">")
		end := strings.LastIndex(trimmed, "</speak>")
		if start >= 0 && end > start {
			trimmed = strings.TrimSpace(trimmed[start+1 : end])
		}
	}
	return fmt.Sprintf(`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='zh-CN'>
	<voice name='%s'>%s</voice>
</speak>`, c.voice, trimmed)
}
