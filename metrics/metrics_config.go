package metrics

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	instance *Metrics
	once     sync.Once
)

type MetricsCounter struct {
	Name       string
	Help       string
	LabelNames []string
}

type Metrics struct {
	Counters   []MetricsCounter
	CounterMap map[string]*prometheus.CounterVec
	mu         sync.RWMutex // Add mutex for map operations
	serverOnce sync.Once    // Add this for server initialization
	server     *http.Server // Add server field
}

// InitMetrics initializes the singleton instance with the given counters
func InitMetrics(counters []MetricsCounter) *Metrics {
	once.Do(func() {
		instance = &Metrics{ // nolint:exhaustruct
			Counters:   counters,
			CounterMap: make(map[string]*prometheus.CounterVec),
		}
		instance.RegisterMetricsCounters()
	})
	return instance
}

// GetMetrics returns the existing metrics instance or panics if not initialized
func GetMetrics() *Metrics {
	if instance == nil {
		log.Fatal().Msg("Metrics not initialized. Call InitMetrics first")
	}
	return instance
}

func (metrics *Metrics) RegisterMetricsCounters() {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	for _, counter := range metrics.Counters {
		labelNames := counter.LabelNames
		if len(labelNames) == 0 {
			labelNames = []string{"address", "topic"}
		}

		counterVec := prometheus.NewCounterVec(
			prometheus.CounterOpts{ // nolint: exhaustruct
				Name: counter.Name,
				Help: counter.Help,
			},
			labelNames,
		)

		prometheus.MustRegister(counterVec)
		metrics.CounterMap[counter.Name] = counterVec
	}
}

// IncrementMetricsCounterWithLabels increments a counter with variable label values
func (metrics *Metrics) IncrementMetricsCounterWithLabels(counterName string, labelValues ...string) {
	metrics.mu.RLock()
	counter := metrics.CounterMap[counterName]
	defer metrics.mu.RUnlock()

	if counter != nil {
		counter.WithLabelValues(labelValues...).Inc()
		log.Debug().
			Str("counter", counterName).
			Strs("labels", labelValues).
			Msg("Incremented counter with labels")
	}
}

// IncrementMetricsCounter maintains backward compatibility
func (metrics *Metrics) IncrementMetricsCounter(counterName string, address string, topic uint64) {
	topicStr := strconv.FormatUint(topic, 10)
	metrics.IncrementMetricsCounterWithLabels(counterName, address, topicStr)
}

// Starts the metrics server, listening on the given port,
// controlling context cancellation and graceful shutdown
func (metrics *Metrics) StartMetricsServer(ctx context.Context, port string) {
	metrics.serverOnce.Do(func() {
		http.Handle("/metrics", promhttp.Handler())
		metrics.server = &http.Server{ // nolint:exhaustruct
			Addr:              port,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
		}

		// Start server in a goroutine
		go func() {
			log.Info().Msgf("Starting metrics server on %s", port)
			if err := metrics.server.ListenAndServe(); err != http.ErrServerClosed {
				log.Error().Err(err).Msg("Metrics server failed unexpectedly")
			}
			log.Info().Msg("Metrics server stopped")
		}()

		// Monitor non-essential context for shutdown
		go func() {
			<-ctx.Done()
			log.Info().Msg("Non-essential context cancelled, shutting down metrics server...")
			if err := metrics.Shutdown(ctx); err != nil {
				log.Error().Err(err).Msg("Error during metrics server shutdown")
			}
		}()
	})
}

// Shutdown gracefully shuts down the metrics server with timeout
func (metrics *Metrics) Shutdown(ctx context.Context) error {
	if metrics.server == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Info().Msg("Shutting down metrics server...")
	return metrics.server.Shutdown(shutdownCtx)
}
