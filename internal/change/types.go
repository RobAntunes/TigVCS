// internal/change/types.go
package change

import (
	"sync"
	"tig/internal/diff"
	"tig/internal/safe"
	"time"
	"tig/shared/types"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// LocalTracker with integrated diff engine
type LocalTracker struct {
	Tracker
	Root         string
	DB           *badger.DB
	ContentSafe  *safe.Safe
	DiffEngine   *diff.Engine
	Tracked      map[string]bool
	Mu           sync.RWMutex
	GatedChanges map[string]shared.Change
	Logger       *zap.Logger
}

// ChangeSet groups related changes together
type ChangeSet struct {
	ID          string    `json:"id"`
	ParentID    string    `json:"parent_id"` // Previous changeset ID
	IntentID    string    `json:"intent_id"` // Associated intent
	Changes     []shared.Change  `json:"changes"`   // List of changes
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	Tags        []string  `json:"tags"`
	Hash        string    `json:"hash"` // Verification hash
}

type Tracker interface {
	Track(paths []string) error
	Untrack(paths []string) error
	Status() ([]shared.Change, error)
	CreateChangeSet(description string) (*ChangeSet, error)
	ShowFileDiff(path string) (*diff.DiffResult, error)
	Gate(path string) error
}


