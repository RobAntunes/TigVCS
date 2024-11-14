// internal/change/auto_tracker.go
package change

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tig/internal/diff"
	"tig/shared/types"
	"tig/shared/utils"

	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// saveTrackedFiles saves the tracked files to the database
func (t *LocalTracker) saveTrackedFiles() error {
	return t.DB.Update(func(txn *badger.Txn) error {
		// First delete all existing tracked entries
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("tracked:")

		it := txn.NewIterator(opts)
		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			if err := txn.Delete(key); err != nil {
				return err
			}

		}
		it.Close()

		// Then save current tracked files
		for path := range t.Tracked {
			key := []byte("tracked:" + path)
			if err := txn.Set(key, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// AutoTracker wraps the LocalTracker with automatic tracking capabilities
type AutoTracker struct {
	*LocalTracker
	watcher    *fsnotify.Watcher
	ignoreDirs map[string]bool
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewAutoTracker creates a new AutoTracker instance
func NewAutoTracker(tracker *LocalTracker, logger *zap.Logger) (*AutoTracker, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}

	at := &AutoTracker{
		LocalTracker: tracker,
		watcher:      watcher,
		ignoreDirs: map[string]bool{
			".git":         true,
			".tig":         true,
			"node_modules": true,
			"vendor":       true,
			"dist":         true,
			"build":        true,
		},
		logger: logger,
	}

	// Start watching goroutine
	go at.watchLoop()

	// Initialize tracking for all files
	if err := at.initializeTracking(); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("initializing tracking: %w", err)
	}

	return at, nil
}

// initializeTracking sets up initial tracking for all files
func (at *AutoTracker) initializeTracking() error {
	return filepath.Walk(at.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip ignored directories
		if info.IsDir() {
			if at.ShouldIgnore(path) {
				return filepath.SkipDir
			}
			// Add directory to watcher
			if err := at.watcher.Add(path); err != nil {
				return fmt.Errorf("adding directory to watcher: %w", err)
			}
			return nil
		}

		// Track the file
		relPath, err := filepath.Rel(at.Root, path)
		if err != nil {
			return fmt.Errorf("getting relative path: %w", err)
		}

		if !at.ShouldIgnore(relPath) {
			at.mu.Lock()
			at.Tracked[relPath] = true
			at.mu.Unlock()
		}

		return nil
	})
}

// watchLoop processes filesystem events
func (at *AutoTracker) watchLoop() {
	for {
		select {
		case event, ok := <-at.watcher.Events:
			if !ok {
				return
			}
			at.handleFSEvent(event)
		case err, ok := <-at.watcher.Errors:
			if !ok {
				return
			}
			at.logger.Error("watcher error", zap.Error(err))
		}
	}
}

// handleFSEvent processes individual filesystem events
func (at *AutoTracker) handleFSEvent(event fsnotify.Event) {
	// Get relative path
	relPath, err := filepath.Rel(at.Root, event.Name)
	if err != nil {
		at.logger.Error("getting relative path", zap.Error(err))
		return
	}

	// Skip ignored paths
	if at.ShouldIgnore(relPath) {
		return
	}

	at.mu.Lock()
	defer at.mu.Unlock()

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		// Handle directory creation
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if err := at.watcher.Add(event.Name); err != nil {
				at.logger.Error("adding new directory to watcher", zap.Error(err))
			}
		}
		at.Tracked[relPath] = true

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		delete(at.Tracked, relPath)

	case event.Op&fsnotify.Write == fsnotify.Write:
		at.Tracked[relPath] = true

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		delete(at.Tracked, relPath)
	}

	// Save tracked files state
	if err := at.saveTrackedFiles(); err != nil {
		at.logger.Error("saving tracked files", zap.Error(err))
	}
}

// shouldIgnore checks if a path should be ignored
func (at *AutoTracker) ShouldIgnore(path string) bool {
	if path == "" {
		return true
	}

	parts := strings.Split(path, string(filepath.Separator))
	for _, part := range parts {
		if at.ignoreDirs[part] {
			return true
		}
	}

	return false
}

// Close cleans up resources
func (at *AutoTracker) Close() error {
	return at.watcher.Close()
}

