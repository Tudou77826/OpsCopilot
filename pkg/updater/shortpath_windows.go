package updater

import "syscall"

// getShortPath converts a Windows path to its 8.3 short-path form (pure ASCII).
// This avoids batch-script encoding issues with Chinese/Unicode paths.
// If conversion fails, the original path is returned as a fallback.
func getShortPath(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, syscall.MAX_PATH)
	n, err := syscall.GetShortPathName(p, &buf[0], syscall.MAX_PATH)
	if err != nil || n == 0 {
		return path
	}
	return syscall.UTF16ToString(buf[:n])
}
