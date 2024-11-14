// internal/api/stream_handlers_test.go
package api

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "tig/internal/intent"
    "tig/internal/stream"
)

// Mock stream store
type MockStreamBox struct {
    streams map[string]*stream.Stream
    intents map[string]*intent.Intent // For testing intent-related operations
}

func NewMockStreamBox() *MockStreamBox {
    return &MockStreamBox{
        streams: make(map[string]*stream.Stream),
        intents: make(map[string]*intent.Intent),
    }
}

func (m *MockStreamBox) Create(s *stream.Stream) error {
    m.streams[s.ID] = s
    return nil
}

func (m *MockStreamBox) Get(id string) (*stream.Stream, error) {
    if s, ok := m.streams[id]; ok {
        return s, nil
    }
    return nil, fmt.Errorf("stream not found: %s", id)
}

func (m *MockStreamBox) Update(s *stream.Stream) error {
    if _, ok := m.streams[s.ID]; !ok {
        return fmt.Errorf("stream not found: %s", s.ID)
    }
    m.streams[s.ID] = s
    return nil
}

func (m *MockStreamBox) Delete(id string) error {
    if _, ok := m.streams[id]; !ok {
        return fmt.Errorf("stream not found: %s", id)
    }
    delete(m.streams, id)
    return nil
}

func (m *MockStreamBox) List() ([]*stream.Stream, error) {
    var list []*stream.Stream
    for _, s := range m.streams {
        list = append(list, s)
    }
    return list, nil
}

func (m *MockStreamBox) AddIntent(streamID string, intentID string) error {
    s, ok := m.streams[streamID]
    if !ok {
        return fmt.Errorf("stream not found: %s", streamID)
    }

    if _, ok := m.intents[intentID]; !ok {
        return fmt.Errorf("intent not found: %s", intentID)
    }

    s.State.Intents = append(s.State.Intents, intentID)
    return nil
}

func (m *MockStreamBox) RemoveIntent(streamID string, intentID string) error {
    s, ok := m.streams[streamID]
    if !ok {
        return fmt.Errorf("stream not found: %s", streamID)
    }

    found := false
    newIntents := make([]string, 0, len(s.State.Intents))
    for _, id := range s.State.Intents {
        if id != intentID {
            newIntents = append(newIntents, id)
        } else {
            found = true
        }
    }

    if !found {
        return fmt.Errorf("intent not found in stream: %s", intentID)
    }

    s.State.Intents = newIntents
    return nil
}

func (m *MockStreamBox) GetIntents(streamID string) ([]*intent.Intent, error) {
    s, ok := m.streams[streamID]
    if !ok {
        return nil, fmt.Errorf("stream not found: %s", streamID)
    }

    var intents []*intent.Intent
    for _, intentID := range s.State.Intents {
        if intent, ok := m.intents[intentID]; ok {
            intents = append(intents, intent)
        }
    }
    return intents, nil
}

func (m *MockStreamBox) SetFeatureFlag(streamID string, flag stream.FeatureFlag) error {
    s, ok := m.streams[streamID]
    if !ok {
        return fmt.Errorf("stream not found: %s", streamID)
    }

    flagFound := false
    for i, f := range s.Config.FeatureFlags {
        if f.Name == flag.Name {
            s.Config.FeatureFlags[i] = flag
            flagFound = true
            break
        }
    }

    if !flagFound {
        s.Config.FeatureFlags = append(s.Config.FeatureFlags, flag)
    }

    return nil
}

func (m *MockStreamBox) GetFeatureFlag(streamID string, flagName string) (*stream.FeatureFlag, error) {
    s, ok := m.streams[streamID]
    if !ok {
        return nil, fmt.Errorf("stream not found: %s", streamID)
    }

    for _, flag := range s.Config.FeatureFlags {
        if flag.Name == flagName {
            return &flag, nil
        }
    }

    return nil, fmt.Errorf("feature flag not found: %s", flagName)
}

func (m *MockStreamBox) FindByType(streamType string) ([]*stream.Stream, error) {   
    var result []*stream.Stream
    for _, s := range m.streams {
        if s.Type == streamType {
            result = append(result, s)
        }
    }
    return result, nil
}

func (m *MockStreamBox) FindActive() ([]*stream.Stream, error) {
    var result []*stream.Stream
    for _, s := range m.streams {
        if s.State.Active {
            result = append(result, s)
        }
    }
    return result, nil
}

// Stream handler tests
func TestStreamHandler_Create(t *testing.T) {
    box := NewMockStreamBox()
    handler := NewStreamHandler(box)

    tests := []struct {
        name       string
        input      map[string]interface{}
        wantStatus int
        wantErr    bool
    }{
        {
            name: "valid stream",
            input: map[string]interface{}{
                "name": "feature/test-stream",
                "type": "feature",
                "config": map[string]interface{}{
                    "auto_merge": true,
                    "feature_flags": []interface{}{},
                    "protection": map[string]interface{}{
                        "required_reviewers": 1,
                        "required_checks":    []string{},
                    },
                },
            },
            wantStatus: http.StatusCreated,
            wantErr:    false,
        },
        {
            name: "missing name",
            input: map[string]interface{}{
                "type": "feature",
            },
            wantStatus: http.StatusBadRequest,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            body, err := json.Marshal(tt.input)
            require.NoError(t, err)

            req := httptest.NewRequest("POST", "/api/streams", bytes.NewBuffer(body))
            rec := httptest.NewRecorder()

            handler.Create(rec, req)

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                var created stream.Stream
                err = json.NewDecoder(rec.Body).Decode(&created)
                require.NoError(t, err)
                assert.NotEmpty(t, created.ID)
                assert.Equal(t, tt.input["name"], created.Name)
                assert.Equal(t, tt.input["type"], created.Type)
            }
        })
    }
}

