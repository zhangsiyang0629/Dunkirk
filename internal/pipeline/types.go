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
	FileRefID     string
	Error         string
}

type IntentResult struct {
	Topic           string   `json:"topic"`
	Style           string   `json:"style"`
	DurationMin     int      `json:"duration_min"`
	Mode            string   `json:"mode"` // "chat" / "chapter" / "book"
	IsAudioRequest  bool     `json:"is_audio_request"`
	Book            string   `json:"book"`
	Reasoning       string   `json:"reasoning"`
	ChatReply       string   `json:"-"`
	Chapters        []int    `json:"chapters,omitempty"`
	CheckpointID    string   `json:"checkpoint_id,omitempty"`
	InterruptID     string   `json:"interrupt_id,omitempty"`
	InterruptOpions []string `json:"-"`
	SkipFile        bool     `json:"skip_file,omitempty"`
}
