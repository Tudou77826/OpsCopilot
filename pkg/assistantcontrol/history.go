package assistantcontrol

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const historyMaxBytes = 4 * 1024 * 1024
const historyMaxMessages = 128

// Message is display history, never an approval or an instruction to execute.
type Message struct {
	ID         string          `json:"id"`
	TurnID     string          `json:"turnId"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Phase      string          `json:"phase,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Time       time.Time       `json:"time"`
	Operation  Operation       `json:"operation,omitempty"`
	References []PageReference `json:"references,omitempty"`
}

type ConversationSnapshot struct {
	ID           string    `json:"id"`
	Messages     []Message `json:"messages"`
	Truncated    bool      `json:"truncated"`
	RecoveryNote string    `json:"recoveryNote,omitempty"`
}

// History contains display records only. Persistence never includes pending
// callbacks, queued work, credentials, or approval payloads.
type History struct {
	mu        sync.Mutex
	id        string
	messages  []Message
	bytes     int
	truncated bool
}

func (h *History) Begin(turnID, question string, refs ...PageReference) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.id == "" {
		h.id = uuid.NewString()
	}
	h.append(Message{ID: turnID + ":user", TurnID: turnID, Role: "user", Content: question, References: append([]PageReference(nil), refs...)})
}

func (h *History) Finish(state State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Only finalize the matching accepted user message, at most once.
	if len(h.messages) == 0 || h.messages[len(h.messages)-1].ID != state.TurnID+":user" {
		return
	}
	if len(state.Messages) > 0 {
		for _, message := range state.Messages {
			h.append(message)
		}
		if state.Phase != "completed" {
			h.append(Message{ID: state.TurnID + ":outcome", TurnID: state.TurnID, Role: "ai", Kind: "notice", Content: state.Message, Phase: state.Phase})
		}
		return
	}
	content := state.Output
	if content == "" {
		content = state.Message
	} else if state.Phase == "failed" && strings.TrimSpace(state.Message) != "" {
		content += "\n\n请求未完成：" + state.Message
	}
	h.append(Message{ID: state.TurnID + ":ai", TurnID: state.TurnID, Role: "ai", Content: content, Phase: state.Phase})
}

func (h *History) append(message Message) {
	if message.Time.IsZero() {
		message.Time = time.Now()
	}
	h.messages = append(h.messages, message)
	h.bytes += messageBytes(message)
	// Evict whole old turns so a retained answer keeps its question. The newest
	// turn stays intact; live output already has the coordinator's per-turn cap.
	for (len(h.messages) > historyMaxMessages || h.bytes > historyMaxBytes) && len(h.messages) > 2 {
		old := h.messages[0].TurnID
		if old == message.TurnID {
			break
		}
		for len(h.messages) > 0 && h.messages[0].TurnID == old {
			h.bytes -= messageBytes(h.messages[0])
			h.messages[0] = Message{}
			h.messages = h.messages[1:]
		}
		h.truncated = true
	}
}

func (h *History) Snapshot() ConversationSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ConversationSnapshot{ID: h.id, Messages: cloneMessages(h.messages), Truncated: h.truncated}
}

func cloneMessages(messages []Message) []Message {
	copy := append([]Message{}, messages...)
	for i := range copy {
		copy[i].References = append([]PageReference(nil), copy[i].References...)
	}
	return copy
}

func (h *History) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id = uuid.NewString()
	h.messages = nil
	h.bytes = 0
	h.truncated = false
}

// Checkpoint projects observed live output without finalizing the in-memory turn.
func (h *History) Checkpoint(state State) ConversationSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	copy := &History{id: h.id, messages: cloneMessages(h.messages), bytes: h.bytes, truncated: h.truncated}
	if len(copy.messages) > 0 && copy.messages[len(copy.messages)-1].ID == state.TurnID+":user" {
		for _, message := range state.Messages {
			copy.append(message)
		}
		if len(state.Messages) == 0 && state.Output != "" {
			copy.append(Message{ID: state.TurnID + ":partial", TurnID: state.TurnID, Role: "ai", Kind: "text", Content: state.Output})
		}
	}
	return ConversationSnapshot{ID: copy.id, Messages: copy.messages, Truncated: copy.truncated}
}

func (h *History) Restore(snapshot ConversationSnapshot) error {
	if err := validateHistory(snapshot); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id, h.messages, h.truncated = snapshot.ID, cloneMessages(snapshot.Messages), snapshot.Truncated
	h.bytes = 0
	for _, m := range h.messages {
		h.bytes += messageBytes(m)
	}
	return nil
}

func (h *History) Note(id, content string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.append(Message{ID: id, TurnID: id, Role: "ai", Kind: "notice", Phase: "stopped", Content: content})
}
