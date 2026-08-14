package swallow

import "os"

func Read() {
	f, err := os.Open("x")
	if err != nil {
	}
	_ = f
}
