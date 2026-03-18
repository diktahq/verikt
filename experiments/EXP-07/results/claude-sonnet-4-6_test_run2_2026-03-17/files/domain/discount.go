```go
package domain

import "errors"

var ErrDiscountNotFound = errors.New("discount not found")

type Discount struct {
	Code       string
	Percentage int
}
```