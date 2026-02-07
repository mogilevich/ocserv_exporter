package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/mogilevich/ocserv_exporter/internal/parser"
)

const (
	// ReconnectWindow is the time window to consider a login as a reconnect
	ReconnectWindow = 5 * time.Minute
	// ProblematicSessionThreshold is the max duration for a session to be considered problematic
	ProblematicSessionThreshold = 60.0 // seconds
	// MaxSessionAge is the maximum age for a session before it's considered stale and cleaned up
	// This prevents "stuck" sessions if disconnect event was missed
	MaxSessionAge = 24 * time.Hour
)

// Session represents an active VPN session
type Session struct {
	Server    string
	Username  string
	ClientIP  string
	Port      int
	VpnIP     string
	Country   string
	SessionID string
	StartTime time.Time
}

// DisconnectRecord tracks recent disconnects for reconnect detection
type DisconnectRecord struct {
	Server    string
	Timestamp time.Time
}

// WorkerContext tracks recent worker events for a session to enrich disconnect reasons
type WorkerContext struct {
	Username    string
	ClientIP    string
	Server      string
	HadBye      bool      // received BYE packet
	DPDWarning  bool      // had DPD warning before disconnect
	DPDSeconds  int       // last DPD warning seconds
	SecModClose bool      // sec-mod temporarily closed session (mobile sleep)
	LastUpdate  time.Time // for cleanup
}

// GeoIPResolver resolves IP addresses to country information
type GeoIPResolver interface {
	Lookup(ip string) (country, countryCode string)
	Close() error
}

// Collector processes ocserv events and updates metrics
type Collector struct {
	mu              sync.RWMutex
	sessions        map[string]*Session          // key: "server:username:clientIP:port"
	lastDisconnects map[string]*DisconnectRecord // key: "server:username" -> last disconnect time
	workerContext   map[string]*WorkerContext    // key: "server:username:clientIP" -> worker context
	parser          *parser.Parser
	geoIP           GeoIPResolver
}

// New creates a new Collector
func New() *Collector {
	return &Collector{
		sessions:        make(map[string]*Session),
		lastDisconnects: make(map[string]*DisconnectRecord),
		workerContext:   make(map[string]*WorkerContext),
		parser:          parser.New(),
	}
}

// SetGeoIPResolver sets the GeoIP resolver
func (c *Collector) SetGeoIPResolver(resolver GeoIPResolver) {
	c.geoIP = resolver
}

// LookupCountry returns the country name for an IP address
func (c *Collector) LookupCountry(ip string) string {
	if c.geoIP == nil {
		return ""
	}
	country, _ := c.geoIP.Lookup(ip)
	return country
}

// ProcessEvent processes a parsed event and updates metrics
func (c *Collector) ProcessEvent(event *parser.Event) {
	// Update last event timestamp
	LastEventTimestamp.Set(float64(event.Timestamp.Unix()))

	switch event.Type {
	case parser.EventUserLogin:
		c.handleLogin(event)
	case parser.EventUserDisconnect:
		c.handleDisconnect(event)
	case parser.EventSessionStart:
		c.handleSessionStart(event)
	case parser.EventVPNIPAssigned:
		c.handleVPNIP(event)
	case parser.EventAuthFailed:
		c.handleAuthFailed(event)
	case parser.EventByePacket:
		c.handleByePacket(event)
	case parser.EventDPDWarning:
		c.handleDPDWarning(event)
	case parser.EventSecModClose:
		c.handleSecModClose(event)
	}
}

// ProcessLogLine parses a log line and processes the resulting event
func (c *Collector) ProcessLogLine(ts time.Time, message string, server string) {
	event := c.parser.Parse(ts, message, server)
	if event.Type != parser.EventUnknown {
		c.ProcessEvent(event)
	}
}

