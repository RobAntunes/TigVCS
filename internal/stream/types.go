package stream

import (
    "time"
    "tig/internal/intent"
)

type Stream struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Type        string    `json:"type"`    // feature, release, hotfix
    Config      Config    `json:"config"`
    State       State     `json:"state"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Config struct {
    AutoMerge    bool           `json:"auto_merge"`
    FeatureFlags []FeatureFlag  `json:"feature_flags"`
    Protection   Protection     `json:"protection"`
}

type FeatureFlag struct {
    Name        string   `json:"name"`
    Conditions  []string `json:"conditions"`
    Enabled     bool     `json:"enabled"`
}

type Protection struct {
    RequiredReviewers int      `json:"required_reviewers"`
    RequiredChecks    []string `json:"required_checks"`
}

type State struct {
    Active    bool      `json:"active"`
    Status    string    `json:"status"`    // stable, integrating, conflict
    LastSync  time.Time `json:"last_sync"`
    Intents   []string  `json:"intents"`   // IDs of associated intents
}

// Box defines the interface for stream storage operations
type Box interface {
    Create(stream *Stream) error
    Get(id string) (*Stream, error)
    Update(stream *Stream) error
    Delete(id string) error
    List() ([]*Stream, error)
    
    // Stream-specific operations
    AddIntent(streamID string, intentID string) error
    RemoveIntent(streamID string, intentID string) error
    GetIntents(streamID string) ([]*intent.Intent, error)
    
    // Feature flag operations
    SetFeatureFlag(streamID string, flag FeatureFlag) error
    GetFeatureFlag(streamID string, flagName string) (*FeatureFlag, error)
    
    // Search operations
    FindByType(streamType string) ([]*Stream, error)
    FindActive() ([]*Stream, error)
}