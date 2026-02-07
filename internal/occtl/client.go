package occtl

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ServerStatus contains parsed data from "occtl show status"
type ServerStatus struct {
	ActiveSessions    int
	TotalSessions     int
	AuthFailures      int
	RxBytes           int64
	TxBytes           int64
	LatencyMedianMs   float64
	LatencyStdevMs    float64
	AvgSessionTimeSec float64
	MaxSessionTimeSec float64
	UptimeSeconds     float64
}

// Session contains parsed data from "occtl show sessions all"
type Session struct {
	SessionID  string
	Username   string
	VHost      string
	ClientIP   string
	UserAgent  string
	CreatedAgo time.Duration
	Status     string
}

// User contains parsed data from "occtl show users"
type User struct {
	ID         int
	Username   string
	VHost      string
	ClientIP   string
	VpnIP      string
	Device     string
	Since      time.Duration
	DTLSCipher string
	Status     string
}

// Client provides interface to occtl command
type Client struct {
	socketPath string
	serverName string
}

// NewClient creates a new occtl client
// socketPath can be empty to use default socket
// serverName is used for metrics labeling
func NewClient(socketPath, serverName string) *Client {
	return &Client{
		socketPath: socketPath,
		serverName: serverName,
	}
}

// ServerName returns the server name for this client
func (c *Client) ServerName() string {
	return c.serverName
}

