package filetransfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSFTP_ListUploadDownload_HappyPath(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	localSrc := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(localSrc, []byte("hello"), 0644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	if _, err := tr.Upload(ctx, localSrc, "a.txt", nil); err != nil {
		t.Fatalf("upload: %v", err)
	}

	entries, err := tr.List(ctx, ".")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "a.txt" && !e.IsDir && e.Size == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("uploaded file not found in list")
	}

	downloadDest := filepath.Join(t.TempDir(), "b.txt")
	if _, err := tr.Download(ctx, "a.txt", downloadDest, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, err := os.ReadFile(downloadDest)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("downloaded content = %q, want %q", string(b), "hello")
	}
}

func TestSFTP_RemoteOps_MkdirRenameRemove_ReadWrite(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Mkdir(ctx, "dir1"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	entries, err := tr.List(ctx, ".")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	foundDir := false
	for _, e := range entries {
		if e.Name == "dir1" && e.IsDir {
			foundDir = true
		}
	}
	if !foundDir {
		t.Fatalf("dir1 not found after mkdir")
	}

	if err := tr.WriteFile(ctx, "dir1/a.txt", []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := tr.ReadFile(ctx, "dir1/a.txt", 1024)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "x" {
		t.Fatalf("content = %q, want %q", string(b), "x")
	}

	if err := tr.Rename(ctx, "dir1/a.txt", "dir1/b.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	_, err = tr.ReadFile(ctx, "dir1/b.txt", 1024)
	if err != nil {
		t.Fatalf("read after rename: %v", err)
	}

	if err := tr.Remove(ctx, "dir1", true); err != nil {
		t.Fatalf("remove recursive: %v", err)
	}
	_, err = tr.Stat(ctx, "dir1")
	if err == nil {
		t.Fatalf("expected stat error after remove")
	}
}

func TestSFTP_NotSupported_SubsystemDisabled(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: false})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err = tr.Check(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	te, ok := err.(*TransferError)
	if !ok {
		t.Fatalf("expected TransferError, got %T: %v", err, err)
	}
	if te.Code != ErrorCodeSFTPNotSupported {
		t.Fatalf("code = %s, want %s", te.Code, ErrorCodeSFTPNotSupported)
	}
}
