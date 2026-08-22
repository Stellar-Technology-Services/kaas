package colima

import "testing"

func TestParseSSHConfig(t *testing.T) {
	text := `
Host colima-demo
  HostName 127.0.0.1
  User pradeep
  Port 6022
  IdentityFile /Users/pradeep/.colima/_lima/_config/user
`
	cfg, err := ParseSSHConfig(text)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" || cfg.User != "pradeep" || cfg.Port != 6022 {
		t.Fatalf("got %+v", cfg)
	}
	if cfg.IdentityFile == "" {
		t.Fatal("missing identity")
	}
}
