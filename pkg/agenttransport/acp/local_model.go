package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// LocalModel contains only a model snapshot and its referenced credentials.
// It is never returned to the frontend, logged or persisted in app settings.
// Strings keep ManagedConfig comparable for conversation configuration checks.
type LocalModel struct {
	Profile     string
	Environment string
	Name        string
	ID          string
	ModelID     string
	Provider    string
}

// LocalModelOption is the only part of a user model profile exposed to the UI.
type LocalModelOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ModelID  string `json:"modelId"`
	Provider string `json:"provider"`
}

// ListLocalModels does not resolve credentials or return endpoints or profiles.
func ListLocalModels(directory string) ([]LocalModelOption, error) {
	profiles, err := readLocalProfiles(directory)
	if err != nil {
		return nil, err
	}
	options := make([]LocalModelOption, 0, len(profiles))
	seen := map[string]bool{}
	for _, profile := range profiles {
		id := stringValue(profile["id"])
		if seen[id] {
			return nil, errors.New("iCode 中存在重复的模型 ID，请先修正模型配置")
		}
		seen[id] = true
		options = append(options, LocalModelOption{ID: id, Name: stringValue(profile["name"]), ModelID: stringValue(profile["model_id"]), Provider: stringValue(profile["provider"])})
	}
	return options, nil
}

func LocalModelDirectory() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "chrys")
	}
	return filepath.Join(home, ".chrys")
}

// LoadLocalModel reads the same user-level active pointer as iCode. No agent,
// hook, history, grant or global settings document is copied into the child.
func LoadLocalModel(directory string) (LocalModel, error) {
	return loadLocalModel(directory, "")
}

func LoadLocalModelByID(directory, id string) (LocalModel, error) {
	if id == "" {
		return LocalModel{}, errors.New("请选择 iCode 中已配置的模型")
	}
	return loadLocalModel(directory, id)
}

