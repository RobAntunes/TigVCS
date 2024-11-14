// internal/stream/storage/store.go
package storage

import (
    "fmt"
    "time"

    "github.com/dgraph-io/badger/v4"
    "tig/internal/intent"
    "tig/internal/stream"
    "tig/internal/storage"
)

// Store handles all stream storage operations
type Store struct {
    store     *storage.BadgerStore
    intentBox intent.Box
}

// NewStore creates a new stream store
func NewStore(db *badger.DB, intentBox intent.Box) *Store {
    return &Store{
        store:     storage.NewBadgerStore(db, "stream"),
        intentBox: intentBox,
    }
}

// streamEntity wraps stream.Stream to implement storage.Entity
type streamEntity struct {
    *stream.Stream
}

func (s *streamEntity) GetID() string {
    return s.ID
}

// validate checks if a stream has all required fields
func validate(s *stream.Stream) error {
    if s.Name == "" {
        return fmt.Errorf("name is required")
    }
    if s.Type == "" {
        return fmt.Errorf("type is required")
    }
    return nil
}

// Create stores a new stream
func (s *Store) Create(st *stream.Stream) error {
    if err := validate(st); err != nil {
        return fmt.Errorf("invalid stream: %w", err)
    }

    // Set timestamps and initial state
    if st.CreatedAt.IsZero() {
        st.CreatedAt = time.Now()
    }
    if st.UpdatedAt.IsZero() {
        st.UpdatedAt = st.CreatedAt
    }
    if st.State.LastSync.IsZero() {
        st.State.LastSync = st.CreatedAt
    }
    
    // Initialize state if not set
    if st.State.Status == "" {
        st.State.Status = "stable"
    }
    st.State.Active = true

    return s.store.Create(&streamEntity{Stream: st})
}

// Get retrieves a stream by ID
func (s *Store) Get(id string) (*stream.Stream, error) {
    var entity streamEntity
    entity.Stream = &stream.Stream{}
    
    if err := s.store.Get(id, &entity); err != nil {
        return nil, fmt.Errorf("getting stream: %w", err)
    }

    return entity.Stream, nil
}

// Update modifies an existing stream
func (s *Store) Update(st *stream.Stream) error {
    if err := validate(st); err != nil {
        return fmt.Errorf("invalid stream: %w", err)
    }

    st.UpdatedAt = time.Now()
    return s.store.Update(&streamEntity{Stream: st})
}

// Delete removes a stream by ID
func (s *Store) Delete(id string) error {
    return s.store.Delete(id)
}

// List returns all streams
func (s *Store) List() ([]*stream.Stream, error) {
    var entities []streamEntity
    if err := s.store.List(&entities); err != nil {
        return nil, fmt.Errorf("listing streams: %w", err)
    }

    streams := make([]*stream.Stream, len(entities))
    for i, entity := range entities {
        streams[i] = entity.Stream
    }
    return streams, nil
}

// AddIntent adds an intent to a stream
func (s *Store) AddIntent(streamID string, intentID string) error {
    // Verify intent exists
    if _, err := s.intentBox.Get(intentID); err != nil {
        return fmt.Errorf("intent not found: %w", err)
    }

    st, err := s.Get(streamID)
    if err != nil {
        return err
    }

    // Check if intent is already in stream
    for _, id := range st.State.Intents {
        if id == intentID {
            return nil // Already exists
        }
    }

    st.State.Intents = append(st.State.Intents, intentID)
    return s.Update(st)
}

// RemoveIntent removes an intent from a stream
func (s *Store) RemoveIntent(streamID string, intentID string) error {
    st, err := s.Get(streamID)
    if err != nil {
        return err
    }

    found := false
    newIntents := make([]string, 0, len(st.State.Intents))
    for _, id := range st.State.Intents {
        if id != intentID {
            newIntents = append(newIntents, id)
        } else {
            found = true
        }
    }

    if !found {
        return fmt.Errorf("intent not found in stream: %s", intentID)
    }

    st.State.Intents = newIntents
    return s.Update(st)
}

// GetIntents returns all intents in a stream
func (s *Store) GetIntents(streamID string) ([]*intent.Intent, error) {
    st, err := s.Get(streamID)
    if err != nil {
        return nil, err
    }

    intents := make([]*intent.Intent, 0, len(st.State.Intents))
    for _, intentID := range st.State.Intents {
        intent, err := s.intentBox.Get(intentID)
        if err != nil {
            return nil, fmt.Errorf("fetching intent %s: %w", intentID, err)
        }
        intents = append(intents, intent)
    }
    return intents, nil
}

// SetFeatureFlag updates or adds a feature flag to a stream
func (s *Store) SetFeatureFlag(streamID string, flag stream.FeatureFlag) error {
    st, err := s.Get(streamID)
    if err != nil {
        return err
    }

    // Update existing flag or add new one
    flagFound := false
    for i, f := range st.Config.FeatureFlags {
        if f.Name == flag.Name {
            st.Config.FeatureFlags[i] = flag
            flagFound = true
            break
        }
    }

    if !flagFound {
        st.Config.FeatureFlags = append(st.Config.FeatureFlags, flag)
    }

    return s.Update(st)
}

// GetFeatureFlag retrieves a specific feature flag from a stream
func (s *Store) GetFeatureFlag(streamID string, flagName string) (*stream.FeatureFlag, error) {
    st, err := s.Get(streamID)
    if err != nil {
        return nil, err
    }

    for _, flag := range st.Config.FeatureFlags {
        if flag.Name == flagName {
            return &flag, nil
        }
    }

    return nil, fmt.Errorf("feature flag not found: %s", flagName)
}

// FindByType returns streams of a specific type
func (s *Store) FindByType(streamType string) ([]*stream.Stream, error) {
    if streamType == "" {
        return nil, fmt.Errorf("stream type is required")
    }

    streams, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*stream.Stream
    for _, st := range streams {
        if st.Type == streamType {
            result = append(result, st)
        }
    }
    return result, nil
}

// FindActive returns all active streams
func (s *Store) FindActive() ([]*stream.Stream, error) {
    streams, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*stream.Stream
    for _, st := range streams {
        if st.State.Active {
            result = append(result, st)
        }
    }
    return result, nil
}