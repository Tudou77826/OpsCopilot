package terminal

import "testing"

func TestCleanInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal input",
			input:    "ls -la",
			expected: "ls -la",
		},
		{
			name:     "Up Arrow",
			input:    "\x1b[A",
			expected: "",
		},
		{
			name:     "Down Arrow",
			input:    "\x1b[B",
			expected: "",
		},
		{
			name:     "Right Arrow with text",
			input:    "cd\x1b[C",
			expected: "cd",
		},
		{
			name:     "Complex CSI sequence",
			input:    "\x1b[1;2A", // Shift+Up
			expected: "",
		},
		{
			name:     "Mixed content",
			input:    "echo \x1b[31mhello\x1b[0m", // Color codes
			expected: "echo hello",
		},
		{
			name:     "Incomplete sequence (should be kept if not strictly ANSI)",
			input:    "\x1b", // Just ESC
			expected: "",     // Strip isolated ESC too? Usually yes for cleanup.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanInput(tt.input); got != tt.expected {
				t.Errorf("CleanInput() = %q, want %q", got, tt.expected)
			}
		})
	}
}