func (lt *LocalTracker) Track(paths []string) error {
	lt.Mu.Lock()
	defer lt.Mu.Unlock()

	for _, path := range paths {
		absPath := filepath.Join(lt.Root, path)
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("accessing path %s: %w", path, err)
		}

		if info.IsDir() {
			// Handle directory recursively
			if err := filepath.WalkDir(absPath, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					relPath, err := filepath.Rel(lt.Root, p)
					if err != nil {
						return err
					}
					lt.Tracked[relPath] = true
				}
				return nil
			}); err != nil {
				return fmt.Errorf("walking directory %s: %w", path, err)
			}
		} else {
			// Handle single file
			relPath, err := filepath.Rel(lt.Root, absPath)
			if err != nil {
				return err
			}
			lt.Tracked[relPath] = true
		}
	}

	return lt.saveTrackedFiles()
}

// Track is a no-op since tracking is automatic
func (at *AutoTracker) Track(paths []string) error {
	at.logger.Info("automatic tracking enabled, explicit tracking not needed")
	return nil
}

func (lt *LocalTracker) Untrack(paths []string) error {
	lt.Mu.Lock()
	defer lt.Mu.Unlock()

	for _, path := range paths {
		// Remove from tracked files map
		delete(lt.Tracked, path)

		// Also remove file state from database
		if err := lt.deleteFileState(path); err != nil && err != badger.ErrKeyNotFound {
			return fmt.Errorf("deleting file state for %s: %w", path, err)
		}
	}

	// Save updated tracked files list
	return lt.saveTrackedFiles()
}

// Untrack removes files from tracking
func (at *AutoTracker) Untrack(paths []string) error {
	at.mu.Lock()
	defer at.mu.Unlock()

	for _, path := range paths {
		delete(at.Tracked, path)
	}

	return at.saveTrackedFiles()
}



// Helper function for checking if a path should be ignored
func shouldIgnore(path string) bool {
	if path == "" {
		return true
	}

	base := filepath.Base(path)
	if base[0] == '.' {
		return true
	}

	ignoreList := map[string]bool{
		".git":         true,
		".tig":         true,
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
	}

	return ignoreList[base]
}

func (lt *LocalTracker) ShowFileDiff(path string) (*diff.DiffResult, error) {
	lt.Mu.RLock()
	defer lt.Mu.RUnlock()

	absPath := filepath.Join(lt.Root, path)

	// Read current content
	currentContent, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Get previous state
	prevState, err := lt.getFileState(path)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, fmt.Errorf("file not previously tracked")
		}
		return nil, err
	}

	// Get old content
	oldContent, err := lt.ContentSafe.Get(prevState.Hash)
	if err != nil {
		return nil, fmt.Errorf("getting previous content: %w", err)
	}

	// Generate diff using the DiffEngine
	return lt.DiffEngine.Diff(oldContent, currentContent)
}

// ShowFileDiff computes the diff for a specific file
func (at *AutoTracker) ShowFileDiff(path string) (*diff.DiffResult, error) {
	at.mu.RLock()
	defer at.mu.RUnlock()

	absPath := filepath.Join(at.Root, path)
	currentContent, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading current file: %w", err)
	}

	// Get old content from last saved state
	oldContent, err := at.ContentSafe.Get(utils.HashContent(currentContent))
	if err != nil {
		// If no old content exists, compare with empty content
		oldContent = []byte{}
	}

	return at.DiffEngine.Diff(oldContent, currentContent)
}

// Add this helper function for generating IDs
func generateChangeSetID() string {
	return uuid.New().String()
}

// GetGatedChange retrieves the gated change details for a given relative path.
func (at *AutoTracker) GetGatedChange(relPath string) (shared.Change, error) {
    at.Mu.RLock()
    defer at.Mu.RUnlock()

    changeObj, exists := at.GatedChanges[relPath]
    if !exists {
        return shared.Change{}, fmt.Errorf("no gated change found for path: %s", relPath)
    }

    return changeObj, nil
}

func (lt *LocalTracker) hashChangeSet(changes []shared.Change) string {
	h := sha256.New()
	for _, change := range changes {
		// Include all relevant fields in hash calculation
		h.Write([]byte(change.Path))
		h.Write([]byte(change.Type))
		h.Write([]byte(change.NewHash))
		h.Write([]byte(change.OldHash))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (lt *LocalTracker) storeChangeSet(cs *ChangeSet) error {
	data, err := json.Marshal(cs)
	if err != nil {
		return fmt.Errorf("marshaling changeset: %w", err)
	}

	return lt.DB.Update(func(txn *badger.Txn) error {
		// Store main changeset data
		key := []byte(fmt.Sprintf("changeset:%s", cs.ID))
		if err := txn.Set(key, data); err != nil {
			return fmt.Errorf("storing changeset: %w", err)
		}

		// Store time index
		timeKey := []byte(fmt.Sprintf("cs_time:%d:%s", cs.CreatedAt.Unix(), cs.ID))
		if err := txn.Set(timeKey, nil); err != nil {
			return fmt.Errorf("storing time index: %w", err)
		}

		// Store path indices for each changed file
		for _, change := range cs.Changes {
			pathKey := []byte(fmt.Sprintf("cs_path:%s:%s", change.Path, cs.ID))
			if err := txn.Set(pathKey, nil); err != nil {
				return fmt.Errorf("storing path index: %w", err)
			}
		}

		return nil
	})
}

func (lt *LocalTracker) updateFileState(change shared.Change) error {
	state := FileState{
		Hash:    change.NewHash,
		ModTime: change.ModTime,
		Size:    change.Size,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling file state: %w", err)
	}

	return lt.DB.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("file_state:%s", change.Path))
		return txn.Set(key, data)
	})
}

