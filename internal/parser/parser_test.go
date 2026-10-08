package parser

import (
	"testing"
	"time"
)

func TestParser(t *testing.T) {
	p := New()
	ts := time.Now()

	tests := []struct {
		name     string
		message  string
		wantType EventType
		check    func(*Event) bool
	}{
		{
			name:     "user login",
			message:  "main[j.doe]:203.0.113.53:30595 user logged in",
			wantType: EventUserLogin,
			check: func(e *Event) bool {
				return e.Username == "j.doe" &&
					e.ClientIP == "203.0.113.53" &&
					e.Port == 30595
			},
		},
		{
			name:     "user disconnect",
			message:  "main[j.doe]:203.0.113.53:30595 user disconnected (reason: user disconnected, rx: 13295, tx: 24650)",
			wantType: EventUserDisconnect,
			check: func(e *Event) bool {
				return e.Username == "j.doe" &&
					e.ClientIP == "203.0.113.53" &&
					e.Reason == "user disconnected" &&
					e.RxBytes == 13295 &&
					e.TxBytes == 24650
			},
		},
		{
			name:     "session start",
			message:  "sec-mod: initiating session for user 'j.doe' (session: yKsy7b)",
			wantType: EventSessionStart,
			check: func(e *Event) bool {
				return e.Username == "j.doe" &&
					e.SessionID == "yKsy7b"
			},
		},
		{
			name:     "session invalidate",
			message:  "sec-mod: invalidating session of user 'j.doe' (session: yKsy7b)",
			wantType: EventSessionInvalidate,
			check: func(e *Event) bool {
				return e.Username == "j.doe" &&
					e.SessionID == "yKsy7b"
			},
		},
		{
			name:     "vpn ip assigned",
			message:  "worker[j.doe]: 203.0.113.53 sending IPv4 10.0.9.156",
			wantType: EventVPNIPAssigned,
			check: func(e *Event) bool {
				return e.Username == "j.doe" &&
					e.VpnIP == "10.0.9.156"
			},
		},
		{
			name:     "unknown message",
			message:  "worker[j.doe]: 203.0.113.53 configured link MTU is 1420",
			wantType: EventUnknown,
			check:    func(e *Event) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := p.Parse(ts, tt.message, "ocserv")
			if event.Type != tt.wantType {
				t.Errorf("got type %v, want %v", event.Type, tt.wantType)
			}
			if event.Server != "ocserv" {
				t.Errorf("got server %v, want ocserv", event.Server)
			}
			if !tt.check(event) {
				t.Errorf("check failed for event: %+v", event)
			}
		})
	}
}
