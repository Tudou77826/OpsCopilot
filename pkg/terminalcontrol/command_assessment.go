package terminalcontrol

import (
	"path"
	"strings"
)

// CommandAssessment is a bounded host-side classification, not a model's claim
// of safety. Unknown shell syntax remains subject to explicit human approval.
type CommandAssessment struct {
	ReadOnly bool
	Category string
	Impact   string
	Reason   string
}

func assessCommand(command string, target AccessTarget, directory string) CommandAssessment {
	if sensitivePath(command) || strings.Contains(command, "printenv") {
		return CommandAssessment{false, "可能读取敏感信息", "可能将凭据或敏感内容发送给模型", "敏感读取需要你确认具体目标，不能按普通只读操作放行"}
	}
	if simpleReadOnly(command, target) || (target.RoutineDiagnostics && routineReadOnly(command, directory)) {
		return CommandAssessment{true, "常规只读检查", "读取当前节点的信息，不修改文件或服务", "本轮已授权的只读排查，自动继续"}
	}
	result := CommandAssessment{false, "无法自动判定", "可能包含额外副作用，请核对完整命令", "未落入已验证的只读命令范围；这不代表已经判定为危险"}
	words, ok := literalWords(command)
	if !ok {
		result.Reason = "包含转义、管道、重定向、展开或复合语法，快速规则不能确认最终影响"
		return result
	}
	switch path.Base(words[0]) {
	case "rm", "rmdir", "unlink", "shred":
		return CommandAssessment{false, "删除数据", "删除文件或目录，可能无法恢复", "删除操作必须逐次明确批准"}
	case "reboot", "shutdown", "kill", "killall", "pkill", "systemctl", "service":
		return CommandAssessment{false, "进程或服务操作", "可能改变进程或服务状态，影响业务可用性", "涉及运行状态的操作需要人工核对目标和影响"}
	case "sudo", "su", "passwd", "chmod", "chown", "usermod", "useradd":
		return CommandAssessment{false, "身份或权限操作", "可能改变执行身份、权限或账号", "身份和权限变化不能按常规排查自动放行"}
	case "mv", "cp", "tee", "sed", "touch", "mkdir":
		return CommandAssessment{false, "可能修改文件", "可能写入、覆盖或改变文件状态", "快速规则不能确认这是纯读取，需确认具体参数"}
	}
	return result
}

func diagnosticPath(value, directory string) (string, bool) {
	if !path.IsAbs(value) {
		if !path.IsAbs(directory) {
			return "", false
		}
		value = path.Join(directory, value)
	}
	value = path.Clean(value)
	if sensitivePath(value) {
		return "", false
	}
	for _, prefix := range []string{"/proc", "/sys", "/dev"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return "", false
		}
	}
	return value, true
}

// Only literal files/directories and finite known flags are accepted. No pipes,
// redirects, shell expansion, recursive execution, find -exec, scripts or env
// dumps are inferred safe from an innocuous outer command name.
func routineReadOnly(command, directory string) bool {
	words, ok := literalWords(command)
	if !ok {
		return false
	}
	name := words[0]
	args := append([]string(nil), words[1:]...)
	if name == "grep" {
		index := 0
		for index < len(args) && strings.HasPrefix(args[index], "-") && args[index] != "--" {
			if !strings.Contains(" -n -i -F -H -l -v -c ", " "+args[index]+" ") {
				return false
			}
			index++
		}
		if index < len(args) && args[index] == "--" {
			index++
		}
		if index >= len(args) || args[index] == "" {
			return false
		}
		index++ // literal search pattern
		if index == len(args) {
			return false
		} // no blocking stdin reads
		for _, value := range args[index:] {
			if strings.HasPrefix(value, "-") {
				return false
			}
			if _, ok := diagnosticPath(value, directory); !ok {
				return false
			}
		}
		return true
	}
	if name != "cat" && name != "head" && name != "tail" && name != "ls" {
		return false
	}
	target := AccessTarget{}
	options, operands := true, 0
	for i := 0; i < len(args); i++ {
		if options && args[i] == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(args[i], "-") {
			if (name == "head" || name == "tail") && (args[i] == "-n" || args[i] == "-c") {
				i++
			}
			continue // simpleReadOnly validates flags/counts below
		}
		value, ok := diagnosticPath(args[i], directory)
		if !ok || strings.Contains(value, "'") {
			return false
		}
		args[i] = "'" + value + "'"
		target.ReadFiles = append(target.ReadFiles, value)
		target.ListDirectories = append(target.ListDirectories, value)
		operands++
	}
	if name == "ls" && operands == 0 {
		value, ok := diagnosticPath(directory, directory)
		if !ok || strings.Contains(value, "'") {
			return false
		}
		args = append(args, "'"+value+"'")
		target.ListDirectories = append(target.ListDirectories, value)
	}
	return simpleReadOnly(name+" "+strings.Join(args, " "), target)
}
