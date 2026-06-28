// services/inventory-service/repository/repository.go
//
// Updated to return *apperrors.AppError domain errors instead of raw Go errors.
// This change means handlers no longer need to map repository errors to HTTP
// status codes — they call response.WriteAppError(w, traceID, ae) directly.

package repository

import (
	"fmt"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/models"
)

// InventoryRepository defines the persistence contract for inventory data.
type InventoryRepository interface {
	GetItem(itemID string) (*models.InventoryItem, error)
	Reserve(itemID string, quantity int) error
	Release(itemID string, quantity int) error
	ListItems() []*models.InventoryItem
}

// InMemoryInventoryRepository is a thread-safe in-memory implementation.
type InMemoryInventoryRepository struct {
	mu    sync.RWMutex
	items map[string]*models.InventoryItem
}

// NewInMemoryInventoryRepository creates a pre-seeded repository.
func NewInMemoryInventoryRepository() *InMemoryInventoryRepository {
	return &InMemoryInventoryRepository{
		items: map[string]*models.InventoryItem{
			"SKU-001": {ItemID: "SKU-001", Name: "Laptop Pro 15\"", Quantity: 50, Reserved: 0, UpdatedAt: time.Now()},
			"SKU-002": {ItemID: "SKU-002", Name: "Wireless Mouse", Quantity: 200, Reserved: 0, UpdatedAt: time.Now()},
			"SKU-003": {ItemID: "SKU-003", Name: "USB-C Hub 7-Port", Quantity: 120, Reserved: 0, UpdatedAt: time.Now()},
			"SKU-004": {ItemID: "SKU-004", Name: "Mechanical Keyboard TKL", Quantity: 75, Reserved: 0, UpdatedAt: time.Now()},
			"SKU-005": {ItemID: "SKU-005", Name: "4K Monitor 27\"", Quantity: 30, Reserved: 0, UpdatedAt: time.Now()},
		},
	}
}

// GetItem returns a copy of the inventory item.
func (r *InMemoryInventoryRepository) GetItem(itemID string) (*models.InventoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[itemID]
	if !ok {
		return nil, apperrors.ErrInventoryItemNotFound.WithMessage(
			fmt.Sprintf("inventory item not found: %s", itemID),
		)
	}
	copy := *item
	return &copy, nil
}

// Reserve atomically checks and reduces available stock.
func (r *InMemoryInventoryRepository) Reserve(itemID string, quantity int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[itemID]
	if !ok {
		return apperrors.ErrInventoryItemNotFound.WithMessage(
			fmt.Sprintf("inventory item not found: %s", itemID),
		)
	}

	available := item.Quantity - item.Reserved
	if available < quantity {
		return apperrors.ErrInventoryUnavailable.WithMessage(
			fmt.Sprintf("insufficient stock for %s: requested %d, available %d", itemID, quantity, available),
		)
	}

	item.Reserved += quantity
	item.UpdatedAt = time.Now()
	return nil
}

// Release returns reserved units to the available pool.
func (r *InMemoryInventoryRepository) Release(itemID string, quantity int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.items[itemID]
	if !ok {
		return apperrors.ErrInventoryItemNotFound.WithMessage(
			fmt.Sprintf("inventory item not found: %s", itemID),
		)
	}

	if quantity > item.Reserved {
		quantity = item.Reserved
	}
	item.Reserved -= quantity
	item.UpdatedAt = time.Now()
	return nil
}

// ListItems returns copies of all items.
func (r *InMemoryInventoryRepository) ListItems() []*models.InventoryItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.InventoryItem, 0, len(r.items))
	for _, item := range r.items {
		copy := *item
		result = append(result, &copy)
	}
	return result
}
