package content

import (
	"sync"
	"tig/internal/safe"
)

// Change represents a single file change
type Change struct {
    Path     string `json:"path"`     // File path relative to repository root
    Type     string `json:"type"`     // add, modify, delete, rename
    Content  []byte `json:"content"`  // New file content (empty for delete)
    OldPath  string `json:"old_path"` // Original path for renames
    Mode     int    `json:"mode"`     // File permissions
    Hash     string `json:"hash"`     // Content hash
}


type Store interface {
    Store(content []byte) (string, error)
    Get(hash string) ([]byte, error)
    Exists(hash string) bool
}

type FileStore struct {
    root  string
    cache map[string][]byte
    mu    sync.RWMutex // Protects cache
    Safe  *safe.Safe
}