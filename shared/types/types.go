// Status represents the status of a file
package shared

import (
	"tig/internal/diff"
	intent "tig/internal/intent"
	"time"
)

type Status struct {
	Path  string
	State string
	Gated bool
}

type Workspace interface {
	Gate(paths []string) error

	// Close cleans up the workspace
	Close() error

	// CreateIntent creates a new intent
	CreateIntent(description string, intentType string) (*intent.Intent, error)

	// UpdateIntent updates an existing intent
	UpdateIntent(i *intent.Intent) error

	// Ungate removes files from being included in the next intent
	Ungate(paths []string) error

	// Status wraps the tracker's Status method
	Status() ([]Change, error)

	// ShowFileDiff wraps the tracker's ShowFileDiff method
	ShowFileDiff(path string) (*diff.DiffResult, error)

	CleanupGatedChanges() error

	LoadGatedChanges() error
}

// Enhanced Change struct with diff information
type Change struct {
	Path      string     `json:"path"`
	Type      string     `json:"type"`
	OldPath   string     `json:"old_path"`
	OldHash   string     `json:"old_hash"`
	NewHash   string     `json:"new_hash"`
	Mode      int        `json:"mode"`
	Size      int64      `json:"size"`
	ModTime   time.Time  `json:"mod_time"`
	Diff      string     `json:"diff,omitempty"`
	DiffHunks []DiffHunk `json:"diff_hunks,omitempty"`
	Gated     bool       `json:"gated"`
	Content   string     `json:"content,omitempty"`
}

// DiffHunk represents a section of changes
type DiffHunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}
