package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

var (
	// BridgeTransactionsTotal counts bridge transactions by chain and status
	BridgeTransactionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bridge_transactions_total",
			Help: "Total number of bridge transactions",
		},
		[]string{"source_chain", "dest_chain", "status"},
	)

	// BridgeTransactionsPending tracks pending transactions
	BridgeTransactionsPending = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bridge_transactions_pending",
			Help: "Number of pending bridge transactions",
		},
	)

	// BridgeTransactionsCompleted tracks completed transactions
	BridgeTransactionsCompleted = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bridge_transactions_completed",
			Help: "Number of completed bridge transactions",
		},
	)

	// LastProcessedBlock tracks last processed block per chain
	LastProcessedBlock = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bridge_last_processed_block",
			Help: "Last processed block number per chain",
		},
		[]string{"chain_id"},
	)

	// OldestPendingTimestamp tracks the unix timestamp of the oldest pending transaction
	OldestPendingTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "bridge_oldest_pending_timestamp",
			Help: "Unix timestamp of the oldest pending bridge transaction (0 if none)",
		},
	)

	registerOnce sync.Once

	// pendingTxTimes tracks when each bridgeID entered pending state
	pendingMu      sync.Mutex
	pendingTxTimes = map[string]float64{}
)

// Register registers all Prometheus metrics
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(
			BridgeTransactionsTotal,
			BridgeTransactionsPending,
			BridgeTransactionsCompleted,
			LastProcessedBlock,
			OldestPendingTimestamp,
		)
	})
}

// StartServer starts the Prometheus metrics HTTP server
func StartServer(ctx context.Context, port int) error {
	Register()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	log.Info().Int("port", port).Msg("Starting metrics server")

	return server.ListenAndServe()
}

// TrackPending records a bridgeID as pending
func TrackPending(bridgeID string) {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	pendingTxTimes[bridgeID] = float64(time.Now().Unix())
	updateOldestPending()
}

// ResolvePending removes a bridgeID from pending tracking
func ResolvePending(bridgeID string) {
	pendingMu.Lock()
	defer pendingMu.Unlock()

	delete(pendingTxTimes, bridgeID)
	updateOldestPending()
}

// updateOldestPending sets the gauge to the oldest pending timestamp
func updateOldestPending() {
	if len(pendingTxTimes) == 0 {
		OldestPendingTimestamp.Set(0)
		return
	}

	var oldest float64
	for _, ts := range pendingTxTimes {
		if oldest == 0 || ts < oldest {
			oldest = ts
		}
	}

	OldestPendingTimestamp.Set(oldest)
}
