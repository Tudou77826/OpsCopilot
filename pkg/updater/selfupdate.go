package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"opscopilot/pkg/installguard"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// UpdateResult records the outcome of a self-update for the new version to display.
type UpdateResult struct {
	Success    bool   `json:"success"`
	Version    string `json:"version"`
	Error      string `json:"error,omitempty"`
	RolledBack bool   `json:"rolledBack,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// fileOp represents a single file operation during update.
type fileOp struct {
	srcPath  string // source in extracted dir
	dstPath  string // destination in app dir
	bakPath  string // backup path
	fileName string // original filename
}

const (
	resultFile = "update_result.json"
	logFile    = "update.log"
	oldSuffix  = ".old"
	bakSuffix  = ".bak"
	maxRetries = 3
	retryDelay = 1 * time.Second
	waitParent = 60 * time.Second
	settleTime = 2 * time.Second
)

// SelfUpdate executes the full Phase 2 update flow:
//  1. Wait for parent process to exit
//  2. Backup existing files
//  3. Copy new files from extractedDir to appDir
//  4. On any failure, rollback all .bak files
//  5. Write UpdateResult
//  6. Launch the new exe
func SelfUpdate(ctx context.Context, m *Manifest, fs Filesystem, waiter ProcessWaiter) error {
	log := newUpdateLogger(m.LogPath)
	defer log.close()

	log.info("OpsCopilot self-update started")
	log.info("extractedDir=%s appDir=%s pid=%d version=%s", m.ExtractedDir, m.AppDir, m.ParentPid, m.Version)

	// 1. Wait for parent process.
	log.info("waiting for parent process %d to exit", m.ParentPid)
	if err := waiter.Wait(m.ParentPid, waitParent); err != nil {
		log.info("ERROR: %v", err)
		return fmt.Errorf("wait parent: %w", err)
	}
	log.info("parent process exited")
	release, err := installguard.AcquireUpdate(m.AppDir)
	if err != nil {
		writeResult(m.AppDir, &UpdateResult{Success: false, Version: m.Version, Error: err.Error(), Timestamp: time.Now().Format(time.RFC3339)}, fs)
		launchExe(m.ExePath)
		return fmt.Errorf("installation busy: %w", err)
	}
	defer release()

	// 2. Settle delay for file handle release.
	time.Sleep(settleTime)

	// 3. Plan update operations.
	ops, err := planUpdate(m.ExtractedDir, m.AppDir, fs)
	if err != nil {
		log.info("ERROR plan update: %v", err)
		return fmt.Errorf("plan update: %w", err)
	}
	log.info("planned %d file operations", len(ops))

	if len(ops) == 0 {
		log.info("no files to update")
		writeResult(m.AppDir, &UpdateResult{Success: true, Version: m.Version, Timestamp: time.Now().Format(time.RFC3339)}, fs)
		launchExe(m.ExePath)
		return nil
	}

	// 4. Rename the running exe (now the parent has exited).
	exeIsTarget := false
	for i, op := range ops {
		if op.dstPath == m.ExePath {
			exeIsTarget = true
			oldPath := m.ExePath + oldSuffix
			log.info("renaming exe: %s -> %s", m.ExePath, oldPath)
			if err := fs.Rename(m.ExePath, oldPath); err != nil {
				log.info("ERROR rename exe: %v", err)
				return fmt.Errorf("rename exe: %w", err)
			}
			ops[i].dstPath = m.ExePath
			break
		}
	}

	// 5. Backup existing files.
	rollback, err := backupFiles(ops, fs, log)
	if err != nil {
		log.info("ERROR backup: %v", err)
		if exeIsTarget {
			fs.Rename(m.ExePath+oldSuffix, m.ExePath)
		}
		return fmt.Errorf("backup: %w", err)
	}

	// 6. Copy new files.
	if err := copyFiles(ops, fs, log, rollback); err != nil {
		log.info("ERROR copy, rolled back: %v", err)
		writeResult(m.AppDir, &UpdateResult{
			Success:    false,
			Version:    m.Version,
			Error:      err.Error(),
			RolledBack: true,
			Timestamp:  time.Now().Format(time.RFC3339),
		}, fs)
		if exeIsTarget {
			launchExe(m.ExePath)
		}
		return err
	}

	// 7. Success.
	log.info("update succeeded")
	writeResult(m.AppDir, &UpdateResult{Success: true, Version: m.Version, Timestamp: time.Now().Format(time.RFC3339)}, fs)
	launchExe(m.ExePath)
	return nil
}

// planUpdate scans extractedDir and builds file operations, excluding protected files.
func planUpdate(extractedDir, appDir string, fs Filesystem) ([]fileOp, error) {
	entries, err := fs.ReadDir(extractedDir)
	if err != nil {
		return nil, fmt.Errorf("read extracted dir: %w", err)
	}

	var ops []fileOp
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if protectedFiles[strings.ToLower(name)] {
			slog.Info("updater: skipping protected file", "file", name)
			continue
		}
		ops = append(ops, fileOp{
			srcPath:  filepath.Join(extractedDir, name),
			dstPath:  filepath.Join(appDir, name),
			bakPath:  filepath.Join(appDir, name+bakSuffix),
			fileName: name,
		})
	}
	return ops, nil
}

// backupFiles copies each existing dstPath to bakPath.
// Returns a rollback function that restores all successfully backed-up files.
func backupFiles(ops []fileOp, fs Filesystem, log *updateLogger) (func(), error) {
	var backedUp []fileOp

	for _, op := range ops {
		if _, err := fs.Stat(op.dstPath); err != nil {
			continue
		}
		log.info("backup: %s -> %s", op.fileName, op.bakPath)
		if err := fs.CopyFile(op.dstPath, op.bakPath); err != nil {
			for _, b := range backedUp {
				fs.Rename(b.bakPath, b.dstPath)
			}
			return nil, fmt.Errorf("backup %s: %w", op.fileName, err)
		}
		backedUp = append(backedUp, op)
	}

	return func() {
		log.info("rolling back %d files", len(backedUp))
		for _, b := range backedUp {
			log.info("rollback: %s", b.fileName)
			if err := fs.Rename(b.bakPath, b.dstPath); err != nil {
				log.info("ERROR rollback %s: %v", b.fileName, err)
			}
		}
	}, nil
}

// copyFiles copies each srcPath to dstPath with retries.
// On failure of any file, calls rollback and returns an error.
func copyFiles(ops []fileOp, fs Filesystem, log *updateLogger, rollback func()) error {
	for _, op := range ops {
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			log.info("copy (%d/%d): %s", attempt, maxRetries, op.fileName)
			if err := fs.CopyFile(op.srcPath, op.dstPath); err != nil {
				lastErr = err
				log.info("copy failed (attempt %d): %v", attempt, err)
				if attempt < maxRetries {
					time.Sleep(retryDelay)
				}
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			log.info("FAILED after %d attempts: %s", maxRetries, op.fileName)
			rollback()
			return fmt.Errorf("copy %s: %w", op.fileName, lastErr)
		}
	}
	return nil
}

// CleanupAfterUpdate removes .bak/.old files and reads the update result.
// Called during normal app startup (Phase 3).
func CleanupAfterUpdate(appDir string, fs Filesystem) (*UpdateResult, error) {
	// Read result first, before cleanup.
	resultPath := filepath.Join(appDir, resultFile)
	var result *UpdateResult
	if data, err := readResultFile(resultPath, fs); err == nil {
		result = data
		fs.Remove(resultPath)
	}

	// Clean up .bak and .old files.
	entries, err := fs.ReadDir(appDir)
	if err != nil {
		return result, fmt.Errorf("read app dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if ext := filepath.Ext(name); ext == bakSuffix || ext == oldSuffix {
			fullPath := filepath.Join(appDir, name)
			if err := fs.Remove(fullPath); err != nil {
				slog.Warn("updater: failed to cleanup", "file", name, "error", err)
			} else {
				slog.Info("updater: cleaned up", "file", name)
			}
		}
	}

	return result, nil
}

// writeResult writes the UpdateResult as JSON to the app directory.
func writeResult(appDir string, r *UpdateResult, fs Filesystem) {
	path := filepath.Join(appDir, resultFile)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		slog.Error("updater: marshal result", "error", err)
		return
	}
	if err := writeFileSync(fs, path, data, 0644); err != nil {
		slog.Error("updater: write result", "path", path, "error", err)
	}
}

func writeFileSync(fs Filesystem, path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	fs.MkdirAll(dir, 0755)
	return fs.WriteFile(path, data, perm)
}

func readResultFile(path string, fs Filesystem) (*UpdateResult, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r UpdateResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// launchExe starts the application executable.
func launchExe(exePath string) {
	cmd := exec.Command(exePath)
	cmd.Start()
}

// updateLogger writes timestamped lines to both slog and a log file.
type updateLogger struct {
	file *os.File
}

func newUpdateLogger(path string) *updateLogger {
	l := &updateLogger{}
	if path != "" {
		dir := filepath.Dir(path)
		os.MkdirAll(dir, 0755)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("updater: open log file", "path", path, "error", err)
			return l
		}
		l.file = f
	}
	return l
}

func (l *updateLogger) info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	slog.Info("updater: " + msg)
	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(l.file, "[%s] %s\n", timestamp, msg)
	}
}

func (l *updateLogger) close() {
	if l.file != nil {
		l.file.Close()
	}
}
