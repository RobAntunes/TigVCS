// internal/content/change.go
package content

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ChangeSet represents a group of related changes
type ChangeSet struct {
    Changes  []Change            `json:"changes"`
    Metadata map[string]string   `json:"metadata"`
}

// ChangeStore manages change tracking
type ChangeStore interface {
    // Record a new change
    RecordChange(change *Change) error
    
    // Get change history for a file
    GetHistory(path string) ([]Change, error)
    
    // Compare two versions
    Compare(oldHash, newHash string) ([]Change, error)
    
    // Get change details
    GetChange(hash string) (*Change, error)
    
    // Create a changeset from staged changes
    CreateChangeSet(changes []Change) (*ChangeSet, error)
}

// changeStore implements ChangeStore using BadgerDB
type changeStore struct {
    db *badger.DB
    contentStore Store
}

// func NewChangeStore(db *badger.DB, contentStore ContentStore) ChangeStore {
//     return &changeStore{
//         db: db,
//         contentStore: contentStore,
//     }
// }

func (s *changeStore) RecordChange(change *Change) error {
    // Store content first
    hash, err := s.contentStore.Store(change.Content)
    if err != nil {
        return fmt.Errorf("storing content: %w", err)
    }
    
    change.Hash = hash
    
    // Store change record
    data, err := json.Marshal(change)
    if err != nil {
        return fmt.Errorf("marshaling change: %w", err)
    }
    
    key := []byte(fmt.Sprintf("change:%s:%s", change.Path, change.Hash))
    return s.db.Update(func(txn *badger.Txn) error {
        return txn.Set(key, data)
    })
}

func (s *changeStore) GetHistory(path string) ([]Change, error) {
    var changes []Change
    
    prefix := []byte(fmt.Sprintf("change:%s:", path))
    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.Prefix = prefix
        
        it := txn.NewIterator(opts)
        defer it.Close()
        
        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            item := it.Item()
            err := item.Value(func(val []byte) error {
                var change Change
                if err := json.Unmarshal(val, &change); err != nil {
                    return err
                }
                changes = append(changes, change)
                return nil
            })
            if err != nil {
                return err
            }
        }
        return nil
    })
    
    return changes, err
}

// func (s *changeStore) Compare(oldHash, newHash string) ([]Change, error) {
//     oldContent, err := s.contentStore.Get(oldHash)
//     if err != nil {
//         return nil, fmt.Errorf("getting old content: %w", err)
//     }
    
//     newContent, err := s.contentStore.Get(newHash)
//     if err != nil {
//         return nil, fmt.Errorf("getting new content: %w", err)
//     }
    
//     // Implement diff logic here
//     // This is where you'd compute the actual changes between versions
    
//     return computeDiff(oldContent, newContent), nil
// }

func (s *changeStore) CreateChangeSet(changes []Change) (*ChangeSet, error) {
    // Record each change
    for i := range changes {
        if err := s.RecordChange(&changes[i]); err != nil {
            return nil, fmt.Errorf("recording change: %w", err)
        }
    }
    
    return &ChangeSet{
        Changes: changes,
        Metadata: map[string]string{
            "timestamp": time.Now().UTC().Format(time.RFC3339),
        },
    }, nil
}