package filetransfer

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("runs with an actual POSIX shell in the Linux simulation")
	}
}

func localRootCommand(t *testing.T) rootCommand {
	t.Helper()
	requirePOSIX(t)
	return func(ctx context.Context, script string) (string, error) {
		cmd := exec.CommandContext(ctx, "/bin/sh")
		cmd.Stdin = strings.NewReader(script + "\n")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("fixture command failed: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

func seedTransferFile(t *testing.T, p string, size int64) {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	block := make([]byte, 64*1024)
	for i := range block {
		block[i] = byte((i*31 + 17) % 251)
	}
	for size > 0 {
		n := int64(len(block))
		if size < n {
			n = size
		}
		if _, err = f.Write(block[:n]); err != nil {
			t.Fatal(err)
		}
		size -= n
	}
}

func assertSameTransferFile(t *testing.T, a, b string) {
	t.Helper()
	ha, e := computeLocalMD5(a)
	if e != nil {
		t.Fatal(e)
	}
	hb, e := computeLocalMD5(b)
	if e != nil || ha != hb {
		t.Fatal("file bytes differ", e)
	}
}

func assertNoTransferTemps(t *testing.T, dir string) {
	t.Helper()
	files, e := filepath.Glob(filepath.Join(dir, ".opscopilot-*"))
	if e != nil || len(files) > 0 {
		t.Fatal("temporary transfer files remain", files, e)
	}
}

func TestBase64TransferSizeGuardsBeforeRemoteCommands(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "too-large")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(maxBase64DirectBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	b := base64Transfer{run: func(context.Context, string) (string, error) {
		t.Fatal("oversized file reached remote")
		return "", nil
	}}
	if _, err = b.upload(context.Background(), source, "/target", nil); err == nil {
		t.Fatal("oversized upload accepted")
	}
	if _, err = b.download(context.Background(), "/source", filepath.Join(root, "out"), maxBase64DirectBytes+1, nil); err == nil {
		t.Fatal("oversized download accepted")
	}
	if err = checkBase64Size(maxBase64DirectBytes); err != nil {
		t.Fatal("100 MiB boundary rejected")
	}
}

func TestBase64ReplyFramingBoundsAndInterruptions(t *testing.T) {
	ctx := context.Background()
	out, err := readBase64Reply(ctx, strings.NewReader("echo noise\r\nSTART\r\nYWJj\r\nOK\r\nprompt"), "START", "OK", "FAIL")
	if err != nil || out != "YWJj" {
		t.Fatal(out, err)
	}
	if _, err = readBase64Reply(ctx, strings.NewReader("START\npartial"), "START", "OK", "FAIL"); err == nil {
		t.Fatal("missing terminator accepted")
	}
	if _, err = readBase64Reply(ctx, strings.NewReader("START\nFAIL"), "START", "OK", "FAIL"); err == nil {
		t.Fatal("remote failure accepted")
	}
	if _, err = readBase64Reply(ctx, io.LimitReader(zeroReader{}, base64CommandOutputLimit+1), "START", "OK", "FAIL"); err == nil {
		t.Fatal("unbounded output accepted")
	}
}

func TestBase64LocalReplacementOnHostOS(t *testing.T) {
	for _, scenario := range []string{"success", "corrupt", "cancel", "empty", "readonly"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "existing-download")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario == "readonly" {
				if err := os.Chmod(target, 0400); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(target, 0600) })
			}
			data := bytes.Repeat([]byte("download-payload"), 60000)
			if scenario == "empty" {
				data = nil
			}
			digest := fmt.Sprintf("%x", md5.Sum(data))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := func(ctx context.Context, cmd string) (string, error) {
				current, e := os.ReadFile(target)
				if e != nil || string(current) != "original" {
					t.Fatal("destination changed before validation completed")
				}
				switch {
				case strings.HasPrefix(cmd, "md5sum "):
					return digest + "  remote-file", nil
				case strings.HasPrefix(cmd, "stat "):
					return fmt.Sprint(len(data)), nil
				case strings.HasPrefix(cmd, "dd if="):
					if scenario == "corrupt" {
						return "not base64!", nil
					}
					start := 0
					if strings.Contains(cmd, "skip=1 ") {
						start = base64BlockBytes
					}
					end := start + base64BlockBytes
					if end > len(data) {
						end = len(data)
					}
					return base64.StdEncoding.EncodeToString(data[start:end]), nil
				default:
					return "", nil
				}
			}
			_, err := (base64Transfer{run: run}).download(ctx, "remote-file", target, int64(len(data)), func(p Progress) {
				if scenario == "cancel" && p.BytesDone > 0 {
					cancel()
				}
			})
			actual, e := os.ReadFile(target)
			if e != nil {
				t.Fatal(e)
			}
			if scenario == "success" || scenario == "empty" || (scenario == "readonly" && runtime.GOOS != "windows") {
				if err != nil || !bytes.Equal(actual, data) {
					t.Fatal("verified replacement failed", err)
				}
			} else {
				if err == nil || string(actual) != "original" {
					t.Fatal("failed download damaged destination", err)
				}
			}
			assertNoTransferTemps(t, dir)
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestBase64POSIXBoundaryRoundTrips(t *testing.T) {
	for _, size := range []int64{0, base64BlockBytes - 1, base64BlockBytes, base64BlockBytes + 1, 10*1024*1024 + 7} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			run := localRootCommand(t)
			dir := t.TempDir()
			src := filepath.Join(dir, "local-source")
			dst := filepath.Join(dir, "remote 资料'file")
			out := filepath.Join(dir, "download")
			seedTransferFile(t, src, size)
			os.WriteFile(dst, []byte("original remote"), 0600)
			os.WriteFile(out, []byte("original local"), 0600)
			b := base64Transfer{run: run}
			if result, err := b.upload(context.Background(), src, dst, nil); err != nil || result.Bytes != size {
				t.Fatal(result, err)
			}
			assertSameTransferFile(t, src, dst)
			if result, err := b.download(context.Background(), dst, out, size, nil); err != nil || result.Bytes != size {
				t.Fatal(result, err)
			}
			assertSameTransferFile(t, src, out)
			assertNoTransferTemps(t, dir)
			for _, p := range []string{dst, out} {
				i, e := os.Stat(p)
				if e != nil || i.Mode().Perm() != 0600 {
					t.Fatal("existing mode changed", p, e)
				}
			}
		})
	}
}

