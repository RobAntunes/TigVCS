package parcel

import (
	"fmt"
	"os"

	"github.com/dgraph-io/badger/v4"
)

// getDBOptions returns BadgerDB options optimized for development use.
// These settings prioritize reduced memory usage and faster operations
// over durability, which is acceptable for development scenarios.
func getDBOptions(path string) badger.Options {
	opts := badger.DefaultOptions(path).
		WithValueDir("").
		WithDir("").
		WithInMemory(true).       // Run in memory for development
		WithNumVersionsToKeep(1). // Only keep latest version
		WithNumGoroutines(1).     // Single goroutine is sufficient for dev
		WithLogger(nil)           // Disable logging noise

	return opts
}

// InitDB initializes and returns a BadgerDB instance
func InitDB(path string) (*badger.DB, error) {
    if err := os.MkdirAll(path, 0755); err != nil {
        return nil, fmt.Errorf("creating database directory: %w", err)
    }

    opts := badger.DefaultOptions(path).
        WithLoggingLevel(badger.WARNING)

    db, err := badger.Open(opts)
    if err != nil {
        return nil, fmt.Errorf("opening database: %w", err)
    }

    return db, nil
}
