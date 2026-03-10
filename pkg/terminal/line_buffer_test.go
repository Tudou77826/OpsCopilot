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
				handleResult := lb.Handle(in)
				if handleResult.Committed {
					result = handleResult.Line
				}
			}
			if result != tt.expected {
				t.Errorf("LineBuffer produced %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLineBuffer_HistoryNavigation(t *testing.T) {
	t.Run("Up arrow sets history navigation flag", func(t *testing.T) {
		lb := NewLineBuffer()

		// Press up arrow
		result := lb.Handle("\x1b[A")
		if result.Committed {
			t.Error("Up arrow should not commit")
		}
		if !result.HistoryNavigation {
			t.Error("Up arrow should set history navigation flag")
		}
	})

	t.Run("Down arrow sets history navigation flag", func(t *testing.T) {
		lb := NewLineBuffer()

		// Press down arrow
		result := lb.Handle("\x1b[B")
		if result.Committed {
			t.Error("Down arrow should not commit")
		}
		if !result.HistoryNavigation {
			t.Error("Down arrow should set history navigation flag")
		}
	})

	t.Run("Enter after history navigation with empty buffer", func(t *testing.T) {
		lb := NewLineBuffer()

		// Press up arrow (simulates selecting history)
		lb.Handle("\x1b[A")

		// Press Enter (user executes the history command without editing)
		// Note: HistoryNavigation is captured before Reset() so it's still true in the result
		result := lb.Handle("\r")
		if !result.Committed {
			t.Error("Enter should commit")
		}
		// After Enter, the buffer is reset, so historyNavigation is reset too
		// But the result should still reflect that history navigation was used
		if result.Line != "" {
			t.Errorf("Expected empty line (command from history), got %q", result.Line)
		}
	})

	t.Run("Enter after history navigation with edited command", func(t *testing.T) {
		lb := NewLineBuffer()

		// Press up arrow
		lb.Handle("\x1b[A")

		// Type something (user modifies the history command)
		lb.Handle("a")
		lb.Handle("b")
		lb.Handle("c")

		// Press Enter
		result := lb.Handle("\r")
		if !result.Committed {
			t.Error("Enter should commit")
		}
		if result.HistoryNavigation {
			t.Error("History navigation should be reset after typing")
		}
		if result.Line != "abc" {
			t.Errorf("Expected 'abc', got %q", result.Line)
		}
	})

	t.Run("Typing after history navigation resets flag", func(t *testing.T) {
		lb := NewLineBuffer()

		// Press up arrow
		lb.Handle("\x1b[A")

		// Type something (user modifies the history command)
		result := lb.Handle("a")
		if result.HistoryNavigation {
			t.Error("Typing should reset history navigation flag")
		}

		// Continue typing and press Enter
		lb.Handle("b")
		lb.Handle("c")
		result = lb.Handle("\r")
		if result.HistoryNavigation {
			t.Error("History navigation should remain reset")
		}
		if result.Line != "abc" {
			t.Errorf("Expected 'abc', got %q", result.Line)
		}
	})
}
