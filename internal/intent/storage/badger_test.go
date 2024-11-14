package storage

import (
	"os"
	"testing"
	"time"

	"tig/internal/api"
	"tig/internal/intent"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*badger.DB, *api.MockIntentBox, func()) {
    dir, err := os.MkdirTemp("", "badger-test")
    require.NoError(t, err)

    opts := badger.DefaultOptions(dir).WithInMemory(true)
    opts.Logger = nil // Disable logging for tests
    opts.Dir = ""
    opts.ValueDir = ""
    
    db, err := badger.Open(opts)
    require.NoError(t, err)

    mockIntentBox := api.NewMockIntentBox()

    cleanup := func() {
        db.Close()
        os.RemoveAll(dir)
    }

    return db, mockIntentBox, cleanup
}

func TestIntentStore(t *testing.T) {
    db, _, cleanup := setupTestDB(t)
    defer cleanup()

    store := NewStore(db, nil)

    t.Run("Create", func(t *testing.T) {
        i := &intent.Intent{
            ID:          uuid.New().String(),
            Type:        "feature",
            Description: "Test feature",
            Impact: intent.Impact{
                Scope:        []string{"service-a"},
                Breaking:     false,
                Dependencies: []string{},
            },
            Metadata: intent.Metadata{
                Author: "test@example.com",
                Refs:   []string{"TICKET-123"},
            },
        }

        err := store.Create(i)
        require.NoError(t, err)
        assert.False(t, i.CreatedAt.IsZero())
        assert.False(t, i.UpdatedAt.IsZero())

        // Try to create duplicate
        err = store.Create(i)
        assert.Error(t, err)
    })

    t.Run("Get", func(t *testing.T) {
        i := &intent.Intent{
            ID:          uuid.New().String(),
            Type:        "feature",
            Description: "Test feature",
        }

        err := store.Create(i)
        require.NoError(t, err)

        retrieved, err := store.Get(i.ID)
        require.NoError(t, err)
        assert.Equal(t, i.ID, retrieved.ID)
        assert.Equal(t, i.Type, retrieved.Type)
        assert.Equal(t, i.Description, retrieved.Description)

        // Try to get non-existent
        _, err = store.Get("does-not-exist")
        assert.Error(t, err)
    })

    t.Run("Update", func(t *testing.T) {
        i := &intent.Intent{
            ID:          uuid.New().String(),
            Type:        "feature",
            Description: "Original description",
        }

        err := store.Create(i)
        require.NoError(t, err)

        originalUpdated := i.UpdatedAt

        time.Sleep(time.Millisecond) // Ensure time difference
        i.Description = "Updated description"
        err = store.Update(i)
        require.NoError(t, err)
        assert.True(t, i.UpdatedAt.After(originalUpdated))

        retrieved, err := store.Get(i.ID)
        require.NoError(t, err)
        assert.Equal(t, "Updated description", retrieved.Description)
    })

    t.Run("Delete", func(t *testing.T) {
        i := &intent.Intent{
            ID:          uuid.New().String(),
            Type:        "feature",
            Description: "To be deleted",
        }

        err := store.Create(i)
        require.NoError(t, err)

        err = store.Delete(i.ID)
        require.NoError(t, err)

        _, err = store.Get(i.ID)
        assert.Error(t, err)
    })

    t.Run("List", func(t *testing.T) {
        // Create test intents
        intents := []*intent.Intent{
            {
                ID:          uuid.New().String(),
                Type:        "feature",
                Description: "First feature",
            },
            {
                ID:          uuid.New().String(),
                Type:        "fix",
                Description: "Second fix",
            },
        }

        for _, i := range intents {
            err := store.Create(i)
            require.NoError(t, err)
        }

        list, err := store.List()
        require.NoError(t, err)
        assert.GreaterOrEqual(t, len(list), len(intents))

        // Verify contents
        found := make(map[string]bool)
        for _, i := range intents {
            found[i.ID] = false
        }

        for _, i := range list {
            if _, ok := found[i.ID]; ok {
                found[i.ID] = true
            }
        }

        for id, wasFound := range found {
            assert.True(t, wasFound, "Intent %s was not found in list", id)
        }
    })
}