package config

import "os"

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
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
