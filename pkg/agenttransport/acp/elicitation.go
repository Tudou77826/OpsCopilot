package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"opscopilot/pkg/assistantcontrol"
)

// Standard ACP elicitation/create is answered only from explicit user input.
// URL-bearing schemas are cancelled outright, unknown shapes are cancelled
// rather than auto-answered, and the response carries nothing the user did not
// type or select.
const (
	maxElicitationQuestions = 8
	maxElicitationOptions   = 64
	maxElicitationNote      = 2048
	maxElicitationText      = 4096
)

type elicitationOption struct {
	Value       string
	Label       string
	Description string
}

type elicitationQuestion struct {
	Key      string
	Question string
	Options  []elicitationOption
	Multiple bool
	Textual  bool // free-form text is part of the answer space
	Boolean  bool // answers convert back to real JSON booleans
	Required bool
}

// elicitation answers one elicitation/create request. ask presents a single
// question card, mirroring the bounded request helper used by permissions.
func (b *Interactions) elicitation(ctx context.Context, data json.RawMessage, ask func(assistantcontrol.Prompt) (assistantcontrol.Answer, error)) (any, error) {
	if b.DenyRequests {
		return map[string]any{"action": "cancel"}, nil
	}
	if b.Audit != nil {
		if err := b.Audit("elicitation", "requested", data); err != nil {
			return nil, err
		}
	}
	action, content := b.elicitationAnswer(ctx, data, ask)
	if action != "accept" && b.Audit != nil {
		_ = b.Audit("elicitation", "cancelled", data)
	}
	if action != "accept" {
		return map[string]any{"action": "cancel"}, nil
	}
	if b.Audit != nil {
		if err := b.Audit("elicitation", "answered", data); err != nil {
			return nil, err
		}
	}
	return map[string]any{"action": "accept", "content": content}, nil
}

func (b *Interactions) elicitationAnswer(ctx context.Context, data json.RawMessage, ask func(assistantcontrol.Prompt) (assistantcontrol.Answer, error)) (string, any) {
	var request struct {
		Message         string          `json:"message"`
		RequestedSchema json.RawMessage `json:"requestedSchema"`
	}
	if json.Unmarshal(data, &request) != nil || len(request.RequestedSchema) == 0 {
		return "cancel", nil
	}
	var root struct {
		Type       string            `json:"type"`
		Mode       string            `json:"mode"`
		URL        string            `json:"url"`
		Properties json.RawMessage   `json:"properties"`
		Required   []string          `json:"required"`
		OneOf      []json.RawMessage `json:"oneOf"`
	}
	if json.Unmarshal(request.RequestedSchema, &root) != nil {
		return "cancel", nil
	}
	// URL elicitation asks the client to open an address; it is never honored.
	if root.URL != "" || root.Mode != "" && root.Mode != "form" {
		return "cancel", nil
	}
	keys, fields, ok := orderedProperties(root.Properties)
	if !ok || len(keys) == 0 || len(keys) > maxElicitationQuestions {
		return "cancel", nil
	}
	required := map[string]bool{}
	for _, key := range root.Required {
		required[key] = true
	}
	content := map[string]any{}
	for index, key := range keys {
		if key == "url" || len(fields[index]) > 32<<10 {
			return "cancel", nil
		}
		question, ok := parseElicitationField(key, fields[index], required[key])
		if !ok || len(question.Options) > maxElicitationOptions {
			return "cancel", nil
		}
		value, answered := b.askElicitationQuestion(ctx, index, len(keys), question, ask)
		if !answered {
			if required[key] {
				// A required question without an answer invalidates the whole form;
				// never submit partial answers to an agent expecting complete data.
				return "cancel", nil
			}
			continue
		}
		content[key] = value
	}
	if len(content) == 0 {
		return "cancel", nil
	}
	return "accept", content
}

