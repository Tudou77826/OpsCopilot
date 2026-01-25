package filetransfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSCP_UploadDownload_HappyPath(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: false, EnableSCP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSCPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, _, err := tr.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !ok {
		t.Fatalf("expected scp supported")
	}

	localSrc := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(localSrc, []byte("hello"), 0644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	if _, err := tr.Upload(ctx, localSrc, "a.txt", nil); err != nil {
		t.Fatalf("upload: %v", err)
	}

	localDst := filepath.Join(t.TempDir(), "b.txt")
	if _, err := tr.Download(ctx, "a.txt", localDst, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, err := os.ReadFile(localDst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("downloaded content = %q, want %q", string(b), "hello")
	}
}
