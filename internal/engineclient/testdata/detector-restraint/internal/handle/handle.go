package handle

import (
	"errors"
	"fmt"
	"os"
)

// ErrMissing is returned when the file is absent.
var ErrMissing = errors.New("missing")

// Read wraps the error with context, which is the documented convention.
func Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", path, ErrMissing)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}
