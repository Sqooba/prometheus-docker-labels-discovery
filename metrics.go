package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

var (
	counterContainerCheck          prometheus.Counter
	counterContainerCheckFailure   prometheus.Counter
	counterContainerRestart        prometheus.Counter
	counterContainerRestartFailure prometheus.Counter
)

func initPrometheus(env envConfig, mux *http.ServeMux) {

	counterContainerCheck = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "check_count",
		Help:      "Count how many container checks has been run",
		Namespace: env.MetricsNamespace,
		Subsystem: env.MetricsSubsystem,
	})
	prometheus.MustRegister(counterContainerCheck)

	counterContainerCheckFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "check_failure_count",
		Help:      "Count how many container check failures happened",
		Namespace: env.MetricsNamespace,
		Subsystem: env.MetricsSubsystem,
	})
	prometheus.MustRegister(counterContainerCheckFailure)

	counterContainerRestart = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "restart_count",
		Help:      "Count how many container restarts has been run",
		Namespace: env.MetricsNamespace,
		Subsystem: env.MetricsSubsystem,
	})
	prometheus.MustRegister(counterContainerRestart)

	counterContainerRestartFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      "restart_failure_count",
		Help:      "Count how many container restart failures happened",
		Namespace: env.MetricsNamespace,
		Subsystem: env.MetricsSubsystem,
	})
	prometheus.MustRegister(counterContainerRestartFailure)

	mux.Handle(env.MetricsPath, promhttp.Handler())
}
