// internal/operator/metrics/metrics.go
//
// Prometheus metrics collectors for the self-healing operator.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type OperatorMetrics struct {
	registry *prometheus.Registry

	AlertsReceived       prometheus.Counter
	IncidentsCreated     prometheus.Counter
	DecisionsTotal       *prometheus.CounterVec
	PolicyMatchesTotal   *prometheus.CounterVec
	ActiveIncidents      prometheus.Gauge
	StateTransitions     *prometheus.CounterVec
	RecoveryPlansCreated *prometheus.CounterVec

	RollbacksTotal             *prometheus.CounterVec
	FailedRollbacksTotal       *prometheus.CounterVec
	VerificationsTotal         *prometheus.CounterVec
	VerificationFailuresTotal  *prometheus.CounterVec
	RecoveryDurationSeconds    *prometheus.HistogramVec
	LastSuccessfulRollbackTime *prometheus.GaugeVec
	RetryTotal                 *prometheus.CounterVec
	ActiveRecoveries           *prometheus.GaugeVec

	ErrorBudgetRemaining   *prometheus.GaugeVec
	BurnRate               *prometheus.GaugeVec
	SLOViolationsTotal     *prometheus.CounterVec
	RootCauseTotal         *prometheus.CounterVec
	DependencyChecksTotal  prometheus.Counter
	PrometheusQueriesTotal prometheus.Counter
	DecisionConfidence     prometheus.Gauge

	RolloutsTotal              prometheus.Counter
	RolloutDurationSeconds     *prometheus.HistogramVec
	RolloutFailuresTotal       prometheus.Counter
	CanaryAnalysisTotal        prometheus.Counter
	CanarySuccessTotal         prometheus.Counter
	CanaryFailuresTotal        prometheus.Counter
	PromotionsTotal            prometheus.Counter
	AbortsTotal                prometheus.Counter
	DeploymentRiskScore        prometheus.Gauge
	DeploymentGuardBlocksTotal prometheus.Counter
}

func New() *OperatorMetrics {
	registry := prometheus.NewRegistry()

	m := &OperatorMetrics{
		registry: registry,

		AlertsReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_alerts_received_total",
			Help: "Total number of webhook alerts received",
		}),

		IncidentsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_incidents_created_total",
			Help: "Total number of new incidents created",
		}),

		DecisionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_decisions_total",
				Help: "Total decisions made by the decision engine",
			},
			[]string{"service", "decision", "policy"},
		),

		PolicyMatchesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_policy_matches_total",
				Help: "Total times alerts matched a configured policy",
			},
			[]string{"service", "policy_id"},
		),

		ActiveIncidents: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "operator_active_incidents",
			Help: "Current count of firing or warning active incidents",
		}),

		StateTransitions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_state_transitions_total",
				Help: "Total incident state machine transitions",
			},
			[]string{"from_state", "to_state"},
		),

		RecoveryPlansCreated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_recovery_plans_created_total",
				Help: "Total recovery plans successfully generated",
			},
			[]string{"service", "action"},
		),

		RollbacksTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_rollbacks_total",
				Help: "Total number of Helm rollbacks executed",
			},
			[]string{"service", "namespace"},
		),

		FailedRollbacksTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_failed_rollbacks_total",
				Help: "Total number of failed Helm rollbacks",
			},
			[]string{"service", "namespace"},
		),

		VerificationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_verifications_total",
				Help: "Total number of recovery verifications performed",
			},
			[]string{"service", "result"},
		),

		VerificationFailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_verification_failures_total",
				Help: "Total number of verification failures",
			},
			[]string{"service"},
		),

		RecoveryDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "operator_recovery_duration_seconds",
				Help:    "Duration of recovery sequences from start to finish",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service"},
		),

		LastSuccessfulRollbackTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "operator_last_successful_rollback",
				Help: "Timestamp of the last successful rollback execution",
			},
			[]string{"service"},
		),

		RetryTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_retry_total",
				Help: "Total recovery retry attempts",
			},
			[]string{"service", "reason"},
		),

		ActiveRecoveries: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "operator_active_recoveries",
				Help: "Number of active recovery operations currently running",
			},
			[]string{"service"},
		),

		ErrorBudgetRemaining: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "operator_error_budget_remaining",
				Help: "Error budget remaining percentage by service and time window",
			},
			[]string{"service", "window"},
		),

		BurnRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "operator_burn_rate",
				Help: "Calculated error budget burn rate",
			},
			[]string{"service", "window"},
		),

		SLOViolationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_slo_violations_total",
				Help: "Total count of SLO violations detected",
			},
			[]string{"service"},
		),

		RootCauseTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "operator_root_cause_total",
				Help: "Total root cause decisions made by the analyzer",
			},
			[]string{"alerted_service", "root_cause_service"},
		),

		DependencyChecksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_dependency_checks_total",
			Help: "Total dependency graph analysis checks performed",
		}),

		PrometheusQueriesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_prometheus_queries_total",
			Help: "Total queries executed against Prometheus HTTP API",
		}),

		DecisionConfidence: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "operator_decision_confidence",
			Help: "Confidence score of the SRE decision engine output",
		}),

		RolloutsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_rollouts_total",
			Help: "Total number of progressive delivery rollouts started",
		}),

		RolloutDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "operator_rollout_duration_seconds",
				Help:    "Duration of progressive delivery rollouts",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service"},
		),

		RolloutFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_rollout_failures_total",
			Help: "Total progressive delivery rollout failures",
		}),

		CanaryAnalysisTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_canary_analysis_total",
			Help: "Total progressive delivery canary analysis executions",
		}),

		CanarySuccessTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_canary_success_total",
			Help: "Total successful progressive delivery canary analysis executions",
		}),

		CanaryFailuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_canary_failures_total",
			Help: "Total failed progressive delivery canary analysis executions",
		}),

		PromotionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_promotions_total",
			Help: "Total number of rollouts successfully promoted to stable",
		}),

		AbortsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_aborts_total",
			Help: "Total number of rollouts aborted",
		}),

		DeploymentRiskScore: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "operator_deployment_risk_score",
			Help: "Risk score of the latest deployment evaluation",
		}),

		DeploymentGuardBlocksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "operator_deployment_guard_blocks_total",
			Help: "Total number of deployments blocked by the guard policy",
		}),
	}

	registry.MustRegister(
		m.AlertsReceived,
		m.IncidentsCreated,
		m.DecisionsTotal,
		m.PolicyMatchesTotal,
		m.ActiveIncidents,
		m.StateTransitions,
		m.RecoveryPlansCreated,
		m.RollbacksTotal,
		m.FailedRollbacksTotal,
		m.VerificationsTotal,
		m.VerificationFailuresTotal,
		m.RecoveryDurationSeconds,
		m.LastSuccessfulRollbackTime,
		m.RetryTotal,
		m.ActiveRecoveries,
		m.ErrorBudgetRemaining,
		m.BurnRate,
		m.SLOViolationsTotal,
		m.RootCauseTotal,
		m.DependencyChecksTotal,
		m.PrometheusQueriesTotal,
		m.DecisionConfidence,
		m.RolloutsTotal,
		m.RolloutDurationSeconds,
		m.RolloutFailuresTotal,
		m.CanaryAnalysisTotal,
		m.CanarySuccessTotal,
		m.CanaryFailuresTotal,
		m.PromotionsTotal,
		m.AbortsTotal,
		m.DeploymentRiskScore,
		m.DeploymentGuardBlocksTotal,
	)

	return m
}

func (m *OperatorMetrics) Registry() *prometheus.Registry {
	return m.registry
}
