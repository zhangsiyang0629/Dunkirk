package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://edge.microsoft.com/translate/auth"
	//wsURL        = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	defaultVoice = "zh-CN-XiaoxiaoNeural"
)

type BinaryFrame struct {
	Type      uint16
	HeaderLen uint16
	Header    string
	Audio     []byte
}

type Client struct {
	voice  string
	outDir string
	mu     sync.Mutex
	token  string
	exp    time.Time
}

func NewClient(voice, outDir string) *Client {
	if voice == "" {
		voice = defaultVoice
	}
	return &Client{voice: voice, outDir: outDir}
}

// getToken 获取并缓存 token（过期前 5 分钟刷新）
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.exp.Add(-5*time.Minute)) {
		return c.token, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	c.token = string(body)
	c.exp = time.Now().Add(10 * time.Minute)
	return c.token, nil
}

// buildSSML 封装 SSML 模板
func (c *Client) buildSSML(text string) string {
	escaped := strings.ReplaceAll(text, "&", "&")
	escaped = strings.ReplaceAll(escaped, "<", "<")
	escaped = strings.ReplaceAll(escaped, ">", ">")
	escaped = strings.ReplaceAll(escaped, "\"", "\"")
	return fmt.Sprintf(`<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='zh-CN'>
	<voice name='%s'>
		<prosody rate='+0%%' pitch='+0Hz'>%s</prosody>
	</voice>
</speak>`, c.voice, escaped)
}

func (c *Client) TextToSpeech(ctx context.Context, text, filename, userID string) (string, error) {
	userDir := filepath.Join(c.outDir, userID)
	os.MkdirAll(userDir, 0755)
	outPath := filepath.Join(userDir, filename+".mp3")
	cmd := exec.CommandContext(context.Background(), "edge-tts",
		"--voice", c.voice,
		"--text", text,
		"--write-media", outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("edge-tts: %w\nstderr: %s", err, stderr.String())
	}
	return outPath, nil
}

// func parseFrame(data []byte) (*BinaryFrame, error) {
// 	if len(data) < 4 {
// 		return nil, fmt.Errorf("frame too short")
// 	}
// 	frameType := binary.BigEndian.Uint16(data[0:2])
// 	headerLen := binary.BigEndian.Uint16(data[2:4])
// 	var header string
// 	if headerLen > 0 && int(4+headerLen) <= len(data) {
// 		header = string(data[4 : 4+headerLen])
// 	}
// 	audio := data[4+headerLen:]
// 	return &BinaryFrame{
// 		Type:      frameType,
// 		HeaderLen: headerLen,
// 		Header:    header,
// 		Audio:     audio,
// 	}, nil
// }
