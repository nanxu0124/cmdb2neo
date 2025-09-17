package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	SyncDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "cmdb_sync_duration_seconds",
		Help:    "单次同步耗时",
		Buckets: prometheus.DefBuckets,
	})

	SyncErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cmdb_sync_errors_total",
		Help: "同步失败次数",
	})
)

// MustRegister 注册指标，可在 main 中调用。
func MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(SyncDuration, SyncErrors)
}
