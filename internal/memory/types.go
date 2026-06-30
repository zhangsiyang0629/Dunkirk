package memory

import "time"

type Role string

const (
	RoleUser  Role = "user"
	RoleAgent Role = "assistant"
)

type Message struct {
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SegmentState struct {
	SegmentIdx int    `json:"segment_idx"`
	Preview    string `json:"preview"`
	Status     string `json:"status"`
	AudioPath  string `json:"audio_path,omitempty"`
}

type ChapterState struct {
	ChapterIdx int            `json:"chapter_idx"`
	ChapterInt int            `json:"chapter_int"`
	Topic      string         `json:"topic"`
	Segments   []SegmentState `json:"segments"`
	Status     string         `json:"status"`
	Error      string         `json:"error,omitempty"`
}

type GenerationRecord struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	BookRef   string         `json:"book_ref"`
	BookName  string         `json:"book_name"`
	CreatedAt time.Time      `json:"created_at"`
	Chapters  []ChapterState `json:"chapters"`
}

const (
	SegmentStatusApproved         = "approved"
	SegmentStatusRejected         = "rejected"
	SegmentStatusRejectedButSaved = "rejected_but_saved"

	ChapterStatusDone    = "done"
	ChapterStatusSkipped = "skipped"
	ChapterStatusError   = "error"
)

type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SummaryEntry struct {
	EndIdx  int    `json:"end_idx"`
	Summary string `json:"summary"`
}

type UserProfile struct {
	PreferredStyle string `json:"preferred_style,omitempty"`
	LastBookName   string `json:"last_book_name,omitempty"`
	LastBookRef    string `json:"last_book_ref,omitempty"`
}
