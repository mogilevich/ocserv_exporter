//go:build !linux

package journal

import (
	"errors"
	"time"
)

// JournalReader is not available on non-Linux systems
type JournalReader struct{}

// NewJournalReader returns an error on non-Linux systems
func NewJournalReader(units []string, since time.Duration) (*JournalReader, error) {
	return nil, errors.New("journald is only available on Linux")
}

// Read is not implemented on non-Linux systems
func (r *JournalReader) Read() (*Entry, error) {
	return nil, errors.New("journald is only available on Linux")
}

// Close is not implemented on non-Linux systems
func (r *JournalReader) Close() error {
	return nil
}
