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
	// Simple ANSI sequence parser for cursor movement
	// We need to iterate runes to handle multibyte characters correctly
	runes := []rune(input)
	
	i := 0
	for i < len(runes) {
		r := runes[i]
		
		// Handle Escape Sequences
		if r == '\x1b' {
			// Check if it's a CSI sequence
			if i+1 < len(runes) && runes[i+1] == '[' {
				// Detect sequence end
				j := i + 2
				for j < len(runes) {
					if runes[j] >= 0x40 && runes[j] <= 0x7E {
						break
					}
					j++
				}
				
				if j < len(runes) {
					seq := string(runes[i : j+1])
					lb.handleSequence(seq)
					i = j + 1
					continue
				}
			}
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
			// Regular character
			if r >= 32 { // Printable
				lb.insert(r)
			}
		}
		i++
	}

	return "", false
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
