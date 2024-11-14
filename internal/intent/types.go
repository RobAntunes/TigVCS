// internal/intent/types.go
package intent

import (
	"time"
)

// Intent represents a semantic grouping of changes
type Intent struct {
    ID          string    `json:"id"`
    Type        string    `json:"type"`
    Description string    `json:"description"`
    Impact      Impact    `json:"impact"`
    Metadata    Metadata  `json:"metadata"`
    ChangeSetID string    `json:"changeset_id"` // Added field
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Impact struct {
	Scope        []string `json:"scope"`        // Affected components
	Breaking     bool     `json:"breaking"`     // Is this a breaking change?
	Dependencies []string `json:"dependencies"` // Impacted dependencies
}

type Metadata struct {
	Author string   `json:"author"`
	Refs   []string `json:"refs"` // Related tickets/docs
}

// Box interface defines how we store/retrieve intents
type Box interface {
	Create(intent *Intent) error
	Get(id string) (*Intent, error)
	Update(intent *Intent) error
	Delete(id string) error
	List() ([]*Intent, error)

	// Search operations
	FindByType(intentType string) ([]*Intent, error)
	FindByAuthor(author string) ([]*Intent, error)
	FindByTimeRange(start, end time.Time) ([]*Intent, error)
	FindWithBreakingChanges() ([]*Intent, error)
}
