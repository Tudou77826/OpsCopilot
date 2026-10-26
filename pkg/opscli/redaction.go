package opscli

import (
	"sort"
	"strings"
)

// Replace the longest credentials first, in one pass. Sequential ReplaceAll
// can expose the suffix of a longer credential or rewrite the replacement text.
func hideCredentials(value string, secrets []string) string {
	ordered := append([]string(nil), secrets...)
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	pairs := make([]string, 0, len(ordered)*2)
	for i, secret := range ordered {
		if secret != "" && (i == 0 || secret != ordered[i-1]) {
			pairs = append(pairs, secret, "[凭据已隐藏]")
		}
	}
	if len(pairs) == 0 {
		return value
	}
	return strings.NewReplacer(pairs...).Replace(value)
}
