// internal/workspace/local.go
package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tig/internal/content"
	"tig/internal/diff"
	"tig/internal/intent"
	"tig/internal/safe"
	"tig/internal/stream"
	"tig/shared/types"
	"tig/shared/utils"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// Add this at the top with the other type definitions
type FileState struct {
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
	Tracked map[string]bool
}

const gatedChangePrefix = "gated/"

var logger, _ = zap.NewDevelopment()

// CleanupGatedChanges removes only gated changes with missing content files
// while preserving valid gated files
func (w *LocalWorkspace) CleanupGatedChanges() error {
	w.Logger.Info("Starting CleanupGatedChanges")

	w.Mu.Lock()
	defer w.Mu.Unlock() // Ensure mutex is unlocked even if an error occurs

	toRemove := make([]string, 0)

	for path, changeObj := range w.GatedChanges {
		// First check if the original file still exists
		absPath := filepath.Join(w.Root, path)
		_, err := os.Stat(absPath)
		if err == nil {
			// File exists, skip cleanup
			continue
		}
		if !os.IsNotExist(err) {
			// An error other than "not exist" occurred
			w.Logger.Warn("Error checking file existence", zap.String("path", path), zap.Error(err))
			continue
		}

		// File is missing, check if content exists in store using NewHash
		newHash := changeObj.NewHash
		if newHash == "" {
			// If NewHash is empty, fallback to OldHash or skip
			newHash = changeObj.OldHash
			if newHash == "" {
				w.Logger.Warn("Both NewHash and OldHash are empty for path", zap.String("path", path))
				continue
			}
		}

		// Construct contentPath using the first two characters of the hash
		if len(newHash) < 2 {
			w.Logger.Warn("Hash length less than 2 characters", zap.String("hash", newHash), zap.String("path", path))
			continue
		}
		contentDir := newHash[:2]
		contentPath := filepath.Join(w.Root, ".tig", "content", contentDir, newHash)

		_, err = os.Stat(contentPath)
		if os.IsNotExist(err) {
			// Both file and content are missing, mark for removal
			toRemove = append(toRemove, path)
			w.Logger.Warn("Identified missing content file for gated change",
				zap.String("path", path),
				zap.String("hash", newHash))
		} else if err != nil {
			// An error other than "not exist" occurred while checking content
			w.Logger.Warn("Error checking content file existence", zap.String("path", contentPath), zap.Error(err))
		}
	}

	w.Logger.Info("Total gated changes found", zap.Int("total", len(w.GatedChanges)))
	w.Logger.Info("Total orphaned changes to remove", zap.Int("toRemove", len(toRemove)))

	// Remove paths from the map
	for _, path := range toRemove {
		delete(w.GatedChanges, path)
		w.Logger.Info("Removed orphaned gated change from map", zap.String("path", path))
	}

	return nil
}

// FindRoot searches for the workspace Root by looking for the ".tig" directory.
func FindRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".tig")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("workspace Root not found")
}

// LocalWorkspace implements Workspace interface
type LocalWorkspace struct {
	Root         string
	DB           *badger.DB
	ContentSafe  *safe.Safe
	GatedChanges map[string]shared.Change
	Intents      map[string]*intent.Intent
	Streams      map[string]*stream.Stream
	Mu           sync.RWMutex
	Logger       *zap.Logger
	Tracked      map[string]bool
}

// GetGatedChanges retrieves gated changes as a slice of content.Change.
func (w *LocalWorkspace) GetGatedChanges() ([]content.Change, error) {
	w.Mu.RLock()
	defer w.Mu.RUnlock()

	changes := make([]content.Change, 0, len(w.GatedChanges))
	for path, change := range w.GatedChanges {
		// Check if the file exists
		absPath := filepath.Join(w.Root, path)
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Skip missing files
				continue
			}
			return nil, fmt.Errorf("checking file %s: %w", path, err)
		}

		// Handle empty files
		if info.Size() == 0 {
			changes = append(changes, content.Change{
				Path:    path,
				Type:    "added",
				Content: []byte{}, // Empty content
				Mode:    0644,
				Hash:    change.NewHash,
			})
			continue
		}

		// Get content for non-empty files
		c, err := w.ContentSafe.Get(change.NewHash)
		if err != nil {
			// Skip files with missing content instead of failing
			logger.Warn("Failed to get content for gated file",
				zap.String("path", path),
				zap.String("hash", change.NewHash),
				zap.Error(err))
			continue
		}

		changes = append(changes, content.Change{
			Path:    path,
			Type:    "added",
			Content: c,
			Mode:    0644,
			Hash:    change.NewHash,
		})
	}
	return changes, nil
}

