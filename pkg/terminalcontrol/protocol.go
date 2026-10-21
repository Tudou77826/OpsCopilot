// Package terminalcontrol provides input ownership for existing SSH shells.
package terminalcontrol

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const maxFrameBytes = 32768

type Prompt struct {
	Request   uint64
	ExitCode  int
	UID       int
	PID       int
	Directory string
}

func (p *protocol) begin(request uint64) string {
	return fmt.Sprintf(`command printf '\036OCB1|%s|%d\037'; `, p.nonce, request)
}

type protocol struct {
	nonce  string
	buffer []byte
}

func newProtocol() (*protocol, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("生成探测标识失败: %w", err)
	}
	return &protocol{nonce: hex.EncodeToString(b[:])}, nil
}

// Bootstrap must only be sent after a human/adapter has confirmed an ordinary
// shell prompt. Sending a probe to an unknown foreground program is unsafe.
// Prompt expansion runs in the existing shell; no eval, subshell for the user's
// command, new SSH exec channel, profile edit, or persistent installation is used.
func (p *protocol) bootstrap() string {
	// Self-contained expansion also works when PS1 is inherited across su but
	// shell functions are not. A login shell that clears PS1 still needs a new
	// explicit ordinary-shell confirmation; no blind reattachment is attempted.
	quoted := shellQuote(p.promptProbe())
	return fmt.Sprintf(`case "${PS1-}" in %s*) ;; *) PS1=%s"${PS1-}" ;; esac`, quoted, quoted)
}

func (p *protocol) uninstall() string {
	// Strip only our exact prefix, including in an inherited child shell; no
	// backup variables or functions need to cross an account boundary.
	return fmt.Sprintf(`PS1=${PS1#%s}`, shellQuote(p.promptProbe()))
}

func (p *protocol) promptProbe() string {
	return fmt.Sprintf(`$(command printf '\036OCH1|%s|%%s|%%s|%%s|%%s\037' "$?" "$(command id -u)" "$$" "$(command printf '%%s' "$PWD" | command od -An -tx1 | command tr -d ' \n')")`, p.nonce)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// feed accepts arbitrarily fragmented PTY bytes. Only complete, bounded frames
// with this session's nonce affect readiness. Other bytes remain display data.
func (p *protocol) feed(data []byte) ([]byte, []Prompt) {
	p.buffer = append(p.buffer, data...)
	var visible []byte
	var prompts []Prompt
	for len(p.buffer) > 0 {
		start := bytes.IndexByte(p.buffer, 0x1e)
		if start < 0 {
			visible = append(visible, p.buffer...)
			p.buffer = nil
			break
		}
		visible = append(visible, p.buffer[:start]...)
		p.buffer = p.buffer[start:]
		// Ordinary programs may emit record separators too. Only buffer a
		// possible control frame for this shell; unrelated output stays live.
		body := p.buffer[1:]
		possible := false
		for _, kind := range []string{"OCH1|", "OCB1|"} {
			header := []byte(kind + p.nonce + "|")
			if bytes.HasPrefix(body, header) || bytes.HasPrefix(header, body) {
				possible = true
				break
			}
		}
		if !possible {
			visible = append(visible, p.buffer[0])
			p.buffer = p.buffer[1:]
			continue
		}
		end := bytes.IndexByte(p.buffer, 0x1f)
		if end < 0 && len(p.buffer) <= maxFrameBytes {
			break
		}
		if end < 0 || end > maxFrameBytes {
			visible = append(visible, p.buffer[0])
			p.buffer = p.buffer[1:]
			continue
		}
		frame := p.buffer[:end+1]
		if prompt, ok := p.parse(string(frame[1 : len(frame)-1])); ok {
			prompts = append(prompts, prompt)
		} else {
			visible = append(visible, frame...)
		}
		p.buffer = p.buffer[end+1:]
	}
	return visible, prompts
}

func (p *protocol) parse(frame string) (Prompt, bool) {
	parts := strings.Split(frame, "|")
	if len(parts) == 3 && parts[0] == "OCB1" && parts[1] == p.nonce {
		request, err := strconv.ParseUint(parts[2], 10, 64)
		return Prompt{Request: request}, err == nil && request > 0
	}
	if len(parts) != 6 || parts[0] != "OCH1" || parts[1] != p.nonce {
		return Prompt{}, false
	}
	rc, e1 := strconv.Atoi(parts[2])
	uid, e2 := strconv.Atoi(parts[3])
	pid, e3 := strconv.Atoi(parts[4])
	dir, e4 := hex.DecodeString(parts[5])
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || rc < 0 || rc > 255 || uid < 0 || pid <= 0 || len(dir) == 0 || dir[0] != '/' || bytes.IndexByte(dir, 0) >= 0 {
		return Prompt{}, false
	}
	return Prompt{ExitCode: rc, UID: uid, PID: pid, Directory: string(dir)}, true
}