// execOcctl runs occtl with given arguments
func (c *Client) execOcctl(args ...string) (string, error) {
	cmdArgs := args
	if c.socketPath != "" {
		cmdArgs = append([]string{"-s", c.socketPath}, args...)
	}

	// Use sudo if available and needed (occtl requires root for socket access)
	cmd := exec.Command("sudo", append([]string{"-n", "occtl"}, cmdArgs...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include stderr in error message for debugging
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}

	return stdout.String(), nil
}

// GetStatus returns server status from "occtl show status"
func (c *Client) GetStatus() (*ServerStatus, error) {
	output, err := c.execOcctl("show", "status")
	if err != nil {
		return nil, err
	}

	return parseStatus(output)
}

// GetSessions returns all sessions from "occtl show sessions all"
func (c *Client) GetSessions() ([]Session, error) {
	output, err := c.execOcctl("show", "sessions", "all")
	if err != nil {
		return nil, err
	}

	return parseSessions(output)
}

// GetUsers returns all users from "occtl show users"
func (c *Client) GetUsers() ([]User, error) {
	output, err := c.execOcctl("show", "users")
	if err != nil {
		return nil, err
	}

	return parseUsers(output)
}

// parseStatus parses output of "occtl show status"
func parseStatus(output string) (*ServerStatus, error) {
	status := &ServerStatus{}

	patterns := map[string]*regexp.Regexp{
		"active":     regexp.MustCompile(`Active sessions:\s*(\d+)`),
		"total":      regexp.MustCompile(`Total sessions:\s*(\d+)`),
		"authFail":   regexp.MustCompile(`Total authentication failures:\s*(\d+)`),
		"rx":         regexp.MustCompile(`RX:\s*([\d.]+)\s*(\w+)`),
		"tx":         regexp.MustCompile(`TX:\s*([\d.]+)\s*(\w+)`),
		"latencyMed": regexp.MustCompile(`Median latency:\s*<?(\d+)m?s?`),
		"latencyStd": regexp.MustCompile(`STDEV latency:\s*<?(\d+)m?s?`),
		"avgSession": regexp.MustCompile(`Average session time:\s*(.+)`),
		"maxSession": regexp.MustCompile(`Max session time:\s*(.+)`),
		"uptime":     regexp.MustCompile(`Up since:.+\(\s*(.+?)\s*\)`),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if m := patterns["active"].FindStringSubmatch(line); m != nil {
			status.ActiveSessions, _ = strconv.Atoi(m[1])
		}
		if m := patterns["total"].FindStringSubmatch(line); m != nil {
			status.TotalSessions, _ = strconv.Atoi(m[1])
		}
		if m := patterns["authFail"].FindStringSubmatch(line); m != nil {
			status.AuthFailures, _ = strconv.Atoi(m[1])
		}
		if m := patterns["rx"].FindStringSubmatch(line); m != nil {
			status.RxBytes = parseBytes(m[1], m[2])
		}
		if m := patterns["tx"].FindStringSubmatch(line); m != nil {
			status.TxBytes = parseBytes(m[1], m[2])
		}
		if m := patterns["latencyMed"].FindStringSubmatch(line); m != nil {
			val, _ := strconv.ParseFloat(m[1], 64)
			status.LatencyMedianMs = val
		}
		if m := patterns["latencyStd"].FindStringSubmatch(line); m != nil {
			val, _ := strconv.ParseFloat(m[1], 64)
			status.LatencyStdevMs = val
		}
		if m := patterns["avgSession"].FindStringSubmatch(line); m != nil {
			status.AvgSessionTimeSec = parseDuration(m[1])
		}
		if m := patterns["maxSession"].FindStringSubmatch(line); m != nil {
			status.MaxSessionTimeSec = parseDuration(m[1])
		}
		if m := patterns["uptime"].FindStringSubmatch(line); m != nil {
			status.UptimeSeconds = parseDuration(m[1])
		}
	}

	return status, nil
}

// parseSessions parses output of "occtl show sessions all"
func parseSessions(output string) ([]Session, error) {
	var sessions []Session

	scanner := bufio.NewScanner(strings.NewReader(output))

	// Skip header line
	headerSkipped := false
	for scanner.Scan() {
		line := scanner.Text()

		// Skip header
		if strings.HasPrefix(strings.TrimSpace(line), "session") {
			headerSkipped = true
			continue
		}
		if !headerSkipped {
			continue
		}

		// Parse session line
		// Format: session     user    vhost             ip               user agent  created   status
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		session := Session{
			SessionID: fields[0],
			Username:  fields[1],
			VHost:     fields[2],
			ClientIP:  fields[3],
		}

		// User agent can contain spaces, so we need to find "created" time pattern
		// Time patterns: 1m:42s, 3h:54m, 58s, etc.
		timePattern := regexp.MustCompile(`\d+[hms](?::\d+[ms])?`)

		// Find the time field from the end
		restOfLine := strings.Join(fields[4:], " ")

		// Find last occurrence of status (authenticated/connected)
		statusIdx := strings.LastIndex(restOfLine, "authenticated")
		if statusIdx == -1 {
			statusIdx = strings.LastIndex(restOfLine, "connected")
		}

		if statusIdx > 0 {
			beforeStatus := strings.TrimSpace(restOfLine[:statusIdx])
			session.Status = strings.TrimSpace(restOfLine[statusIdx:])

			// Find time in beforeStatus
			timeLocs := timePattern.FindAllStringIndex(beforeStatus, -1)
			if len(timeLocs) > 0 {
				lastTimeLoc := timeLocs[len(timeLocs)-1]
				timeStr := beforeStatus[lastTimeLoc[0]:lastTimeLoc[1]]
				session.CreatedAgo = time.Duration(parseDuration(timeStr)) * time.Second
				session.UserAgent = strings.TrimSpace(beforeStatus[:lastTimeLoc[0]])
			} else {
				session.UserAgent = beforeStatus
			}
		}

		if session.Username != "" {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// parseUsers parses output of "occtl show users"
// Format:       id     user    vhost             ip         vpn-ip device   since    dtls-cipher    status
//
//	3800826 a.zakiev  default   172.30.30.30    10.88.18.67 ocserv-ru3    35s      (no-dtls) connected
func parseUsers(output string) ([]User, error) {
	var users []User

	scanner := bufio.NewScanner(strings.NewReader(output))

	// Skip header line
	headerSkipped := false
	for scanner.Scan() {
		line := scanner.Text()

		// Skip header (starts with "id")
		if strings.Contains(line, "id") && strings.Contains(line, "user") && strings.Contains(line, "vpn-ip") {
			headerSkipped = true
			continue
		}
		if !headerSkipped {
			continue
		}

		// Parse user line
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		id, _ := strconv.Atoi(fields[0])
		user := User{
			ID:       id,
			Username: fields[1],
			VHost:    fields[2],
			ClientIP: fields[3],
			VpnIP:    fields[4],
			Device:   fields[5],
			Since:    time.Duration(parseDuration(fields[6])) * time.Second,
			Status:   fields[len(fields)-1], // last field is status
		}

		// DTLS cipher is second to last (may contain parentheses)
		if len(fields) >= 9 {
			user.DTLSCipher = fields[len(fields)-2]
		}

		if user.Username != "" {
			users = append(users, user)
		}
	}

	return users, nil
}

// parseBytes converts value and unit (KB, MB, GB) to bytes
func parseBytes(valueStr, unit string) int64 {
	value, _ := strconv.ParseFloat(valueStr, 64)

	switch strings.ToUpper(unit) {
	case "KB":
		return int64(value * 1024)
	case "MB":
		return int64(value * 1024 * 1024)
	case "GB":
		return int64(value * 1024 * 1024 * 1024)
	case "TB":
		return int64(value * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(value)
	}
}

// parseDuration parses time strings like "3h:54m", "18m:00s", "58s"
func parseDuration(s string) float64 {
	s = strings.TrimSpace(s)

	var totalSeconds float64

	// Handle "3h:54m" format
	if strings.Contains(s, "h") {
		parts := strings.Split(s, "h")
		hours, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		totalSeconds += hours * 3600
		if len(parts) > 1 {
			s = strings.TrimPrefix(parts[1], ":")
		} else {
			return totalSeconds
		}
	}

	// Handle minutes
	if strings.Contains(s, "m") {
		parts := strings.Split(s, "m")
		minutes, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		totalSeconds += minutes * 60
		if len(parts) > 1 {
			s = strings.TrimPrefix(parts[1], ":")
		} else {
			return totalSeconds
		}
	}

	// Handle seconds
	if strings.Contains(s, "s") {
		secStr := strings.TrimSuffix(s, "s")
		seconds, _ := strconv.ParseFloat(strings.TrimSpace(secStr), 64)
		totalSeconds += seconds
	}

	return totalSeconds
}

// GetUserAgentStats returns aggregated user agent statistics
func (c *Client) GetUserAgentStats() (map[string]int, error) {
	sessions, err := c.GetSessions()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	for _, s := range sessions {
		clientType := classifyUserAgent(s.UserAgent)
		stats[clientType]++
	}

	return stats, nil
}

// GetUserSessionCounts returns number of concurrent sessions per username
func (c *Client) GetUserSessionCounts() (map[string]int, error) {
	sessions, err := c.GetSessions()
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, s := range sessions {
		counts[s.Username]++
	}

	return counts, nil
}

// GetUserClientTypes returns client type per username
func (c *Client) GetUserClientTypes() (map[string]string, error) {
	sessions, err := c.GetSessions()
	if err != nil {
		return nil, err
	}

	types := make(map[string]string)
	for _, s := range sessions {
		types[s.Username] = classifyUserAgent(s.UserAgent)
	}

	return types, nil
}

// classifyUserAgent categorizes user agent string into client type
func classifyUserAgent(ua string) string {
	ua = strings.ToLower(ua)

	switch {
	case strings.Contains(ua, "android"):
		return "AnyConnect Mobile (Android)"
	case strings.Contains(ua, "applesslvpn") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		return "AnyConnect Mobile (iOS)"
	case strings.Contains(ua, "openconnect-gui"):
		return "OpenConnect GUI"
	case strings.Contains(ua, "openconnect vpn agent"):
		return "OpenConnect VPN Agent"
	case strings.Contains(ua, "open anyconnect"):
		return "Open AnyConnect"
	case strings.Contains(ua, "anyconnect darwin"):
		return "AnyConnect (macOS)"
	case strings.Contains(ua, "anyconnect windows"):
		return "AnyConnect (Windows)"
	case strings.Contains(ua, "anyconnect"):
		return "AnyConnect (Other)"
	case strings.Contains(ua, "openconnect"):
		return "OpenConnect (CLI)"
	default:
		if ua == "" {
			return "Unknown"
		}
		return "Other"
	}
}
