package javamonitor

import (
	"fmt"
	"opscopilot/pkg/sshclient"
	"strconv"
	"strings"
)

const maxBlockSize = 64 * 1024

func ListJavaProcesses(client *sshclient.Client) ([]JavaProcess, error) {
	out, err := client.Run("ps -eo pid,user,etime,comm,args --no-headers")
	if err != nil {
		return nil, err
	}

	var res []JavaProcess
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		comm := fields[3]
		if comm != "java" {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		cmd := strings.Join(fields[4:], " ")
		if len(cmd) > 300 {
			cmd = cmd[:300] + "..."
		}

		res = append(res, JavaProcess{
			PID:   pid,
			User:  fields[1],
			ETime: fields[2],
			Cmd:   cmd,
		})
	}

	return res, nil
}

func GetSnapshot(client *sshclient.Client, pid int) (Snapshot, error) {
	s := Snapshot{PID: pid}

	s.Tools = ToolsAvailability{
		JCmd:  hasCommand(client, "jcmd"),
		JStat: hasCommand(client, "jstat"),
		JPS:   hasCommand(client, "jps"),
	}

	proc, err := getProcessInfo(client, pid)
	if err != nil {
		return Snapshot{}, err
	}
	s.Proc = proc

	s.Host = HostSnapshot{
		Uptime:  runBlock(client, "uptime"),
		MemInfo: runBlock(client, "cat /proc/meminfo | head -n 20"),
	}

	if s.Tools.JCmd {
		s.JVM.VMVersion = runBlock(client, fmt.Sprintf("jcmd %d VM.version", pid))
		s.JVM.HeapInfo = runBlock(client, fmt.Sprintf("jcmd %d GC.heap_info", pid))
	} else {
		s.JVM.VMVersion = CommandResult{Command: "jcmd <pid> VM.version", Error: "jcmd not found"}
		s.JVM.HeapInfo = CommandResult{Command: "jcmd <pid> GC.heap_info", Error: "jcmd not found"}
	}

	if s.Tools.JStat {
		s.JVM.GCUtilOnce = runBlock(client, fmt.Sprintf("jstat -gcutil %d 1000 1", pid))
	} else {
		s.JVM.GCUtilOnce = CommandResult{Command: "jstat -gcutil <pid> 1000 1", Error: "jstat not found"}
	}

	return s, nil
}

func hasCommand(client *sshclient.Client, name string) bool {
	out, err := client.Run(fmt.Sprintf("command -v %s >/dev/null 2>&1 && echo 1 || echo 0", name))
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "1"
}

func runBlock(client *sshclient.Client, cmd string) CommandResult {
	out, err := client.Run(cmd)
	if err != nil {
		return CommandResult{
			Command: cmd,
			Error:   err.Error(),
		}
	}
	out = strings.TrimSpace(out)
	if len(out) > maxBlockSize {
		out = out[:maxBlockSize] + "\n... (truncated)"
	}
	return CommandResult{
		Command: cmd,
		Output:  out,
	}
}

func getProcessInfo(client *sshclient.Client, pid int) (ProcessInfo, error) {
	info := ProcessInfo{PID: pid}

	psCmd := fmt.Sprintf("ps -p %d -o pid=,ppid=,user=,%%cpu=,%%mem=,etime=,cmd=", pid)
	psOut, psErr := client.Run(psCmd)
	if psErr == nil {
		line := strings.TrimSpace(psOut)
		fields := strings.Fields(line)
		if len(fields) >= 7 {
			info.User = fields[2]
			info.CPU = fields[3]
			info.Mem = fields[4]
			info.ETime = fields[5]
			info.Cmd = strings.Join(fields[6:], " ")
			ppid, _ := strconv.Atoi(fields[1])
			info.PPID = ppid
		}
	}

	statusCmd := fmt.Sprintf("cat /proc/%d/status 2>/dev/null | egrep '^(Threads|VmRSS|VmSize):' || true", pid)
	statusOut, _ := client.Run(statusCmd)
	for _, line := range strings.Split(statusOut, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Threads:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				v, _ := strconv.Atoi(parts[1])
				info.Threads = v
			}
		}
	}

	fdCmd := fmt.Sprintf("ls /proc/%d/fd 2>/dev/null | wc -l || true", pid)
	fdOut, _ := client.Run(fdCmd)
	fdCount, _ := strconv.Atoi(strings.TrimSpace(fdOut))
	info.FdCount = fdCount

	if info.User == "" && psErr != nil {
		return ProcessInfo{}, fmt.Errorf("failed to inspect process %d: %w", pid, psErr)
	}

	return info, nil
}
