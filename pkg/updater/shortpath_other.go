//go:build !windows

package updater

func getShortPath(path string) string {
	return path
}
