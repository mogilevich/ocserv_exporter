package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mogilevich/ocserv_exporter/internal/collector"
	"github.com/mogilevich/ocserv_exporter/internal/geoip"
	"github.com/mogilevich/ocserv_exporter/internal/journal"
	"github.com/mogilevich/ocserv_exporter/internal/occtl"
)

var (
	version = "dev"
)

func main() {
	var (
		listenAddress = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").
				Default(":9617").String()
		metricsPath = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").
				Default("/metrics").String()
		journalUnits = kingpin.Flag("journal.unit", "Systemd unit name to read logs from (can be specified multiple times).").
				Default("ocserv").Strings()
		journalSince = kingpin.Flag("journal.since", "How far back to read logs on startup.").
				Default("1h").Duration()
		logFile = kingpin.Flag("log.file", "Read logs from file instead of journald (for testing).").
			String()
		geoipDB = kingpin.Flag("geoip.db", "Path to GeoLite2-Country.mmdb file for GeoIP lookups.").
			String()

		// occtl flags
		occtlEnabled = kingpin.Flag("occtl.enabled", "Enable occtl polling for additional metrics.").
				Default("false").Bool()
		occtlSockets = kingpin.Flag("occtl.socket", "occtl socket path in format 'name:path' or just 'name' for default socket (can be specified multiple times).").
				Strings()
		occtlInterval = kingpin.Flag("occtl.interval", "Interval between occtl polls.").
				Default("30s").Duration()
	)

	kingpin.Version(version)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	log.Printf("Starting ocserv_exporter %s", version)

	// Register metrics
	reg := prometheus.DefaultRegisterer
	collector.RegisterMetrics(reg)
	collector.Info.WithLabelValues(version).Set(1)

	// Create collector
	coll := collector.New()

	// Initialize GeoIP if database path provided
	if *geoipDB != "" {
		resolver, err := geoip.NewResolver(*geoipDB)
		if err != nil {
			log.Printf("Warning: Failed to load GeoIP database: %v", err)
		} else {
			coll.SetGeoIPResolver(resolver)
			log.Printf("GeoIP database loaded: %s", *geoipDB)
			defer resolver.Close()
		}
	}

	// Start log reader
	ctx, cancel := context.WithCancel(context.Background())

	// Start periodic cleanup goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				coll.CleanupOldDisconnects()
			}
		}
	}()

	// Initialize occtl polling if enabled
	if *occtlEnabled {
		collector.RegisterOcctlMetrics(reg)

		// Parse socket configurations
		var clients []*occtl.Client
		if len(*occtlSockets) == 0 {
			// Default: use "ocserv" with default socket
			clients = append(clients, occtl.NewClient("", "ocserv"))
		} else {
			for _, socketCfg := range *occtlSockets {
				// Format: "name:path" or just "name" for default socket
				parts := strings.SplitN(socketCfg, ":", 2)
				name := parts[0]
				socketPath := ""
				if len(parts) > 1 {
					socketPath = parts[1]
				}
				clients = append(clients, occtl.NewClient(socketPath, name))
			}
		}

		log.Printf("occtl polling enabled with %d server(s), interval: %s", len(clients), *occtlInterval)

		// Start occtl polling goroutine
		go func() {
			ticker := time.NewTicker(*occtlInterval)
			defer ticker.Stop()

			// Initial poll
			pollOcctl(clients, coll)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pollOcctl(clients, coll)
				}
			}
		}()
	}
	defer cancel()

	go func() {
		var reader journal.Reader
		var err error

		if *logFile != "" {
			reader, err = journal.NewFileReader(*logFile)
			if err != nil {
				log.Fatalf("Failed to open log file: %v", err)
			}
			log.Printf("Reading logs from file: %s", *logFile)
		} else {
			if runtime.GOOS != "linux" {
				log.Fatal("journald is only available on Linux. Use --log.file to read from a file instead.")
			}
			reader, err = journal.NewJournalReader(*journalUnits, *journalSince)
			if err != nil {
				log.Fatalf("Failed to open journal: %v", err)
			}
			log.Printf("Reading logs from journald units: %v (since %s)", *journalUnits, *journalSince)
		}
		defer reader.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			entry, err := reader.Read()
			if err != nil {
				log.Printf("Error reading log: %v", err)
				continue
			}
			if entry == nil {
				// EOF for file reader
				time.Sleep(100 * time.Millisecond)
				continue
			}

			coll.ProcessLogLine(entry.Timestamp, entry.Message, entry.Unit)
		}
	}()

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle(*metricsPath, promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><title>ocserv Exporter</title></head>
<body>
<h1>ocserv Exporter</h1>
<p><a href="` + *metricsPath + `">Metrics</a></p>
</body>
</html>`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    *listenAddress,
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Printf("Listening on %s", *listenAddress)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// pollOcctl fetches metrics from all occtl clients
func pollOcctl(clients []*occtl.Client, coll *collector.Collector) {
	// Collect all stats first, then update metrics atomically
	allUserAgentStats := make(map[string]map[string]int)
	allUserSessionCounts := make(map[string]map[string]int)
	allUsers := make(map[string][]occtl.User)
	allUserClientTypes := make(map[string]map[string]string)

	for _, client := range clients {
		serverName := client.ServerName()

		// Get server status
		status, err := client.GetStatus()
		if err != nil {
			log.Printf("Warning: Failed to get occtl status for %s: %v", serverName, err)
			continue
		}

		// Update server metrics
		collector.ServerRxBytesTotal.WithLabelValues(serverName).Set(float64(status.RxBytes))
		collector.ServerTxBytesTotal.WithLabelValues(serverName).Set(float64(status.TxBytes))
		collector.ServerActiveSessions.WithLabelValues(serverName).Set(float64(status.ActiveSessions))
		collector.ServerTotalSessions.WithLabelValues(serverName).Set(float64(status.TotalSessions))
		collector.ServerLatencyMedian.WithLabelValues(serverName).Set(status.LatencyMedianMs / 1000.0)
		collector.ServerLatencyStdev.WithLabelValues(serverName).Set(status.LatencyStdevMs / 1000.0)
		collector.ServerUptime.WithLabelValues(serverName).Set(status.UptimeSeconds)
		collector.ServerAvgSessionTime.WithLabelValues(serverName).Set(status.AvgSessionTimeSec)

		// Get user agent statistics
		userAgentStats, err := client.GetUserAgentStats()
		if err != nil {
			log.Printf("Warning: Failed to get occtl sessions for %s: %v", serverName, err)
			continue
		}
		allUserAgentStats[serverName] = userAgentStats

		// Get user session counts (for concurrent sessions detection)
		userSessionCounts, err := client.GetUserSessionCounts()
		if err != nil {
			log.Printf("Warning: Failed to get user session counts for %s: %v", serverName, err)
			continue
		}
		allUserSessionCounts[serverName] = userSessionCounts

		// Get users list for session info
		users, err := client.GetUsers()
		if err != nil {
			log.Printf("Warning: Failed to get users for %s: %v", serverName, err)
			continue
		}
		allUsers[serverName] = users

		// Get user client types for session info
		userClientTypes, err := client.GetUserClientTypes()
		if err != nil {
			log.Printf("Warning: Failed to get user client types for %s: %v", serverName, err)
			continue
		}
		allUserClientTypes[serverName] = userClientTypes
	}

	// Reset and update all client type metrics at once
	collector.SessionsByClientType.Reset()
	for serverName, stats := range allUserAgentStats {
		for clientType, count := range stats {
			collector.SessionsByClientType.WithLabelValues(serverName, clientType).Set(float64(count))
		}
	}

	// Reset and update user concurrent sessions metrics
	collector.UserConcurrentSessions.Reset()
	for serverName, counts := range allUserSessionCounts {
		for username, count := range counts {
			collector.UserConcurrentSessions.WithLabelValues(serverName, username).Set(float64(count))
		}
	}

	// Reset and update session info from occtl users (accurate real-time data)
	collector.SessionInfo.Reset()
	for serverName, users := range allUsers {
		clientTypes := allUserClientTypes[serverName]
		for _, user := range users {
			country := ""
			if coll != nil {
				country = coll.LookupCountry(user.ClientIP)
			}
			clientType := ""
			if clientTypes != nil {
				clientType = clientTypes[user.Username]
			}
			// Value is session start timestamp (now - since duration)
			startTime := time.Now().Add(-user.Since)
			collector.SessionInfo.WithLabelValues(serverName, user.Username, user.VpnIP, country, clientType).Set(float64(startTime.Unix()))
		}
	}
}
