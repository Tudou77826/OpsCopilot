package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"opscopilot/pkg/llm"
)

// LLMChecker LLM 命令风险检查器
type LLMChecker struct {
	provider llm.Provider
}

// NewLLMChecker 创建 LLM 检查器
func NewLLMChecker(provider llm.Provider) *LLMChecker {
	return &LLMChecker{
		provider: provider,
	}
}

// AssessCommand 评估命令风险
func (c *LLMChecker) AssessCommand(ctx context.Context, command string) (*RiskAssessment, error) {
	if c.provider == nil {
		return nil, fmt.Errorf("LLM provider 未初始化")
	}

	prompt := buildRiskAssessmentPrompt(command)

	messages := []llm.ChatMessage{
		{
			Role:    "system",
			Content: getRiskAssessmentSystemPrompt(),
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	response, err := c.provider.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	// 解析 JSON 响应
	assessment, err := parseRiskAssessment(response)
	if err != nil {
		// 解析失败时返回默认评估
		return &RiskAssessment{
			IsRisky:   true,
			RiskLevel: RiskMedium,
			Reason:    "无法解析命令风险，默认视为中风险",
		}, nil
	}

	return assessment, nil
}

// getRiskAssessmentSystemPrompt 返回系统提示词
func getRiskAssessmentSystemPrompt() string {
	return `你是一个 Linux 命令安全分析专家，负责评估远程执行命令的风险。

请分析用户提供的命令，判断其风险等级并给出中文说明。

风险判定标准：
- low（低风险）：只读命令，无副作用，不修改系统状态
  例如：ls, cat, ps, top, free, df, netstat, journalctl

- medium（中风险）：临时性修改，可恢复，影响范围有限
  例如：systemctl restart, docker restart, service restart

- high（高风险）：永久性变更，不可逆，可能影响系统稳定性
  例如：rm, dd, mkfs, fdisk, chmod, chown（系统目录）, iptables, firewall-cmd

你需要以 JSON 格式输出评估结果，格式如下：
{
  "is_risky": boolean,        // true 表示有风险（medium 或 high）
  "risk_level": "low|medium|high",
  "reason": "中文说明风险原因",
  "suggestions": "如果有更安全的替代方案，用中文说明"
}

只输出 JSON，不要输出其他内容。`
}

// buildRiskAssessmentPrompt 构建评估提示
func buildRiskAssessmentPrompt(command string) string {
	return fmt.Sprintf("请评估以下 Linux 命令的执行风险：\n\n%s", command)
}

// parseRiskAssessment 解析 LLM 返回的 JSON
func parseRiskAssessment(response string) (*RiskAssessment, error) {
	// 尝试提取 JSON（可能包含 markdown 代码块）
	jsonStr := extractJSON(response)

	var assessment RiskAssessment
	if err := json.Unmarshal([]byte(jsonStr), &assessment); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	// 验证风险等级
	switch assessment.RiskLevel {
	case RiskLow, RiskMedium, RiskHigh:
		// 有效
	default:
		assessment.RiskLevel = RiskMedium
	}

	return &assessment, nil
}

// extractJSON 从响应中提取 JSON（处理可能的 markdown 代码块）
func extractJSON(response string) string {
	// 去除可能的 markdown 代码块标记
	response = trimCodeBlock(response)
	return response
}

// trimCodeBlock 去除 markdown 代码块标记
func trimCodeBlock(s string) string {
	// 去除开头的 ```json 或 ```
	if len(s) > 7 && (s[:7] == "```json\n" || s[:7] == "```JSON\n") {
		s = s[7:]
	} else if len(s) > 4 && s[:4] == "```\n" {
		s = s[4:]
	} else if len(s) > 3 && s[:3] == "```" {
		s = s[3:]
		if len(s) > 0 && s[0] == '\n' {
			s = s[1:]
		}
	}

	// 去除结尾的 ```
	if len(s) > 3 && s[len(s)-3:] == "```" {
		s = s[:len(s)-3]
	}

	return s
}