// FileState represents the tracked state of a file
type FileState struct {
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

// getFileState retrieves the last known state of a file
func (lt *LocalTracker) getFileState(path string) (*FileState, error) {
	var state FileState

	err := lt.DB.View(func(txn *badger.Txn) error {
		// Construct key for file state
		key := []byte(fmt.Sprintf("file_state:%s", path))

		// Get state from database
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &state)
		})
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, err // Preserve ErrKeyNotFound for proper handling
		}
		return nil, fmt.Errorf("retrieving file state: %w", err)
	}

	return &state, nil
}

// storeFileState saves the current state of a file
func (lt *LocalTracker) storeFileState(path string, state *FileState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling file state: %w", err)
	}

	return lt.DB.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("file_state:%s", path))
		return txn.Set(key, data)
	})
}

// deleteFileState removes the stored state for a file
func (lt *LocalTracker) deleteFileState(path string) error {
	return lt.DB.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("file_state:%s", path))
		return txn.Delete(key)
	})
}

// CreateChangeSet with fixed error handling for empty changes
func (lt *LocalTracker) CreateChangeSet(description string) (*ChangeSet, error) {
    lt.Mu.Lock()
    defer lt.Mu.Unlock()

    // Convert gated changes map to slice
    changes := make([]shared.Change, 0, len(lt.GatedChanges))
    for _, change := range lt.GatedChanges {
        changes = append(changes, change)
    }

    // Don't return error for empty changes during status check
    if len(changes) == 0 && description == "status" {
        return &ChangeSet{
            Changes: changes,
            CreatedAt: time.Now(),
            Description: description,
        }, nil
    }

    // For non-status operations, require changes
    if len(changes) == 0 {
        return nil, fmt.Errorf("no changes to commit")
    }

    id := uuid.New().String()
    cs := &ChangeSet{
        ID:          id,
        Changes:     changes,
        CreatedAt:   time.Now(),
        Description: description,
        Hash:        lt.hashChangeSet(changes),
    }

    // Store changeset
    if err := lt.storeChangeSet(cs); err != nil {
        return nil, fmt.Errorf("storing changeset: %w", err)
    }

    return cs, nil
}

// Status implementation with proper change tracking
func (lt *LocalTracker) Status() ([]shared.Change, error) {
    // Create a changeset just for status (won't error on empty changes)
    cs, err := lt.CreateChangeSet("status")
    if err != nil {
        return nil, fmt.Errorf("creating status changeset: %w", err)
    }

    return cs.Changes, nil
}

// Improved gating to properly track changes
func (lt *LocalTracker) Gate(path string) error {
    lt.Mu.Lock()
    defer lt.Mu.Unlock()

    absPath := filepath.Join(lt.Root, path)
    info, err := os.Stat(absPath)
    if err != nil {
        return fmt.Errorf("accessing file %s: %w", path, err)
    }

    // Read current content
    content, err := os.ReadFile(absPath)
    if err != nil {
        return fmt.Errorf("reading file %s: %w", path, err)
    }

    // Generate hash for current content
    currentHash := utils.HashContent(content)

    // Store content if it doesn't exist
    if _, err := lt.ContentSafe.Store(content); err != nil {
        return fmt.Errorf("storing content: %w", err)
    }

    // Create change record
    change := shared.Change{
        Path:    path,
        Type:    "modify",
        NewHash: currentHash,
        Mode:    int(info.Mode()),
        Size:    info.Size(),
        ModTime: info.ModTime(),
        Gated:   true,
    }

    // Add to gated changes
    lt.GatedChanges[path] = change

    return lt.DB.Update(func(txn *badger.Txn) error {
        key := []byte(fmt.Sprintf("gated:%s", path))
        data, err := json.Marshal(change)
        if err != nil {
            return fmt.Errorf("marshaling change: %w", err)
        }
        return txn.Set(key, data)
    })
}

