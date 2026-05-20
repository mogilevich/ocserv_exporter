package occtl

import (
	"math"
	"testing"
)

// vpn1Status is verbatim output of `sudo occtl show status` from a production
// vpn1 host (ocserv 1.3.0), captured 2026-05-20. Used as a regression fixture
// for parseStatus / parseDuration / parseLatency.
const vpn1Status = `Note: the printed statistics are not real-time; session time
as well as RX and TX data are updated on user disconnect

General info:
    Status: online
    Server PID: 3382332
    Sec-mod PID: 3382334
    Sec-mod instance count: 2
    Up since: 2026-04-27 17:34 (22days)
    Active sessions: 50
    Total sessions: 15760
    Total authentication failures: 1308
    IPs in ban list: 0
    Median latency: <1ms
    STDEV latency: <1ms

Current stats period:
    Last stats reset: 2026-05-18 17:37 (41h:06m)
    Sessions handled: 2570
    Timed out sessions: 0
    Timed out (idle) sessions: 17
    Closed due to error sessions: 2196
    Authentication failures: 131
    Average auth time:     0s
    Max auth time:    39s
    Average session time:  2m:00s
    Max session time:  7days
    Min MTU: 576
    Max MTU: 1354
    RX: 20.4 GB
    TX: 31.0 GB
`

func TestParseStatus_VPN1(t *testing.T) {
	s, err := parseStatus(vpn1Status)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}

	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"ActiveSessions", float64(s.ActiveSessions), 50},
		{"TotalSessions", float64(s.TotalSessions), 15760},
		{"AuthFailures", float64(s.AuthFailures), 1308},
		{"RxBytes", float64(s.RxBytes), 20.4 * 1024 * 1024 * 1024},
		{"TxBytes", float64(s.TxBytes), 31.0 * 1024 * 1024 * 1024},
		{"LatencyMedianMs", s.LatencyMedianMs, 1},
		{"LatencyStdevMs", s.LatencyStdevMs, 1},
		{"AvgSessionTimeSec", s.AvgSessionTimeSec, 120},
		{"MaxSessionTimeSec", s.MaxSessionTimeSec, 7 * 86400},
		{"UptimeSeconds", s.UptimeSeconds, 22 * 86400},
	}
	for _, c := range checks {
		// Bytes use float conversion of float64(int64), so allow tiny rounding.
		if math.Abs(c.got-c.want) > 1 {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"22days", 22 * 86400},
		{"7days", 7 * 86400},
		{"1day", 86400},
		{"1 day", 86400},
		{"41h:06m", 41*3600 + 6*60},
		{"3h:54m", 3*3600 + 54*60},
		{"18m:00s", 18 * 60},
		{"2m:00s", 120},
		{"58s", 58},
		{"39s", 39},
		{"0s", 0},
		{"", 0},
		{"1w", 7 * 86400},
		{"1 week", 7 * 86400},
		{"2 weeks", 14 * 86400},
		// Defensive: combined day+sub-units, comma-separated.
		{"1d:3h:45m", 86400 + 3*3600 + 45*60},
		{"1 day, 14h", 86400 + 14*3600},
	}
	for _, tt := range tests {
		got := parseDuration(tt.in)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseLatency(t *testing.T) {
	tests := []struct {
		in   string
		want float64 // ms
	}{
		{"<1ms", 1},
		{"1ms", 1},
		{"1.5ms", 1.5},
		{"<0.5ms", 0.5},
		{"500us", 0.5},
		{"500μs", 0.5},
		{"2s", 2000},
		{"42", 42}, // bare number → default ms
		{"", 0},
	}
	for _, tt := range tests {
		got := parseLatency(tt.in)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("parseLatency(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
