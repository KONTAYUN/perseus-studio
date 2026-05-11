package workspace

import "time"

type TreeNode struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Type       string     `json:"type"`
	Size       int64      `json:"size,omitempty"`
	ModifiedAt time.Time  `json:"modified_at"`
	Children   []TreeNode `json:"children,omitempty"`
}

type FileReadResult struct {
	Path       string    `json:"path"`
	Content    string    `json:"content"`
	Size       int64     `json:"size"`
	Hash       string    `json:"hash"`
	ModifiedAt time.Time `json:"modified_at"`
}

type FileWriteResult struct {
	Path         string   `json:"path"`
	Hash         string   `json:"hash"`
	VersionID    string   `json:"version_id,omitempty"`
	Size         int64    `json:"size"`
	ChangedPaths []string `json:"changed_paths,omitempty"`
}

type TextReadResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

type PathChangeResult struct {
	Path         string   `json:"path,omitempty"`
	OldPath      string   `json:"old_path,omitempty"`
	NewPath      string   `json:"new_path,omitempty"`
	ChangedPaths []string `json:"changed_paths,omitempty"`
}

type TrashItem struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	OriginalPath string    `json:"original_path"`
	Type         string    `json:"type"`
	Size         int64     `json:"size,omitempty"`
	ModifiedAt   time.Time `json:"modified_at"`
	DeletedAt    time.Time `json:"deleted_at"`
}

type CommandResult struct {
	Command    string    `json:"command"`
	Cwd        string    `json:"cwd"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	ExitCode   int       `json:"exit_code"`
	TimedOut   bool      `json:"timed_out"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}
