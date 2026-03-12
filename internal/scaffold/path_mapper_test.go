package scaffold

import (
	"testing"
)

func TestPathMapper_Map_Hexagonal(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "adapter/httphandler",
		"adapter/pgxrepo":     "adapter/pgxrepo",
		"domain":              "domain",
		"port":                "port",
		"service":             "service",
	})

	tests := []struct {
		input string
		want  string
	}{
		{"adapter/httphandler/handler.go", "adapter/httphandler/handler.go"},
		{"adapter/pgxrepo/repo.go", "adapter/pgxrepo/repo.go"},
		{"domain/order.go", "domain/order.go"},
		{"port/repository.go", "port/repository.go"},
		{"service/order_service.go", "service/order_service.go"},
		{"unrelated/file.go", "unrelated/file.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pm.Map(tt.input)
			if got != tt.want {
				t.Errorf("Map(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathMapper_Map_Layered(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "internal/handler",
		"adapter/pgxrepo":     "internal/repository",
		"adapter/grpchandler": "internal/handler",
		"adapter/mysqlrepo":   "internal/repository",
		"adapter/redisrepo":   "internal/repository",
		"domain":              "internal/model",
		"port":                "internal/service",
		"service":             "internal/service",
	})

	tests := []struct {
		input string
		want  string
	}{
		{"adapter/httphandler/handler.go", "internal/handler/handler.go"},
		{"adapter/pgxrepo/repo.go", "internal/repository/repo.go"},
		{"domain/order.go", "internal/model/order.go"},
		{"port/repository.go", "internal/service/repository.go"},
		{"service/order_service.go", "internal/service/order_service.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pm.Map(tt.input)
			if got != tt.want {
				t.Errorf("Map(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathMapper_Map_Clean(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "internal/interface/http",
		"adapter/pgxrepo":     "internal/infrastructure/postgres",
		"adapter/grpchandler": "internal/interface/grpc",
		"domain":              "internal/entity",
		"port":                "internal/usecase",
		"service":             "internal/usecase",
	})

	tests := []struct {
		input string
		want  string
	}{
		{"adapter/httphandler/handler.go", "internal/interface/http/handler.go"},
		{"adapter/pgxrepo/repo.go", "internal/infrastructure/postgres/repo.go"},
		{"domain/order.go", "internal/entity/order.go"},
		{"port/repository.go", "internal/usecase/repository.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pm.Map(tt.input)
			if got != tt.want {
				t.Errorf("Map(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathMapper_Map_Flat(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "handler",
		"adapter/pgxrepo":     "repository",
		"domain":              "model",
		"port":                "",
		"service":             "",
	})

	tests := []struct {
		input string
		want  string
	}{
		{"adapter/httphandler/handler.go", "handler/handler.go"},
		{"adapter/pgxrepo/repo.go", "repository/repo.go"},
		{"domain/order.go", "model/order.go"},
		{"port/repository.go", "repository.go"},
		{"service/order_service.go", "order_service.go"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pm.Map(tt.input)
			if got != tt.want {
				t.Errorf("Map(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPathMapper_Map_LongestPrefixWins(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter":             "infra",
		"adapter/httphandler": "internal/handler",
	})

	got := pm.Map("adapter/httphandler/routes.go")
	want := "internal/handler/routes.go"
	if got != want {
		t.Errorf("Map() = %q, want %q (longest prefix should win)", got, want)
	}

	got = pm.Map("adapter/other/file.go")
	want = "infra/other/file.go"
	if got != want {
		t.Errorf("Map() = %q, want %q (shorter prefix should match)", got, want)
	}
}

func TestPathMapper_Map_ExactMatch(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"domain": "internal/entity",
	})

	got := pm.Map("domain")
	if got != "internal/entity" {
		t.Errorf("Map(\"domain\") = %q, want \"internal/entity\"", got)
	}
}

func TestPathMapper_Map_NoMappings(t *testing.T) {
	pm := NewPathMapper(nil)
	got := pm.Map("adapter/httphandler/handler.go")
	if got != "adapter/httphandler/handler.go" {
		t.Errorf("nil mapper should return path unchanged, got %q", got)
	}
}

func TestPathMapper_Map_NoMatch(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"domain": "internal/entity",
	})
	got := pm.Map("config/config.go")
	if got != "config/config.go" {
		t.Errorf("unmatched path should be unchanged, got %q", got)
	}
}

func TestPathMapper_ArchPaths(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "adapter/httphandler",
		"adapter/pgxrepo":     "adapter/pgxrepo",
		"adapter/grpchandler": "adapter/grpchandler",
		"adapter/mysqlrepo":   "adapter/mysqlrepo",
		"adapter/redisrepo":   "adapter/redisrepo",
		"domain":              "domain",
		"port":                "port",
		"service":             "service",
	})

	ap := pm.ArchPaths()

	expected := map[string]string{
		"Domain":       "domain",
		"Ports":        "port",
		"Service":      "service",
		"HTTPHandler":  "adapter/httphandler",
		"GRPCHandler":  "adapter/grpchandler",
		"PostgresRepo": "adapter/pgxrepo",
		"MySQLRepo":    "adapter/mysqlrepo",
		"RedisRepo":    "adapter/redisrepo",
	}

	for key, want := range expected {
		got, ok := ap[key]
		if !ok {
			t.Errorf("ArchPaths() missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("ArchPaths()[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestPathMapper_ArchPaths_Layered(t *testing.T) {
	pm := NewPathMapper(map[string]string{
		"adapter/httphandler": "internal/handler",
		"adapter/pgxrepo":     "internal/repository",
		"domain":              "internal/model",
		"port":                "internal/service",
		"service":             "internal/service",
	})

	ap := pm.ArchPaths()

	if ap["Domain"] != "internal/model" {
		t.Errorf("Domain = %q, want \"internal/model\"", ap["Domain"])
	}
	if ap["HTTPHandler"] != "internal/handler" {
		t.Errorf("HTTPHandler = %q, want \"internal/handler\"", ap["HTTPHandler"])
	}
	if ap["PostgresRepo"] != "internal/repository" {
		t.Errorf("PostgresRepo = %q, want \"internal/repository\"", ap["PostgresRepo"])
	}
}