func TestBase64POSIXFailureKeepsOriginalFiles(t *testing.T) {
	for _, direction := range []string{"upload", "download"} {
		for _, failure := range []string{"cancel", "network", "checksum", "commit"} {
			if direction == "download" && failure == "commit" {
				continue
			} // OS rename race is exercised below.
			t.Run(direction+"/"+failure, func(t *testing.T) {
				real := localRootCommand(t)
				dir := t.TempDir()
				source := filepath.Join(dir, "source")
				target := filepath.Join(dir, "target")
				seedTransferFile(t, source, 2*base64BlockBytes+1)
				os.WriteFile(target, []byte("keep-original"), 0644)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				blocks := 0
				run := func(ctx context.Context, cmd string) (string, error) {
					data := strings.Contains(cmd, "{ base64 -d >>") || strings.HasPrefix(cmd, "dd if=")
					if data {
						blocks++
						if blocks == 2 {
							if failure == "network" {
								return "", io.ErrUnexpectedEOF
							}
							if failure == "cancel" {
								cancel()
								return "", ctx.Err()
							}
						}
					}
					if failure == "commit" && strings.Contains(cmd, "mv -f --") {
						return "", fmt.Errorf("commit denied")
					}
					out, err := real(ctx, cmd)
					if failure == "checksum" && strings.HasPrefix(cmd, "md5sum ") {
						return strings.Repeat("0", 32) + "  file", nil
					}
					return out, err
				}
				b := base64Transfer{run: run}
				var err error
				if direction == "upload" {
					_, err = b.upload(ctx, source, target, nil)
				} else {
					_, err = b.download(ctx, source, target, 2*base64BlockBytes+1, nil)
				}
				if err == nil {
					t.Fatal("injected failure ignored")
				}
				contents, _ := os.ReadFile(target)
				if string(contents) != "keep-original" {
					t.Fatal("original destination overwritten")
				}
				assertNoTransferTemps(t, dir)
			})
		}
	}
}

