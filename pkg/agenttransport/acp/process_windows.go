package acp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// prepareCommand hides the console and routes .cmd/.bat launchers through
// cmd.exe the way the reference ACP client does: /d /s /c followed by one raw
// tail in which the script and every argument are quoted separately. Go's
// default escaping quotes each argument on its own, and the /S rule then
// strips the quotes around a script whose path contains spaces.
func prepareCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	script := command.Path
	switch strings.ToLower(filepath.Ext(script)) {
	case ".cmd", ".bat":
	default:
		return nil
	}
	// cmd.exe expands or reinterprets these characters inside a batch call.
	for _, part := range command.Args {
		if strings.ContainsAny(part, "\"%!\r\n") {
			return errors.New("批处理路径或参数包含不支持的字符（引号、% 或 !）")
		}
	}
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	interpreter := filepath.Join(windir, "System32", "cmd.exe")
	tail := "\"\"" + script + "\""
	for _, arg := range command.Args[1:] {
		tail += " \"" + arg + "\""
	}
	tail += "\""
	command.Path = interpreter
	command.Args[0] = interpreter
	command.SysProcAttr.CmdLine = syscall.EscapeArg(interpreter) + " /d /s /c " + tail
	return nil
}

// bindProcessTree places the just-started child into a Job Object whose last
// closed handle terminates the whole tree (KILL_ON_JOB_CLOSE). A crashed or
// killed desktop host therefore cannot orphan ACP agents — the OS reaps them
// the moment the host's handle table is destroyed.
func bindProcessTree(process *os.Process) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	// The job keeps its own process reference, so the opened handle is released
	// immediately after assignment.
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		_ = windows.CloseHandle(handle)
		_ = windows.CloseHandle(job)
		return nil, err
	}
	_ = windows.CloseHandle(handle)
	return func() { _ = windows.CloseHandle(job) }, nil
}
