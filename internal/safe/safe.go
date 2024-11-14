// internal/safe/safe.go
package safe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	lru "github.com/hashicorp/golang-lru/v2"
)

var (
	ErrContentNotFound = errors.New("content not found")
	ErrInvalidHash    = errors.New("invalid content hash")
)

// ContentMeta stores metadata about stored content
type ContentMeta struct {
	Hash       string    `json:"hash"`
	Size       int64     `json:"size"`
	RefCount   uint32    `json:"ref_count"`
	Compressed bool      `json:"compressed"`
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
}

// Safe provides secure, deduplicated content storage
type Safe struct {
	root      string           // Root directory for content files
	db        *badger.DB       // Metadata database
	cache     *lru.Cache[string, []byte] // Content cache
	mu        sync.RWMutex
	batchSize int             // Size for batch operations
	decompress func([]byte) ([]byte, error)
}

// Options configures Safe behavior
type Options struct {
	Root          string // Root directory path
	CacheSize     int    // Number of items to cache
	BatchSize     int    // Size for batch operations
	CompressAfter time.Duration // When to compress old content
}

// New creates a new Safe instance
func New(db *badger.DB, opts Options) (*Safe, error) {
	if opts.Root == "" {
		return nil, fmt.Errorf("root directory is required")
	}

	// Create content directory if it doesn't exist
	if err := os.MkdirAll(opts.Root, 0755); err != nil {
		return nil, fmt.Errorf("creating root directory: %w", err)
	}

	// Set up LRU cache
	cache, err := lru.New[string, []byte](opts.CacheSize)
	if err != nil {
		return nil, fmt.Errorf("creating cache: %w", err)
	}

	// Use reasonable defaults
	if opts.BatchSize == 0 {
		opts.BatchSize = 1000
	}
	if opts.CompressAfter == 0 {
		opts.CompressAfter = 30 * 24 * time.Hour // 30 days
	}

	return &Safe{
		root:      opts.Root,
		db:        db,
		cache:     cache,
		batchSize: opts.BatchSize,
	}, nil
}

// Store saves content and returns its hash
func (s *Safe) Store(content []byte) (string, error) {
	if len(content) == 0 {
		content = []byte{} // Convert nil to empty slice
	}

	// Generate hash
	hash := s.hashContent(content)

	// Check if content already exists
	exists, err := s.Exists(hash)
	if err != nil {
		return "", fmt.Errorf("checking existence: %w", err)
	}

	if exists {
		// Increment reference count
		if err := s.incrementRefCount(hash); err != nil {
			return "", fmt.Errorf("incrementing ref count: %w", err)
		}
		return hash, nil
	}

	// Prepare content path
	contentPath := s.contentPath(hash)
	if err := os.MkdirAll(filepath.Dir(contentPath), 0755); err != nil {
		return "", fmt.Errorf("creating content directory: %w", err)
	}

	// Write content file
	if err := os.WriteFile(contentPath, content, 0644); err != nil {
		return "", fmt.Errorf("writing content file: %w", err)
	}

	// Create metadata
	meta := ContentMeta{
		Hash:       hash,
		Size:       int64(len(content)),
		RefCount:   1,
		Compressed: false,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	// Store metadata
	if err := s.storeMeta(meta); err != nil {
		// Cleanup on failure
		os.Remove(contentPath)
		return "", fmt.Errorf("storing metadata: %w", err)
	}

	// Update cache
	s.cache.Add(hash, content)

	return hash, nil
}

// Get retrieves content by hash
func (s *Safe) Get(hash string) ([]byte, error) {
	if !s.isValidHash(hash) {
		return nil, ErrInvalidHash
	}

	// Check cache first
	if content, ok := s.cache.Get(hash); ok {
		return content, nil
	}

	// Get metadata
	meta, err := s.getMeta(hash)
	if err != nil {
		return nil, fmt.Errorf("getting metadata: %w", err)
	}

	// Read content file
	contentPath := s.contentPath(hash)
	content, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrContentNotFound
		}
		return nil, fmt.Errorf("reading content: %w", err)
	}

	// Decompress if needed
	if meta.Compressed {
		content, err = s.decompress(content)
		if err != nil {
			return nil, fmt.Errorf("decompressing content: %w", err)
		}
	}

	// Verify hash
	if s.hashContent(content) != hash {
		return nil, fmt.Errorf("content hash mismatch")
	}

	// Update cache and access time
	s.cache.Add(hash, content)
	meta.AccessedAt = time.Now()
	if err := s.storeMeta(meta); err != nil {
		return nil, fmt.Errorf("updating metadata: %w", err)
	}

	return content, nil
}

