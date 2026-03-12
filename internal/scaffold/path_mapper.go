package scaffold

import (
	"path/filepath"
	"sort"
	"strings"
)

// PathMapper translates capability template paths (written for hexagonal) to the
// target architecture's directory structure using longest-prefix matching.
type PathMapper struct {
	mappings []pathMapping
}

type pathMapping struct {
	from string
	to   string
}

// NewPathMapper creates a PathMapper from a source→target mapping.
// A nil or empty map produces a no-op mapper (identity).
func NewPathMapper(mappings map[string]string) *PathMapper {
	sorted := make([]pathMapping, 0, len(mappings))
	for from, to := range mappings {
		sorted = append(sorted, pathMapping{from: from, to: to})
	}
	// Sort by descending length so longest prefix wins.
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].from) > len(sorted[j].from)
	})
	return &PathMapper{mappings: sorted}
}

// Map rewrites sourcePath by replacing the longest matching prefix.
// If no prefix matches, the path is returned unchanged.
func (pm *PathMapper) Map(sourcePath string) string {
	if len(pm.mappings) == 0 {
		return sourcePath
	}
	for _, m := range pm.mappings {
		if m.from == "" {
			continue
		}
		if sourcePath == m.from {
			return m.to
		}
		if strings.HasPrefix(sourcePath, m.from+"/") {
			suffix := sourcePath[len(m.from)+1:]
			if m.to == "" {
				return suffix
			}
			return m.to + "/" + suffix
		}
	}
	return sourcePath
}

// ArchPaths returns a cleaned-up map for template use with friendly keys.
// Templates can reference {{.ArchPaths.Domain}}, {{.ArchPaths.HTTPHandler}}, etc.
func (pm *PathMapper) ArchPaths() map[string]string {
	keyMap := map[string]string{
		"domain":               "Domain",
		"port":                 "Ports",
		"service":              "Service",
		"adapter/httphandler":  "HTTPHandler",
		"adapter/grpchandler":  "GRPCHandler",
		"adapter/pgxrepo":      "PostgresRepo",
		"adapter/mysqlrepo":    "MySQLRepo",
		"adapter/redisrepo":    "RedisRepo",
		"adapter/esrepo":       "ElasticRepo",
		"adapter/kafkahandler": "KafkaHandler",
		"adapter/natshandler":  "NATSHandler",
		"adapter/bffgateway":   "BFFGateway",
		"adapter/mailpit":      "Mailpit",
	}
	out := make(map[string]string, len(keyMap)*2)
	for _, m := range pm.mappings {
		if friendly, ok := keyMap[m.from]; ok {
			out[friendly] = m.to
			// Add Pkg suffix key with the Go package name (last path segment).
			if m.to != "" {
				out[friendly+"Pkg"] = filepath.Base(m.to)
			}
		}
	}
	return out
}
