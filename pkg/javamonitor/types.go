package javamonitor

type JavaProcess struct {
	PID   int    `json:"pid"`
	User  string `json:"user"`
	ETime string `json:"etime"`
	Cmd   string `json:"cmd"`
}

type CommandResult struct {
	Command string `json:"command"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

type ToolsAvailability struct {
	JCmd  bool `json:"jcmd"`
	JStat bool `json:"jstat"`
	JPS   bool `json:"jps"`
}

type ProcessInfo struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid,omitempty"`
	User    string `json:"user,omitempty"`
	CPU     string `json:"cpu,omitempty"`
	Mem     string `json:"mem,omitempty"`
	ETime   string `json:"etime,omitempty"`
	Threads int    `json:"threads,omitempty"`
	FdCount int    `json:"fd_count,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
}

type JVMSnapshot struct {
	VMVersion  CommandResult `json:"vm_version"`
	HeapInfo   CommandResult `json:"heap_info"`
	GCUtilOnce CommandResult `json:"gcutil_once"`
}

type HostSnapshot struct {
	Uptime  CommandResult `json:"uptime"`
	MemInfo CommandResult `json:"meminfo"`
}

type Snapshot struct {
	PID   int               `json:"pid"`
	Tools ToolsAvailability `json:"tools"`
	Proc  ProcessInfo       `json:"process"`
	JVM   JVMSnapshot       `json:"jvm"`
	Host  HostSnapshot      `json:"host"`
}
