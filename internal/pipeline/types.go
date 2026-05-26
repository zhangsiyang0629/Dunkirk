package pipeline

type ChapterTask struct {
	Topic         string
	Style         string
	DurationMin   int
	ChapterIdx    int
	TotalChapters int
	Content       string
	Script        string
	AudioPath     string
	Error         string
}

type IntentResult struct {
	Topic          string `json:"topic"`
	Style          string `json:"style"`
	DurationMin    int    `json:"duration_min"`
	Mode           string `json:"mode"` // "chat" / "chapter" / "book"
	IsAudioRequest bool   `json:"is_audio_request"`
	Reasoning      string `json:"reasoning"`
	ChatReply      string `json:"-"`
}
