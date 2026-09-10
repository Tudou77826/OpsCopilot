// Package atomicfile 提供原子文件写入。
//
// 写入流程为"同目录临时文件 → 写入 → fsync → rename 覆盖目标"。
// rename 在同一文件系统内是原子操作，因此读取方永远不会看到半截文件；
// 进程在写入中途崩溃最多留下一个临时文件残留，原文件保持完整。
//
// 这与 os.WriteFile 的行为差异是本质性的：后者直接截断目标文件再写入，
// 崩溃或断电会留下被截断的内容，对 sessions.json 这类"全量覆写"的
// 单文件存储是不可接受的。
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write 原子地把 data 写入 path，perm 为最终文件权限。
//
// 临时文件创建在 path 所在目录（而非系统临时目录），以保证与目标处于
// 同一文件系统——跨文件系统的 rename 会退化为"复制+删除"，不再原子。
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := f.Name()

	// 正常路径下 rename 之后 tmpPath 已不存在，Remove 失败无所谓；
	// 异常路径下它负责清理残留。
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("刷盘失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("设置文件权限失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("替换目标文件失败: %w", err)
	}

	tmpPath = "" // rename 成功，临时文件已就位，禁止再删
	return nil
}