func (c *Collector) handleLogin(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	userKey := fmt.Sprintf("%s:%s", event.Server, event.Username)
	sessionKey := sessionKey(event.Server, event.Username, event.ClientIP, event.Port)

	// Check for reconnect (login within ReconnectWindow of last disconnect)
	if lastDisconnect, ok := c.lastDisconnects[userKey]; ok {
		if event.Timestamp.Sub(lastDisconnect.Timestamp) < ReconnectWindow {
			ReconnectsTotal.WithLabelValues(event.Server, event.Username).Inc()
		}
	}

	// GeoIP lookup for country
	var country string
	if c.geoIP != nil {
		country, _ = c.geoIP.Lookup(event.ClientIP)
	}

	// Store session
	c.sessions[sessionKey] = &Session{
		Server:    event.Server,
		Username:  event.Username,
		ClientIP:  event.ClientIP,
		Port:      event.Port,
		Country:   country,
		StartTime: event.Timestamp,
	}

	// Set session info metric (VPN IP will be updated later when assigned)
	SessionInfo.WithLabelValues(event.Server, event.Username, "", country, "").Set(float64(event.Timestamp.Unix()))

	// Update metrics
	ActiveSessions.WithLabelValues(event.Server, event.Username).Inc()
	ConnectionsTotal.WithLabelValues(event.Server, event.Username, event.ClientIP).Inc()

	// ConnectionsByCountry (uses countryCode too)
	if c.geoIP != nil && country != "" {
		_, countryCode := c.geoIP.Lookup(event.ClientIP)
		ConnectionsByCountry.WithLabelValues(event.Server, event.Username, country, countryCode).Inc()
	}
}

func (c *Collector) handleDisconnect(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	userKey := fmt.Sprintf("%s:%s", event.Server, event.Username)
	key := sessionKey(event.Server, event.Username, event.ClientIP, event.Port)
	ctxKey := workerContextKey(event.Server, event.Username, event.ClientIP)

	var duration float64
	var vpnIP, country string
	sessionExists := false

	if session, ok := c.sessions[key]; ok {
		sessionExists = true
		vpnIP = session.VpnIP
		country = session.Country
		duration = event.Timestamp.Sub(session.StartTime).Seconds()
		if duration > 0 {
			SessionDuration.WithLabelValues(event.Server, event.Username).Observe(duration)
		}
		// Remove session info metric
		SessionInfo.DeleteLabelValues(event.Server, event.Username, vpnIP, country, "")
		delete(c.sessions, key)
	}

	// Enrich disconnect reason based on worker context
	reason := c.enrichDisconnectReason(event.Reason, ctxKey, event.Server, event.Username)

	// Track problematic sessions (short duration + actual error reason)
	// "client bye", "user disconnected", and "mobile sleep" are not errors - expected behavior
	isProblematicReason := reason != "user disconnected" && reason != "client bye" && reason != "mobile sleep" && reason != ""
	if sessionExists && duration < ProblematicSessionThreshold && duration > 0 && isProblematicReason {
		ProblematicSessionsTotal.WithLabelValues(event.Server, event.Username, reason).Inc()
	}

	// Store disconnect time for reconnect detection
	c.lastDisconnects[userKey] = &DisconnectRecord{
		Server:    event.Server,
		Timestamp: event.Timestamp,
	}

	// Update metrics - only decrement active sessions if we tracked the login
	if sessionExists {
		ActiveSessions.WithLabelValues(event.Server, event.Username).Dec()
	}
	DisconnectionsTotal.WithLabelValues(event.Server, event.Username, reason).Inc()
	ReceivedBytesTotal.WithLabelValues(event.Server, event.Username).Add(float64(event.RxBytes))
	SentBytesTotal.WithLabelValues(event.Server, event.Username).Add(float64(event.TxBytes))

	// Clean up worker context after disconnect
	delete(c.workerContext, ctxKey)
	// Also clean up sec-mod context (stored with empty ClientIP)
	secModKey := workerContextKey(event.Server, event.Username, "")
	delete(c.workerContext, secModKey)
}

// enrichDisconnectReason enriches the disconnect reason based on worker context
func (c *Collector) enrichDisconnectReason(originalReason, ctxKey string, server, username string) string {
	ctx, ok := c.workerContext[ctxKey]

	// Also check for sec-mod close context (stored with empty ClientIP)
	secModKey := workerContextKey(server, username, "")
	secModCtx, secModOk := c.workerContext[secModKey]

	// If "unspecified error", try to enrich the reason
	if originalReason == "unspecified error" {
		// Check for sec-mod close (mobile sleep) - this takes priority
		if secModOk && secModCtx.SecModClose {
			return "mobile sleep"
		}
		if ok && ctx.SecModClose {
			return "mobile sleep"
		}

		// Check for BYE packet - client-initiated disconnect
		if ok && ctx.HadBye {
			return "client bye"
		}

		// Check for DPD warning
		if ok && ctx.DPDWarning {
			return "dpd issue"
		}
	}

	return originalReason
}

func (c *Collector) handleSessionStart(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store session by ID for potential future use
	c.sessions["sid:"+event.Server+":"+event.SessionID] = &Session{
		Server:    event.Server,
		Username:  event.Username,
		SessionID: event.SessionID,
		StartTime: event.Timestamp,
	}
}