// UpdateIntent updates an existing intent in the storage.
func (w *LocalWorkspace) UpdateIntent(i *intent.Intent) error {
	if i == nil {
		return fmt.Errorf("intent cannot be nil")
	}

	w.Mu.Lock()
	defer w.Mu.Unlock()

	existingIntent, exists := w.Intents[i.ID]
	if !exists {
		return fmt.Errorf("intent with ID %s does not exist", i.ID)
	}

	// Update the existing intent's fields
	existingIntent.Description = i.Description
	existingIntent.Type = i.Type
	// Update other fields as necessary

	// Persist the changes to storage
	err := w.persistIntent(existingIntent)
	if err != nil {
		return fmt.Errorf("failed to persist intent: %w", err)
	}

	w.Logger.Info("Intent updated", zap.String("intentID", i.ID))
	return nil
}

// persistIntent saves the intent to the storage backend.
// Modify this function based on your storage mechanism (e.g., file system, database).
func (w *LocalWorkspace) persistIntent(i *intent.Intent) error {
	// Example: Persisting intent as a JSON file
	intentPath := filepath.Join(w.Root, ".tig", "intents", fmt.Sprintf("%s.json", i.ID))
	data, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal intent: %w", err)
	}

	err = os.WriteFile(intentPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write intent to file: %w", err)
	}

	return nil
}

// CreateIntent adds a new intent to the storage.
func (w *LocalWorkspace) CreateIntent(s1 string, s2 string) (i *intent.Intent, e error) {
	w.Mu.Lock()
	defer w.Mu.Unlock()

	newIntent := &intent.Intent{
		ID:          generateIntentID(),
		Description: s1,
		Type:        s2,
		CreatedAt:   time.Now(),
	}
	// Save the intent to storage
	w.Intents[newIntent.ID] = newIntent
	w.Logger.Info("Intent created", zap.String("intentID", newIntent.ID))
	return i, e
}

// DeleteIntent removes an intent from the storage.
func (w *LocalWorkspace) DeleteIntent(intentID string) error {
	w.Mu.Lock()
	defer w.Mu.Unlock()

	intent, exists := w.Intents[intentID]
	if !exists {
		return fmt.Errorf("intent with ID %s does not exist", intentID)
	}

	delete(w.Intents, intentID)

	// Remove the intent from storage
	err := w.removeIntentFromStorage(intent)
	if err != nil {
		return fmt.Errorf("failed to remove intent from storage: %w", err)
	}

	w.Logger.Info("Intent deleted", zap.String("intentID", intentID))
	return nil
}

// removeIntentFromStorage deletes the intent from the storage backend.
// Modify this function based on your storage mechanism (e.g., file system, database).
func (w *LocalWorkspace) removeIntentFromStorage(i *intent.Intent) error {
	// Example: Deleting the JSON file
	intentPath := filepath.Join(w.Root, ".tig", "intents", fmt.Sprintf("%s.json", i.ID))
	err := os.Remove(intentPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Logger.Warn("Intent file already removed", zap.String("intentID", i.ID))
			return nil
		}
		return fmt.Errorf("failed to delete intent file: %w", err)
	}

	return nil
}

// GetIntent retrieves an intent by its ID.
func (w *LocalWorkspace) GetIntent(intentID string) (*intent.Intent, error) {
	w.Mu.RLock()
	defer w.Mu.RUnlock()

	intent, exists := w.Intents[intentID]
	if !exists {
		return nil, fmt.Errorf("intent with ID %s does not exist", intentID)
	}

	return intent, nil
}

// ListIntents returns all intents.
func (w *LocalWorkspace) ListIntents() ([]*intent.Intent, error) {
	w.Mu.RLock()
	defer w.Mu.RUnlock()

	intents := make([]*intent.Intent, 0, len(w.Intents))
	for _, intent := range w.Intents {
		intents = append(intents, intent)
	}

	return intents, nil
}

// CreateStream creates a new stream.
func (w *LocalWorkspace) CreateStream(name, streamType string) (*stream.Stream, error) {
	newStream := &stream.Stream{
		ID:        generateStreamID(),
		Name:      name,
		Type:      streamType,
		CreatedAt: time.Now(),
		State: stream.State{
			Active: true,
		},
	}
	// Save stream to storage
	// Implementation details...
	return newStream, nil
}

