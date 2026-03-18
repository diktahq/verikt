package service

// ProcessOrder has many parameters — triggers max_params detector.
func ProcessOrder(id string, name string, price float64, qty int, note string) error {
	_ = id
	_ = name
	_ = price
	_ = qty
	_ = note
	return nil
}
