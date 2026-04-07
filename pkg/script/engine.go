package script

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// PlaybackContext 运行时上下文
type PlaybackContext struct {
	Variables map[string]string
	Cancelled bool
}

// NewPlaybackContext 创建运行时上下文
func NewPlaybackContext(varValues map[string]string) *PlaybackContext {
	return &PlaybackContext{
		Variables: varValues,
	}
}

// varRegex matches ${var_name} patterns
var varRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// SubstituteVariables 替换命令中的变量 ${var_name}
func SubstituteVariables(template string, vars map[string]string) string {
	if vars == nil {
		return template
	}
	return varRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := vars[varName]; ok {
			return val
		}
		return match
	})
}

// ExecuteSteps 执行步骤列表
func ExecuteSteps(steps []ScriptStep, ctx *PlaybackContext, sender CommandSender, sessionID string) error {
	for i := range steps {
		if ctx.Cancelled {
			return fmt.Errorf("playback cancelled")
		}

		step := &steps[i]
		if !step.Enabled {
			log.Printf("[Engine] Skipping disabled command: %s", step.Command)
			continue
		}

		if step.Delay > 0 {
			time.Sleep(time.Duration(step.Delay) * time.Millisecond)
		}

		command := SubstituteVariables(step.Command, ctx.Variables)
		log.Printf("[Engine] Executing command: %s", command)

		if err := sender.SendCommand(sessionID, command+"\n"); err != nil {
			return fmt.Errorf("failed to execute command '%s': %w", step.Command, err)
		}

		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// ExportStepsToBash 将步骤导出为 bash 脚本
func ExportStepsToBash(steps []ScriptStep, sb *strings.Builder) {
	for i := range steps {
		step := &steps[i]
		if !step.Enabled {
			sb.WriteString(fmt.Sprintf("# %s (disabled)\n", step.Command))
			continue
		}

		if step.Comment != "" {
			sb.WriteString(fmt.Sprintf("# %s\n", step.Comment))
		}

		sb.WriteString(fmt.Sprintf("%s\n", step.Command))

		if step.Delay > 0 {
			sb.WriteString(fmt.Sprintf("sleep %g\n", float64(step.Delay)/1000))
		}
	}
}