func TestBase64CommandQueueCancellation(t *testing.T) {
	s := &rootShellSession{commands: make(chan struct{}, 1)}
	unlock, err := s.lockCommand(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err = s.lockCommand(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("queued cancellation ignored", err)
	}
	unlock()
	next, err := s.lockCommand(context.Background())
	if err != nil {
		t.Fatal("queue leaked reservation", err)
	}
	next()
}

func TestBase64SessionSetupCancellation(t *testing.T) {
	requested := make(chan struct{})
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: t.TempDir(), BlockedPTY: requested})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	defer tr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := tr.getSession(ctx); done <- err }()
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		t.Fatal("PTY request did not arrive")
	}
	queued, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if _, err := tr.getSession(queued); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued setup did not cancel: %v", err)
	}
	closed := make(chan struct{})
	go func() { tr.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited for a stalled PTY")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled setup succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("PTY setup ignored cancellation")
	}
}

func TestBase64StaleFailureDoesNotInvalidateReplacement(t *testing.T) {
	old := &rootShellSession{}
	replacement := &rootShellSession{}
	tr := &RootRelayTransport{shell: replacement}
	tr.invalidateSession(old)
	if tr.shell != replacement {
		t.Fatal("stale command discarded a newer session")
	}
}

func TestBase64POSIXCreationReplyLostAndCleanupFailure(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanupFails), func(t *testing.T) {
			real := localRootCommand(t)
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			target := filepath.Join(dir, "target")
			seedTransferFile(t, source, 1)
			os.WriteFile(target, []byte("original"), 0644)
			run := func(ctx context.Context, cmd string) (string, error) {
				if strings.Contains(cmd, "set -C;") {
					if _, err := real(ctx, cmd); err != nil {
						return "", err
					}
					return "", io.ErrUnexpectedEOF
				}
				if cleanupFails && strings.HasPrefix(cmd, "rm -f --") {
					return "", io.ErrUnexpectedEOF
				}
				return real(ctx, cmd)
			}
			_, err := (base64Transfer{run: run}).upload(context.Background(), source, target, nil)
			if err == nil {
				t.Fatal("lost reply accepted")
			}
			if cleanupFails {
				if !strings.Contains(err.Error(), "临时文件可能保留") {
					t.Fatal("cleanup failure was hidden", err)
				}
			} else {
				assertNoTransferTemps(t, dir)
			}
			out, _ := os.ReadFile(target)
			if string(out) != "original" {
				t.Fatal("original replaced")
			}
		})
	}
}

func TestBase64POSIXConcurrentLocalDestinationChange(t *testing.T) {
	real := localRootCommand(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	seedTransferFile(t, source, 17)
	os.WriteFile(target, []byte("original"), 0644)
	run := func(ctx context.Context, cmd string) (string, error) {
		out, err := real(ctx, cmd)
		if strings.HasPrefix(cmd, "stat -Lc ") {
			if e := os.Rename(target, target+".saved"); e != nil {
				t.Fatal(e)
			}
			if e := os.Mkdir(target, 0700); e != nil {
				t.Fatal(e)
			}
			os.WriteFile(filepath.Join(target, "keep"), []byte("concurrent user change"), 0600)
		}
		return out, err
	}
	if _, err := (base64Transfer{run: run}).download(context.Background(), source, target, 17, nil); err == nil {
		t.Fatal("concurrent destination change accepted")
	}
	original, _ := os.ReadFile(target + ".saved")
	if string(original) != "original" {
		t.Fatal("old destination lost")
	}
	concurrent, _ := os.ReadFile(filepath.Join(target, "keep"))
	if string(concurrent) != "concurrent user change" {
		t.Fatal("concurrent destination destroyed")
	}
	assertNoTransferTemps(t, dir)
}

func TestBase64SSHEchoedHereDocument(t *testing.T) {
	requirePOSIX(t)
	dir := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true, EchoShellInput: true})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	tr.SetSkipRelay(true)
	defer tr.Close()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "target")
	out := filepath.Join(dir, "download")
	seedTransferFile(t, src, 2*base64BlockBytes+1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := tr.Upload(ctx, src, dst, nil); err != nil {
		t.Fatal("echoed upload failed", err)
	}
	if _, err := tr.Download(ctx, dst, out, nil); err != nil {
		t.Fatal("echoed download failed", err)
	}
	assertSameTransferFile(t, src, out)
	assertNoTransferTemps(t, dir)
}

