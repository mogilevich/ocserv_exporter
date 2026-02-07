package parser

import (
	"regexp"
	"strconv"
	"time"
)

// EventType represents the type of ocserv log event
type EventType int

const (
	EventUnknown EventType = iota
	EventUserLogin
	EventUserDisconnect
	EventSessionStart
	EventSessionInvalidate
	EventVPNIPAssigned
	EventAuthFailed
	EventByePacket    // worker received BYE packet from client
	EventDPDWarning   // worker DPD timeout warning
	EventSecModClose  // sec-mod temporarily closing session (mobile sleep)
)

// Event represents a parsed ocserv log event
type Event struct {
	Type      EventType
	Timestamp time.Time
	Server    string // VPN server name (e.g., "ocserv", "ocserv-ru")
	Username  string
	ClientIP  string
	Port      int
	VpnIP     string
	SessionID string
	Reason    string
	RxBytes   uint64
	TxBytes   uint64
	Raw       string
	DPDSeconds int // seconds since last DPD (for EventDPDWarning)
}

// Parser parses ocserv log lines
type Parser struct {
	reLogin             *regexp.Regexp
	reDisconnect        *regexp.Regexp
	reSessionStart      *regexp.Regexp
	reSessionInvalidate *regexp.Regexp
	reVPNIP             *regexp.Regexp
	reAuthFailed        *regexp.Regexp
	reCookieAuthFailed  *regexp.Regexp
	reByePacket         *regexp.Regexp
	reDPDWarning        *regexp.Regexp
	reSecModClose       *regexp.Regexp
}

// New creates a new Parser
func New() *Parser {
	return &Parser{
		// main[a.mogilevich]:62.4.32.53:30595 user logged in
		reLogin: regexp.MustCompile(`main\[([^\]]+)\]:([^:]+):(\d+) user logged in`),

		// main[a.mogilevich]:62.4.32.53:30595 user disconnected (reason: user disconnected, rx: 13295, tx: 24650)
		reDisconnect: regexp.MustCompile(`main\[([^\]]+)\]:([^:]+):(\d+) user disconnected \(reason: ([^,]+), rx: (\d+), tx: (\d+)\)`),

		// sec-mod: initiating session for user 'a.mogilevich' (session: yKsy7b)
		reSessionStart: regexp.MustCompile(`sec-mod: initiating session for user '([^']+)' \(session: ([^)]+)\)`),

		// sec-mod: invalidating session of user 'a.mogilevich' (session: yKsy7b)
		reSessionInvalidate: regexp.MustCompile(`sec-mod: invalidating session of user '([^']+)' \(session: ([^)]+)\)`),

		// worker[a.mogilevich]: 62.4.32.53 sending IPv4 10.88.9.156
		reVPNIP: regexp.MustCompile(`worker\[([^\]]+)\]: [^ ]+ sending IPv4 ([0-9.]+)`),

		// main:172.30.30.30:56078 failed authentication attempt for user ''
		// main[username]:ip:port failed authentication attempt for user 'username'
		reAuthFailed: regexp.MustCompile(`main(?:\[([^\]]*)\])?:([^:]+):(\d+) failed authentication attempt`),

		// worker: 172.30.30.30 failed cookie authentication attempt
		reCookieAuthFailed: regexp.MustCompile(`worker(?:\[([^\]]*)\])?: ([^ ]+) failed cookie authentication attempt`),

		// worker[username]: 172.30.30.30 received BYE packet; exiting
		reByePacket: regexp.MustCompile(`worker\[([^\]]+)\]: ([^ ]+) received BYE packet`),

		// worker[username]: 172.30.30.30 have not received TCP DPD for long (137 secs)
		reDPDWarning: regexp.MustCompile(`worker\[([^\]]+)\]: ([^ ]+) have not received TCP DPD for long \((\d+) secs\)`),

		// sec-mod: temporarily closing session for a.mogilevich (session: u7N/JC)
		reSecModClose: regexp.MustCompile(`sec-mod: temporarily closing session for ([^ ]+) \(session: ([^)]+)\)`),
	}
}

// Parse parses a log line and returns an Event
func (p *Parser) Parse(ts time.Time, message string, server string) *Event {
	event := &Event{
		Type:      EventUnknown,
		Timestamp: ts,
		Server:    server,
		Raw:       message,
	}

	// Try login pattern
	if matches := p.reLogin.FindStringSubmatch(message); matches != nil {
		event.Type = EventUserLogin
		event.Username = matches[1]
		event.ClientIP = matches[2]
		event.Port, _ = strconv.Atoi(matches[3])
		return event
	}

	// Try disconnect pattern
	if matches := p.reDisconnect.FindStringSubmatch(message); matches != nil {
		event.Type = EventUserDisconnect
		event.Username = matches[1]
		event.ClientIP = matches[2]
		event.Port, _ = strconv.Atoi(matches[3])
		event.Reason = matches[4]
		event.RxBytes, _ = strconv.ParseUint(matches[5], 10, 64)
		event.TxBytes, _ = strconv.ParseUint(matches[6], 10, 64)
		return event
	}

	// Try session start pattern
	if matches := p.reSessionStart.FindStringSubmatch(message); matches != nil {
		event.Type = EventSessionStart
		event.Username = matches[1]
		event.SessionID = matches[2]
		return event
	}

	// Try session invalidate pattern
	if matches := p.reSessionInvalidate.FindStringSubmatch(message); matches != nil {
		event.Type = EventSessionInvalidate
		event.Username = matches[1]
		event.SessionID = matches[2]
		return event
	}

	// Try VPN IP pattern
	if matches := p.reVPNIP.FindStringSubmatch(message); matches != nil {
		event.Type = EventVPNIPAssigned
		event.Username = matches[1]
		event.VpnIP = matches[2]
		return event
	}

	// Try auth failed pattern
	if matches := p.reAuthFailed.FindStringSubmatch(message); matches != nil {
		event.Type = EventAuthFailed
		event.Username = matches[1] // may be empty
		event.ClientIP = matches[2]
		event.Port, _ = strconv.Atoi(matches[3])
		return event
	}

	// Try cookie auth failed pattern
	if matches := p.reCookieAuthFailed.FindStringSubmatch(message); matches != nil {
		event.Type = EventAuthFailed
		event.Username = matches[1] // may be empty
		event.ClientIP = matches[2]
		return event
	}

	// Try BYE packet pattern
	if matches := p.reByePacket.FindStringSubmatch(message); matches != nil {
		event.Type = EventByePacket
		event.Username = matches[1]
		event.ClientIP = matches[2]
		return event
	}

	// Try DPD warning pattern
	if matches := p.reDPDWarning.FindStringSubmatch(message); matches != nil {
		event.Type = EventDPDWarning
		event.Username = matches[1]
		event.ClientIP = matches[2]
		event.DPDSeconds, _ = strconv.Atoi(matches[3])
		return event
	}

	// Try sec-mod close pattern (mobile sleep)
	if matches := p.reSecModClose.FindStringSubmatch(message); matches != nil {
		event.Type = EventSecModClose
		event.Username = matches[1]
		event.SessionID = matches[2]
		return event
	}

	return event
}
