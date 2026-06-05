package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type QwenClient struct {
	apiKey       string
	voice        string
	outDir       string
	instructions string
}

func NewQwenClient(apiKey, voice, outDir string) *QwenClient {
	if voice == "" {
		voice = "Arthur"
	}
	return &QwenClient{
		apiKey: apiKey,
		voice:  voice,
		outDir: outDir,
	}
}

func (c *QwenClient) TextToSpeech(ctx context.Context, text, filename, userID string) (string, error) {
	userDir := filepath.Join(c.outDir, userID)
	os.MkdirAll(userDir, 0755)

	chunks := splitBySentences(text, 600)
	if len(chunks) == 1 {
		outPath := filepath.Join(userDir, filename+".wav")
		return outPath, c.synthesize(ctx, text, outPath)
	}

	var tmpFiles []string
	for i, chunk := range chunks {
		tmpFile := filepath.Join(userDir, fmt.Sprintf("%s_tmp_%d.wav", filename, i))
		if err := c.synthesize(ctx, chunk, tmpFile); err != nil {
			return "", fmt.Errorf("chunk %d: %w", i, err)
		}
		tmpFiles = append(tmpFiles, tmpFile)
	}

	outPath := filepath.Join(userDir, filename+".wav")
	if err := mergeWAVs(tmpFiles, outPath); err != nil {
		return "", fmt.Errorf("merge: %w", err)
	}

	for _, f := range tmpFiles {
		os.Remove(f)
	}
	return outPath, nil
}

func (c *QwenClient) synthesize(ctx context.Context, text, outPath string) error {
	body := map[string]any{
		"model": "qwen3-tts-instruct-flash",
		"input": map[string]any{
			"text":                  text,
			"voice":                 c.voice,
			"instructions":          c.instructions,
			"optimize_instructions": true,
		},
	}
	jsonData, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation",
		bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qwen error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output struct {
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if result.Output.Audio.URL == "" {
		return fmt.Errorf("no audio url in response")
	}

	audioResp, err := http.Get(result.Output.Audio.URL)
	if err != nil {
		return fmt.Errorf("download audio: %w", err)
	}
	defer audioResp.Body.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, audioResp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func style2instruction(style string) string {
	switch {
	case strings.Contains(style, "小朋友"), strings.Contains(style, "儿童"),
		strings.Contains(style, "小孩"):
		return "请使用适合儿童的声音和语气朗读，语速较慢，清晰发音，富有童趣"
	default:
		return "沉稳的中年男性，语速缓慢，音色低沉有磁性，适合朗读新闻或纪录片解说"
	}
}

func splitBySentences(text string, maxBytes int) []string {
	runes := []rune(text)
	var chunks []string
	var buf strings.Builder

	sentStart := 0
	for i, r := range runes {
		if strings.ContainsRune("。！？!?\n", r) {
			sentence := string(runes[sentStart : i+1])
			// 用 len() 算字节数
			if buf.Len() > 0 && buf.Len()+len(sentence) > maxBytes {
				chunks = append(chunks, buf.String())
				buf.Reset()
			}
			buf.WriteString(sentence)
			sentStart = i + 1
		}
	}
	if sentStart < len(runes) {
		sentence := string(runes[sentStart:])
		if buf.Len() > 0 && buf.Len()+len(sentence) > maxBytes {
			chunks = append(chunks, buf.String())
			buf.Reset()
		}
		buf.WriteString(sentence)
	}
	if buf.Len() > 0 {
		chunks = append(chunks, buf.String())
	}
	return chunks
}

func mergeWAVs(paths []string, outPath string) error {
	var audioData [][]byte
	var header []byte
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if len(data) < 44 {
			continue
		}
		if header == nil {
			header = data[:44]
		}
		audioData = append(audioData, data[44:])
	}
	if header == nil {
		return fmt.Errorf("no valid wav files")
	}
	var totalSize int
	for _, d := range audioData {
		totalSize += len(d)
	}
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+totalSize))
	binary.LittleEndian.PutUint32(header[40:44], uint32(totalSize))
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	out.Write(header)
	for _, d := range audioData {
		out.Write(d)
	}
	return nil
}
