package remote

import "context"

// CommandRunner is optional so existing desktop protocols keep their current
// behaviour. A caller owns the connection lifetime and closes it on return.
type CommandRunner interface {
	RunCommand(context.Context, CommandRequest) (CommandResult, error)
}
type CommandRequest struct {
	Command      string
	Input        string
	CaptureBytes int
}
type CommandResult struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        *int   `json:"exitCode"`
	Started         bool   `json:"started"`
	StartAttempted  bool   `json:"startAttempted"`
	CompletionKnown bool   `json:"completionKnown"`
	Truncated       bool   `json:"truncated"`
	StdoutBytes     int64  `json:"stdoutBytes"`
	StderrBytes     int64  `json:"stderrBytes"`
}
