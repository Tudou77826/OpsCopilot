package acp

import (
	"encoding/json"
	"strings"
	"sync"
)

// SessionOption and AgentCommand are presentation metadata supplied by an
// ACP agent. They never grant permission or cause a command to execute.
type SessionOption struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Category string          `json:"category"`
	Current  string          `json:"current"`
	Choices  []SessionChoice `json:"choices"`
}
type SessionChoice struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}
type AgentCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Unavailable string `json:"unavailable,omitempty"`
}
type SessionFeatures struct {
	Options  []SessionOption `json:"options"`
	Commands []AgentCommand  `json:"commands"`
}
type sessionFeatureStore struct {
	mu          sync.RWMutex
	features    SessionFeatures
	local       []AgentCommand
	unavailable []AgentCommand
}

func (c *Conversation) SessionFeatures() SessionFeatures { return c.features.snapshot() }

// AgentCommandMatches recognizes only advertised commands, so ordinary user
// messages beginning with a slash keep the usual page context.
func (c *Conversation) AgentCommandMatches(text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return false
	}
	name := strings.TrimPrefix(parts[0], "/")
	for _, command := range c.features.snapshot().Commands {
		if command.Name == name {
			return true
		}
	}
	return false
}

func (s *sessionFeatureStore) snapshot() SessionFeatures {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := SessionFeatures{Commands: append([]AgentCommand(nil), s.features.Commands...)}
	seen := make(map[string]bool, len(result.Commands))
	for _, command := range result.Commands {
		seen[command.Name] = true
	}
	for _, command := range s.local {
		if !seen[command.Name] {
			result.Commands = append(result.Commands, command)
		}
	}
	result.Commands = append(result.Commands, s.unavailable...)
	for _, o := range s.features.Options {
		o.Choices = append([]SessionChoice(nil), o.Choices...)
		result.Options = append(result.Options, o)
	}
	return result
}

func (s *sessionFeatureStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.features = SessionFeatures{}
	s.local = nil
	s.unavailable = nil
}

func (s *sessionFeatureStore) setLocalCommands(commands []AgentCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.local = append([]AgentCommand(nil), commands...)
}

func (s *sessionFeatureStore) setUnavailableCommands(commands []AgentCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unavailable = append([]AgentCommand(nil), commands...)
}

// update accepts a session/new result or a standard session/update. Limits
// prevent a third-party agent from causing unbounded UI allocations.
func (s *sessionFeatureStore) update(raw json.RawMessage) {
	if len(raw) > 1<<20 {
		return
	}
	var envelope struct {
		ConfigOptions json.RawMessage `json:"configOptions"`
		Update        struct {
			Type              string          `json:"sessionUpdate"`
			ConfigOptions     json.RawMessage `json:"configOptions"`
			AvailableCommands []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"availableCommands"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return
	}
	options := envelope.ConfigOptions
	if envelope.Update.Type == "config_option_update" {
		options = envelope.Update.ConfigOptions
	}
	if len(options) > 0 && string(options) != "null" {
		var items []struct {
			ID, Name, Category, Type, CurrentValue string
			Options                                []struct{ Value, Name string } `json:"options"`
		}
		if json.Unmarshal(options, &items) == nil && len(items) <= 32 {
			var result []SessionOption
			for _, item := range items {
				if item.Type != "select" || item.ID == "" || len(item.ID) > 128 || len(item.Options) > 512 {
					continue
				}
				// CodeAgent sends model/mode options keyed only by id (no
				// category/name); the reference client backfills both before
				// consumers filter on category. Same backfill here.
				category := boundedFeatureText(item.Category, 64)
				name := boundedFeatureText(item.Name, 256)
				if category == "" && (item.ID == "model" || item.ID == "mode") {
					category = item.ID
					if name == "" {
						if item.ID == "model" {
							name = "模型"
						} else {
							name = "模式"
						}
					}
				}
				option := SessionOption{ID: item.ID, Name: name, Category: category, Current: boundedFeatureText(item.CurrentValue, 256)}
				for _, choice := range item.Options {
					if choice.Value != "" && len(choice.Value) <= 256 {
						option.Choices = append(option.Choices, SessionChoice{Value: choice.Value, Name: boundedFeatureText(choice.Name, 256)})
					}
				}
				result = append(result, option)
			}
			s.mu.Lock()
			s.features.Options = result
			s.mu.Unlock()
		}
	}
	if envelope.Update.Type == "available_commands_update" && len(envelope.Update.AvailableCommands) <= 512 {
		var commands []AgentCommand
		seen := map[string]bool{}
		for _, item := range envelope.Update.AvailableCommands {
			name := strings.TrimSpace(item.Name)
			if !validAgentCommandName(name) || seen[name] {
				continue
			}
			seen[name] = true
			commands = append(commands, AgentCommand{Name: name, Description: boundedFeatureText(item.Description, 512)})
		}
		s.mu.Lock()
		s.features.Commands = commands
		s.mu.Unlock()
	}
}

func boundedFeatureText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return ""
	}
	return s
}

func validAgentCommandName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, ch := range name {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}
