// Package terminalhandoff contains the opt-in real-PTY validation harness.
package terminalhandoff

import "opscopilot/pkg/terminalcontrol"

type Controller = terminalcontrol.Controller
type Snapshot = terminalcontrol.Snapshot

var New = terminalcontrol.New
var ErrNeedConfirmation = terminalcontrol.ErrNeedConfirmation
var ErrOwned = terminalcontrol.ErrOwned
var ErrRevoked = terminalcontrol.ErrRevoked
