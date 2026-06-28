// internal/operator/utils/cooldown.go
//
// Cooldown Manager to prevent repeated recovery execution within cooldown windows.

package utils

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CooldownManager struct {
	mu            sync.RWMutex
	cooldownUntil map[string]time.Time // key: "service/alertName"
}

func NewCooldownManager() *CooldownManager {
	return &CooldownManager{
		cooldownUntil: make(map[string]time.Time),
	}
}

// IsCoolingDown checks if the service/alert combination is in a cooldown window.
// Returns true and the remaining cooldown duration if so.
func (cm *CooldownManager) IsCoolingDown(ctx context.Context, service string, alertName string) (bool, time.Duration) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", service, alertName)
	until, exists := cm.cooldownUntil[key]
	if !exists {
		return false, 0
	}

	remaining := time.Until(until)
	if remaining > 0 {
		return true, remaining
	}

	return false, 0
}

// RecordRecovery sets a cooldown window for a service/alert combination.
func (cm *CooldownManager) RecordRecovery(ctx context.Context, service string, alertName string, duration time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := fmt.Sprintf("%s/%s", service, alertName)
	cm.cooldownUntil[key] = time.Now().Add(duration)
}
