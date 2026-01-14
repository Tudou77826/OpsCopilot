package completion

import (
	"strings"
	"unicode"
)

// Service provides command completion functionality
type Service struct {
	db *Database
}

// NewService creates a new completion service
func NewService(db *Database) *Service {
	return &Service{db: db}
}

// GetCompletions returns completion suggestions based on input
func (s *Service) GetCompletions(req CompletionRequest) (*CompletionResponse, error) {
	input := req.Input
	cursor := req.Cursor

	// Ensure cursor is within bounds
	if cursor > len(input) {
		cursor = len(input)
	}

	// Get the current word being typed
	beforeCursor := input[:cursor]
	currentWord, startIdx := s.getCurrentWord(beforeCursor)

	// Check if we're completing a command name or option
	if s.isAtCommandStart(beforeCursor) {
		// Completing command name
		return s.completeCommands(currentWord, startIdx, cursor)
	}

	// Completing command options
	return s.completeOptions(beforeCursor, currentWord, startIdx, cursor)
}

// getCurrentWord extracts the word being typed and its start position
func (s *Service) getCurrentWord(beforeCursor string) (word string, startIdx int) {
	// Find the start of the current word
	i := len(beforeCursor) - 1
	for i >= 0 && !unicode.IsSpace(rune(beforeCursor[i])) {
		i--
	}

	startIdx = i + 1
	word = beforeCursor[startIdx:]
	return word, startIdx
}

// isAtCommandStart checks if cursor is at a position where a command name is expected
func (s *Service) isAtCommandStart(beforeCursor string) bool {
	trimmed := strings.TrimSpace(beforeCursor)

	// Empty or only whitespace - expecting command
	if trimmed == "" {
		return true
	}

	// Check if we just finished a command with pipe, semicolon, or logical operators
	if len(trimmed) > 0 {
		lastChar := trimmed[len(trimmed)-1]
		if lastChar == '|' || lastChar == ';' || lastChar == '&' || lastChar == '\n' {
			return true
		}
	}

	return false
}

// completeCommands provides command name completions
func (s *Service) completeCommands(prefix string, startIdx, cursorIdx int) (*CompletionResponse, error) {
	commands := s.db.FindCommands(prefix)

	var suggestions []CompletionSuggestion
	seen := make(map[string]bool)

	for _, cmd := range commands {
		// Avoid duplicates from aliases
		if seen[cmd.Name] {
			continue
		}
		seen[cmd.Name] = true

		suggestions = append(suggestions, CompletionSuggestion{
			Type:        "command",
			Text:        cmd.Name,
			DisplayText: cmd.Name,
			Description: cmd.Description,
		})

		// Add aliases as separate suggestions if they match prefix
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, prefix) && !seen[alias] {
				seen[alias] = true
				suggestions = append(suggestions, CompletionSuggestion{
					Type:        "command",
					Text:        alias,
					DisplayText: alias + " (别名)",
					Description: cmd.Description,
				})
			}
		}
	}

	return &CompletionResponse{
		Suggestions: suggestions,
		ReplaceFrom: startIdx,
		ReplaceTo:   cursorIdx,
	}, nil
}

// completeOptions provides command option completions
func (s *Service) completeOptions(beforeCursor, currentWord string, startIdx, cursorIdx int) (*CompletionResponse, error) {
	// Extract command name
	tokens := strings.Fields(beforeCursor)
	if len(tokens) == 0 {
		return s.completeCommands(currentWord, startIdx, cursorIdx)
	}

	commandName := tokens[0]
	cmd, exists := s.db.GetCommand(commandName)
	if !exists {
		// Command not in database, return empty
		return &CompletionResponse{
			Suggestions: []CompletionSuggestion{},
			ReplaceFrom: cursorIdx,
			ReplaceTo:   cursorIdx,
		}, nil
	}

	// If we're not typing a flag (currentWord doesn't start with -), suggest all options
	if !strings.HasPrefix(currentWord, "-") {
		var suggestions []CompletionSuggestion
		for _, opt := range cmd.Options {
			suggestions = append(suggestions, CompletionSuggestion{
				Type:        "option",
				Text:        " " + opt.Flag,
				DisplayText: opt.Flag + " " + opt.Description,
				Description: opt.Description,
			})
		}
		return &CompletionResponse{
			Suggestions: suggestions,
			ReplaceFrom: cursorIdx,
			ReplaceTo:   cursorIdx,
		}, nil
	}

	// We're typing a flag, complete it
	var suggestions []CompletionSuggestion
	for _, opt := range cmd.Options {
		flag := opt.Flag
		if strings.HasPrefix(flag, currentWord) {
			suggestions = append(suggestions, CompletionSuggestion{
				Type:        "option",
				Text:        flag,
				DisplayText: flag + " " + opt.Description,
				Description: opt.Description,
			})
		}

		// Check short flag
		if opt.ShortFlag != "" && strings.HasPrefix(opt.ShortFlag, currentWord) {
			suggestions = append(suggestions, CompletionSuggestion{
				Type:        "option",
				Text:        opt.ShortFlag,
				DisplayText: opt.ShortFlag + " " + opt.Description,
				Description: opt.Description,
			})
		}
	}

	return &CompletionResponse{
		Suggestions: suggestions,
		ReplaceFrom: startIdx,
		ReplaceTo:   cursorIdx,
	}, nil
}
