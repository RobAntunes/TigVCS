// internal/parcel/storage/store.go
package storage

import (
	"fmt"
	"path/filepath"
	"tig/internal/parcel"
	genericStorage "tig/internal/storage"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Store implements parcel.Box interface
type Store struct {
    store *genericStorage.BadgerStore
    config *parcel.ParcelConfig
    state  *parcel.ParcelState
}

// NewStore creates a new parcel store
func NewStore(db *badger.DB) *Store {
    return &Store{
        store: genericStorage.NewBadgerStore(db, "parcel"),
    }
}

// parcelEntity wraps parcel state for storage
type parcelEntity struct {
    Config *parcel.ParcelConfig `json:"config"`
    State  *parcel.ParcelState  `json:"state"`
}

func (p *parcelEntity) GetID() string {
    if p.Config == nil {
        return ""
    }
    return p.Config.Root // Use root path as unique identifier
}

// isInitialized checks if the store has been properly initialized
func (s *Store) isInitialized() bool {
    return s.config != nil && s.state != nil
}

func (s *Store) Create(path string) error {
    absPath, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("resolving path: %w", err)
    }

    // Create new config
    config := &parcel.ParcelConfig{
        Version: "1",
        Created: time.Now(),
        Root:    absPath,
    }

    // Create new state
    state := &parcel.ParcelState{
        GatedChanges: make(map[string]string),
        LastSync:     time.Now(),
    }

    entity := &parcelEntity{
        Config: config,
        State:  state,
    }

    if err := s.store.Create(entity); err != nil {
        return fmt.Errorf("creating parcel: %w", err)
    }

    s.config = config
    s.state = state
    return nil
}

func (s *Store) Open(path string) error {
    absPath, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("resolving path: %w", err)
    }

    var entity parcelEntity
    if err := s.store.Get(absPath, &entity); err != nil {
        return fmt.Errorf("getting parcel: %w", err)
    }

    // Validate loaded entity
    if entity.Config == nil || entity.State == nil {
        return fmt.Errorf("invalid parcel data: missing config or state")
    }

    if entity.State.GatedChanges == nil {
        entity.State.GatedChanges = make(map[string]string)
    }

    s.config = entity.Config
    s.state = entity.State
    return nil
}

func (s *Store) Close() error {
    if !s.isInitialized() {
        return nil // Nothing to save if not initialized
    }

    entity := &parcelEntity{
        Config: s.config,
        State:  s.state,
    }

    if err := s.store.Update(entity); err != nil {
        return fmt.Errorf("saving parcel state: %w", err)
    }

    return nil
}

func (s *Store) GetState() (*parcel.ParcelState, error) {
    if !s.isInitialized() {
        return nil, fmt.Errorf("parcel not initialized")
    }
    
    // Return a copy of the state to prevent direct modifications
    stateCopy := *s.state
    return &stateCopy, nil
}

func (s *Store) UpdateState(newState *parcel.ParcelState) error {
    if !s.isInitialized() {
        return fmt.Errorf("parcel not initialized")
    }
    if newState == nil {
        return fmt.Errorf("cannot update with nil state")
    }

    // Create a copy of the new state
    stateCopy := *newState

    // Ensure GatedChanges is initialized
    if stateCopy.GatedChanges == nil {
        stateCopy.GatedChanges = make(map[string]string)
    }

    s.state = &stateCopy
    entity := &parcelEntity{
        Config: s.config,
        State:  s.state,
    }

    return s.store.Update(entity)
}

func (s *Store) GetConfig() (*parcel.ParcelConfig, error) {
    if !s.isInitialized() {
        return nil, fmt.Errorf("parcel not initialized")
    }
    
    // Return a copy of the config to prevent direct modifications
    configCopy := *s.config
    return &configCopy, nil
}

func (s *Store) UpdateConfig(newConfig *parcel.ParcelConfig) error {
    if !s.isInitialized() {
        return fmt.Errorf("parcel not initialized")
    }
    if newConfig == nil {
        return fmt.Errorf("cannot update with nil config")
    }

    // Create a copy of the new config
    configCopy := *newConfig
    s.config = &configCopy

    entity := &parcelEntity{
        Config: s.config,
        State:  s.state,
    }

    return s.store.Update(entity)
}

// Create initializes a new parcel at the specified path
func Create(path string, db *badger.DB) error {
    store := NewStore(db)
    return store.Create(path)
}