package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds every Prometheus metric named in the product spec (§31), so
// call sites reference a field instead of re-registering ad hoc collectors.
type Metrics struct {
	ClusterProvisionDuration prometheus.Histogram
	ClusterUpgradeDuration   prometheus.Histogram
	MachineUpgradeDuration   prometheus.Histogram
	WorkflowDuration         *prometheus.HistogramVec
	WorkflowFailures         *prometheus.CounterVec
	TalosAPILatency          *prometheus.HistogramVec
	KubernetesAPILatency     *prometheus.HistogramVec
	GitHubAPILatency         *prometheus.HistogramVec
	GitOpsSyncDuration       prometheus.Histogram
	ClusterHealth            *prometheus.GaugeVec
	MachineHealth            *prometheus.GaugeVec

	registry *prometheus.Registry
}

// NewMetrics constructs and registers every platform metric on a fresh
// registry (kept isolated from the default global registry so tests can
// build multiple independent Metrics instances).
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	f := promauto.With(reg)

	return &Metrics{
		registry: reg,
		ClusterProvisionDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "cluster_provision_duration_seconds",
			Help:    "Duration of full cluster provisioning workflows.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}),
		ClusterUpgradeDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "cluster_upgrade_duration_seconds",
			Help:    "Duration of cluster upgrade workflows.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}),
		MachineUpgradeDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "machine_upgrade_duration_seconds",
			Help:    "Duration of single-machine upgrade operations.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		}),
		WorkflowDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "workflow_duration_seconds",
			Help:    "Duration of workflow runs by type.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}, []string{"workflow_type"}),
		WorkflowFailures: f.NewCounterVec(prometheus.CounterOpts{
			Name: "workflow_failures_total",
			Help: "Count of failed workflow runs by type.",
		}, []string{"workflow_type"}),
		TalosAPILatency: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "talos_api_latency_seconds",
			Help: "Latency of calls through the TalosClient port.",
		}, []string{"operation"}),
		KubernetesAPILatency: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "kubernetes_api_latency_seconds",
			Help: "Latency of calls to the Kubernetes API.",
		}, []string{"operation"}),
		GitHubAPILatency: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "github_api_latency_seconds",
			Help: "Latency of calls through the GitProvider port.",
		}, []string{"operation"}),
		GitOpsSyncDuration: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "gitops_sync_duration_seconds",
			Help:    "Time from commit merge to observed Argo CD sync.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		}),
		ClusterHealth: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cluster_health",
			Help: "Observed cluster health (1=healthy, 0=unhealthy) by cluster.",
		}, []string{"cluster_id", "cluster_name"}),
		MachineHealth: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "machine_health",
			Help: "Observed machine health (1=healthy, 0=unhealthy) by machine.",
		}, []string{"machine_id", "hostname"}),
	}
}

// Handler exposes the registry in the Prometheus exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
