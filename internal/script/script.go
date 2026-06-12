package script

type ScriptInfo struct {
	Hash        string `json:"hash"`
	Topic       string `json:"topic"`
	ChapterIdx  int    `json:"chapter_idx"`
	CreatedAt   string `json:"created_at"`
	SegmentIndx int    `json:"segment_idx"`
	Preview     string `json:"preview"`
	Content     string `json:"content,omitempty"`
	BookRef     string `json:"book_ref"`
}
