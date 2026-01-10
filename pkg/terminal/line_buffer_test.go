package terminal

import "testing"

func TestLineBuffer(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []string
		expected string // The final committed line
	}{
		{
			name:     "Simple typing",
			inputs:   []string{"h", "e", "l", "l", "o", "\r"},
			expected: "hello",
		},
		{
			name:     "Backspace",
			inputs:   []string{"h", "e", "l", "\x7f", "l", "o", "\r"}, // \x7f is DEL
			expected: "helo",                                          // hel -> he -> hel -> helo
		},
		{
			name:     "Backspace 2",
			inputs:   []string{"a", "b", "\x7f", "c", "\r"},
			expected: "ac",
		},
		{
			name:     "Left Arrow insertion",
			inputs:   []string{"a", "c", "\x1b[D", "b", "\r"}, // ac -> left -> between a and c -> insert b -> abc
			expected: "abc",
		},
		{
			name:     "Right Arrow",
			inputs:   []string{"a", "\x1b[D", "b", "\x1b[C", "c", "\r"}, // a -> left -> before a -> insert b -> ba -> right -> after a -> insert c -> bac
			expected: "bac",
		},
		{
			name:     "Home and End",
			inputs:   []string{"c", "d", "\x1b[H", "a", "b", "\x1b[F", "e", "\r"}, // cd -> home -> insert ab -> abcd -> end -> insert e -> abcde
			expected: "abcde",
		},
		{
			name:     "Delete key",
			inputs:   []string{"a", "b", "c", "\x1b[D", "\x1b[D", "\x1b[3~", "\r"}, // abc -> left -> left (at b) -> delete (removes b) -> ac
			expected: "ac",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLineBuffer()
			var result string
			for _, in := range tt.inputs {
				if line, ok := lb.Handle(in); ok {
					result = line
				}
			}
			if result != tt.expected {
				t.Errorf("LineBuffer produced %q, want %q", result, tt.expected)
			}
		})
	}
}
