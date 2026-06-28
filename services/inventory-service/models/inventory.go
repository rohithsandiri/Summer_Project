// services/inventory-service/models/inventory.go
//
// WHY THIS EXISTS:
//   Models are the pure data structures that represent business concepts.
//   They have no HTTP knowledge, no database logic — just the shape of the domain.
//   This makes them usable by every layer: handler, service, repository.
//
// FUTURE PHASES:
//   - Add validation tags (e.g. github.com/go-playground/validator)
//   - Add database scan/value methods when PostgreSQL is introduced
//   - Add protobuf annotations when gRPC is added

package models

import "time"

// InventoryItem represents a stock-keeping unit (SKU) in the inventory.
type InventoryItem struct {
	// ItemID is the unique identifier for a product SKU.
	ItemID string `json:"item_id"`

	// Name is the human-readable product name.
	Name string `json:"name"`

	// Quantity is the currently available (unreserved) stock.
	Quantity int `json:"quantity"`

	// Reserved is the quantity currently held by pending orders.
	// Available = Quantity - Reserved (enforced by the service layer).
	Reserved int `json:"reserved"`

	// UpdatedAt is the last time this record was modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// ReserveRequest is the payload for POST /inventory/reserve.
type ReserveRequest struct {
	// ItemID identifies the SKU to reserve.
	ItemID string `json:"item_id"`

	// Quantity is how many units to reserve for an order.
	Quantity int `json:"quantity"`

	// OrderID links the reservation to a specific order for release tracking.
	OrderID string `json:"order_id"`
}

// ReserveResponse is returned after a successful reservation.
type ReserveResponse struct {
	ItemID       string    `json:"item_id"`
	OrderID      string    `json:"order_id"`
	ReservedQty  int       `json:"reserved_quantity"`
	RemainingQty int       `json:"remaining_quantity"`
	ReservedAt   time.Time `json:"reserved_at"`
}

// ReleaseRequest is the payload for POST /inventory/release.
// Called when an order is cancelled or payment fails.
type ReleaseRequest struct {
	ItemID   string `json:"item_id"`
	OrderID  string `json:"order_id"`
	Quantity int    `json:"quantity"`
}

// ReleaseResponse is returned after a successful stock release.
type ReleaseResponse struct {
	ItemID       string    `json:"item_id"`
	OrderID      string    `json:"order_id"`
	ReleasedQty  int       `json:"released_quantity"`
	AvailableQty int       `json:"available_quantity"`
	ReleasedAt   time.Time `json:"released_at"`
}
