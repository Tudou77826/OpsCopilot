//go:build !windows

package main

import "context"

// The separate system popup is currently implemented for Windows desktops.
// Other platforms retain the in-app approval card.
func (a *App) startQuickApproval(context.Context) {}
