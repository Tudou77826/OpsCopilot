package config

import (
	"os"
)

type LLMConfig struct {
	APIKey       string `json:"APIKey"`
	BaseURL      string `json:"BaseURL"`
	FastModel    string `json:"FastModel"`
	ComplexModel string `json:"ComplexModel"`
	Model        string `json:"Model,omitempty"`
}

func LoadLLMConfig() *LLMConfig {
	// 优先从环境变量读取，后续可以增加从文件读取
	// 默认使用 DeepSeek (兼容 OpenAI 协议)
	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	fastModel := os.Getenv("LLM_FAST_MODEL")
	complexModel := os.Getenv("LLM_COMPLEX_MODEL")
	model := os.Getenv("LLM_MODEL")

	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	if fastModel == "" {
		if model != "" {
			fastModel = model
		} else {
			fastModel = "deepseek-chat"
		}
	}
	if complexModel == "" {
		complexModel = "glm46"
	}

	return &LLMConfig{
		APIKey:       apiKey,
		BaseURL:      baseURL,
		FastModel:    fastModel,
		ComplexModel: complexModel,
	}
}
