package handler

import "sync"

type FileInfo struct {
	RefID      string
	FilePath   string
	FileName   string
	BookName   string
	Status     string // "ready" / "processing" / "failed"
	Hash       string
	Error      string
	UserID     string
	Visibility string
}

type FileStatus struct {
	mu    sync.RWMutex
	files map[string]*FileInfo
}

func NewFileStatus() *FileStatus {
	return &FileStatus{files: make(map[string]*FileInfo)}
}

func (fs *FileStatus) Add(info *FileInfo) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[info.RefID] = info
}

func (fs *FileStatus) Get(refID string) (*FileInfo, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	info, ok := fs.files[refID]
	return info, ok
}

func (fs *FileStatus) SetStatus(refID, status string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if info, ok := fs.files[refID]; ok {
		info.Status = status
	}
}

func (fs *FileStatus) SetError(refID, errStr string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if info, ok := fs.files[refID]; ok {
		info.Status = "failed"
		info.Error = errStr
	}
}

func (fs *FileStatus) FindByHash(hash string) *FileInfo {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	for _, info := range fs.files {
		if info.Hash == hash {
			return info
		}
	}
	return nil
}
func (fs *FileStatus) Remove(refID string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.files, refID)
}
