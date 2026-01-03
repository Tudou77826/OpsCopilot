package config

import (
	"os"
)

type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func LoadLLMConfig() *LLMConfig {
	// 优先从环境变量读取，后续可以增加从文件读取
	// 默认使用 DeepSeek (兼容 OpenAI 协议)
	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	model := os.Getenv("LLM_MODEL")

	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	if model == "" {
		model = "deepseek-chat"
	}

	return &LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}
}
