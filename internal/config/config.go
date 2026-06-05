package config

import "os"

const (
	TTS_EDGE  = "edge"
	TTS_AZURE = "azure"
	TTS_QWEN  = "qwen"
	TTS_COSY  = "cosy"
)

type Config struct {
	ArkAPIKey         string
	ArkChatModel      string
	ArkEmbeddingModel string
	RedisAddr         string
	TTSAppID          string
	TTSToken          string
	TTSCluster        string
	TTSVoice          string
	Port              string
	AudioDir          string
	UploadDir         string
	TTSProvider       string
	AzureSpeechKey    string
	AzureRegion       string
	DashScopeAPIKey   string
}

func Load() *Config {
	return &Config{
		ArkAPIKey:         os.Getenv("ARK_API_KEY"),
		ArkChatModel:      os.Getenv("ARK_CHAT_MODEL"),
		ArkEmbeddingModel: os.Getenv("ARK_EMBEDDING_MODEL"),
		RedisAddr:         envOrDefault("REDIS_ADDR", "127.0.0.1:6379"),
		TTSAppID:          os.Getenv("TTS_APP_ID"),
		TTSToken:          os.Getenv("TTS_TOKEN"),
		TTSCluster:        os.Getenv("TTS_CLUSTER"),
		TTSVoice:          envOrDefault("TTS_VOICE", "BV701_streaming"),
		Port:              envOrDefault("PORT", "8080"),
		AudioDir:          envOrDefault("AUDIO_DIR", "audio"),
		UploadDir:         envOrDefault("UPLOAD_DIR", "uploads"),
		TTSProvider:       envOrDefault("TTS_PROVIDER", TTS_EDGE),
		AzureSpeechKey:    os.Getenv("AZURE_SPEECH_KEY"),
		AzureRegion:       envOrDefault("AZURE_REGION", "eastasia"),
		DashScopeAPIKey:   os.Getenv("DASHSCOPE_API_KEY"),
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
