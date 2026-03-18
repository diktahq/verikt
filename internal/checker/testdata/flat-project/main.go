package main

import "fmt"

type Order struct {
	ID   string
	Name string
}

func CreateOrder(name string) (*Order, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	return &Order{ID: "1", Name: name}, nil
}

func main() {
	o, _ := CreateOrder("alice")
	fmt.Println(o)
}
