package intent

import (
	"tig/internal/errors"
)

// ValidateIntent validates an intent
func ValidateIntent(i *Intent) error {
    if i.Description == "" {
        return errors.ValidationError("description is required", nil)
    }
    
    validTypes := map[string]bool{
        "feature":   true,
        "fix":       true,
        "refactor": true,
        "security": true,
    }
    
    if !validTypes[i.Type] {
        return errors.ValidationError("invalid intent type", nil)
    }
    
    return nil
}