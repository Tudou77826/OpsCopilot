package javamonitor

import (
	"fmt"
	"opscopilot/pkg/sshclient"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type rawTopThread struct {
	tid int
	cpu float64
}

func GetTopCPUThreads(client *sshclient.Client, pid int, limit int) ([]TopThread, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid pid")
	}
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}

	out, err := client.Run(fmt.Sprintf("top -H -b -n 1 -p %d 2>/dev/null | head -n 60", pid))
	if err != nil {
		return nil, err
	}
	threads := parseTopHOutput(out, pid)
	if len(threads) == 0 {
		return []TopThread{}, nil
	}
	if len(threads) > limit {
		threads = threads[:limit]
	}

	jstackOut, jstackErr := client.Run(fmt.Sprintf("command -v jstack >/dev/null 2>&1 && jstack %d 2>/dev/null || true", pid))
	jstackOut = strings.TrimSpace(jstackOut)
	jstackAvailable := jstackErr == nil && jstackOut != ""

	var res []TopThread
	for _, t := range threads {
		item := TopThread{
			TID:    t.tid,
			TIDHex: fmt.Sprintf("0x%x", t.tid),
			CPU:    fmt.Sprintf("%.1f", t.cpu),
		}
		if jstackAvailable {
			name, state, top := findThreadInJstack(jstackOut, item.TIDHex)
			item.JavaName = name
			item.JavaState = state
			item.StackTop = top
		}
		res = append(res, item)
	}
	return res, nil
}

func parseTopHOutput(out string, pid int) []rawTopThread {
	lines := strings.Split(out, "\n")
	var candidates []rawTopThread

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "top -") || strings.HasPrefix(line, "Tasks:") || strings.HasPrefix(line, "%Cpu") || strings.HasPrefix(line, "MiB Mem") || strings.HasPrefix(line, "KiB Mem") || strings.HasPrefix(line, "PID ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		tid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if tid == pid {
			continue
		}
		cpu, err := strconv.ParseFloat(fields[8], 64)
		if err != nil {
			continue
		}
		candidates = append(candidates, rawTopThread{tid: tid, cpu: cpu})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cpu > candidates[j].cpu
	})
	return candidates
}

func GetThreadStateCounts(client *sshclient.Client, pid int) (ThreadStateCounts, error) {
	if pid <= 0 {
		return ThreadStateCounts{}, fmt.Errorf("invalid pid")
	}
	out, err := client.Run(fmt.Sprintf("command -v jstack >/dev/null 2>&1 && jstack %d 2>/dev/null || true", pid))
	if err != nil {
		return ThreadStateCounts{}, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return ThreadStateCounts{}, nil
	}
	return countThreadStatesFromJstack(out), nil
}

func countThreadStatesFromJstack(out string) ThreadStateCounts {
	var c ThreadStateCounts

	re := regexp.MustCompile(`java\.lang\.Thread\.State:\s+([A-Z_]+)`)
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		if len(m) < 2 {
			continue
		}
		switch m[1] {
		case "RUNNABLE":
			c.Runnable++
		case "BLOCKED":
			c.Blocked++
		case "WAITING":
			c.Waiting++
		case "TIMED_WAITING":
			c.TimedWaiting++
		case "NEW":
			c.New++
		case "TERMINATED":
			c.Terminated++
		default:
			c.Unknown++
		}
	}
	return c
}

func findThreadInJstack(out string, tidHex string) (name string, state string, top string) {
	headerRe := regexp.MustCompile(`^"([^"]+)"[^\n]*nid=(` + regexp.QuoteMeta(tidHex) + `)\b[^\n]*$`)
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		m := headerRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) >= 2 {
			name = m[1]
			for j := i + 1; j < len(lines) && j < i+25; j++ {
				l := strings.TrimSpace(lines[j])
				if strings.HasPrefix(l, "\"") {
					break
				}
				if strings.HasPrefix(l, "java.lang.Thread.State:") {
					state = strings.TrimSpace(strings.TrimPrefix(l, "java.lang.Thread.State:"))
				}
				if top == "" && strings.HasPrefix(l, "at ") {
					top = l
				}
			}
			return
		}
	}
	return "", "", ""
}
