// internal/operator/policy/engine.go
//
// Policy Engine to match incoming alerts against configured recovery policies.

package policy

import (
	"context"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type Engine struct {
	policies []models.Policy
	m        *metrics.OperatorMetrics
}

func NewEngine(policies []models.Policy, m *metrics.OperatorMetrics) *Engine {
	return &Engine{
		policies: policies,
		m:        m,
	}
}

// Match searches configured policies for a match based on service name and alert name.
func (e *Engine) Match(ctx context.Context, alert *models.Alert) (*models.Policy, bool) {
	for _, p := range e.policies {
		if p.Service == alert.Service && p.AlertName == alert.Name {
			// Increment policy match metrics
			e.m.PolicyMatchesTotal.WithLabelValues(alert.Service, p.ID).Inc()
			return &p, true
		}
	}
	return nil, false
}
