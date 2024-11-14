package parcel

import (
	"fmt"
	"time"

	"tig/internal/content"
	"tig/internal/intent"
	"tig/internal/stream"
	"tig/shared/types"

	"tig/internal/change"
	"tig/internal/safe"

	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

// Parcel represents a self-contained unit of version-controlled content
type Parcel struct {
	Root         string
	DB           *badger.DB
	ContentStore content.Store
	Workspace    shared.Workspace
	IntentStore  intent.Box
	StreamStore  stream.Box
	Safe         *safe.Safe
	Tracker      change.Tracker
	Logger       *zap.Logger
}

// ParcelConfig defines the configuration settings for a parcel
type ParcelConfig struct {
	Version string    `json:"version"`
	Created time.Time `json:"created"`
	Root    string    `json:"root"`   // Root directory path
	Remote  string    `json:"remote"` // Remote URL if any
}

// ParcelState represents the current operational state of a parcel
type ParcelState struct {
	CurrentStream string            `json:"current_stream"` // Currently active stream
	GatedChanges  map[string]string `json:"gated_changes"`  // Path to content hash mapping
	LastSync      time.Time         `json:"last_sync"`
}

// Box defines the interface for parcel storage operations
type Box interface {
	Create(path string) error
	Open(path string) error
	Close() error
	GetState() (*ParcelState, error)
	UpdateState(*ParcelState) error
	GetConfig() (*ParcelConfig, error)
	UpdateConfig(*ParcelConfig) error
}

func (p *Parcel) GetIntent(id string) (*intent.Intent, error) { return p.IntentStore.Get(id) }
func (p *Parcel) ListIntents() ([]*intent.Intent, error)      { return p.IntentStore.List() }
func (p *Parcel) FindIntentsByType(t string) ([]*intent.Intent, error) {
	return p.IntentStore.FindByType(t)
}

// Stream operations
func (p *Parcel) CreateStream(name, streamType string) (*stream.Stream, error) {
	s := &stream.Stream{
		Name: name,
		Type: streamType,
		Config: stream.Config{
			AutoMerge:    true,
			FeatureFlags: []stream.FeatureFlag{},
			Protection: stream.Protection{
				RequiredReviewers: 1,
				RequiredChecks:    []string{},
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

	if err := p.StreamStore.Create(s); err != nil {
		return nil, fmt.Errorf("creating stream: %w", err)
	}

	return s, nil
}

func (p *Parcel) GetStream(id string) (*stream.Stream, error)  { return p.StreamStore.Get(id) }
func (p *Parcel) ListStreams() ([]*stream.Stream, error)       { return p.StreamStore.List() }
func (p *Parcel) FindActiveStreams() ([]*stream.Stream, error) { return p.StreamStore.FindActive() }
func (p *Parcel) FindStreamsByType(t string) ([]*stream.Stream, error) {
	return p.StreamStore.FindByType(t)
}

// Stream management operations
func (p *Parcel) AddIntentToStream(streamID, intentID string) error {
	return p.StreamStore.AddIntent(streamID, intentID)
}
func (p *Parcel) RemoveIntentFromStream(streamID, intentID string) error {
	return p.StreamStore.RemoveIntent(streamID, intentID)
}
func (p *Parcel) GetStreamIntents(streamID string) ([]*intent.Intent, error) {
	return p.StreamStore.GetIntents(streamID)
}

// Feature flag operations
func (p *Parcel) SetFeatureFlag(streamID string, flag stream.FeatureFlag) error {
	return p.StreamStore.SetFeatureFlag(streamID, flag)
}
func (p *Parcel) GetFeatureFlag(streamID, flagName string) (*stream.FeatureFlag, error) {
	return p.StreamStore.GetFeatureFlag(streamID, flagName)
}

func (p *Parcel) FindIntentsByAuthor(author string) ([]*intent.Intent, error) {
	return p.IntentStore.FindByAuthor(author)
}

func (p *Parcel) FindIntentsByTimeRange(start, end time.Time) ([]*intent.Intent, error) {
	return p.IntentStore.FindByTimeRange(start, end)
}

func (p *Parcel) FindIntentsWithBreakingChanges() ([]*intent.Intent, error) {
	return p.IntentStore.FindWithBreakingChanges()
}
