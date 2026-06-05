package tts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	wsURL              = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	winEpoch           = 11644473600
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	secMsGecVersion    = "1-143.0.3650.75"
	defaultVoice       = "zh-CN-XiaoxiaoNeural"
)

type WSClient struct {
	voice  string
	outDir string
}

func NewWSClient(voice, outDir string) *WSClient {
	if voice == "" {
		voice = defaultVoice
	}
	return &WSClient{voice: voice, outDir: outDir}
}

func (c *WSClient) TextToSpeech(ctx context.Context, text, filename, userID string) (string, error) {
	userDir := filepath.Join(c.outDir, userID)
	os.MkdirAll(userDir, 0755)
	outPath := filepath.Join(userDir, filename+".mp3")
	connID := uuid.New().String()
	secMsGec := generateSecMsGec()
	muid := generateMuid()
	url := fmt.Sprintf("%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		wsURL, connID, secMsGec, secMsGecVersion)
	header := http.Header{}
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0")
	header.Set("Cookie", "muid="+muid+";")
	header.Set("Sec-WebSocket-Protocol", "access_token_exchange")
	header.Set("X-RequestId", uuid.NewString())
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return "", fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// 发送配置消息
	config := map[string]any{
		"context": map[string]any{
			"synthesis": map[string]any{
				"audio": map[string]any{
					"metadataoptions": map[string]any{
						"sentenceBoundaryEnabled": false,
						"wordBoundaryEnabled":     false,
					},
					"outputFormat": "audio-24khz-96kbitrate-mono-mp3",
				},
			},
		},
	}
	configMsg, _ := json.Marshal(config)
	configFrame := "Content-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" + string(configMsg)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(configFrame)); err != nil {
		return "", fmt.Errorf("send config: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	ssml := buildWSML(c.voice, text)
	log.Println(ssml)
	now := time.Now().UTC()
	timestamp := now.Format("Mon, 02 Jan 2006 15:04:05 GMT")
	ssmlFrame := fmt.Sprintf("X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%s\r\nPath:ssml\r\n\r\n%s",
		uuid.New().String(), timestamp, ssml)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlFrame)); err != nil {
		return "", fmt.Errorf("send ssml: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}

		//log.Printf("recv: type=%d len=%s", messageType, data)
		switch messageType {
		case websocket.TextMessage: // type=1
			if bytes.Contains(data, []byte("turn.end")) {
				return outPath, nil
			}
			continue

		case websocket.BinaryMessage: // type=2
			if len(data) < 2 {
				continue
			}

			headerSize := uint16(data[0])<<8 | uint16(data[1])
			if int(2+headerSize) > len(data) {
				log.Printf("invalid header size")
				continue
			}

			jsonHeader := string(data[2 : 2+headerSize])
			audioData := data[2+headerSize:]

			log.Printf("header: %s, audio len: %d", jsonHeader, len(audioData))

			// 处理音频数据
			if len(audioData) > 0 {
				f.Write(audioData)
			}
		}
	}
}

func generateSecMsGec() string {
	ticks := time.Now().Unix() + winEpoch
	ticks -= ticks % 300
	hundredNs := ticks * 10000000
	str := fmt.Sprintf("%d%s", hundredNs, trustedClientToken)
	hash := sha256.Sum256([]byte(str))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

func generateMuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func buildWSML(voice, text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.Contains(trimmed, "<prosody") {
		return fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN"><voice name="%s">%s</voice></speak>`, voice, trimmed)
	}
	// 纯文本 → 基本 prosody 包装
	return fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN"><voice name="%s">%s</voice></speak>`, voice, trimmed)
}
