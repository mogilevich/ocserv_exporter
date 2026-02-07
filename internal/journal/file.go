package journal

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"time"
)

// FileReader reads log entries from a file (tail -f style)
type FileReader struct {
	file    *os.File
	scanner *bufio.Scanner
	reTime  *regexp.Regexp
}

// NewFileReader creates a new file reader
// If follow is true, it will wait for new lines (like tail -f)
func NewFileReader(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &FileReader{
		file:    f,
		scanner: bufio.NewScanner(f),
		// Match: Feb 03 07:46:56 hostname ocserv[pid]: message
		// or:    Feb 03 07:46:56 hostname ocserv-ru[pid]: message
		reTime: regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+(ocserv[^\[]*)\[\d+\]:\s+(.+)$`),
	}, nil
}

// Read returns the next log entry
func (r *FileReader) Read() (*Entry, error) {
	for r.scanner.Scan() {
		line := r.scanner.Text()

		matches := r.reTime.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		// Parse timestamp (use current year since syslog doesn't include it)
		ts, err := time.Parse("Jan 02 15:04:05 2006", matches[1]+" "+fmt.Sprint(time.Now().Year()))
		if err != nil {
			ts = time.Now()
		}

		return &Entry{
			Timestamp: ts,
			Message:   matches[3],
			Unit:      matches[2], // e.g., "ocserv" or "ocserv-ru"
		}, nil
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, nil // EOF
}

// Close closes the file reader
func (r *FileReader) Close() error {
	return r.file.Close()
}
