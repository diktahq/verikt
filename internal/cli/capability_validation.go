package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/diktahq/verikt/internal/config"
	"github.com/diktahq/verikt/internal/provider"
)

// validateConfigCapabilities rejects capabilities in verikt.yaml that the
// language provider does not define. `verikt add` validates its arguments, but
// nothing validated the config field, so a typo degraded quietly: check and
// guide accepted it and the guide rendered it as a real capability.
//
// Validation is skipped when the capability set cannot be determined (no
// provider for the language, or an unreadable template FS). Those are reported
// by the commands that actually need the provider — treating them as capability
// errors here would fail `verikt check` for languages it currently supports.
func validateConfigCapabilities(cfg *config.VeriktConfig) error {
	if cfg == nil || len(cfg.Capabilities) == 0 {
		return nil
	}

	p, err := provider.Get(cfg.Language)
	if err != nil {
		return nil
	}
	known, err := listAvailableCapabilities(p.GetTemplateFS())
	if err != nil {
		return nil
	}

	errs := config.ValidateCapabilities(cfg, known)
	if len(errs) == 0 {
		return nil
	}

	messages := make([]string, 0, len(errs))
	for _, e := range errs {
		messages = append(messages, e.Error())
	}
	return fmt.Errorf("invalid verikt.yaml: %w", errors.New(strings.Join(messages, "; ")))
}
