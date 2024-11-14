// internal/intent/storage/store.go
package storage

import (
    "fmt"
    "time"

    "github.com/dgraph-io/badger/v4"
    "tig/internal/intent"
    "tig/internal/storage"
    "tig/shared/types"
)

type Store struct {
    store     *storage.BadgerStore
    workspace shared.Workspace
}

func NewStore(db *badger.DB, ws shared.Workspace) *Store {
    return &Store{
        store:     storage.NewBadgerStore(db, "intent"),
        workspace: ws,
    }
}

// intentEntity wraps intent.Intent to implement storage.Entity
type intentEntity struct {
    *intent.Intent
}

func (i *intentEntity) GetID() string {
    return i.ID
}

func validate(i *intent.Intent) error {
    if i.Description == "" {
        return fmt.Errorf("description is required")
    }
    if i.Type == "" {
        return fmt.Errorf("type is required")
    }
    return nil
}

func (s *Store) Create(i *intent.Intent) error {
    if err := validate(i); err != nil {
        return fmt.Errorf("invalid intent: %w", err)
    }

    // Set timestamps
    if i.CreatedAt.IsZero() {
        i.CreatedAt = time.Now()
    }
    if i.UpdatedAt.IsZero() {
        i.UpdatedAt = i.CreatedAt
    }

    // Create an intentEntity wrapper
    entity := &intentEntity{Intent: i}
    return s.store.Create(entity)
}

func (s *Store) Get(id string) (*intent.Intent, error) {
    var entity intentEntity
    entity.Intent = &intent.Intent{}
    
    if err := s.store.Get(id, &entity); err != nil {
        return nil, fmt.Errorf("getting intent: %w", err)
    }

    return entity.Intent, nil
}

func (s *Store) Update(i *intent.Intent) error {
    if err := validate(i); err != nil {
        return fmt.Errorf("invalid intent: %w", err)
    }

    i.UpdatedAt = time.Now()
    return s.store.Update(&intentEntity{Intent: i})
}

func (s *Store) Delete(id string) error {
    return s.store.Delete(id)
}

func (s *Store) List() ([]*intent.Intent, error) {
    var entities []intentEntity
    if err := s.store.List(&entities); err != nil {
        return nil, fmt.Errorf("listing intents: %w", err)
    }

    intents := make([]*intent.Intent, len(entities))
    for i, entity := range entities {
        intents[i] = entity.Intent
    }
    return intents, nil
}

func (s *Store) FindByType(intentType string) ([]*intent.Intent, error) {
    if intentType == "" {
        return nil, fmt.Errorf("intent type is required")
    }

    intents, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*intent.Intent
    for _, i := range intents {
        if i.Type == intentType {
            result = append(result, i)
        }
    }
    return result, nil
}

func (s *Store) FindByAuthor(author string) ([]*intent.Intent, error) {
    if author == "" {
        return nil, fmt.Errorf("author is required")
    }

    intents, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*intent.Intent
    for _, i := range intents {
        if i.Metadata.Author == author {
            result = append(result, i)
        }
    }
    return result, nil
}

func (s *Store) FindByTimeRange(start, end time.Time) ([]*intent.Intent, error) {
    if start.IsZero() || end.IsZero() {
        return nil, fmt.Errorf("start and end times are required")
    }
    if end.Before(start) {
        return nil, fmt.Errorf("end time cannot be before start time")
    }

    intents, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*intent.Intent
    for _, i := range intents {
        if !i.CreatedAt.Before(start) && !i.CreatedAt.After(end) {
            result = append(result, i)
        }
    }
    return result, nil
}

func (s *Store) FindWithBreakingChanges() ([]*intent.Intent, error) {
    intents, err := s.List()
    if err != nil {
        return nil, err
    }

    var result []*intent.Intent
    for _, i := range intents {
        if i.Impact.Breaking {
            result = append(result, i)
        }
    }
    return result, nil
}