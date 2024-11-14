package change

import (
	"fmt"

	"tig/internal/diff"
	"tig/internal/safe"
	"tig/shared/types"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

func NewLocalTracker(root string, db *badger.DB, contentSafe *safe.Safe) (*LocalTracker, error) {
	if root == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}
	if contentSafe == nil {
		return nil, fmt.Errorf("contentSafe cannot be nil")
	}

	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, fmt.Errorf("initializing logger: %w", err)
	}

	return &LocalTracker{
		Root:         root,
		DB:           db,
		ContentSafe:  contentSafe,
		DiffEngine:   diff.NewEngine(3),
		Tracked:      make(map[string]bool),
		GatedChanges: make(map[string]shared.Change),
		Logger:       logger,
	}, nil
}

// NewTracker creates a new tracker with automatic tracking enabled
func NewTracker(root string, db *badger.DB, contentSafe *safe.Safe, logger *zap.Logger) (Tracker, error) {
	// Create base LocalTracker
	localTracker, err := NewLocalTracker(root, db, contentSafe)
	if err != nil {
		return nil, fmt.Errorf("creating local tracker: %w", err)
	}

	// Wrap with AutoTracker
	autoTracker, err := NewAutoTracker(localTracker, logger)
	if err != nil {
		return nil, fmt.Errorf("creating auto tracker: %w", err)
	}

	return autoTracker, nil
}