func TestBase64SSHRealLinuxPTY(t *testing.T) {
	requirePOSIX(t)
	for _, binary := range []string{"script", "timeout"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip("PTY test requires script and timeout")
		}
	}
	dir := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true, SystemPTY: true})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	tr.SetSkipRelay(true)
	defer tr.Close()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "target")
	out := filepath.Join(dir, "download")
	seedTransferFile(t, src, 2*base64BlockBytes+17)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := tr.Upload(ctx, src, dst, nil); err != nil {
		t.Fatal("actual PTY upload failed", err)
	}
	if _, err := tr.Download(ctx, dst, out, nil); err != nil {
		t.Fatal("actual PTY download failed", err)
	}
	assertSameTransferFile(t, src, out)
	assertNoTransferTemps(t, dir)
}

func TestBase64SSHDecoderFailureDrainsInput(t *testing.T) {
	requirePOSIX(t)
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("disk-full simulation requires /dev/full")
	}
	dir := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	defer tr.Close()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	seedTransferFile(t, source, base64BlockBytes)
	os.WriteFile(target, []byte("original"), 0600)
	b := base64Transfer{run: tr.runBase64Command, uploadChunk: func(ctx context.Context, temp string, data []byte) error {
		if err := os.Remove(temp); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/dev/full", temp); err != nil {
			t.Fatal(err)
		}
		return tr.uploadBase64Block(ctx, temp, data)
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := b.upload(ctx, source, target, nil); err == nil {
		t.Fatal("decoder disk-full error ignored")
	}
	contents, _ := os.ReadFile(target)
	if string(contents) != "original" {
		t.Fatal("disk-full overwrote destination")
	}
	assertNoTransferTemps(t, dir)
	if out, err := tr.runBase64Command(ctx, "printf recovered"); err != nil || out != "recovered" {
		t.Fatal("encoded data desynchronized next command", err)
	}
}

func TestBase64SSHCanonicalFallback(t *testing.T) {
	requirePOSIX(t)
	for _, binary := range []string{"script", "timeout"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skip("PTY fallback test requires script and timeout")
		}
	}
	dir := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true, SystemPTY: true})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	tr.SetSkipRelay(true)
	defer tr.Close()
	shell, err := tr.getSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := shell.lockCommand(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shell.base64Ready = true
	shell.base64Stream = false
	unlock()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	seedTransferFile(t, source, 96*1024+5)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := tr.Upload(ctx, source, target, nil); err != nil {
		t.Fatal("canonical fallback failed", err)
	}
	assertSameTransferFile(t, source, target)
	assertNoTransferTemps(t, dir)
}

func TestBase64POSIXSymlinksAndMissingChecksum(t *testing.T) {
	real := localRootCommand(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "target")
	link := filepath.Join(dir, "target-link")
	seedTransferFile(t, src, base64BlockBytes+3)
	os.WriteFile(dst, []byte("original"), 0600)
	if err := os.Symlink(dst, link); err != nil {
		t.Fatal(err)
	}
	b := base64Transfer{run: real}
	if _, err := b.upload(context.Background(), src, link, nil); err != nil {
		t.Fatal(err)
	}
	assertSameTransferFile(t, src, dst)
	if i, e := os.Lstat(link); e != nil || i.Mode()&os.ModeSymlink == 0 {
		t.Fatal("target symlink replaced")
	}
	os.WriteFile(dst, []byte("original"), 0600)
	b.run = func(ctx context.Context, cmd string) (string, error) {
		if strings.HasPrefix(cmd, "md5sum ") {
			return "", fmt.Errorf("md5sum unavailable")
		}
		return real(ctx, cmd)
	}
	if _, err := b.upload(context.Background(), src, dst, nil); err == nil {
		t.Fatal("missing checksum accepted")
	}
	actual, _ := os.ReadFile(dst)
	if string(actual) != "original" {
		t.Fatal("original lost")
	}
	assertNoTransferTemps(t, dir)
}

