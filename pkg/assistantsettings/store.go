// Package assistantsettings owns private assistant preferences independently of growth and public profiles.
package assistantsettings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"opscopilot/pkg/filetxn"
	"strings"
	"unicode/utf8"
)

const MaxPersonaLength = 4000
const DefaultPersona = "你是陪伴用户日常工作的运维伙伴。态度温和、可靠，表达自然、直接、简洁。认真回应用户的问题，根据需要解释原因和下一步。可以适度亲切，但不要刻意卖萌、使用固定口头禅或自称无所不能。"

type Snapshot struct {
	PersonaPrompt string `json:"personaPrompt"`
	DefaultPrompt string `json:"defaultPrompt"`
	Revision      string `json:"revision"`
}
type Store struct{ path string }

func New(path string) *Store      { return &Store{path: path} }
func revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func decode(data []byte) (Snapshot, error) {
	var file struct {
		PersonaPrompt string `json:"personaPrompt"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &file); err != nil {
			return Snapshot{}, fmt.Errorf("读取伙伴人设失败: %w", err)
		}
	}
	if !utf8.ValidString(file.PersonaPrompt) || utf8.RuneCountInString(file.PersonaPrompt) > MaxPersonaLength {
		return Snapshot{}, fmt.Errorf("伙伴人设超过 %d 字或包含无效字符", MaxPersonaLength)
	}
	return Snapshot{PersonaPrompt: file.PersonaPrompt, DefaultPrompt: DefaultPersona, Revision: revision(data)}, nil
}
func (s *Store) Read() (Snapshot, error) {
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	data, err := filetxn.Read(s.path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取伙伴人设失败: %w", err)
	}
	return decode(data)
}
func (s *Store) Save(prompt, expectedRevision string) (Snapshot, error) {
	prompt = strings.TrimSpace(prompt)
	if !utf8.ValidString(prompt) || utf8.RuneCountInString(prompt) > MaxPersonaLength {
		return Snapshot{}, fmt.Errorf("伙伴人设最多 %d 字", MaxPersonaLength)
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	defer release()
	current, err := filetxn.Read(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	if _, err = decode(current); err != nil {
		return Snapshot{}, err
	}
	if expectedRevision != revision(current) {
		return Snapshot{}, filetxn.ErrConflict
	}
	data, err := json.Marshal(struct {
		PersonaPrompt string `json:"personaPrompt"`
	}{prompt})
	if err != nil {
		return Snapshot{}, err
	}
	if err = filetxn.Write(s.path, data); err != nil {
		return Snapshot{}, fmt.Errorf("保存伙伴人设失败: %w", err)
	}
	return decode(data)
}
func (s Snapshot) EffectivePersona() string {
	if strings.TrimSpace(s.PersonaPrompt) == "" {
		if s.DefaultPrompt != "" {
			return s.DefaultPrompt
		}
		return DefaultPersona
	}
	return s.PersonaPrompt
}