// Delete removes content and decrements its reference count
func (s *Safe) Delete(hash string) error {
	if !s.isValidHash(hash) {
		return ErrInvalidHash
	}

	meta, err := s.getMeta(hash)
	if err != nil {
		return fmt.Errorf("getting metadata: %w", err)
	}

	meta.RefCount--
	if meta.RefCount == 0 {
		// Remove content file
		contentPath := s.contentPath(hash)
		if err := os.Remove(contentPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing content file: %w", err)
		}

		// Remove metadata
		if err := s.deleteMeta(hash); err != nil {
			return fmt.Errorf("deleting metadata: %w", err)
		}

		// Remove from cache
		s.cache.Remove(hash)
	} else {
		// Update metadata with new ref count
		if err := s.storeMeta(meta); err != nil {
			return fmt.Errorf("updating metadata: %w", err)
		}
	}

	return nil
}

// Exists checks if content exists
func (s *Safe) Exists(hash string) (bool, error) {
	if !s.isValidHash(hash) {
		return false, ErrInvalidHash
	}

	// Check cache first
	if s.cache.Contains(hash) {
		return true, nil
	}

	// Check metadata
	_, err := s.getMeta(hash)
	if err != nil {
		if err == ErrContentNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Verify checks content integrity
func (s *Safe) Verify(hash string) error {
	content, err := s.Get(hash)
	if err != nil {
		return err
	}

	if s.hashContent(content) != hash {
		return fmt.Errorf("content hash mismatch")
	}

	return nil
}

// StoreBatch stores multiple content items efficiently
func (s *Safe) StoreBatch(contents [][]byte) ([]string, error) {
	hashes := make([]string, len(contents))
	for i, content := range contents {
		hash, err := s.Store(content)
		if err != nil {
			// Cleanup on failure
			for j := 0; j < i; j++ {
				s.Delete(hashes[j])
			}
			return nil, fmt.Errorf("storing content %d: %w", i, err)
		}
		hashes[i] = hash
	}
	return hashes, nil
}

// GetBatch retrieves multiple content items efficiently
func (s *Safe) GetBatch(hashes []string) ([][]byte, error) {
	contents := make([][]byte, len(hashes))
	for i, hash := range hashes {
		content, err := s.Get(hash)
		if err != nil {
			return nil, fmt.Errorf("getting content %s: %w", hash, err)
		}
		contents[i] = content
	}
	return contents, nil
}

// Internal helper functions

func (s *Safe) hashContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func (s *Safe) contentPath(hash string) string {
	return filepath.Join(s.root, hash[:2], hash[2:])
}

func (s *Safe) isValidHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func (s *Safe) incrementRefCount(hash string) error {
	meta, err := s.getMeta(hash)
	if err != nil {
		return err
	}

	meta.RefCount++
	return s.storeMeta(meta)
}

func (s *Safe) storeMeta(meta ContentMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("content:%s", meta.Hash))
		return txn.Set(key, data)
	})
}

func (s *Safe) getMeta(hash string) (ContentMeta, error) {
	var meta ContentMeta

	err := s.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("content:%s", hash))
		item, err := txn.Get(key)
		if err == badger.ErrKeyNotFound {
			return ErrContentNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})
	})

	return meta, err
}

func (s *Safe) deleteMeta(hash string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("content:%s", hash))
		return txn.Delete(key)
	})
}