func (c *Collector) handleVPNIP(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Try to find and update session with VPN IP
	for _, session := range c.sessions {
		if session.Username == event.Username && session.Server == event.Server && session.VpnIP == "" {
			// Delete old metric (without VPN IP) and set new one (with VPN IP)
			SessionInfo.DeleteLabelValues(session.Server, session.Username, "", session.Country, "")
			session.VpnIP = event.VpnIP
			SessionInfo.WithLabelValues(session.Server, session.Username, session.VpnIP, session.Country, "").Set(float64(session.StartTime.Unix()))
			break
		}
	}
}

func (c *Collector) handleAuthFailed(event *parser.Event) {
	country := "Unknown"
	countryCode := ""
	if c.geoIP != nil {
		country, countryCode = c.geoIP.Lookup(event.ClientIP)
		if country == "" {
			country = "Unknown"
		}
	}
	AuthFailedTotal.WithLabelValues(event.Server, event.Username, event.ClientIP, country, countryCode).Inc()
}

func (c *Collector) handleByePacket(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := workerContextKey(event.Server, event.Username, event.ClientIP)
	ctx := c.getOrCreateWorkerContext(key, event)
	ctx.HadBye = true
	ctx.LastUpdate = event.Timestamp
}

func (c *Collector) handleDPDWarning(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := workerContextKey(event.Server, event.Username, event.ClientIP)
	ctx := c.getOrCreateWorkerContext(key, event)
	ctx.DPDWarning = true
	ctx.DPDSeconds = event.DPDSeconds
	ctx.LastUpdate = event.Timestamp
}

func (c *Collector) handleSecModClose(event *parser.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// sec-mod close doesn't have ClientIP in the log, so we need to find existing context by username
	// Mark all contexts for this user as having sec-mod close
	for key, ctx := range c.workerContext {
		if ctx.Username == event.Username && ctx.Server == event.Server {
			ctx.SecModClose = true
			ctx.LastUpdate = event.Timestamp
			c.workerContext[key] = ctx
		}
	}

	// If no existing context, create one with empty ClientIP (will be matched by username in enrichDisconnectReason)
	// This handles the case where sec-mod close happens before any worker events
	key := workerContextKey(event.Server, event.Username, "")
	if _, ok := c.workerContext[key]; !ok {
		c.workerContext[key] = &WorkerContext{
			Username:    event.Username,
			Server:      event.Server,
			SecModClose: true,
			LastUpdate:  event.Timestamp,
		}
	}
}

func (c *Collector) getOrCreateWorkerContext(key string, event *parser.Event) *WorkerContext {
	if ctx, ok := c.workerContext[key]; ok {
		return ctx
	}
	ctx := &WorkerContext{
		Username:   event.Username,
		ClientIP:   event.ClientIP,
		Server:     event.Server,
		LastUpdate: event.Timestamp,
	}
	c.workerContext[key] = ctx
	return ctx
}

func workerContextKey(server, username, clientIP string) string {
	return fmt.Sprintf("%s:%s:%s", server, username, clientIP)
}

// GetActiveSessions returns current active session count
func (c *Collector) GetActiveSessions() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for k := range c.sessions {
		// Only count real sessions, not session IDs
		if len(k) > 4 && k[:4] != "sid:" {
			count++
		}
	}
	return count
}

// CleanupOldDisconnects removes disconnect records older than ReconnectWindow
// and stale sessions older than MaxSessionAge (in case disconnect event was missed)
func (c *Collector) CleanupOldDisconnects() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, record := range c.lastDisconnects {
		if now.Sub(record.Timestamp) > ReconnectWindow*2 {
			delete(c.lastDisconnects, key)
		}
	}

	// Also clean up stale worker contexts (in case disconnect was missed)
	for key, ctx := range c.workerContext {
		if now.Sub(ctx.LastUpdate) > ReconnectWindow*2 {
			delete(c.workerContext, key)
		}
	}

	// Clean up stale sessions (if disconnect event was missed)
	for key, session := range c.sessions {
		// Skip session ID entries (they have different lifecycle)
		if len(key) > 4 && key[:4] == "sid:" {
			continue
		}
		if now.Sub(session.StartTime) > MaxSessionAge {
			// Remove stale session info metric
			SessionInfo.DeleteLabelValues(session.Server, session.Username, session.VpnIP, session.Country, "")
			ActiveSessions.WithLabelValues(session.Server, session.Username).Dec()
			delete(c.sessions, key)
		}
	}
}

func sessionKey(server, username, clientIP string, port int) string {
	return fmt.Sprintf("%s:%s:%s:%d", server, username, clientIP, port)
}
