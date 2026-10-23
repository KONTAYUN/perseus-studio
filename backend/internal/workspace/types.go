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

type SearchResult struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	ParentPath string    `json:"parent_path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

type TextSearchResult struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Match     string `json:"match"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
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

type TextEditMatchKind string

const (
	TextEditMatchExact               TextEditMatchKind = "exact"
	TextEditMatchTrimmedLines        TextEditMatchKind = "trimmed_lines"
	TextEditMatchIndentationFlexible TextEditMatchKind = "indentation_flexible"
)

type TextEditResult struct {
	WriteResult  FileWriteResult   `json:"write_result"`
	Replacements int               `json:"replacements"`
	MatchKind    TextEditMatchKind `json:"match_kind"`
}

type LineReplaceResult struct {
	WriteResult      FileWriteResult `json:"write_result"`
	StartLine        int             `json:"start_line"`
	EndLine          int             `json:"end_line"`
	InsertedLines    int             `json:"inserted_lines"`
	TotalLinesBefore int             `json:"total_lines_before"`
	TotalLinesAfter  int             `json:"total_lines_after"`
}

type TextReadResult struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	NumberedContent string `json:"numbered_content,omitempty"`
	Truncated       bool   `json:"truncated"`
	StartLine       int    `json:"start_line,omitempty"`
	EndLine         int    `json:"end_line,omitempty"`
	TotalLines      int    `json:"total_lines,omitempty"`
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
	ExecutionOutcome    string    `json:"execution_outcome"`
	ObservationComplete bool      `json:"observation_complete"`
	ObservationError    string    `json:"observation_error,omitempty"`
	Command             string    `json:"command"`
	Cwd                 string    `json:"cwd"`
	Stdout              string    `json:"stdout"`
	Stderr              string    `json:"stderr"`
	ExitCode            int       `json:"exit_code"`
	TimedOut            bool      `json:"timed_out"`
	StartedAt           time.Time `json:"started_at"`
	FinishedAt          time.Time `json:"finished_at"`
	ChangedPaths        []string  `json:"changed_paths,omitempty"`
	WorkspaceMutation   bool      `json:"workspace_mutation,omitempty"`
	Hash                string    `json:"hash,omitempty"`
}