func loadLocalModel(directory, id string) (LocalModel, error) {
	settings := map[string]any{}
	if data, err := readModelFile(filepath.Join(directory, "settings.yaml")); err == nil {
		if yaml.Unmarshal(data, &settings) != nil {
			return LocalModel{}, errors.New("iCode 模型设置无法解析，请在 iCode 中重新保存默认模型")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalModel{}, errors.New("无法读取本机 iCode 模型设置")
	}
	variables := map[string]string{}
	if data, err := readModelFile(filepath.Join(directory, ".env")); err == nil {
		variables, err = godotenv.Unmarshal(strings.TrimPrefix(string(data), "\ufeff"))
		if err != nil {
			return LocalModel{}, errors.New("iCode 的模型环境配置无法解析，请检查其 .env 文件")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return LocalModel{}, errors.New("无法读取 iCode 的模型环境配置")
	}
	lookup := func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		return variables[name]
	}
	selector := lookup("CHRYS_MODEL_PROFILE")
	if selector == "" {
		selector = modelSetting(settings, "model.profile.active")
	}
	if id != "" {
		selector = id
	}
	profiles, err := readLocalProfiles(directory)
	if err != nil {
		return LocalModel{}, err
	}
	var selected map[string]any
	if id != "" {
		for _, profile := range profiles {
			if stringValue(profile["id"]) == id {
				if selected != nil {
					return LocalModel{}, errors.New("iCode 中存在重复的模型 ID，请先修正模型配置")
				}
				selected = profile
			}
		}
	} else {
		selected = selectLocalModel(profiles, selector)
	}
	if selected == nil {
		return LocalModel{}, errors.New("无法确定 iCode 模型，请在 iCode 中检查所选模型或默认模型配置")
	}
	return prepareLocalModel(selected, lookup)
}

func readLocalProfiles(directory string) ([]map[string]any, error) {
	entries, err := os.ReadDir(filepath.Join(directory, "models"))
	if err != nil {
		return nil, errors.New("未找到本机 iCode 的模型配置，请先在 iCode 中配置模型")
	}
	var profiles []map[string]any
	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yaml" && filepath.Ext(entry.Name()) != ".yml") {
			continue
		}
		data, err := readModelFile(filepath.Join(directory, "models", entry.Name()))
		if err != nil {
			return nil, errors.New("无法读取 iCode 模型文件")
		}
		profile := map[string]any{}
		if yaml.Unmarshal(data, &profile) != nil {
			continue
		} // iCode also skips malformed profiles.
		if stringValue(profile["id"]) == "" {
			profile["id"] = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if stringValue(profile["model_id"]) != "" && stringValue(profile["name"]) != "" {
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}

func prepareLocalModel(selected map[string]any, lookup func(string) string) (LocalModel, error) {
	name := stringValue(selected["name"])
	id, modelID := stringValue(selected["id"]), stringValue(selected["model_id"])
	// Provider environment fallbacks are materialized into this model, rather
	// than importing the user's entire environment or its policy overrides.
	provider := stringValue(selected["provider"])
	if provider == "" {
		provider = "openai"
	}
	prefix := map[string]string{"openai": "OPENAI", "anthropic": "ANTHROPIC", "deepseek-openai": "DEEPSEEK", "glm-openai": "ZAI"}[provider]
	if prefix != "" {
		if stringValue(selected["api_key"]) == "" {
			selected["api_key"] = lookup(prefix + "_API_KEY")
		}
		if stringValue(selected["base_url"]) == "" {
			selected["base_url"] = lookup(prefix + "_BASE_URL")
		}
	}
	environment := map[string]string{}
	for _, field := range []string{"api_key", "http_headers", "chat_options"} {
		value := stringValue(selected[field])
		value = strings.ReplaceAll(strings.ReplaceAll(value, "{{{{", "\x00OPEN\x00"), "}}}}", "\x00CLOSE\x00")
		var missing bool
		// Rebind references to generated names, so a credential named HOME or
		// CHRYS_* cannot alter the child's isolation or approval configuration.
		value = modelEnvReference.ReplaceAllStringFunc(value, func(reference string) string {
			key := modelEnvReference.FindStringSubmatch(reference)[1]
			secret := lookup(key)
			if secret == "" {
				missing = true
			}
			variable := fmt.Sprintf("OPSCOPILOT_IMPORTED_MODEL_%d", len(environment))
			environment[variable] = secret
			return "{{" + variable + "}}"
		})
		if missing {
			return LocalModel{}, errors.New("iCode 模型引用的认证或请求参数环境变量未设置，请检查 iCode 的模型配置")
		}
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\x00OPEN\x00", "{{{{"), "\x00CLOSE\x00", "}}}}")
		selected[field] = value
	}
	if key := stringValue(selected["api_key"]); key != "" && !strings.Contains(key, "{{") {
		environment["OPSCOPILOT_IMPORTED_MODEL_KEY"] = key
		selected["api_key"] = "{{OPSCOPILOT_IMPORTED_MODEL_KEY}}"
	}
	selected["id"], selected["name"] = "opscopilot", "OpsCopilot"
	// Host retries must not replay operations with unknown outcomes.
	selected["http_max_retries"] = 0
	encoded, err := json.Marshal(selected)
	if err != nil {
		return LocalModel{}, errors.New("iCode 模型配置无法载入")
	}
	env, _ := json.Marshal(environment)
	return LocalModel{Profile: string(encoded), Environment: string(env), Name: name, ID: id, ModelID: modelID, Provider: provider}, nil
}

var modelEnvReference = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)

func stringValue(value any) string { result, _ := value.(string); return result }

func modelSetting(settings map[string]any, key string) string {
	if value, ok := settings[key]; ok {
		return stringValue(value)
	}
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 2 {
		if child, ok := settings[parts[0]].(map[string]any); ok {
			return modelSetting(child, parts[1])
		}
	}
	return ""
}

func selectLocalModel(profiles []map[string]any, selector string) map[string]any {
	if selector == "" {
		return nil
	} // Never silently pick a different, potentially billed endpoint.
	for _, profile := range profiles {
		if stringValue(profile["id"]) == selector {
			return profile
		}
	}
	for _, folded := range []bool{false, true} {
		var matches []map[string]any
		for _, profile := range profiles {
			name := stringValue(profile["name"])
			if name == selector || (folded && strings.EqualFold(name, selector)) {
				matches = append(matches, profile)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
		if len(matches) > 1 {
			return nil
		}
	}
	return nil
}

func readModelFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const limit = 2 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(data) > limit {
		return nil, errors.New("模型配置文件过大")
	}
	return data, err
}