func TestBase64SSH100MiBAndSlowLink(t *testing.T) {
	requirePOSIX(t)
	if testing.Short() {
		t.Skip("100 MiB SSH simulation")
	}
	dir := t.TempDir()
	realPTY := os.Getenv("OPSCOPILOT_TEST_BASE64_REAL_PTY") == "1"
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true, SystemPTY: realPTY, ShellLifetimeSeconds: 600})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	tr.SetSkipRelay(true)
	defer tr.Close()
	src := filepath.Join(dir, "100MiB-source")
	dst := filepath.Join(dir, "remote target")
	out := filepath.Join(dir, "download")
	seedTransferFile(t, src, maxBase64DirectBytes)
	counts := map[string]int{}
	largest := 0
	largestInput := 0
	run := func(ctx context.Context, cmd string) (string, error) {
		if strings.HasPrefix(cmd, "dd if=") {
			counts["download"]++
		}
		if len(cmd) > largest {
			largest = len(cmd)
		}
		// Inject one five-millisecond command turnaround, independently of byte
		// throughput, simulating the latency penalty of a bastion connection.
		select {
		case <-time.After(5 * time.Millisecond):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return tr.runBase64Command(ctx, cmd)
	}
	send := func(ctx context.Context, temp string, data []byte) error {
		counts["upload"]++
		if len(data) > largestInput {
			largestInput = len(data)
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		return tr.uploadBase64Block(ctx, temp, data)
	}
	b := base64Transfer{run: run, uploadChunk: send}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	started := time.Now()
	var upProgress, downProgress []int64
	var peakHeap uint64
	sampleHeap := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		if m.HeapAlloc > peakHeap {
			peakHeap = m.HeapAlloc
		}
	}
	if r, e := b.upload(ctx, src, dst, func(p Progress) {
		sampleHeap()
		if p.Step == "" {
			upProgress = append(upProgress, p.BytesDone)
		}
	}); e != nil || r.Bytes != maxBase64DirectBytes {
		t.Fatal(r, e)
	}
	uploadTime := time.Since(started)
	started = time.Now()
	if r, e := b.download(ctx, dst, out, maxBase64DirectBytes, func(p Progress) {
		sampleHeap()
		if p.Step == "" {
			downProgress = append(downProgress, p.BytesDone)
		}
	}); e != nil || r.Bytes != maxBase64DirectBytes {
		t.Fatal(r, e)
	}
	assertSameTransferFile(t, src, out)
	assertNoTransferTemps(t, dir)
	expected := int((maxBase64DirectBytes + base64BlockBytes - 1) / base64BlockBytes)
	if counts["upload"] != expected || counts["download"] != expected {
		t.Fatal("unexpected per-block command budget", counts)
	}
	if largest > 2*1024*1024 {
		t.Fatal("command grew with entire file", largest)
	}
	if largestInput > base64BlockBytes {
		t.Fatal("input buffer exceeded the block limit")
	}
	if realPTY && !tr.shell.base64Stream {
		t.Fatal("real PTY did not enable streamed input")
	}
	for _, p := range [][]int64{upProgress, downProgress} {
		if len(p) < expected || p[len(p)-1] != maxBase64DirectBytes {
			t.Fatal("missing incremental progress")
		}
		for i := 1; i < len(p); i++ {
			if p[i] < p[i-1] {
				t.Fatal("progress moved backwards")
			}
		}
	}
	t.Logf("100 MiB byte-identical round trip; realPTY=%t upload=%s download=%s dataCommands=%v maxMetadataCommandBytes=%d maxInputBlockBytes=%d sampledProcessHeapMiB=%.1f; old upload would require %d data commands", realPTY, uploadTime, time.Since(started), counts, largest, largestInput, float64(peakHeap)/(1024*1024), (maxBase64DirectBytes+3999)/4000)
}

func TestBase64SSHCancelledReadAndSessionRecovery(t *testing.T) {
	requirePOSIX(t)
	dir := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: dir, SystemShell: true})
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tr := NewRootRelayTransport(client, "", "")
	defer tr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = tr.runBase64Command(ctx, "sleep 1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("silent command not cancelled", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation was not responsive")
	}
	if out, err := tr.runBase64Command(context.Background(), "printf recovered"); err != nil || out != "recovered" {
		t.Fatal("session not recreated", out, err)
	}
}
