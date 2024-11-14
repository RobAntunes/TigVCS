// internal/parcel/parcel.go
package parcel

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tig/internal/diff"
	"tig/internal/intent"

	"tig/internal/safe"

	"tig/shared/types"

	"tig/internal/workspace"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// Ungate removes files from being included in the next intent
func (p *Parcel) Ungate(paths []string) error {
	return p.Workspace.Ungate(paths)
}

// UpdateIntent updates an existing intent
func (p *Parcel) UpdateIntent(i *intent.Intent) error {
	return p.IntentStore.Update(i)
}

// CreateIntent creates a new intent
func (p *Parcel) CreateIntent(description string, intentType string) (*intent.Intent, error) {
	i := &intent.Intent{
		Description: description,
		Type:        intentType,
	}

	if err := p.IntentStore.Create(i); err != nil {
		return nil, err
	}

	return i, nil
}

// Untrack wraps the tracker's Untrack method
func (p *Parcel) Untrack(paths []string) error {
	return p.Tracker.Untrack(paths)
}

// ShowFileDiff wraps the tracker's ShowFileDiff method
func (p *Parcel) ShowFileDiff(path string) (*diff.DiffResult, error) {
	return p.Tracker.ShowFileDiff(path)
}

func Initialize(root string) error {
	// Create .tig directory
	tigDir := filepath.Join(root, ".tig")
	if err := os.MkdirAll(tigDir, 0755); err != nil {
		return fmt.Errorf("creating .tig directory: %w", err)
	}

	// Create subdirectories
	dirs := []string{
		filepath.Join(tigDir, "db"),
		filepath.Join(tigDir, "content"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return nil
}

func New(path string, logger *zap.Logger) (*Parcel, error) {
	// Convert path to absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path for root %s: %w", path, err)
	}

	// Verify and initialize directories with absolute path
	if err := Initialize(absPath); err != nil {
		return nil, fmt.Errorf("initializing directories: %w", err)
	}

	tigDir := filepath.Join(absPath, ".tig")

	// Initialize BadgerDB with optimized settings
	opts := badger.DefaultOptions(filepath.Join(tigDir, "db"))
	opts.Logger = nil // Disable logging noise

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Initialize Safe
	contentSafe, err := safe.New(db, safe.Options{
		Root:      filepath.Join(tigDir, "content"),
		CacheSize: 1000,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing content safe: %w", err)
	}

	workspace, err := workspace.NewLocalWorkspace(absPath, db, contentSafe)
	if err != nil {
		return nil, fmt.Errorf("creating local workspace: %w", err)
	}

	p := &Parcel{
		Root:      absPath,
		Workspace: workspace,
		Logger:    logger,
	}

	return p, nil
}

func (p *Parcel) gateDirectory(dirPath string) error {
    return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        // Skip if it's a directory
        if d.IsDir() {
            return nil
        }

        relPath, err := filepath.Rel(p.Root, path)
        if err != nil {
            p.Logger.Warn("Failed to compute relative path", 
                zap.String("path", path), 
                zap.Error(err))
            return nil
        }

        // Skip hidden files and special system files
        if strings.HasPrefix(filepath.Base(relPath), ".") {
            return nil
        }

        return p.Workspace.Gate([]string{relPath})
    })
}

func (p *Parcel) gateAll() error {
    return filepath.WalkDir(p.Root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        // Skip directories
        if d.IsDir() {
            return nil
        }

        relPath, err := filepath.Rel(p.Root, path)
        if err != nil {
            p.Logger.Warn("Failed to compute relative path", 
                zap.String("path", path), 
                zap.Error(err))
            return nil
        }

        // Skip hidden files, special system files, and ignored directories
        if strings.HasPrefix(filepath.Base(relPath), ".") || 
           strings.Contains(relPath, "node_modules/") ||
           strings.Contains(relPath, "vendor/") ||
           strings.Contains(relPath, ".git/") {
            return nil
        }

        return p.Workspace.Gate([]string{relPath})
    })
}

// internal/parcel/parcel.go

// Gate gates files for tracking
func (p *Parcel) Gate(paths []string) error {
    if p.Workspace == nil {
        return fmt.Errorf("workspace not initialized")
    }

    p.Logger.Info("Gating paths in workspace")

    // Handle paths
    var pathsToGate []string
    for _, path := range paths {
        if path == "." {
            // For ".", walk the workspace to collect all eligible files
            err := filepath.WalkDir(p.Root, func(path string, d fs.DirEntry, err error) error {
                if err != nil {
                    return err
                }

                // Skip directories
                if d.IsDir() {
                    return nil
                }

                // Get path relative to workspace root
                relPath, err := filepath.Rel(p.Root, path)
                if err != nil {
                    return nil
                }

                // Check if file should be ignored
                if !shouldIgnorePath(relPath) {
                    pathsToGate = append(pathsToGate, relPath)
                }
                return nil
            })
            if err != nil {
                return fmt.Errorf("collecting files: %w", err)
            }
            break // No need to process other paths if "." was specified
        }

        // For specific paths, add them directly
        cleanPath := filepath.Clean(path)
        if !shouldIgnorePath(cleanPath) {
            pathsToGate = append(pathsToGate, cleanPath)
        }
    }

    // Gate the collected paths
    if err := p.Workspace.Gate(pathsToGate); err != nil {
        return fmt.Errorf("gating paths: %w", err)
    }

    p.Logger.Info("Successfully gated paths", zap.Int("count", len(pathsToGate)))
    return nil
}

// shouldIgnorePath checks if a path should be ignored
func shouldIgnorePath(path string) bool {
    if path == "" {
        return true
    }

    // Check each path component
    components := strings.Split(path, string(filepath.Separator))
    for _, comp := range components {
        if comp == "" {
            continue
        }

        // Ignore hidden files and directories
        if strings.HasPrefix(comp, ".") {
            return true
        }

        // Ignore common directories
        switch comp {
        case "node_modules", "vendor", "dist", "build", ".git", ".tig":
            return true
        }
    }

    return false
}

// internal/parcel/parcel.go

// Status returns the current status of the workspace
func (p *Parcel) Status() ([]shared.Change, error) {
    if p.Workspace == nil {
        return nil, fmt.Errorf("workspace not initialized")
    }

    p.Logger.Debug("Getting workspace status")
    
    // Get status from workspace
    changes, err := p.Workspace.Status()
    if err != nil {
        return nil, fmt.Errorf("getting workspace status: %w", err)
    }

    p.Logger.Debug("Retrieved workspace status", 
        zap.Int("changes", len(changes)))
    
    return changes, nil
}

// initParcel helper function
func initParcel(root string) (*Parcel, error) {
    // Initialize logger
    logger, err := zap.NewDevelopment()
    if err != nil {
        return nil, fmt.Errorf("initializing logger: %w", err)
    }

    // Initialize Parcel
    p, err := New(root, logger)
    if err != nil {
        return nil, fmt.Errorf("initializing parcel: %w", err)
    }

    return p, nil
}

// Close ensures proper cleanup of resources
func (p *Parcel) Close() error {
    if p == nil {
        return nil
    }

    var errs []error

    // Close workspace if initialized
    if p.Workspace != nil {
        if err := p.Workspace.Close(); err != nil {
            errs = append(errs, fmt.Errorf("closing workspace: %w", err))
        }
    }

    // Close DB if initialized
    if p.DB != nil {
        if err := p.DB.Close(); err != nil {
            errs = append(errs, fmt.Errorf("closing database: %w", err))
        }
    }

    if len(errs) > 0 {
        return fmt.Errorf("closing parcel: %v", errs)
    }

    return nil
}