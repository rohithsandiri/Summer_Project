// internal/operator/alert/parser.go
//
// Alert parser and validator for incoming Alertmanager webhook payloads.

package alert

import (
	"errors"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// Parse parses the raw Alertmanager webhook payload, validates it, and translates
// it into strongly typed internal models.Alert structs.
func (p *Parser) Parse(payload *models.AlertmanagerPayload) ([]*models.Alert, error) {
	if payload == nil {
		return nil, errors.New("nil payload")
	}

	if payload.Version == "" {
		return nil, errors.New("missing version in Alertmanager payload")
	}

	if len(payload.Alerts) == 0 {
		return nil, errors.New("payload contains no alerts")
	}

	var alerts []*models.Alert

	for i, raw := range payload.Alerts {
		if raw.Fingerprint == "" {
			return nil, fmt.Errorf("alert at index %d is missing fingerprint", i)
		}
		if raw.Status != "firing" && raw.Status != "resolved" {
			return nil, fmt.Errorf("alert at index %d has invalid status: %q", i, raw.Status)
		}

		alertName := raw.Labels["alertname"]
		if alertName == "" {
			return nil, fmt.Errorf("alert at index %d is missing alertname label", i)
		}

		// Try to resolve the target service name
		service := raw.Labels["service"]
		if service == "" {
			service = raw.Labels["app"]
		}
		if service == "" {
			service = raw.Labels["helm_release"]
		}
		if service == "" {
			return nil, fmt.Errorf("alert at index %d is missing service/app/helm_release label", i)
		}

		severity := raw.Labels["severity"]
		if severity == "" {
			severity = "warning"
		}

		startsAt := raw.StartsAt
		if startsAt.IsZero() {
			startsAt = time.Now().UTC()
		}

		alert := &models.Alert{
			Fingerprint:  raw.Fingerprint,
			Name:         alertName,
			Service:      service,
			Severity:     severity,
			Status:       raw.Status,
			StartsAt:     startsAt,
			EndsAt:       raw.EndsAt,
			GeneratorURL: raw.GeneratorURL,
			Labels:       raw.Labels,
			Annotations:  raw.Annotations,
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}