func TestStreamHandler_AddIntent(t *testing.T) {
    box := NewMockStreamBox()
    handler := NewStreamHandler(box)

    // Create test stream and intent
    testStream := &stream.Stream{
        ID:   "test-stream-1",
        Name: "feature/test",
        Type: "feature",
        State: stream.State{
            Active:   true,
            Status:   "stable",
            LastSync: time.Now(),
            Intents:  []string{},
        },
    }
    err := box.Create(testStream)
    require.NoError(t, err)

    testIntent := &intent.Intent{
        ID:          "test-intent-1",
        Type:        "feature",
        Description: "Test feature",
    }
    box.intents[testIntent.ID] = testIntent

    tests := []struct {
        name       string
        streamID   string
        intentID   string
        wantStatus int
        wantErr    bool
    }{
        {
            name:       "valid addition",
            streamID:   "test-stream-1",
            intentID:   "test-intent-1",
            wantStatus: http.StatusOK,
            wantErr:    false,
        },
        {
            name:       "non-existent stream",
            streamID:   "does-not-exist",
            intentID:   "test-intent-1",
            wantStatus: http.StatusInternalServerError,
            wantErr:    true,
        },
        {
            name:       "non-existent intent",
            streamID:   "test-stream-1",
            intentID:   "does-not-exist",
            wantStatus: http.StatusInternalServerError,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            body, err := json.Marshal(map[string]string{
                "intent_id": tt.intentID,
            })
            require.NoError(t, err)

            req := httptest.NewRequest("POST", "/api/streams/"+tt.streamID+"/intents", bytes.NewBuffer(body))
            rec := httptest.NewRecorder()

            handler.AddIntent(rec, req.WithContext(
                WithURLParams(req.Context(), map[string]string{"id": tt.streamID}),
            ))

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                s, err := box.Get(tt.streamID)
                require.NoError(t, err)
                assert.Contains(t, s.State.Intents, tt.intentID)
            }
        })
    }
}

func TestStreamHandler_SetFeatureFlag(t *testing.T) {
    box := NewMockStreamBox()
    handler := NewStreamHandler(box)

    // Create test stream
    testStream := &stream.Stream{
        ID:   "test-stream-1",
        Name: "feature/test",
        Type: "feature",
        Config: stream.Config{
            FeatureFlags: []stream.FeatureFlag{},
        },
    }
    err := box.Create(testStream)
    require.NoError(t, err)

    tests := []struct {
        name       string
        streamID   string
        flag       stream.FeatureFlag
        wantStatus int
        wantErr    bool
    }{
        {
            name:     "valid flag",
            streamID: "test-stream-1",
            flag: stream.FeatureFlag{
                Name:       "test-flag",
                Conditions: []string{"env=test"},
                Enabled:    true,
            },
            wantStatus: http.StatusOK,
            wantErr:    false,
        },
        {
            name:     "non-existent stream",
            streamID: "does-not-exist",
            flag: stream.FeatureFlag{
                Name:       "test-flag",
                Conditions: []string{"env=test"},
                Enabled:    true,
            },
            wantStatus: http.StatusInternalServerError,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            body, err := json.Marshal(tt.flag)
            require.NoError(t, err)

            req := httptest.NewRequest("POST", "/api/streams/"+tt.streamID+"/feature-flags", bytes.NewBuffer(body))
            rec := httptest.NewRecorder()

            handler.SetFeatureFlag(rec, req.WithContext(
                WithURLParams(req.Context(), map[string]string{"id": tt.streamID}),
            ))

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                flag, err := box.GetFeatureFlag(tt.streamID, tt.flag.Name)
                require.NoError(t, err)
                assert.Equal(t, tt.flag.Name, flag.Name)
                assert.Equal(t, tt.flag.Enabled, flag.Enabled)
                assert.Equal(t, tt.flag.Conditions, flag.Conditions)
            }
        })
    }
}

func TestStreamHandler_GetFeatureFlags(t *testing.T) {
    box := NewMockStreamBox()
    handler := NewStreamHandler(box)

    // Create test stream with feature flags
    testStream := &stream.Stream{
        ID:   "test-stream-1",
        Name: "feature/test",
        Type: "feature",
        Config: stream.Config{
            FeatureFlags: []stream.FeatureFlag{
                {
                    Name:       "test-flag-1",
                    Conditions: []string{"env=test"},
                    Enabled:    true,
                },
                {
                    Name:       "test-flag-2",
                    Conditions: []string{"env=prod"},
                    Enabled:    false,
                },
            },
        },
    }
    err := box.Create(testStream)
    require.NoError(t, err)

    tests := []struct {
        name       string
        streamID   string
        wantStatus int
        wantFlags  int
        wantErr    bool
    }{
        {
            name:       "existing stream",
            streamID:   "test-stream-1",
            wantStatus: http.StatusOK,
            wantFlags:  2,
            wantErr:    false,
        },
        {
            name:       "non-existent stream",
            streamID:   "does-not-exist",
            wantStatus: http.StatusNotFound,
            wantFlags:  0,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest("GET", "/api/streams/"+tt.streamID+"/feature-flags", nil)
            rec := httptest.NewRecorder()

            handler.GetFeatureFlags(rec, req.WithContext(
                WithURLParams(req.Context(), map[string]string{"id": tt.streamID}),
            ))

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                var flags []stream.FeatureFlag
                err = json.NewDecoder(rec.Body).Decode(&flags)
                require.NoError(t, err)
                assert.Len(t, flags, tt.wantFlags)
            }
        })
    }
}