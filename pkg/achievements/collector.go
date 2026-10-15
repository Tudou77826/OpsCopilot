package achievements

import (
	"fmt"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/script"
	"strings"
)

func EligibleScript(sc *script.Script) bool {
	if sc == nil {
		return false
	}
	if len(sc.Steps) > 0 {
		for _, step := range sc.Steps {
			if step.Enabled && strings.TrimSpace(step.Command) != "" {
				return true
			}
		}
		return false
	}
	for _, cmd := range sc.Commands {
		if cmd.Enabled && strings.TrimSpace(cmd.Content) != "" {
			return true
		}
	}
	return false
}
func (s *Store) TrackExisting(kind social.MedalKind, reference string, eligible bool, write func() error) error {
	known, err := s.Has(kind, reference)
	if err != nil {
		return err
	}
	if !known {
		return write()
	}
	return s.Do(kind, reference, eligible, write)
}
func (s *Store) TrackNew(kind social.MedalKind, reference string, eligible bool, write func() error) error {
	known, err := s.Has(kind, reference)
	if err != nil {
		return err
	}
	if known {
		return fmt.Errorf("成果标识已存在")
	}
	return s.Do(kind, reference, eligible, write)
}
