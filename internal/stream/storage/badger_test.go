// internal/stream/storage/badger_test.go
package storage

import (
	"fmt"
	"os"
	"testing"
	"time"

	"tig/internal/intent"
	"tig/internal/stream"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*badger.DB, *MockIntentBox, func()) {
    dir, err := os.MkdirTemp("", "badger-test")
    require.NoError(t, err)

    opts := badger.DefaultOptions(dir).WithInMemory(true)
    opts.Logger = nil // Disable logging for tests
    opts.Dir = ""
    opts.ValueDir = ""
    
    db, err := badger.Open(opts)
    require.NoError(t, err)

    mockIntentBox := NewMockIntentBox()

    cleanup := func() {
        db.Close()
        os.RemoveAll(dir)
    }

    return db, mockIntentBox, cleanup
}

// MockIntentBox implements intent.Box interface for testing
type MockIntentBox struct {
    intents map[string]*intent.Intent
}

func NewMockIntentBox() *MockIntentBox {
    return &MockIntentBox{
        intents: make(map[string]*intent.Intent),
    }
}

func (m *MockIntentBox) Create(i *intent.Intent) error {
    m.intents[i.ID] = i
    return nil
}

func (m *MockIntentBox) Get(id string) (*intent.Intent, error) {
    if i, ok := m.intents[id]; ok {
        return i, nil
    }
    return nil, fmt.Errorf("intent not found: %s", id)
}

func (m *MockIntentBox) Update(i *intent.Intent) error {
    if _, ok := m.intents[i.ID]; !ok {
        return fmt.Errorf("intent not found: %s", i.ID)
    }
    m.intents[i.ID] = i
    return nil
}

func (m *MockIntentBox) Delete(id string) error {
    if _, ok := m.intents[id]; !ok {
        return fmt.Errorf("intent not found: %s", id)
    }
    delete(m.intents, id)
    return nil
}

func (m *MockIntentBox) List() ([]*intent.Intent, error) {
    var list []*intent.Intent
    for _, i := range m.intents {
        list = append(list, i)
    }
    return list, nil
}

func (m *MockIntentBox) FindByType(intentType string) ([]*intent.Intent, error) {
    var result []*intent.Intent
    for _, i := range m.intents {
        if i.Type == intentType {
            result = append(result, i)
        }
    }
    return result, nil
}

func (m *MockIntentBox) FindByAuthor(author string) ([]*intent.Intent, error) {
    var result []*intent.Intent
    for _, i := range m.intents {
        if i.Metadata.Author == author {
            result = append(result, i)
        }
    }
    return result, nil
}

func (m *MockIntentBox) FindByTimeRange(start, end time.Time) ([]*intent.Intent, error) {
    var result []*intent.Intent
    for _, i := range m.intents {
        if i.CreatedAt.After(start) && i.CreatedAt.Before(end) {
            result = append(result, i)
        }
    }
    return result, nil
}

func (m *MockIntentBox) FindWithBreakingChanges() ([]*intent.Intent, error) {
    var result []*intent.Intent
    for _, i := range m.intents {
        if i.Impact.Breaking {
            result = append(result, i)
        }
    }
    return result, nil
}

