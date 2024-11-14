package validation

import (
    "encoding/json"
    "net/http"
    "tig/internal/errors"
	"tig/internal/intent"
)

type Validator interface {
    Validate() error
}

func ValidateIntentRequest(r *http.Request) (*intent.Intent, error) {
    var i intent.Intent
    if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
        return nil, errors.ValidationError("invalid request body", nil)
    }
    
    if err := intent.ValidateIntent(&i); err != nil {
        return nil, err
    }
    
    return &i, nil
}