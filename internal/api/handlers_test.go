package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tig/internal/intent"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)



// Intent handler tests
func TestIntentHandler_Create(t *testing.T) {
    box := NewMockIntentBox()
    handler := NewIntentHandler(box)

    tests := []struct {
        name       string
        input      map[string]interface{}
        wantStatus int
        wantErr    bool
    }{
        {
            name: "valid intent",
            input: map[string]interface{}{
                "type":        "feature",
                "description": "Test feature intent",
                "impact": map[string]interface{}{
                    "scope":        []string{"service-a"},
                    "breaking":     false,
                    "dependencies": []string{},
                },
            },
            wantStatus: http.StatusCreated,
            wantErr:    false,
        },
        {
            name: "missing description",
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

            req := httptest.NewRequest("POST", "/api/intents", bytes.NewBuffer(body))
            rec := httptest.NewRecorder()

            handler.Create(rec, req)

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                var created intent.Intent
                err = json.NewDecoder(rec.Body).Decode(&created)
                require.NoError(t, err)
                assert.NotEmpty(t, created.ID)
                assert.Equal(t, tt.input["type"], created.Type)
                assert.Equal(t, tt.input["description"], created.Description)
            }
        })
    }
}

func TestIntentHandler_Get(t *testing.T) {
    box := NewMockIntentBox()
    handler := NewIntentHandler(box)

    // Create test intent
    testIntent := &intent.Intent{
        ID:          "test-intent-1",
        Type:        "feature",
        Description: "Test feature",
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    err := box.Create(testIntent)
    require.NoError(t, err)

    tests := []struct {
        name       string
        intentID   string
        wantStatus int
        wantErr    bool
    }{
        {
            name:       "existing intent",
            intentID:   "test-intent-1",
            wantStatus: http.StatusOK,
            wantErr:    false,
        },
        {
            name:       "non-existent intent",
            intentID:   "does-not-exist",
            wantStatus: http.StatusNotFound,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest("GET", fmt.Sprintf("/api/intents/%s", tt.intentID), nil)
            rec := httptest.NewRecorder()

            handler.Get(rec, req.WithContext(
                WithURLParams(req.Context(), map[string]string{"id": tt.intentID}),
            ))

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                var got intent.Intent
                err = json.NewDecoder(rec.Body).Decode(&got)
                require.NoError(t, err)
                assert.Equal(t, testIntent.ID, got.ID)
                assert.Equal(t, testIntent.Type, got.Type)
                assert.Equal(t, testIntent.Description, got.Description)
            }
        })
    }
}

func TestIntentHandler_Update(t *testing.T) {
    box := NewMockIntentBox()
    handler := NewIntentHandler(box)

    // Create test intent
    testIntent := &intent.Intent{
        ID:          "test-intent-1",
        Type:        "feature",
        Description: "Original description",
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    err := box.Create(testIntent)
    require.NoError(t, err)

    tests := []struct {
        name       string
        intentID   string
        input      map[string]interface{}
        wantStatus int
        wantErr    bool
    }{
        {
            name:     "valid update",
            intentID: "test-intent-1",
            input: map[string]interface{}{
                "type":        "feature",
                "description": "Updated description",
            },
            wantStatus: http.StatusOK,
            wantErr:    false,
        },
        {
            name:     "non-existent intent",
            intentID: "does-not-exist",
            input: map[string]interface{}{
                "type":        "feature",
                "description": "Updated description",
            },
            wantStatus: http.StatusNotFound,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            body, err := json.Marshal(tt.input)
            require.NoError(t, err)

            req := httptest.NewRequest("PUT", "/api/intents/"+tt.intentID, bytes.NewBuffer(body))
            rec := httptest.NewRecorder()

            handler.Update(rec, req.WithContext(
                WithURLParams(req.Context(), map[string]string{"id": tt.intentID}),
            ))

            assert.Equal(t, tt.wantStatus, rec.Code)

            if !tt.wantErr {
                var updated intent.Intent
                err = json.NewDecoder(rec.Body).Decode(&updated)
                require.NoError(t, err)
                assert.Equal(t, tt.intentID, updated.ID)
                assert.Equal(t, tt.input["description"], updated.Description)
            }
        })
    }
}