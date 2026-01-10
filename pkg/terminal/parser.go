package terminal

import "regexp"

// ansiRegex matches standard ANSI escape sequences (CSI codes)
// Starts with ESC [ (\x1b\[)
// Followed by optional parameter bytes (0x30-0x3f)
// Followed by optional intermediate bytes (0x20-0x2f)
// Ends with a final byte (0x40-0x7e)
var ansiRegex = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]`)

// CleanInput removes ANSI escape sequences from the input string
// to prevent garbage characters in the session recording.
func CleanInput(input string) string {
	// Remove ANSI sequences
	cleaned := ansiRegex.ReplaceAllString(input, "")
	
	// Also remove isolated ESC characters if any remain
	// using a simple loop or string replacement as regex might be overkill for single char
	// but keeping it simple for now:
	if len(cleaned) > 0 {
		cleaned = regexp.MustCompile(`\x1b`).ReplaceAllString(cleaned, "")
	}
	
	return cleaned
}
