// internal/content/store.go
package content

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func NewFileStore(root string) (*FileStore, error) {
    if err := os.MkdirAll(root, 0755); err != nil {
        return nil, fmt.Errorf("creating content store directory: %w", err)
    }

    return &FileStore{
        root:  root,
        cache: make(map[string][]byte),
        mu:    sync.RWMutex{},
    }, nil
}

// Store handles storing content and returns its hash
func (s *FileStore) Store(content []byte) (string, error) {
    // Allow empty content (empty files are valid)
    if content == nil {
        content = []byte{} // Convert nil to empty slice
    }

    // Generate hash
    h := sha256.Sum256(content)
    hash := hex.EncodeToString(h[:])

    // Create path for content
    path := filepath.Join(s.root, hash[:2], hash[2:])
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return "", fmt.Errorf("creating content directory: %w", err)
    }

    // Write content if it doesn't exist
    if _, err := os.Stat(path); os.IsNotExist(err) {
        if err := os.WriteFile(path, content, 0644); err != nil {
            return "", fmt.Errorf("writing content: %w", err)
        }
    }

    // Cache the content
    s.mu.Lock()
    s.cache[hash] = content
    s.mu.Unlock()

    return hash, nil
}

var ErrContentNotFound = errors.New("content not found")

func (cs *FileStore) Get(hash string) ([]byte, error) {
    hashPrefix := hash[:2]
    contentPath := filepath.Join(cs.root, ".tig", "content", hashPrefix, hash)

    contentBytes, err := os.ReadFile(contentPath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrContentNotFound
        }
        return nil, fmt.Errorf("reading content: %w", err)
    }
    return contentBytes, nil
}

// Exists checks if content exists
func (s *FileStore) Exists(hash string) bool {
    if hash == "" {
        return false
    }

    // Check cache first
    s.mu.RLock()
    _, ok := s.cache[hash]
    s.mu.RUnlock()
    if ok {
        return true
    }

    path := filepath.Join(s.root, hash[:2], hash[2:])
    _, err := os.Stat(path)
    return err == nil
}