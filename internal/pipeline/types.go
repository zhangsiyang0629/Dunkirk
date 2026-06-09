package pipeline

type ChapterTask struct {
	Topic         string
	Style         string
	DurationMin   int
	ChapterIdx    int
	ChapterInt    int
	TotalChapters int
	Content       string
	Script        string
	AudioPath     string
	FileRefID     string
	Error         string
	UseSSML       bool
	PrevEnding    string
	IsExactSerach bool
}

const (
	INTERRUPT_BOOK_SELECT = "_book_select"
	INTERRUPT_GEN_SELECT  = "_generate_select"
)

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
	InterruptType   string   `json:"-"`
	InterruptOpions []string `json:"-"`
	SkipFile        bool     `json:"skip_file,omitempty"`
}

func (r *IntentResult) interruptInfo() map[string]any {
	var question string
	switch r.InterruptType {
	case INTERRUPT_BOOK_SELECT:
		question = "您指的是哪本书？"
	case INTERRUPT_GEN_SELECT:
		question = "没有找到相关书籍，是否要继续"
	}
	return map[string]any{
		"question": question,
		"options":  r.InterruptOpions,
	}
}
