package assistantcontrol

import (
	"strings"
	"sync"

	"github.com/google/uuid"
)

const historyMaxBytes = 4 * 1024 * 1024
const historyMaxMessages = 128

// Message is display history, never an approval or an instruction to execute.
type Message struct {
	ID      string `json:"id"`
	TurnID  string `json:"turnId"`
	Role    string `json:"role"`
	Content string `json:"content"`
	Phase   string `json:"phase,omitempty"`
}

type ConversationSnapshot struct {
	ID        string    `json:"id"`
	Messages  []Message `json:"messages"`
	Truncated bool      `json:"truncated"`
}

// History lives only as long as the application process/current conversation.
// It deliberately stores no queued messages, credentials or approval payloads.
type History struct {
	mu        sync.Mutex
	id        string
	messages  []Message
	bytes     int
	truncated bool
}

func (h *History) Begin(turnID, question string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.id == "" {
		h.id = uuid.NewString()
	}
	h.append(Message{ID: turnID + ":user", TurnID: turnID, Role: "user", Content: question})
}

func (h *History) Finish(state State) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Only finalize the matching accepted user message, at most once.
	if len(h.messages) == 0 || h.messages[len(h.messages)-1].ID != state.TurnID+":user" {
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
	h.messages = append(h.messages, message)
	h.bytes += len(message.Content)
	// Evict whole old turns so a retained answer keeps its question. The newest
	// turn stays intact; live output already has the coordinator's per-turn cap.
	for (len(h.messages) > historyMaxMessages || h.bytes > historyMaxBytes) && len(h.messages) > 2 {
		old := h.messages[0].TurnID
		for len(h.messages) > 0 && h.messages[0].TurnID == old {
			h.bytes -= len(h.messages[0].Content)
			h.messages[0] = Message{}
			h.messages = h.messages[1:]
		}
		h.truncated = true
	}
}

func (h *History) Snapshot() ConversationSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ConversationSnapshot{ID: h.id, Messages: append([]Message{}, h.messages...), Truncated: h.truncated}
}

func (h *History) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id = uuid.NewString()
	h.messages = nil
	h.bytes = 0
	h.truncated = false
}
