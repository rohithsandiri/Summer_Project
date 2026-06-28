// cmd/operator/main.go
//
// Self-Healing Control Plane Operator Main Entrypoint.
// Initializes the alert parsing, SRE engines (SLO, Budget, Burn Rate, Graph, Root Cause),
// and Helm rollback execution pipeline.

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rohithsandiri/Summer_Project/internal/operator/alert"
	"github.com/rohithsandiri/Summer_Project/internal/operator/argo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/config"
	"github.com/rohithsandiri/Summer_Project/internal/operator/decision"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/executor"
	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/incident"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/planner"
	"github.com/rohithsandiri/Summer_Project/internal/operator/policy"
	"github.com/rohithsandiri/Summer_Project/internal/operator/progressive"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/reliability"
	"github.com/rohithsandiri/Summer_Project/internal/operator/retry"
	"github.com/rohithsandiri/Summer_Project/internal/operator/rootcause"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/state"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
	"github.com/rohithsandiri/Summer_Project/internal/operator/utils"
	"github.com/rohithsandiri/Summer_Project/internal/operator/verification"
	"github.com/rohithsandiri/Summer_Project/internal/operator/webhook"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		panic("Failed to load operator config: " + err.Error())
	}

	// 2. Initialize structured logger
	log := logger.New(cfg.OperatorVersion)
	ctx := context.Background()
	log.Info(ctx, "starting self-healing operator control & execution plane", logger.Fields{}, "port", cfg.Port)

	// 3. Initialize metrics registry
	m := metrics.New()

	// 4. Initialize storage (PostgreSQL with In-Memory fallback)
	var dbConn *sql.DB
	var incidentStore interfaces.IncidentStore = storage.NewInMemoryIncidentStore()
	var historyStore interfaces.DecisionHistoryStore = storage.NewInMemoryDecisionHistoryStore()
	var rollbackStore interfaces.RollbackHistoryStore = storage.NewInMemoryRollbackHistoryStore()

	dbConn, err = storage.ConnectPostgres()
	if err == nil && dbConn != nil {
		pStore, pErr := storage.NewPostgresIncidentStore(dbConn)
		if pErr == nil {
			incidentStore = pStore
			historyStore = storage.NewPostgresDecisionHistoryStore(dbConn)
			rollbackStore = storage.NewPostgresRollbackHistoryStore(dbConn)
			log.Info(ctx, "successfully initialized postgresql persistent stores", logger.Fields{})
		} else {
			log.Warn(ctx, "failed to initialize postgresql store, falling back to in-memory", logger.Fields{Reason: pErr.Error()})
		}
	} else {
		log.Info(ctx, "postgresql config not present or connection failed, falling back to in-memory stores", logger.Fields{})
	}

	// Initialize SRE Timeline Engine
	var timeline reliability.TimelineEngine
	if dbConn != nil {
		timeline = reliability.NewSQLTimelineEngine(dbConn)
	} else {
		timeline = reliability.NewInMemoryTimelineEngine()
	}

	// Initialize Reliability Engine
	reliabilityEngine := reliability.NewReliabilityEngine(incidentStore, rollbackStore, timeline, dbConn)

	// 5. Initialize Prometheus Query Engine
	var promClient prometheus.PrometheusClient
	if cfg.PrometheusAddr == "mock" {
		promClient = prometheus.NewMockClient()
	} else {
		promClient, err = prometheus.NewClient(cfg.PrometheusAddr, m)
		if err != nil {
			log.Warn(ctx, "failed to initialize prometheus API client, falling back to mock", logger.Fields{Reason: err.Error()})
			promClient = prometheus.NewMockClient()
		}
	}

	// 6. Initialize SRE Data Engines
	depGraph := dependency.NewGraph()
	sloEngine := slo.NewEngine(cfg.SLOs, promClient, m)
	budgetMgr := budget.NewManager(cfg.SLOs, promClient, m)
	burnRateEngine := burnrate.NewEngine(cfg.SLOs, promClient, m)
	rootcauseAnal := rootcause.NewAnalyzer(depGraph, sloEngine, promClient, m)

	// 7. Initialize Kubernetes Client-go
	var clientset kubernetes.Interface
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to local kubeconfig
		home, homeErr := os.UserHomeDir()
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" && homeErr == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		if kubeconfig != "" {
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}

	if err == nil && k8sConfig != nil {
		clientset, err = kubernetes.NewForConfig(k8sConfig)
	}

	if err != nil || clientset == nil {
		log.Warn(ctx, "failed to initialize k8s clientset, using mock clientset for local fallback", logger.Fields{Reason: err.Error()})
		clientset = fake.NewSimpleClientset()
	}

	// 8. Initialize Helm Manager and Execution Engine
	helmClient := helm.NewClient()
	stateMachine := state.NewStateMachine(log, m, timeline)

	executorInstance := executor.NewHelmRollbackExecutor(helmClient, stateMachine, m, log)
	verifierInstance := verification.NewK8sVerificationEngine(clientset, m, log)
	retryEngineInstance := retry.NewRetryEngine(m, log)

	// 9. Initialize decision & planning engines
	policyEngine := policy.NewEngine(cfg.Policies, m)
	cooldownManager := utils.NewCooldownManager()
	decisionEngine := decision.NewEngine(
		cfg.OperatorVersion,
		m,
		sloEngine,
		budgetMgr,
		burnRateEngine,
		rootcauseAnal,
		rollbackStore,
	)
	recoveryPlanner := planner.NewPlanner(m)

	// 10. Initialize Incident Manager Orchestrator
	incidentManager := incident.NewManager(
		incidentStore,
		historyStore,
		policyEngine,
		decisionEngine,
		recoveryPlanner,
		stateMachine,
		cooldownManager,
		log,
		m,
		executorInstance,
		verifierInstance,
		retryEngineInstance,
		rollbackStore,
		timeline,
		reliabilityEngine,
	)

	// 10b. Start Leader Election if enabled
	if cfg.LeaderElectionEnabled {
		incidentManager.SetLeader(false) // Start as standby
		log.Info(ctx, "leader election enabled, starting standby mode", logger.Fields{}, "pod", cfg.LeaderElectionID, "namespace", cfg.LeaderElectionNamespace)

		// Create the lease lock resource
		lock := &resourcelock.LeaseLock{
			LeaseMeta: metav1.ObjectMeta{
				Name:      "self-healing-operator-lock",
				Namespace: cfg.LeaderElectionNamespace,
			},
			Client: clientset.CoordinationV1(),
			LockConfig: resourcelock.ResourceLockConfig{
				Identity: cfg.LeaderElectionID,
			},
		}

		// Run leader election in a background goroutine
		go func() {
			leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
				Lock:          lock,
				LeaseDuration: 15 * time.Second,
				RenewDeadline: 10 * time.Second,
				RetryPeriod:   2 * time.Second,
				Callbacks: leaderelection.LeaderCallbacks{
					OnStartedLeading: func(ctx context.Context) {
						log.Info(ctx, "acquired leadership, transitioning to active leader", logger.Fields{})
						incidentManager.SetLeader(true)
					},
					OnStoppedLeading: func() {
						log.Info(ctx, "lost leadership, transitioning to standby mode", logger.Fields{})
						incidentManager.SetLeader(false)
					},
					OnNewLeader: func(identity string) {
						if identity == cfg.LeaderElectionID {
							return
						}
						log.Info(ctx, "new leader elected", logger.Fields{}, "leader_identity", identity)
					},
				},
			})
		}()
	} else {
		log.Info(ctx, "leader election disabled, running as active leader by default", logger.Fields{})
		incidentManager.SetLeader(true)
	}

	// 11. Initialize Webhook Handler & Parser
	alertParser := alert.NewParser()
	webhookHandler := webhook.NewHandler(alertParser, incidentManager, log)

	// 11b. Initialize Progressive Delivery Manager & Sub-Engines
	var argoClient progressive.RolloutManager
	if k8sConfig != nil {
		ac, err := argo.NewArgoClient(k8sConfig)
		if err == nil {
			argoClient = ac
		}
	}
	if argoClient == nil {
		log.Warn(ctx, "failed to initialize K8s Argo Rollout client, using mock for local fallback", logger.Fields{})
		argoClient = argo.NewMockArgoClient()
	}

	guardConfig := models.DeploymentGuardConfig{
		MaxAllowedBurnRate:       14.4,
		MinRemainingBudget:       10.0,
		BlockOnCriticalIncidents: true,
		MinDecisionConfidence:    0.8,
	}

	analysisEngine := progressive.NewAnalysisEngine(promClient, sloEngine, budgetMgr, burnRateEngine)
	riskEngine := progressive.NewRiskEngine(depGraph, budgetMgr, burnRateEngine, rollbackStore)
	guardEngine := progressive.NewGuard(guardConfig, incidentStore, depGraph, sloEngine, budgetMgr, burnRateEngine)
	releaseVerifier := progressive.NewReleaseVerificationEngine(clientset, sloEngine, log)

	deliveryManager := progressive.NewDeliveryManager(
		argoClient,
		guardEngine,
		riskEngine,
		analysisEngine,
		releaseVerifier,
		executorInstance,
		log,
		m,
	)

	progressiveHandler := progressive.NewProgressiveHandler(deliveryManager, riskEngine)
	reliabilityHandler := reliability.NewReliabilityHandler(incidentStore, timeline, reliabilityEngine, log)

	// 12. Register handlers
	mux := http.NewServeMux()
	mux.Handle("/webhook", webhookHandler)
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	progressiveHandler.RegisterRoutes(mux)
	reliabilityHandler.RegisterRoutes(mux)

	// Ready/Live check endpoints
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 13. Graceful shutdown execution block
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info(ctx, "http server listening for webhook requests", logger.Fields{}, "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(ctx, "failed to start http server", logger.Fields{Reason: err.Error()})
		}
	}()

	sig := <-shutdownChan
	log.Info(ctx, "shutdown signal received, initiating graceful shutdown", logger.Fields{}, "signal", sig.String())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal(ctx, "graceful shutdown failed", logger.Fields{Reason: err.Error()})
	}

	log.Info(ctx, "self-healing operator stopped successfully", logger.Fields{})
}
