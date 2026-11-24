//go:build !windows

package acp

import "os"

// bindProcessTree is a no-op outside Windows: the direct child kill plus the
// exit wait cover the common case, and POSIX process groups are not introduced
// here to keep the transport minimal.
func bindProcessTree(*os.Process) (func(), error) { return func() {}, nil }
