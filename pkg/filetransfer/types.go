package filetransfer

import "time"

type ErrorCode string

const (
	ErrorCodeUnknown          ErrorCode = "UNKNOWN"
	ErrorCodeSFTPNotSupported ErrorCode = "SFTP_NOT_SUPPORTED"
	ErrorCodeNotSupported     ErrorCode = "NOT_SUPPORTED"
	ErrorCodePermissionDenied ErrorCode = "PERMISSION_DENIED"
	ErrorCodeNotFound         ErrorCode = "NOT_FOUND"
	ErrorCodeAuthFailed       ErrorCode = "AUTH_FAILED"
	ErrorCodeNetwork          ErrorCode = "NETWORK"
	ErrorCodeRelayFailed      ErrorCode = "RELAY_FAILED"
	ErrorCodeRelayNoSpace     ErrorCode = "RELAY_NO_SPACE"
)

type TransferError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *TransferError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

type Entry struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	Mode    uint32    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

type Progress struct {
	BytesDone  int64 `json:"bytesDone"`
	BytesTotal int64 `json:"bytesTotal"`
	SpeedBps   int64 `json:"speedBps"`
}

type TransferResult struct {
	Bytes int64 `json:"bytes"`
}
