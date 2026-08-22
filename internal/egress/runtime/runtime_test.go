package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateControlReloadAndNoCredentialPersistence(t *testing.T) {
	root := filepath.Join(os.TempDir(), fmt.Sprintf("ke-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	secret := "runtime-sentinel-secret"
	b := Bootstrap{Policy: config.Egress{SchemaVersion: 1, Mode: "proxy_preferred", Default: "deny"}, Credentials: map[string]string{"TOKEN": secret}, Listen: "127.0.0.1:0"}
	var input bytes.Buffer
	if err := json.NewEncoder(&input).Encode(b); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, root, "box", &input) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, e := Call(root, "box", Request{Action: "status"}); e == nil && st.Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, e := Call(root, "box", Request{Action: "reload", Credentials: map[string]string{"TOKEN": "rotated"}}); e != nil {
		t.Fatal(e)
	}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e != nil || info.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e == nil && strings.Contains(string(b), secret) {
			t.Fatalf("secret persisted in %s", path)
		}
		return nil
	})
	info, e := os.Stat(Socket(root, "box"))
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("socket mode %o", info.Mode().Perm())
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop")
	}
}
