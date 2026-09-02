package shellsidecar

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"opscopilot/pkg/remote"
)

// MonitorSample：远端资源采样（S8 监控 v1）。
// 采样 = 在远端执行一条只读复合命令，解析 /proc 与 df 输出。非 Linux 远端
// 返回 error 由前端展示，不伪装数据。
type MonitorSample struct {
	Load1       string  `json:"load1"`
	MemTotalMB  float64 `json:"memTotalMB"`
	MemUsedMB   float64 `json:"memUsedMB"`
	MemUsedPct  float64 `json:"memUsedPct"`
	DiskUsedPct float64 `json:"diskUsedPct"`
	DiskPath    string  `json:"diskPath"`
	SampledAt   int64   `json:"sampledAt"`
}

const monitorCommand = `cat /proc/loadavg 2>/dev/null; grep -E 'MemTotal|MemAvailable' /proc/meminfo 2>/dev/null; df -P . 2>/dev/null | tail -1`

// Sample 采一次样（连接归属 TerminalService）。
func (s *TerminalService) SampleMonitor(connectionID string) (*MonitorSample, error) {
	conn, err := s.Connection(connectionID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := conn.Run(ctx, monitorCommand)
	if err != nil {
		return nil, fmt.Errorf("采样命令执行失败: %w", err)
	}
	return parseMonitorSample(out)
}

var (
	loadavgRe  = regexp.MustCompile(`^(\d+\.\d+)\s`)
	memFieldRe = regexp.MustCompile(`^(MemTotal|MemAvailable):\s+(\d+)\s+kB`)
	dfRe       = regexp.MustCompile(`(\d+)%\s+(.+)$`)
)

func parseMonitorSample(out string) (*MonitorSample, error) {
	sample := &MonitorSample{SampledAt: timeNowUnixMilli()}
	var memTotalKB, memAvailKB float64
	haveLoad, haveMem, haveDisk := false, false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := loadavgRe.FindStringSubmatch(line); m != nil && !haveLoad {
			sample.Load1 = m[1]
			haveLoad = true
			continue
		}
		if m := memFieldRe.FindStringSubmatch(line); m != nil {
			v, _ := strconv.ParseFloat(m[2], 64)
			if m[1] == "MemTotal" {
				memTotalKB = v
			} else {
				memAvailKB = v
			}
			haveMem = haveMem || m[1] == "MemAvailable"
			continue
		}
		if m := dfRe.FindStringSubmatch(line); m != nil && !haveDisk {
			pct, _ := strconv.ParseFloat(m[1], 64)
			sample.DiskUsedPct = pct
			sample.DiskPath = strings.TrimSpace(m[2])
			haveDisk = true
		}
	}
	if !haveLoad && !haveMem && !haveDisk {
		return nil, fmt.Errorf("远端输出不可解析（非 Linux 或命令不可用）")
	}
	sample.MemTotalMB = memTotalKB / 1024
	sample.MemUsedMB = (memTotalKB - memAvailKB) / 1024
	if memTotalKB > 0 {
		sample.MemUsedPct = (memTotalKB - memAvailKB) / memTotalKB * 100
	}
	return sample, nil
}

var _ = remote.ProtocolSSH
