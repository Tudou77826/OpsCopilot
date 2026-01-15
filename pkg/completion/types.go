package completion

// Command represents a Linux command with metadata
type Command struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`        // Chinese description
	Category    string          `json:"category,omitempty"` // e.g., "file", "network", "system"
	Options     []CommandOption `json:"options,omitempty"`
	Combos      []CommandCombo  `json:"combos,omitempty"`
	Examples    []string        `json:"examples,omitempty"`
	Aliases     []string        `json:"aliases,omitempty"`
}

// CommandCombo represents a commonly used combination of flags/options
type CommandCombo struct {
	Text        string `json:"text"`                  // e.g., "-rni", "-cxvf"
	Description string `json:"description,omitempty"` // Chinese description
}

// CommandOption represents a flag or parameter
type CommandOption struct {
	Flag              string   `json:"flag"`                 // e.g., "-l", "--all", "-v"
	ShortFlag         string   `json:"short_flag,omitempty"` // e.g., "-a" for "--all"
	Description       string   `json:"description"`          // Chinese description
	RequiresValue     bool     `json:"requires_value,omitempty"`
	MutuallyExclusive []string `json:"mutually_exclusive,omitempty"`
}

// CompletionRequest - Frontend request for suggestions
type CompletionRequest struct {
	Input  string `json:"input"`  // Current command line
	Cursor int    `json:"cursor"` // Cursor position in input
}

// CompletionResponse - Backend response with suggestions
type CompletionResponse struct {
	Suggestions []CompletionSuggestion `json:"suggestions"`
	ReplaceFrom int                    `json:"replace_from"` // Start index for replacement
	ReplaceTo   int                    `json:"replace_to"`   // End index for replacement
}

// CompletionSuggestion represents a single suggestion
type CompletionSuggestion struct {
	Type        string `json:"type"`         // "command" | "option" | "argument"
	Text        string `json:"text"`         // The text to insert
	DisplayText string `json:"display_text"` // Text to show in UI (may include description)
	Description string `json:"description"`  // Detailed description (Chinese)
}
