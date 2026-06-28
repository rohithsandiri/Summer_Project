// services/order-service/repository/repository.go
//
// Updated to return *apperrors.AppError.

package repository

import (
	"fmt"
	"sync"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/services/order-service/models"
)

// OrderRepository defines persistence operations for orders.
type OrderRepository interface {
	Create(order *models.Order) error
	GetByID(orderID string) (*models.Order, error)
	Update(order *models.Order) error
	List() []*models.Order
}

// InMemoryOrderRepository is a thread-safe in-memory implementation.
type InMemoryOrderRepository struct {
	mu     sync.RWMutex
	orders map[string]*models.Order
}

// NewInMemoryOrderRepository creates an empty order store.
func NewInMemoryOrderRepository() *InMemoryOrderRepository {
	return &InMemoryOrderRepository{
		orders: make(map[string]*models.Order),
	}
}

// Create stores a new order record.
func (r *InMemoryOrderRepository) Create(order *models.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.orders[order.OrderID]; exists {
		return apperrors.ErrDuplicateOrder.WithMessage(
			fmt.Sprintf("order already exists: %s", order.OrderID),
		)
	}

	r.orders[order.OrderID] = order
	return nil
}

// GetByID returns a deep copy of the order.
func (r *InMemoryOrderRepository) GetByID(orderID string) (*models.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	o, ok := r.orders[orderID]
	if !ok {
		return nil, apperrors.ErrOrderNotFound.WithMessage(
			fmt.Sprintf("order not found: %s", orderID),
		)
	}
	copy := *o
	copy.Items = make([]models.OrderItem, len(o.Items))
	for i, item := range o.Items {
		copy.Items[i] = item
	}
	return &copy, nil
}

// Update replaces the stored order record.
func (r *InMemoryOrderRepository) Update(order *models.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.orders[order.OrderID]; !ok {
		return apperrors.ErrOrderNotFound.WithMessage(
			fmt.Sprintf("cannot update: order not found: %s", order.OrderID),
		)
	}
	r.orders[order.OrderID] = order
	return nil
}

// List returns copies of all stored orders.
func (r *InMemoryOrderRepository) List() []*models.Order {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*models.Order, 0, len(r.orders))
	for _, o := range r.orders {
		copy := *o
		result = append(result, &copy)
	}
	return result
}
