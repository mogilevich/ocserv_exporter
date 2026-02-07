package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "ocserv"

var (
	// ActiveSessions tracks current active sessions per user
	ActiveSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "active_sessions",
			Help:      "Number of currently active VPN sessions",
		},
		[]string{"server", "username"},
	)

	// ConnectionsTotal counts total connections
	ConnectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of VPN connections",
		},
		[]string{"server", "username", "client_ip"},
	)

	// DisconnectionsTotal counts disconnections by reason
	DisconnectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "disconnections_total",
			Help:      "Total number of VPN disconnections",
		},
		[]string{"server", "username", "reason"},
	)

	// ReceivedBytesTotal tracks total received bytes per user
	ReceivedBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "received_bytes_total",
			Help:      "Total bytes received from VPN clients",
		},
		[]string{"server", "username"},
	)

	// SentBytesTotal tracks total sent bytes per user
	SentBytesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sent_bytes_total",
			Help:      "Total bytes sent to VPN clients",
		},
		[]string{"server", "username"},
	)

	// SessionDuration tracks session duration distribution
	SessionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "session_duration_seconds",
			Help:      "VPN session duration in seconds",
			Buckets:   []float64{60, 300, 900, 1800, 3600, 7200, 14400, 28800, 43200, 86400},
		},
		[]string{"server", "username"},
	)

	// Info provides exporter info
	Info = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "exporter_info",
			Help:      "Exporter information",
		},
		[]string{"version"},
	)

	// LastEventTimestamp tracks when the last log event was processed
	LastEventTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "last_event_timestamp_seconds",
			Help:      "Unix timestamp of the last processed log event",
		},
	)

	// ReconnectsTotal tracks rapid reconnections (login within 5 min of disconnect)
	ReconnectsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reconnects_total",
			Help:      "Total number of rapid reconnections (login within 5 minutes of disconnect)",
		},
		[]string{"server", "username"},
	)

	// ProblematicSessionsTotal tracks sessions that ended with error and lasted < 60s
	ProblematicSessionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "problematic_sessions_total",
			Help:      "Total number of problematic sessions (duration < 60s with error)",
		},
		[]string{"server", "username", "reason"},
	)

	// ConnectionsByCountry tracks connections by country (GeoIP)
	ConnectionsByCountry = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_by_country_total",
			Help:      "Total connections by country",
		},
		[]string{"server", "username", "country", "country_code"},
	)

	// AuthFailedTotal tracks failed authentication attempts
	AuthFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_failed_total",
			Help:      "Total number of failed authentication attempts",
		},
		[]string{"server", "username", "client_ip", "country", "country_code"},
	)

	// SessionInfo provides detailed info about each active session
	// Value is session start timestamp (unix), labels provide session details
	SessionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "session_info",
			Help:      "Information about active sessions (value is session start timestamp)",
		},
		[]string{"server", "username", "vpn_ip", "country", "client_type"},
	)

	// Server-level metrics from occtl

	// ServerRxBytesTotal tracks total received bytes at server level (from occtl)
	ServerRxBytesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_rx_bytes_total",
			Help:      "Total bytes received by server (from occtl show status)",
		},
		[]string{"server"},
	)

	// ServerTxBytesTotal tracks total sent bytes at server level (from occtl)
	ServerTxBytesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_tx_bytes_total",
			Help:      "Total bytes sent by server (from occtl show status)",
		},
		[]string{"server"},
	)

	// ServerActiveSessions tracks active sessions from occtl (more accurate than journal-based)
	ServerActiveSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_active_sessions",
			Help:      "Number of active sessions reported by occtl",
		},
		[]string{"server"},
	)

	// ServerTotalSessions tracks total sessions since last stats reset
	ServerTotalSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_total_sessions",
			Help:      "Total sessions since stats reset (from occtl show status)",
		},
		[]string{"server"},
	)

	// ServerLatencyMedian tracks median latency
	ServerLatencyMedian = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_latency_median_seconds",
			Help:      "Median server latency in seconds",
		},
		[]string{"server"},
	)

	// ServerLatencyStdev tracks latency standard deviation
	ServerLatencyStdev = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_latency_stdev_seconds",
			Help:      "Server latency standard deviation in seconds",
		},
		[]string{"server"},
	)

	// ServerUptime tracks server uptime
	ServerUptime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_uptime_seconds",
			Help:      "Server uptime in seconds",
		},
		[]string{"server"},
	)

	// ServerAvgSessionTime tracks average session time
	ServerAvgSessionTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "server_avg_session_time_seconds",
			Help:      "Average session time in seconds",
		},
		[]string{"server"},
	)

	// SessionsByClientType tracks sessions by VPN client type
	SessionsByClientType = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "sessions_by_client_type",
			Help:      "Current sessions by VPN client type (user agent)",
		},
		[]string{"server", "client_type"},
	)

	// UserConcurrentSessions tracks current concurrent sessions per user (from occtl)
	UserConcurrentSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "user_concurrent_sessions",
			Help:      "Current number of concurrent sessions per user (from occtl)",
		},
		[]string{"server", "username"},
	)
)

// RegisterMetrics registers all metrics with the provided registry
func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		ActiveSessions,
		ConnectionsTotal,
		DisconnectionsTotal,
		ReceivedBytesTotal,
		SentBytesTotal,
		SessionDuration,
		Info,
		LastEventTimestamp,
		ReconnectsTotal,
		ProblematicSessionsTotal,
		ConnectionsByCountry,
		AuthFailedTotal,
		SessionInfo,
	)
}

// RegisterOcctlMetrics registers occtl-specific metrics
func RegisterOcctlMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		ServerRxBytesTotal,
		ServerTxBytesTotal,
		ServerActiveSessions,
		ServerTotalSessions,
		ServerLatencyMedian,
		ServerLatencyStdev,
		ServerUptime,
		ServerAvgSessionTime,
		SessionsByClientType,
		UserConcurrentSessions,
	)
}
