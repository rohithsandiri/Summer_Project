// internal/operator/alert/parser_test.go

package alert

import (
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

func TestParserParseValid(t *testing.T) {
	p := NewParser()
	rawAlerts := []models.AlertmanagerAlert{
		{
			Status:       "firing",
			Fingerprint:  "fp1",
			GeneratorURL: "http://generator",
			StartsAt:     time.Now().Add(-5 * time.Minute),
			Labels: map[string]string{
				"alertname": "HighErrorRate",
				"service":   "payment-service",
				"severity":  "critical",
			},
			Annotations: map[string]string{
				"summary": "High HTTP error rate on payment",
			},
		},
	}
	payload := &models.AlertmanagerPayload{
		Version: "4",
		Alerts:  rawAlerts,
	}

	alerts, err := p.Parse(payload)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}

	a := alerts[0]
	if a.Fingerprint != "fp1" {
		t.Errorf("Expected fingerprint fp1, got %q", a.Fingerprint)
	}
	if a.Name != "HighErrorRate" {
		t.Errorf("Expected name HighErrorRate, got %q", a.Name)
	}
	if a.Service != "payment-service" {
		t.Errorf("Expected service payment-service, got %q", a.Service)
	}
}

func TestParserParseInvalid(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name    string
		payload *models.AlertmanagerPayload
	}{
		{
			name:    "nil payload",
			payload: nil,
		},
		{
			name: "missing version",
			payload: &models.AlertmanagerPayload{
				Version: "",
				Alerts: []models.AlertmanagerAlert{
					{Fingerprint: "fp"},
				},
			},
		},
		{
			name: "empty alerts",
			payload: &models.AlertmanagerPayload{
				Version: "4",
				Alerts:  []models.AlertmanagerAlert{},
			},
		},
		{
			name: "missing fingerprint",
			payload: &models.AlertmanagerPayload{
				Version: "4",
				Alerts: []models.AlertmanagerAlert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "HighErrorRate",
							"service":   "payment-service",
						},
					},
				},
			},
		},
		{
			name: "missing service label",
			payload: &models.AlertmanagerPayload{
				Version: "4",
				Alerts: []models.AlertmanagerAlert{
					{
						Status:      "firing",
						Fingerprint: "fp",
						Labels: map[string]string{
							"alertname": "HighErrorRate",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Parse(tt.payload)
			if err == nil {
				t.Error("Expected error for invalid payload")
			}
		})
	}
}