func TestStreamStore_Create(t *testing.T) {
    db, mockIntentBox, cleanup := setupTestDB(t)
    defer cleanup()

    store := NewStore(db, mockIntentBox)

    testStream := &stream.Stream{
        ID:   uuid.New().String(),
        Name: "feature/test-stream",
        Type: "feature",
        Config: stream.Config{
            AutoMerge: true,
            FeatureFlags: []stream.FeatureFlag{
                {
                    Name:       "test-flag",
                    Conditions: []string{"env=test"},
                    Enabled:    true,
                },
            },
            Protection: stream.Protection{
                RequiredReviewers: 1,
                RequiredChecks:    []string{"unit-tests"},
            },
        },
        State: stream.State{
            Active:   true,
            Status:   "stable",
            LastSync: time.Now(),
            Intents:  []string{},
        },
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    // Test Create
    err := store.Create(testStream)
    assert.NoError(t, err)

    // Verify creation
    stored, err := store.Get(testStream.ID)
    assert.NoError(t, err)
    assert.Equal(t, testStream.ID, stored.ID)
    assert.Equal(t, testStream.Name, stored.Name)
    assert.Equal(t, testStream.Type, stored.Type)
    assert.Equal(t, testStream.Config.AutoMerge, stored.Config.AutoMerge)
    assert.Len(t, stored.Config.FeatureFlags, 1)
}

func TestStreamStore_AddIntent(t *testing.T) {
    db, mockIntentBox, cleanup := setupTestDB(t)
    defer cleanup()

    store := NewStore(db, mockIntentBox)

    // Create test stream
    testStream := &stream.Stream{
        ID:   uuid.New().String(),
        Name: "feature/test-stream",
        Type: "feature",
        State: stream.State{
            Active:   true,
            Status:   "stable",
            LastSync: time.Now(),
            Intents:  []string{},
        },
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err := store.Create(testStream)
    require.NoError(t, err)

    // Create test intent in mock
    testIntent := &intent.Intent{
        ID:          uuid.New().String(),
        Type:        "feature",
        Description: "Test feature",
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    err = mockIntentBox.Create(testIntent)
    require.NoError(t, err)

    // Test AddIntent
    err = store.AddIntent(testStream.ID, testIntent.ID)
    assert.NoError(t, err)

    // Verify intent was added
    stored, err := store.Get(testStream.ID)
    assert.NoError(t, err)
    assert.Contains(t, stored.State.Intents, testIntent.ID)

    // Test adding non-existent intent
    err = store.AddIntent(testStream.ID, "non-existent-intent")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "intent not found")
}

func TestStreamStore_RemoveIntent(t *testing.T) {
    db, mockIntentBox, cleanup := setupTestDB(t)
    defer cleanup()

    store := NewStore(db, mockIntentBox)

    // Create test intent
    testIntent := &intent.Intent{
        ID:          uuid.New().String(),
        Type:        "feature",
        Description: "Test feature",
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    err := mockIntentBox.Create(testIntent)
    require.NoError(t, err)

    // Create test stream with intent
    testStream := &stream.Stream{
        ID:   uuid.New().String(),
        Name: "feature/test-stream",
        Type: "feature",
        State: stream.State{
            Active:   true,
            Status:   "stable",
            LastSync: time.Now(),
            Intents:  []string{testIntent.ID},
        },
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err = store.Create(testStream)
    require.NoError(t, err)

    // Test RemoveIntent
    err = store.RemoveIntent(testStream.ID, testIntent.ID)
    assert.NoError(t, err)

    // Verify intent was removed
    stored, err := store.Get(testStream.ID)
    assert.NoError(t, err)
    assert.NotContains(t, stored.State.Intents, testIntent.ID)
}

func TestStreamStore_GetIntents(t *testing.T) {
    db, mockIntentBox, cleanup := setupTestDB(t)
    defer cleanup()

    store := NewStore(db, mockIntentBox)

    // Create test intents
    testIntents := []*intent.Intent{
        {
            ID:          uuid.New().String(),
            Type:        "feature",
            Description: "First feature",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        },
        {
            ID:          uuid.New().String(),
            Type:        "fix",
            Description: "Second feature",
            CreatedAt:   time.Now(),
            UpdatedAt:   time.Now(),
        },
    }

    for _, i := range testIntents {
        err := mockIntentBox.Create(i)
        require.NoError(t, err)
    }

    // Create test stream with intents
    testStream := &stream.Stream{
        ID:   uuid.New().String(),
        Name: "feature/test-stream",
        Type: "feature",
        State: stream.State{
            Active:   true,
            Status:   "stable",
            LastSync: time.Now(),
            Intents:  []string{testIntents[0].ID, testIntents[1].ID},
        },
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err := store.Create(testStream)
    require.NoError(t, err)

    // Test GetIntents
    intents, err := store.GetIntents(testStream.ID)
    assert.NoError(t, err)
    assert.Len(t, intents, 2)

    // Verify intent contents
    intentMap := make(map[string]*intent.Intent)
    for _, i := range intents {
        intentMap[i.ID] = i
    }

    for _, expected := range testIntents {
        actual, ok := intentMap[expected.ID]
        assert.True(t, ok)
        assert.Equal(t, expected.Type, actual.Type)
        assert.Equal(t, expected.Description, actual.Description)
    }
}