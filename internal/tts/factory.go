package tts

import (
	"dunkirk/internal/config"
)

func GetTTSProvider(cfg *config.Config) TTSProvider {
	switch cfg.TTSProvider {
	case config.TTS_AZURE:
		return NewAzureClient(cfg.AzureSpeechKey, cfg.AzureRegion, cfg.TTSVoice, cfg.AudioDir)
	case config.TTS_QWEN:
		return NewQwenClient(cfg.DashScopeAPIKey, "", cfg.AudioDir)
	case config.TTS_COSY:
		return NewCosyVoiceClient(cfg.DashScopeAPIKey, "", cfg.AudioDir)
	default:
		return NewWSClient(cfg.TTSVoice, cfg.AudioDir)
	}
}