// ListStreams lists all streams.
func (w *LocalWorkspace) ListStreams() ([]*stream.Stream, error) {
	// Retrieve streams from storage
	// Implementation details...
	return []*stream.Stream{}, nil
}

// AddIntentToStream adds an intent to a specific stream.
func (w *LocalWorkspace) AddIntentToStream(streamID, intentID string) error {
	// Retrieve the stream
	s, err := w.GetStream(streamID)
	if err != nil {
		return err
	}
	// Add intent to stream
	s.State.Intents = append(s.State.Intents, intentID)
	// Update stream in storage
	// Implementation details...
	return nil
}

// GetStream retrieves a stream by ID.
func (w *LocalWorkspace) GetStream(id string) (*stream.Stream, error) {
	// Implementation to get stream from storage
	return &stream.Stream{}, nil
}

// GetGatedChange retrieves a gated change by path.
func (w *LocalWorkspace) GetGatedChange(path string) (shared.Change, error) {
	w.Mu.RLock()
	defer w.Mu.RUnlock()

	change, exists := w.GatedChanges[path]
	if !exists {
		return shared.Change{}, fmt.Errorf("gated change not found for path: %s", path)
	}
	return change, nil
}

// Helper functions to generate IDs
func generateIntentID() string {
	// Generate a unique intent ID
	return "intent123"
}

func generateStreamID() string {
	// Generate a unique stream ID
	return "stream123"
}

// NewLocalWorkspace creates a new workspace instance
func NewLocalWorkspace(Root string, DB *badger.DB, ContentSafe *safe.Safe) (*LocalWorkspace, error) {
	ws := &LocalWorkspace{
		Root:         Root,
		DB:           DB,
		ContentSafe:  ContentSafe,
		GatedChanges: make(map[string]shared.Change),
		Logger:       logger,
	}

	// Load any existing gated changes
	if err := ws.LoadGatedChanges(); err != nil {
		return &LocalWorkspace{}, err
	}

	return ws, nil
}

// Ungate implements Workspace.Ungate
func (w *LocalWorkspace) Ungate(paths []string) error {
	w.Mu.Lock()
	defer w.Mu.Unlock()

	for _, path := range paths {
		// Remove from GatedChanges map
		delete(w.GatedChanges, path)

		// Delete from BadgerDB
		err := w.DB.Update(func(txn *badger.Txn) error {
			key := []byte(gatedChangePrefix + path)
			return txn.Delete(key)
		})
		if err != nil {
			return fmt.Errorf("deleting gated change for %s: %w", path, err)
		}
	}

	return nil
}

// gateDirectory gates all eligible files within a directory.
func (w *LocalWorkspace) gateDirectory(dirPath string) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(w.Root, path)
		if err != nil {
			return err
		}
		if w.shouldIgnore(relPath) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return w.gateFile(path)
		}
		return nil
	})
}

func (w *LocalWorkspace) Close() error {
	return w.DB.Close()
}

// gateAll gates all eligible files in the workspace.
func (w *LocalWorkspace) gateAll() error {
	// First, get current status to determine file states
	statuses, err := w.Status()
	if err != nil {
		return fmt.Errorf("getting status: %w", err)
	}

	// Process each file based on its status
	for _, status := range statuses {
		if status.Type == "delete" {
			// Handle deleted files
			w.GatedChanges[status.Path] = status
			continue
		}

		absPath := filepath.Join(w.Root, status.Path)
		if err := w.gateFile(absPath); err != nil {
			w.Logger.Warn("Failed to gate file",
				zap.String("path", status.Path),
				zap.Error(err))
			// Continue with other files rather than failing completely
			continue
		}
	}

	return w.saveGatedChanges()
}

// getFileState retrieves the last known state of a file
func (w *LocalWorkspace) getFileState(path string) (*FileState, error) {
	var state FileState

	err := w.DB.View(func(txn *badger.Txn) error {
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
func (w *LocalWorkspace) storeFileState(path string, state *FileState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling file state: %w", err)
	}

	return w.DB.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("file_state:%s", path))
		return txn.Set(key, data)
	})
}

// deleteFileState removes the stored state for a file
func (w *LocalWorkspace) deleteFileState(path string) error {
	return w.DB.Update(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("file_state:%s", path))
		return txn.Delete(key)
	})
}

