// Package domain is the domain layer — must NOT import service.
// This file intentionally imports service to trigger a dependency violation.
package domain

import _ "example.com/hexagonal/service"

// Entity is a domain entity.
type Entity struct{ ID string }
