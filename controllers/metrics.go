package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ForensicCrashesTotal counts the total number of crashes detected by the controller
	ForensicCrashesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "forensics_crashes_total",
			Help: "Total number of crashes detected by the forensic controller",
		},
		[]string{"namespace", "reason"},
	)

	// ForensicPodsCreatedTotal counts the number of forensic pods successfully created
	ForensicPodsCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "forensics_pods_created_total",
			Help: "Total number of forensic pods created",
		},
		[]string{"source_namespace"},
	)

	// ForensicPodCreationErrorsTotal counts errors during forensic pod creation
	ForensicPodCreationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "forensics_pod_creation_errors_total",
			Help: "Total number of errors encountered while creating forensic pods",
		},
		[]string{"source_namespace", "step"},
	)
)

func init() {
	// Register custom metrics with the global controller-runtime registry
	metrics.Registry.MustRegister(
		ForensicCrashesTotal,
		ForensicPodsCreatedTotal,
		ForensicPodCreationErrorsTotal,
	)
}