// internal/workspace/local.go

// Fixed ShowFileDiff to avoid infinite recursion
func (w *LocalWorkspace) ShowFileDiff(path string) (*diff.DiffResult, error) {
	w.Mu.RLock()
	defer w.Mu.RUnlock()

	absPath := filepath.Join(w.Root, path)

	// Read current content
	currentContent, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Get previous state
	prevState, err := w.getFileState(path)
	if err != nil && err != badger.ErrKeyNotFound {
		return nil, err
	}

	var oldContent []byte
	if prevState != nil {
		oldContent, err = w.ContentSafe.Get(prevState.Hash)
		if err != nil {
			oldContent = []byte{} // Use empty content if previous not found
		}
	}

	// Use diff engine directly
	return diff.NewEngine(3).Diff(oldContent, currentContent)
}

// Helper function to properly load gated changes after the gate operation
func (w *LocalWorkspace) LoadGatedChanges() error {
	w.GatedChanges = make(map[string]shared.Change)
	return w.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("gated:")
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			path := string(bytes.TrimPrefix(key, []byte("gated:")))
			err := item.Value(func(val []byte) error {
				var change shared.Change
				if err := json.Unmarshal(val, &change); err != nil {
					return err
				}
				w.GatedChanges[path] = change
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Gate handles gating files for tracking
func (w *LocalWorkspace) Gate(paths []string) error {
    w.Mu.Lock()
    defer w.Mu.Unlock()

    if len(paths) == 0 {
        return fmt.Errorf("no paths specified")
    }

    processed := make(map[string]bool)

    for _, path := range paths {
        // Clean and normalize path
        cleanPath := filepath.Clean(path)
        absPath := filepath.Join(w.Root, cleanPath)
        relPath, err := filepath.Rel(w.Root, absPath)
        
        // Skip if we can't get relative path or already processed
        if err != nil || processed[relPath] || w.shouldIgnore(relPath) {
            continue
        }

        info, err := os.Stat(absPath)
        if err != nil {
            if os.IsNotExist(err) && w.GatedChanges[relPath].Type == "delete" {
                processed[relPath] = true
                continue
            }
            w.Logger.Error("Failed to stat path",
                zap.String("path", absPath),
                zap.Error(err))
            continue
        }

        if info.IsDir() {
            // Handle directory recursively
            err = filepath.WalkDir(absPath, func(p string, d fs.DirEntry, err error) error {
                if err != nil {
                    return err
                }

                if d.IsDir() {
                    return nil
                }

                fileRelPath, err := filepath.Rel(w.Root, p)
                if err != nil {
                    w.Logger.Warn("Failed to get relative path",
                        zap.String("path", p),
                        zap.Error(err))
                    return nil
                }

                if processed[fileRelPath] || w.shouldIgnore(fileRelPath) {
                    return nil
                }

                if err := w.gateFile(fileRelPath); err != nil {
                    w.Logger.Warn("Failed to gate file",
                        zap.String("path", fileRelPath),
                        zap.Error(err))
                    return nil
                }

                processed[fileRelPath] = true
                return nil
            })
            if err != nil {
                w.Logger.Error("Failed to walk directory",
                    zap.String("path", absPath),
                    zap.Error(err))
            }
            continue
        }

        // Handle single file
        if err := w.gateFile(relPath); err != nil {
            w.Logger.Warn("Failed to gate file",
                zap.String("path", relPath),
                zap.Error(err))
            continue
        }

        processed[relPath] = true
    }

    return w.saveGatedChanges()
}

// gateFile handles gating a single file
func (w *LocalWorkspace) gateFile(relPath string) error {
    absPath := filepath.Join(w.Root, relPath)
    
    content, err := os.ReadFile(absPath)
    if err != nil {
        return fmt.Errorf("reading file: %w", err)
    }

    currentHash := utils.HashContent(content)

    // Store content in ContentSafe
    if _, err := w.ContentSafe.Store(content); err != nil {
        return fmt.Errorf("storing content: %w", err)
    }

    info, err := os.Stat(absPath)
    if err != nil {
        return fmt.Errorf("getting file info: %w", err)
    }

    // Determine change type
    changeType := "modify"
    if _, exists := w.GatedChanges[relPath]; !exists {
        changeType = "add"
    }

    w.GatedChanges[relPath] = shared.Change{
        Path:    relPath,
        Type:    changeType,
        NewHash: currentHash,
        Mode:    int(info.Mode()),
        Size:    info.Size(),
        ModTime: info.ModTime(),
        Gated:   true,
    }

    return nil
}

// shouldIgnore checks if a path should be ignored
func (w *LocalWorkspace) shouldIgnore(path string) bool {
    if path == "" {
        return true
    }

    // Get absolute path for checking
    absPath := filepath.Join(w.Root, path)
    
    // Don't ignore the root directory itself
    if absPath == w.Root {
        return false
    }

    // Check each path component
    for _, part := range strings.Split(path, string(filepath.Separator)) {
        // Skip empty parts
        if part == "" {
            continue
        }

        // Ignore hidden files and directories
        if strings.HasPrefix(part, ".") {
            return true
        }

        // Ignore common directories
        switch part {
        case "node_modules", "vendor", "dist", "build", ".git", ".tig":
            return true
        }
    }

    return false
}

// saveGatedChanges persists gated changes to storage
func (w *LocalWorkspace) saveGatedChanges() error {
    return w.DB.Update(func(txn *badger.Txn) error {
        for path, change := range w.GatedChanges {
            data, err := json.Marshal(change)
            if err != nil {
                return fmt.Errorf("marshaling change for %s: %w", path, err)
            }
            
            key := []byte(fmt.Sprintf("gated:%s", path))
            if err := txn.Set(key, data); err != nil {
                return fmt.Errorf("storing change for %s: %w", path, err)
            }
        }
        return nil
    })
}

// internal/workspace/local.go

// Status returns the current state of the workspace
func (w *LocalWorkspace) Status() ([]shared.Change, error) {
    w.Mu.RLock()
    defer w.Mu.RUnlock()

    if w.DB == nil {
        return nil, fmt.Errorf("database not initialized")
    }

    var changes []shared.Change
    seenPaths := make(map[string]bool)

    // First, include all gated changes
    for path, change := range w.GatedChanges {
        seenPaths[path] = true
        changes = append(changes, change)
    }

    // Walk through workspace to find other changes
    err := filepath.WalkDir(w.Root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        // Skip directories
        if d.IsDir() {
            if w.shouldIgnore(path) {
                return filepath.SkipDir
            }
            return nil
        }

        // Get path relative to workspace root
        relPath, err := filepath.Rel(w.Root, path)
        if err != nil {
            w.Logger.Warn("Failed to get relative path",
                zap.String("path", path),
                zap.Error(err))
            return nil
        }

        // Skip if already seen or should be ignored
        if seenPaths[relPath] || w.shouldIgnore(relPath) {
            return nil
        }

        // Get file info
        info, err := d.Info()
        if err != nil {
            w.Logger.Warn("Failed to get file info",
                zap.String("path", relPath),
                zap.Error(err))
            return nil
        }

        // Read current content
        content, err := os.ReadFile(path)
        if err != nil {
            w.Logger.Warn("Failed to read file",
                zap.String("path", relPath),
                zap.Error(err))
            return nil
        }

        currentHash := utils.HashContent(content)

        // Get previous state if any
        var changeType string
        _, err = w.getFileState(relPath)
        if err != nil {
            if err != badger.ErrKeyNotFound {
                w.Logger.Warn("Failed to get file state",
                    zap.String("path", relPath),
                    zap.Error(err))
            }
            changeType = "untracked"
        } else {
            changeType = "modify"
        }

        // Create change record
        change := shared.Change{
            Path:    relPath,
            Type:    changeType,
            NewHash: currentHash,
            Mode:    int(info.Mode()),
            Size:    info.Size(),
            ModTime: info.ModTime(),
            Gated:   false,
        }

        changes = append(changes, change)
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("walking workspace: %w", err)
    }

    // Check for deleted files
    err = w.DB.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.Prefix = []byte("file_state:")
        it := txn.NewIterator(opts)
        defer it.Close()

        for it.Rewind(); it.Valid(); it.Next() {
            item := it.Item()
            key := item.Key()
            path := string(bytes.TrimPrefix(key, []byte("file_state:")))

            if seenPaths[path] {
                continue
            }

            absPath := filepath.Join(w.Root, path)
            if _, err := os.Stat(absPath); os.IsNotExist(err) {
                changes = append(changes, shared.Change{
                    Path:  path,
                    Type:  "delete",
                    Gated: false,
                })
            }
        }
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("checking deleted files: %w", err)
    }

    return changes, nil
}