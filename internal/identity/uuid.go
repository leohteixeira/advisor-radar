// Package identity parses and generates UUIDv7 identifiers.
package identity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidID is returned when a string is not a UUIDv7.
var ErrInvalidID = errors.New("invalid uuidv7")

// ParseV7 parses s as a UUID and requires version 7.
// Uppercase and lowercase hex are accepted; the returned string is lowercase.
func ParseV7(s string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	if id.Version() != 7 {
		return "", fmt.Errorf("%w: version %d", ErrInvalidID, id.Version())
	}
	return id.String(), nil
}

// IsV7 reports whether s is a UUIDv7.
func IsV7(s string) bool {
	_, err := ParseV7(s)
	return err == nil
}

// NewV7 returns a fresh UUIDv7 string.
func NewV7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("identity: generate uuidv7: %w", err)
	}
	return id.String(), nil
}

// MustNewV7 returns a fresh UUIDv7 string or panics.
func MustNewV7() string {
	id, err := NewV7()
	if err != nil {
		panic(err)
	}
	return id
}
