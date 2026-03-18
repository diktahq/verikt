package domain

// This file intentionally imports from the service layer to trigger a
// dependency violation: domain must not depend on service in hexagonal architecture.
import _ "example.com/testproject/service"
