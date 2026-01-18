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
	PID      int    `json:"pid"`
	PPID     int    `json:"ppid,omitempty"`
	User     string `json:"user,omitempty"`
	CPU      string `json:"cpu,omitempty"`
	Mem      string `json:"mem,omitempty"`
	ETime    string `json:"etime,omitempty"`
	Threads  int    `json:"threads,omitempty"`
	FdCount  int    `json:"fd_count,omitempty"`
	FdLimit  int    `json:"fd_limit,omitempty"`
	VmRSSKB  int    `json:"vm_rss_kb,omitempty"`
	VmSizeKB int    `json:"vm_size_kb,omitempty"`
	Cmd      string `json:"cmd,omitempty"`
}

type ThreadStateCounts struct {
	Runnable     int `json:"runnable"`
	Blocked      int `json:"blocked"`
	Waiting      int `json:"waiting"`
	TimedWaiting int `json:"timed_waiting"`
	New          int `json:"new"`
	Terminated   int `json:"terminated"`
	Unknown      int `json:"unknown"`
}

type TopThread struct {
	TID       int    `json:"tid"`
	TIDHex    string `json:"tid_hex"`
	CPU       string `json:"cpu"`
	JavaName  string `json:"java_name,omitempty"`
	JavaState string `json:"java_state,omitempty"`
	StackTop  string `json:"stack_top,omitempty"`
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
