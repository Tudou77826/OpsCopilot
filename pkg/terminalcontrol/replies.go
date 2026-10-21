package terminalcontrol

import "regexp"

// Exact terminal status replies are protocol traffic, not a human input line.
// In particular BusyBox/readline requests CPR while rendering its prompt.
// Mixed/fragmented/unrecognized input stays conservative and is not exempted.
var statusReply = regexp.MustCompile("^\x1b\\[(?:\\??[0-9]+;[0-9]+R|[0-9]+n|[?>][0-9;]+c)$")

func isStatusReply(data []byte) bool { return statusReply.Match(data) }
