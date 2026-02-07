//go:build linux

package journal

import (
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
)

// JournalReader reads from systemd journal
type JournalReader struct {
	journal *sdjournal.Journal
	units   []string
}

// NewJournalReader creates a new journal reader for the specified units
func NewJournalReader(units []string, since time.Duration) (*JournalReader, error) {
	j, err := sdjournal.NewJournal()
	if err != nil {
		return nil, fmt.Errorf("failed to open journal: %w", err)
	}

	// Filter by _SYSTEMD_UNIT (OR between units)
	// Note: We use _SYSTEMD_UNIT instead of SYSLOG_IDENTIFIER because ocserv
	// uses hardcoded "ocserv" as syslog identifier regardless of SyslogIdentifier= setting.
	for i, unit := range units {
		// Strip .service suffix if present (for backward compatibility)
		unit = strings.TrimSuffix(unit, ".service")
		match := "_SYSTEMD_UNIT=" + unit + ".service"
		if err := j.AddMatch(match); err != nil {
			j.Close()
			return nil, fmt.Errorf("failed to add match for %s: %w", unit, err)
		}
		// Add disjunction (OR) between units
		if i < len(units)-1 {
			if err := j.AddDisjunction(); err != nil {
				j.Close()
				return nil, fmt.Errorf("failed to add disjunction: %w", err)
			}
		}
	}

	// Seek to starting position
	if since > 0 {
		startTime := time.Now().Add(-since)
		usec := uint64(startTime.UnixMicro())
		if err := j.SeekRealtimeUsec(usec); err != nil {
			j.Close()
			return nil, fmt.Errorf("failed to seek: %w", err)
		}
	} else {
		// Start from the end (only new entries)
		if err := j.SeekTail(); err != nil {
			j.Close()
			return nil, fmt.Errorf("failed to seek to tail: %w", err)
		}
	}

	return &JournalReader{
		journal: j,
		units:   units,
	}, nil
}

// Read returns the next log entry
func (r *JournalReader) Read() (*Entry, error) {
	for {
		// Try to advance to next entry
		n, err := r.journal.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to advance journal: %w", err)
		}

		if n == 0 {
			// No more entries, wait for new ones
			r.journal.Wait(sdjournal.IndefiniteWait)
			continue
		}

		// Get entry data
		entry, err := r.journal.GetEntry()
		if err != nil {
			return nil, fmt.Errorf("failed to get entry: %w", err)
		}

		message, ok := entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]
		if !ok {
			continue
		}

		// Get systemd unit name (e.g., "ocserv.service" or "ocserv-ru.service")
		// We use _SYSTEMD_UNIT because ocserv uses hardcoded "ocserv" as SYSLOG_IDENTIFIER
		unit := strings.TrimSuffix(entry.Fields["_SYSTEMD_UNIT"], ".service")

		timestamp := time.Unix(0, int64(entry.RealtimeTimestamp)*1000)

		return &Entry{
			Timestamp: timestamp,
			Message:   message,
			Unit:      unit,
		}, nil
	}
}

// Close closes the journal reader
func (r *JournalReader) Close() error {
	return r.journal.Close()
}
