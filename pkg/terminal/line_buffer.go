package terminal

// LineBuffer maintains the state of a terminal input line
type LineBuffer struct {
	buffer []rune
	cursor int
}

func NewLineBuffer() *LineBuffer {
	return &LineBuffer{
		buffer: make([]rune, 0),
		cursor: 0,
	}
}

// Handle processes input string and returns (committed_line, true) if Enter is pressed
func (lb *LineBuffer) Handle(input string) (string, bool) {
	// Enhanced ANSI sequence parser
	// We need to iterate runes to handle multibyte characters correctly
	runes := []rune(input)

	i := 0
	for i < len(runes) {
		r := runes[i]

		// Handle escape sequences (cursor movement, etc.)
		if r == '\x1b' {
			seq, nextIdx := lb.extractEscapeSequence(runes, i)
			if seq != "" {
				lb.handleSequence(seq)
			}
			i = nextIdx
			continue
		}

		// Handle Control Characters
		switch r {
		case '\r', '\n':
			line := string(lb.buffer)
			lb.Reset()
			return line, true
		case '\x7f', '\x08': // Backspace
			lb.backspace()
		default:
			// Only record printable characters
			// ASCII printable: 32-126
			// Multi-byte characters (like Chinese) are > 126
			if r >= 32 && r <= 126 {
				lb.insert(r)
			} else if r > 126 {
				// Multi-byte UTF-8 characters (Chinese, Japanese, etc.)
				lb.insert(r)
			}
			// Ignore control characters (0-31, except \r, \n, \x7f, \x08)
		}
		i++
	}

	return "", false
}

// extractEscapeSequence extracts a complete escape sequence and returns (sequence, nextIndex)
func (lb *LineBuffer) extractEscapeSequence(runes []rune, start int) (string, int) {
	if start >= len(runes) || runes[start] != '\x1b' {
		return "", start + 1
	}

	// ESC + [ (CSI sequence - most common)
	// Format: ESC [ <parameters> <intermediate> <final>
	// Final character range: 0x40-0x7E
	if start+1 < len(runes) && runes[start+1] == '[' {
		end := start + 2
		for end < len(runes) {
			if runes[end] >= 0x40 && runes[end] <= 0x7E {
				seq := string(runes[start : end+1])
				return seq, end + 1
			}
			end++
			// Prevent malicious data from causing infinite loops
			if end-start > 20 {
				break
			}
		}
		return "", end
	}

	// ESC + O (PF1-PF4 function keys)
	if start+1 < len(runes) && runes[start+1] == 'O' {
		end := start + 2
		if end < len(runes) {
			seq := string(runes[start : end+1])
			return seq, end + 1
		}
		return "", end
	}

	// Other escape sequences - skip up to 4 characters
	end := start + 1
	maxSkip := start + 5
	for end < len(runes) && end < maxSkip {
		if runes[end] >= 0x20 && runes[end] <= 0x7E {
			end++
		} else {
			break
		}
	}

	// Return the sequence if we found one
	if end > start+1 {
		seq := string(runes[start:end])
		return seq, end
	}
	return "", end
}

func (lb *LineBuffer) Reset() {
	lb.buffer = make([]rune, 0)
	lb.cursor = 0
}

func (lb *LineBuffer) insert(r rune) {
	if lb.cursor == len(lb.buffer) {
		lb.buffer = append(lb.buffer, r)
	} else {
		lb.buffer = append(lb.buffer[:lb.cursor+1], lb.buffer[lb.cursor:]...)
		lb.buffer[lb.cursor] = r
	}
	lb.cursor++
}

func (lb *LineBuffer) backspace() {
	if lb.cursor > 0 {
		lb.buffer = append(lb.buffer[:lb.cursor-1], lb.buffer[lb.cursor:]...)
		lb.cursor--
	}
}

func (lb *LineBuffer) delete() {
	if lb.cursor < len(lb.buffer) {
		lb.buffer = append(lb.buffer[:lb.cursor], lb.buffer[lb.cursor+1:]...)
	}
}

func (lb *LineBuffer) handleSequence(seq string) {
	switch seq {
	case "\x1b[D": // Left
		if lb.cursor > 0 {
			lb.cursor--
		}
	case "\x1b[C": // Right
		if lb.cursor < len(lb.buffer) {
			lb.cursor++
		}
	case "\x1b[H", "\x1b[1~": // Home
		lb.cursor = 0
	case "\x1b[F", "\x1b[4~": // End
		lb.cursor = len(lb.buffer)
	case "\x1b[3~": // Delete
		lb.delete()
	}
}