// Status retrieves the current status of the workspace.
func (at *AutoTracker) Status() ([]shared.Change, error) {
    at.mu.RLock()
    defer at.mu.RUnlock()

    var changes []shared.Change

    // Walk through directories and accumulate changes
    err := filepath.WalkDir(at.Root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        if d.IsDir() {
            if at.ShouldIgnore(path) {
                return filepath.SkipDir
            }
            return nil
        }

        relPath, err := filepath.Rel(at.Root, path)
        if err != nil || at.ShouldIgnore(relPath) {
            return nil
        }

        gatedChange, isGated := at.GatedChanges[relPath]
        if isGated {
            // Use the gated change details
            gatedChange.Gated = true
            changes = append(changes, gatedChange)
            return nil
        }

        // For non-gated files, check if they're tracked
        isTracked := at.Tracked[relPath]

        content, err := os.ReadFile(path)
        if err != nil {
            at.Logger.Warn("Failed to read file content", zap.String("path", relPath), zap.Error(err))
            return nil
        }

        info, err := os.Stat(path)
        if err != nil {
            at.Logger.Warn("Failed to get file info", zap.String("path", relPath), zap.Error(err))
            return nil
        }

        currentHash := utils.HashContent(content)
        changeType := "untracked"
        if isTracked {
            changeType = "modify"
        }

        change := shared.Change{
            Path:    relPath,
            Type:    changeType,
            Gated:   false,
            NewHash: currentHash,
            Mode:    int(info.Mode()),
            Size:    info.Size(),
            ModTime: info.ModTime(),
        }

        changes = append(changes, change)
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("walking workspace: %w", err)
    }

    // Check for deleted files
    for path, wasTracked := range at.Tracked {
        if wasTracked {
            absPath := filepath.Join(at.Root, path)
            if _, err := os.Stat(absPath); os.IsNotExist(err) {
                changes = append(changes, shared.Change{
                    Path:  path,
                    Type:  "delete",
                    Gated: false,
                })
            }
        }
    }

    return changes, nil
}

// Gate implements Tracker.Gate
func (at *AutoTracker) Gate(path string) error {
    at.mu.Lock()
    defer at.mu.Unlock()

    absPath := filepath.Join(at.Root, path)
    info, err := os.Stat(absPath)
    if err != nil {
        if os.IsNotExist(err) {
            // Handle deleted files
            if at.Tracked[path] {
                at.GatedChanges[path] = shared.Change{
                    Path:  path,
                    Type:  "delete",
                    Gated: true,
                }
                return nil
            }
            return fmt.Errorf("file does not exist and was not tracked: %s", path)
        }
        return fmt.Errorf("accessing file %s: %w", path, err)
    }

    if info.IsDir() {
        return filepath.WalkDir(absPath, func(subpath string, d fs.DirEntry, err error) error {
            if err != nil {
                return err
            }
            if !d.IsDir() {
                relPath, err := filepath.Rel(at.Root, subpath)
                if err != nil {
                    return err
                }
                if err := at.gateFile(relPath); err != nil {
                    return err
                }
            }
            return nil
        })
    }

    relPath, err := filepath.Rel(at.Root, absPath)
    if err != nil {
        return err
    }

    return at.gateFile(relPath)
}

// gateFile handles gating a single file
func (at *AutoTracker) gateFile(relPath string) error {
    if at.ShouldIgnore(relPath) {
        return nil
    }

    absPath := filepath.Join(at.Root, relPath)
    content, err := os.ReadFile(absPath)
    if err != nil {
        return fmt.Errorf("reading file: %w", err)
    }

    currentHash := utils.HashContent(content)

    // Store content in ContentSafe
    if _, err := at.ContentSafe.Store(content); err != nil {
        return fmt.Errorf("storing content: %w", err)
    }

    info, err := os.Stat(absPath)
    if err != nil {
        return fmt.Errorf("getting file info: %w", err)
    }

    changeType := "modify"
    if !at.Tracked[relPath] {
        changeType = "add"
    }

    at.GatedChanges[relPath] = shared.Change{
        Path:    relPath,
        Type:    changeType,
        NewHash: currentHash,
        Mode:    int(info.Mode()),
        Size:    info.Size(),
        ModTime: info.ModTime(),
        Gated:   true,
    }

    // Make sure the file is tracked
    at.Tracked[relPath] = true

    return nil
}
