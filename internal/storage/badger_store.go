// internal/storage/badger_store.go
package storage

import (
    "encoding/json"
    "fmt"
    "strings"

    "github.com/dgraph-io/badger/v4"
)

// Entity represents any storable entity with an ID
type Entity interface {
    GetID() string
}

// BadgerStore provides generic storage operations
type BadgerStore struct {
    db        *badger.DB
    prefix    string
}

func NewBadgerStore(db *badger.DB, prefix string) *BadgerStore {
    return &BadgerStore{
        db:     db,
        prefix: prefix,
    }
}

func (s *BadgerStore) makeKey(id string) []byte {
    return []byte(fmt.Sprintf("%s:%s", s.prefix, id))
}

func (s *BadgerStore) stripPrefix(key []byte) string {
    return strings.TrimPrefix(string(key), fmt.Sprintf("%s:", s.prefix))
}

func (s *BadgerStore) Create(entity Entity) error {
    if entity.GetID() == "" {
        return fmt.Errorf("entity ID cannot be empty")
    }

    data, err := json.Marshal(entity)
    if err != nil {
        return fmt.Errorf("marshaling entity: %w", err)
    }

    key := s.makeKey(entity.GetID())
    return s.db.Update(func(txn *badger.Txn) error {
        // Check if key already exists
        _, err := txn.Get(key)
        if err == nil {
            return fmt.Errorf("entity already exists: %s", entity.GetID())
        } else if err != badger.ErrKeyNotFound {
            return err
        }

        return txn.Set(key, data)
    })
}

func (s *BadgerStore) Get(id string, entity Entity) error {
    key := s.makeKey(id)

    err := s.db.View(func(txn *badger.Txn) error {
        item, err := txn.Get(key)
        if err != nil {
            return err
        }

        return item.Value(func(val []byte) error {
            return json.Unmarshal(val, entity)
        })
    })

    if err == badger.ErrKeyNotFound {
        return fmt.Errorf("entity not found: %s", id)
    }
    return err
}

func (s *BadgerStore) Update(entity Entity) error {
    if entity.GetID() == "" {
        return fmt.Errorf("entity ID cannot be empty")
    }

    data, err := json.Marshal(entity)
    if err != nil {
        return fmt.Errorf("marshaling entity: %w", err)
    }

    key := s.makeKey(entity.GetID())
    return s.db.Update(func(txn *badger.Txn) error {
        // Check if exists
        _, err := txn.Get(key)
        if err == badger.ErrKeyNotFound {
            return fmt.Errorf("entity not found: %s", entity.GetID())
        } else if err != nil {
            return err
        }

        return txn.Set(key, data)
    })
}

func (s *BadgerStore) Delete(id string) error {
    key := s.makeKey(id)

    return s.db.Update(func(txn *badger.Txn) error {
        // Check if exists
        _, err := txn.Get(key)
        if err == badger.ErrKeyNotFound {
            return fmt.Errorf("entity not found: %s", id)
        } else if err != nil {
            return err
        }

        return txn.Delete(key)
    })
}

func (s *BadgerStore) List(results interface{}) error {
    err := s.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        it := txn.NewIterator(opts)
        defer it.Close()

        prefix := []byte(s.prefix + ":")
        var values []json.RawMessage

        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            item := it.Item()
            err := item.Value(func(val []byte) error {
                values = append(values, val)
                return nil
            })
            if err != nil {
                return err
            }
        }

        // Marshal collected values into final result
        data, err := json.Marshal(values)
        if err != nil {
            return err
        }

        return json.Unmarshal(data, results)
    })

    if err != nil {
        return fmt.Errorf("listing entities: %w", err)
    }
    return nil
}