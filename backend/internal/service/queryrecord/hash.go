package queryrecord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// canonicalizeSpec parses the incoming spec JSON and re-marshals it so that two
// logically identical specs (different key order, insignificant whitespace) map
// to the exact same byte sequence. The deterministic output is what we hash for
// deduplication, so the same query built two different ways still collides.
//
// Go's encoding/json sorts map keys during marshaling, which gives us a stable
// canonical form without pulling in an extra canonicalization library.
func canonicalizeSpec(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, fmt.Errorf("parse spec json: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("re-marshal spec json: %w", err)
	}
	return out, nil
}

// hashSpec returns the hex SHA-256 of the canonical spec bytes.
func hashSpec(canon []byte) string {
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}
