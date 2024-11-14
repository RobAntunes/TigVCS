package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"tig/shared/types"
)

// mapToSlice converts a map of changes to a slice of changes
// Make generic MapToSlice function
func MapToSlice(m map[string]shared.Change) []shared.Change {
	var s []shared.Change
	for _, v := range m {
		s = append(s, v)
	}
	return s
}

func HashContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}



