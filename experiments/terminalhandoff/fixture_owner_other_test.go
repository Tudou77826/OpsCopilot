//go:build !windows

package terminalhandoff

import "errors"

func fixtureOwner(raw string) (func() bool, func(), error) {
	if raw != "" {
		return nil, nil, errors.New("desktop fixture owner tracking requires Windows")
	}
	return func() bool { return true }, func() {}, nil
}
