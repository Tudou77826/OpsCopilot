package terminalcontrol

import (
	"path"
	"strconv"
	"strings"
)

// This recognizes a deliberately small literal subset, not the shell language.
// Everything else requires review. Neither model claims nor regex prefixes can
// extend this subset. The user's existing shell/executables remain trusted.
func literalWords(command string) ([]string, bool) {
	if strings.ContainsAny(command, "\\$`;|&<>(){}*?[]~\n\r") {
		return nil, false
	}
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	for _, char := range command {
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			started = true
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
			started = true
		case ' ', '\t':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if started {
		words = append(words, word.String())
	}
	if len(words) > 0 && words[0] == "command" {
		words = words[1:]
	}
	return words, len(words) > 0
}

func sensitivePath(p string) bool {
	p = strings.ToLower(p)
	for _, part := range []string{".ssh", ".aws", ".kube", "credential", "password", "secret", "/shadow", "/environ", "/.env", ".pem", ".key", "id_rsa", "id_ed25519",
		".netrc", ".npmrc", ".pypirc", ".pgpass", ".my.cnf", ".docker/config.json", ".azure", ".config/gcloud", ".config/gh/hosts", ".config/git/credentials", "/ssl/private", "/etc/kubernetes",
		".bash_history", ".zsh_history", ".mysql_history", ".python_history", "keychain", "cookies", "wallet.dat"} {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func exactPath(p string, allowed []string) bool {
	if !path.IsAbs(p) || path.Clean(p) != p || sensitivePath(p) {
		return false
	}
	for _, candidate := range allowed {
		if p == candidate {
			return true
		}
	}
	return false
}

func simpleReadOnly(command string, target AccessTarget) bool {
	w, ok := literalWords(command)
	if !ok {
		return false
	}
	flags := map[string]string{
		"pwd": " -L -P ", "id": " -u -g -G -un -gn ", "whoami": " ", "uptime": " ",
		"uname": " -a -s -r -m -n ", "date": " ", "ps": " -ef -e -f aux -aux ",
		"df": " -h -k -i ", "free": " -h -m -k ",
	}
	if accepted, found := flags[w[0]]; found {
		for _, arg := range w[1:] {
			if !strings.Contains(accepted, " "+arg+" ") || arg == "" {
				return false
			}
		}
		return true
	}
	args := w[1:]
	switch w[0] {
	case "cat", "head", "tail", "ls":
		operands := 0
		options := true
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if options && arg == "--" {
				options = false
				continue
			}
			if options && strings.HasPrefix(arg, "-") {
				if (w[0] == "head" || w[0] == "tail") && (arg == "-n" || arg == "-c") && i+1 < len(args) {
					i++
					n, err := strconv.Atoi(args[i])
					if err != nil || n < 1 || n > 10000 {
						return false
					}
					continue
				}
				if w[0] == "ls" && strings.Contains(" -l -a -h -la -al -lah -alh -ld ", " "+arg+" ") {
					continue
				}
				return false
			}
			paths := target.ReadFiles
			if w[0] == "ls" {
				paths = target.ListDirectories
			}
			if !exactPath(arg, paths) {
				return false
			}
			operands++
		}
		return operands > 0
	}
	return false
}