// parseElicitationField normalizes both conventional JSON-schema forms and the
// CodeAgent shape (question text in the field description, option explanations
// in option titles, real values in const entries) into one question model.
func parseElicitationField(key string, raw json.RawMessage, required bool) (elicitationQuestion, bool) {
	var field struct {
		Type        string            `json:"type"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Enum        []json.RawMessage `json:"enum"`
		EnumNames   map[string]string `json:"enumNames"`
		OneOf       []json.RawMessage `json:"oneOf"`
		AnyOf       []json.RawMessage `json:"anyOf"`
		Items       *struct {
			Type  string            `json:"type"`
			Enum  []json.RawMessage `json:"enum"`
			OneOf []json.RawMessage `json:"oneOf"`
			AnyOf []json.RawMessage `json:"anyOf"`
		} `json:"items"`
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &field) != nil || field.URL != "" {
		return elicitationQuestion{}, false
	}
	question := elicitationQuestion{Key: key, Required: required}
	// CodeAgent forms put the real question in description; conventional forms
	// put it in title. Prefer whichever reads like a sentence.
	question.Question = strings.TrimSpace(field.Title)
	if question.Question == "" || question.Question == key {
		question.Question = strings.TrimSpace(field.Description)
	}
	if question.Question == "" {
		question.Question = key
	}
	if len(question.Question) > 512 {
		return elicitationQuestion{}, false
	}
	optionSources := field.OneOf
	if len(optionSources) == 0 {
		optionSources = field.AnyOf
	}
	enumerated := field.Enum
	if len(optionSources) == 0 && field.Items != nil {
		// Array fields carry their choices on the items schema.
		if field.Items.OneOf != nil {
			optionSources = field.Items.OneOf
		} else if field.Items.AnyOf != nil {
			optionSources = field.Items.AnyOf
		} else if field.Items.Enum != nil {
			enumerated = field.Items.Enum
		}
	}
	for _, rawOption := range optionSources {
		option, ok := parseElicitationOption(rawOption)
		if !ok || option.Value == "" || len(option.Value) > 512 {
			return elicitationQuestion{}, false
		}
		question.Options = append(question.Options, option)
	}
	for _, rawValue := range enumerated {
		value, ok := decodeElicitationValue(rawValue)
		if !ok || value == "" || len(value) > 512 {
			return elicitationQuestion{}, false
		}
		option := elicitationOption{Value: value, Label: value}
		if name, found := field.EnumNames[value]; found && name != "" {
			option.Label = name
		}
		question.Options = append(question.Options, option)
	}
	switch {
	case len(question.Options) > 0:
		question.Multiple = field.Type == "array"
		// String/array choice fields accept a free-form supplement; a bare
		// boolean or unknown-type enum is constrained to its options.
		question.Textual = field.Type == "string" || field.Type == "array"
	case field.Type == "boolean":
		question.Boolean = true
		question.Options = []elicitationOption{{Value: "true", Label: "是"}, {Value: "false", Label: "否"}}
	case field.Type == "string" || field.Type == "array" || field.Type == "":
		question.Textual = true
	default:
		// Numbers and exotic schemas have no safe bounded input shape here.
		return elicitationQuestion{}, false
	}
	return question, true
}

func parseElicitationOption(raw json.RawMessage) (elicitationOption, bool) {
	var option struct {
		Const       json.RawMessage   `json:"const"`
		Enum        []json.RawMessage `json:"enum"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
	}
	if json.Unmarshal(raw, &option) != nil {
		return elicitationOption{}, false
	}
	value, ok := decodeElicitationValue(option.Const)
	if !ok && len(option.Enum) == 1 {
		value, ok = decodeElicitationValue(option.Enum[0])
	}
	if !ok {
		return elicitationOption{}, false
	}
	label := strings.TrimSpace(option.Title)
	if label == "" || label == value {
		label = value
	}
	if len(label) > 512 {
		return elicitationOption{}, false
	}
	return elicitationOption{Value: value, Label: label, Description: strings.TrimSpace(option.Description)}, true
}

func decodeElicitationValue(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		if typed {
			return "true", true
		}
		return "false", true
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%f", typed), "0"), "."), true
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

// askElicitationQuestion presents one bounded card and merges an optional
// written supplement into the structured answer.
func (b *Interactions) askElicitationQuestion(ctx context.Context, index, total int, question elicitationQuestion, ask func(assistantcontrol.Prompt) (assistantcontrol.Answer, error)) (any, bool) {
	description := ""
	if total > 1 {
		description = fmt.Sprintf("问题 %d / %d", index+1, total)
	}
	choices := make([]assistantcontrol.Choice, 0, len(question.Options))
	for i, option := range question.Options {
		label := option.Label
		if option.Description != "" && option.Description != option.Label {
			label += " · " + option.Description
		}
		choices = append(choices, assistantcontrol.Choice{ID: fmt.Sprintf("%d", i), Label: label})
	}
	prompt := assistantcontrol.Prompt{Kind: "input", Title: question.Question, Description: description, Choices: choices, Multiple: question.Multiple}
	answer, err := ask(prompt)
	if err != nil {
		return nil, false
	}
	values := make([]string, 0, len(answer.Selected))
	for _, id := range answer.Selected {
		number, valid := elicitationChoiceIndex(id, len(question.Options))
		if !valid {
			return nil, false
		}
		values = append(values, question.Options[number].Value)
	}
	note := strings.TrimSpace(answer.Text)
	if len(note) > maxElicitationNote {
		return nil, false
	}
	switch {
	case len(values) > 0 && note != "":
		if question.Multiple {
			values = append(values, note)
			return values, true
		}
		// Note merging only applies to string choice fields; booleans carry no
		// supplement because their constrained value space cannot hold one.
		return values[0] + "\n补充说明：" + note, true
	case len(values) > 0:
		if question.Multiple {
			return values, true
		}
		return question.answerValue(values[0]), true
	case note != "" && question.Textual:
		if len(note) > maxElicitationText {
			return nil, false
		}
		return note, true
	default:
		return nil, false
	}
}

// answerValue converts synthetic choice values (booleans) back to their JSON
// representation while enum strings pass through unchanged.
func (q elicitationQuestion) answerValue(value string) any {
	if !q.Boolean {
		return value
	}
	return value == "true"
}

// elicitationChoiceIndex resolves a choice ID that is always the decimal
// position generated above, bounded by the option count.
func elicitationChoiceIndex(id string, count int) (int, bool) {
	number := 0
	if _, err := fmt.Sscanf(id, "%d", &number); err != nil || number < 0 || number >= count {
		return 0, false
	}
	return number, true
}

// orderedProperties preserves JSON object member order, which maps lose, so
// questions render in the order the agent composed them.
func orderedProperties(raw json.RawMessage) ([]string, []json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, false
	}
	var keys []string
	var values []json.RawMessage
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, nil, false
		}
		key, ok := keyToken.(string)
		if !ok || len(keys) > maxElicitationQuestions {
			return nil, nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, false
		}
		keys = append(keys, key)
		values = append(values, value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, false
	}
	return keys, values, true
}
