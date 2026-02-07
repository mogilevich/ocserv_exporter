package journal

import (
	"time"
)

// Entry represents a log entry
type Entry struct {
	Timestamp time.Time
	Message   string
	Unit      string // systemd unit name without .service suffix (e.g., "ocserv", "ocserv-ru")
}

// Reader is the interface for reading log entries
type Reader interface {
	// Read returns the next log entry, blocking if necessary
	// Returns nil when the reader is closed
	Read() (*Entry, error)

	// Close closes the reader
	Close() error
}

// Handler is called for each log entry
type Handler func(entry *Entry)
