package collector

import (
	"testing"
	"time"

	"github.com/mogilevich/ocserv_exporter/internal/parser"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// val reads a counter or gauge value without pulling in prometheus/testutil.
func val(m prometheus.Metric) float64 {
	var d dto.Metric
	if err := m.Write(&d); err != nil {
		return -1
	}
	if d.Counter != nil {
		return d.Counter.GetValue()
	}
	return d.Gauge.GetValue()
}

func login(c *Collector, ts time.Time, server, user, ip string, port int) {
	c.ProcessEvent(&parser.Event{Type: parser.EventUserLogin, Timestamp: ts, Server: server, Username: user, ClientIP: ip, Port: port})
}

func disconnect(c *Collector, ts time.Time, server, user, ip string, port int) {
	c.ProcessEvent(&parser.Event{Type: parser.EventUserDisconnect, Timestamp: ts, Server: server, Username: user,
		ClientIP: ip, Port: port, Reason: "user disconnected", RxBytes: 100, TxBytes: 200})
}

// Journal replay on startup must rebuild sessions without counting the replayed
// history again; otherwise every exporter restart adds a day of traffic.
func TestReplayedEventsRebuildStateButDoNotCount(t *testing.T) {
	const srv, user, ip = "replay-test", "j.doe", "203.0.113.7"
	c := New()
	start := time.Now()
	c.SetCountFrom(start)
	old := start.Add(-2 * time.Hour)

	login(c, old, srv, user, ip, 1001)
	disconnect(c, old.Add(time.Minute), srv, user, ip, 1001)
	login(c, old.Add(2*time.Minute), srv, user, ip, 1002) // still open at startup
	c.ProcessEvent(&parser.Event{Type: parser.EventAuthFailed, Timestamp: old, Server: srv, Username: user, ClientIP: ip})

	for name, got := range map[string]float64{
		"connections":   val(ConnectionsTotal.WithLabelValues(srv, user, ip)),
		"disconnects":   val(DisconnectionsTotal.WithLabelValues(srv, user, "user disconnected")),
		"received":      val(ReceivedBytesTotal.WithLabelValues(srv, user)),
		"sent":          val(SentBytesTotal.WithLabelValues(srv, user)),
		"reconnects":    val(ReconnectsTotal.WithLabelValues(srv, user)),
		"auth failures": val(AuthFailedTotal.WithLabelValues(srv, user, ip, "Unknown", "")),
	} {
		if got != 0 {
			t.Errorf("replayed %s counted: %v", name, got)
		}
	}
	if got := val(ActiveSessions.WithLabelValues(srv, user)); got != 1 {
		t.Errorf("active sessions after replay = %v, want 1 (open session rebuilt)", got)
	}

	// After startup: the replayed session ends, the user reconnects at once.
	disconnect(c, start.Add(time.Second), srv, user, ip, 1002)
	login(c, start.Add(2*time.Second), srv, user, ip, 1003)

	for name, tc := range map[string]struct{ got, want float64 }{
		"connections": {val(ConnectionsTotal.WithLabelValues(srv, user, ip)), 1},
		"disconnects": {val(DisconnectionsTotal.WithLabelValues(srv, user, "user disconnected")), 1},
		"received":    {val(ReceivedBytesTotal.WithLabelValues(srv, user)), 100},
		"sent":        {val(SentBytesTotal.WithLabelValues(srv, user)), 200},
		"reconnects":  {val(ReconnectsTotal.WithLabelValues(srv, user)), 1},
		"active":      {val(ActiveSessions.WithLabelValues(srv, user)), 1},
	} {
		if tc.got != tc.want {
			t.Errorf("live %s = %v, want %v", name, tc.got, tc.want)
		}
	}
}

// Without SetCountFrom (e.g. --log.file replays) every event is counted.
func TestZeroCountFromCountsEverything(t *testing.T) {
	const srv, user, ip = "file-mode-test", "j.roe", "203.0.113.8"
	c := New()
	old := time.Now().Add(-48 * time.Hour)
	login(c, old, srv, user, ip, 2001)
	disconnect(c, old.Add(time.Minute), srv, user, ip, 2001)
	if got := val(ConnectionsTotal.WithLabelValues(srv, user, ip)); got != 1 {
		t.Errorf("connections = %v, want 1", got)
	}
	if got := val(SentBytesTotal.WithLabelValues(srv, user)); got != 200 {
		t.Errorf("sent = %v, want 200", got)
	}
}
