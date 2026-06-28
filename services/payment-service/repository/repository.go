// services/payment-service/repository/repository.go
//
// Updated to return *apperrors.AppError domain errors.

package repository

import (
	"fmt"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/services/payment-service/models"
)

// PaymentRepository defines persistence operations for payment records.
type PaymentRepository interface {
	Create(p *models.Payment) error
	GetByID(paymentID string) (*models.Payment, error)
	GetByOrderID(orderID string) (*models.Payment, error)
	UpdateStatus(paymentID string, status models.PaymentStatus, failureReason string) error
}

// InMemoryPaymentRepository is a thread-safe in-memory implementation.
type InMemoryPaymentRepository struct {
	mu       sync.RWMutex
	payments map[string]*models.Payment
}

// NewInMemoryPaymentRepository creates an empty payment repository.
func NewInMemoryPaymentRepository() *InMemoryPaymentRepository {
	return &InMemoryPaymentRepository{
		payments: make(map[string]*models.Payment),
	}
}

// Create stores a new payment record.
func (r *InMemoryPaymentRepository) Create(p *models.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[p.PaymentID] = p
	return nil
}

// GetByID returns a copy of the payment record.
func (r *InMemoryPaymentRepository) GetByID(paymentID string) (*models.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.payments[paymentID]
	if !ok {
		return nil, apperrors.ErrPaymentNotFound.WithMessage(
			fmt.Sprintf("payment not found: %s", paymentID),
		)
	}
	copy := *p
	return &copy, nil
}

// GetByOrderID returns the most recent payment for an order.
func (r *InMemoryPaymentRepository) GetByOrderID(orderID string) (*models.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *models.Payment
	for _, p := range r.payments {
		if p.OrderID == orderID {
			if latest == nil || p.CreatedAt.After(latest.CreatedAt) {
				latest = p
			}
		}
	}
	if latest == nil {
		return nil, apperrors.ErrPaymentNotFound.WithMessage(
			fmt.Sprintf("no payment found for order: %s", orderID),
		)
	}
	copy := *latest
	return &copy, nil
}

// UpdateStatus modifies the status of a payment record.
func (r *InMemoryPaymentRepository) UpdateStatus(paymentID string, status models.PaymentStatus, failureReason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.payments[paymentID]
	if !ok {
		return apperrors.ErrPaymentNotFound.WithMessage(
			fmt.Sprintf("payment not found for update: %s", paymentID),
		)
	}
	p.Status = status
	p.FailureReason = failureReason
	p.UpdatedAt = time.Now()
	return nil
}
