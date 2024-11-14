// internal/api/handlers.go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tig/internal/errors"
	"tig/internal/intent"
	"tig/internal/stream"

	"github.com/google/uuid"
)

// Mock intent store
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

type IntentHandler struct {
    box intent.Box
}

func NewIntentHandler(box intent.Box) *IntentHandler {
    return &IntentHandler{box: box}
}

func (h *IntentHandler) Create(w http.ResponseWriter, r *http.Request) {
    var i intent.Intent
    if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // Validate required fields
    if i.Description == "" {
        http.Error(w, "description is required", http.StatusBadRequest)
        return
    }

    // Set system fields
    i.ID = uuid.New().String()
    i.CreatedAt = time.Now()
    i.UpdatedAt = i.CreatedAt

    if err := h.box.Create(&i); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(i)
}

func (h *IntentHandler) Get(w http.ResponseWriter, r *http.Request) {
    // Get ID from URL params stored in context
    params := r.Context().Value("url_params").(map[string]string)
    id := params["id"]
    if id == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }

    i, err := h.box.Get(id)
    if err != nil {
        // Check if it's a not found error
        if err.Error() == "intent not found: "+id {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(i)
}

func (h *IntentHandler) Update(w http.ResponseWriter, r *http.Request) {
    // Get ID from URL params stored in context
    params := r.Context().Value("url_params").(map[string]string)
    id := params["id"]
    if id == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }

    // Get existing intent
    existing, err := h.box.Get(id)
    if err != nil {
        if err.Error() == "intent not found: "+id {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Decode updates
    var updates intent.Intent
    if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // Apply updates while preserving system fields
    updates.ID = existing.ID
    updates.CreatedAt = existing.CreatedAt
    updates.UpdatedAt = time.Now()

    if err := h.box.Update(&updates); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(updates)
}

func (h *IntentHandler) Delete(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }

    if err := h.box.Delete(id); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func (h *IntentHandler) List(w http.ResponseWriter, r *http.Request) {
    intents, err := h.box.List()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(intents)
}

// StreamHandler handles HTTP requests for Stream operations
type StreamHandler struct {
    box stream.Box
}

func NewStreamHandler(box stream.Box) *StreamHandler {
    return &StreamHandler{box: box}
}

func (h *StreamHandler) Create(w http.ResponseWriter, r *http.Request) {
    var st stream.Stream
    if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    // Validate required fields
    if st.Name == "" {
        http.Error(w, "name is required", http.StatusBadRequest)
        return
    }

    // Set system fields
    st.ID = uuid.New().String()
    st.CreatedAt = time.Now()
    st.UpdatedAt = st.CreatedAt
    st.State.Active = true
    st.State.Status = "stable"
    st.State.LastSync = st.CreatedAt

    if err := h.box.Create(&st); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(st)
}

func (h *StreamHandler) Get(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }

    st, err := h.box.Get(id)
    if err != nil {
        if _, ok := err.(*errors.Error); ok {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(st)
}

func (h *StreamHandler) Delete(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }

    if err := h.box.Delete(id); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func (h *StreamHandler) AddIntent(w http.ResponseWriter, r *http.Request) {
    streamID := r.PathValue("id")
    if streamID == "" {
        http.Error(w, "missing stream id", http.StatusBadRequest)
        return
    }

    var req struct {
        IntentID string `json:"intent_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    if err := h.box.AddIntent(streamID, req.IntentID); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func (h *StreamHandler) RemoveIntent(w http.ResponseWriter, r *http.Request) {
    streamID := r.PathValue("id")
    if streamID == "" {
        http.Error(w, "missing stream id", http.StatusBadRequest)
        return
    }

    var req struct {
        IntentID string `json:"intent_id"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    if err := h.box.RemoveIntent(streamID, req.IntentID); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func (h *StreamHandler) GetIntents(w http.ResponseWriter, r *http.Request) {
    streamID := r.PathValue("id")
    if streamID == "" {
        http.Error(w, "missing stream id", http.StatusBadRequest)
        return
    }

    intents, err := h.box.GetIntents(streamID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(intents)
}

func (h *StreamHandler) SetFeatureFlag(w http.ResponseWriter, r *http.Request) {
    streamID := r.PathValue("id")
    if streamID == "" {
        http.Error(w, "missing stream id", http.StatusBadRequest)
        return
    }

    var flag stream.FeatureFlag
    if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    if err := h.box.SetFeatureFlag(streamID, flag); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
}

func (h *StreamHandler) GetFeatureFlags(w http.ResponseWriter, r *http.Request) {
    streamID := r.PathValue("id")
    if streamID == "" {
        http.Error(w, "missing stream id", http.StatusBadRequest)
        return
    }

    st, err := h.box.Get(streamID)
    if err != nil {
        if _, ok := err.(*errors.Error); ok {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(st.Config.FeatureFlags)
}

// WithURLParams adds URL parameters to request context for testing
func WithURLParams(ctx context.Context, params map[string]string) context.Context {
    return context.WithValue(ctx, "url_params", params)
}

// Additional helper for mocking request context
type mockRequestContext struct {
    id string
}

func (m *mockRequestContext) Value(key interface{}) interface{} {
    if key == "url_params" {
        return map[string]string{
            "id": m.id,
        }
    }
    return nil
}