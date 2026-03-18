// Package service is the service layer — should not be imported by domain.
package service

// OrderService handles order business logic.
type OrderService struct{}

func (s *OrderService) CreateOrder(name string) error {
	return nil
}
