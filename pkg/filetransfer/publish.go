package filetransfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DownloadNewFile stages a download beside its destination and publishes it
// without replacing any existing file or symlink, including one created while
// the download was running. Unsupported hard links fail closed, never rename.
func DownloadNewFile(ctx context.Context, destination string, download func(string) (TransferResult, error)) (TransferResult, error) {
	var empty TransferResult
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	stage, err := os.CreateTemp(filepath.Dir(destination), ".opscopilot-download-*")
	if err != nil {
		return empty, err
	}
	staged := stage.Name()
	defer os.Remove(staged)
	if err = stage.Close(); err != nil {
		return empty, err
	}
	result, err := download(staged)
	if err != nil {
		return empty, err
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if err = os.Link(staged, destination); err != nil {
		return empty, fmt.Errorf("下载未发布，已有目标不会被覆盖；请核对目标是否存在及文件系统是否支持安全发布: %w", err)
	}
	return result, nil
}
