package identity

import (
	"strings"
	"testing"
)

func validInput() NewInput {
	id, err := ParseSandboxID("my-app")
	if err != nil {
		panic(err)
	}
	return NewInput{
		ID: id,
		ControlRepo: ControlRepository{
			Path: "/Users/me/projects/kaas",
			Name: "kaas",
		},
		Workspace: WorkspaceIdentity{
			Repository: "git@github.com:acme/app.git",
			Branch:     "main",
			Directory:  "~/workspace/app",
		},
		Endpoint: Endpoint{
			Port:         6022,
			IdentityFile: "/tmp/id",
		},
		Resources: Resources{
			CPUs:      4,
			MemoryGiB: 8,
			DiskGiB:   60,
			Arch:      "aarch64",
		},
		ConfigSource: "/Users/me/.kaas/kaas.yaml",
	}
}

func TestNewDefaultsHostAndProvider(t *testing.T) {
	got, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint.Host != DefaultSSHHost {
		t.Fatalf("host: %q", got.Endpoint.Host)
	}
	if got.Provider.ProfileName != "my-app" {
		t.Fatalf("provider: %+v", got.Provider)
	}
}

func TestNewRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*NewInput)
		wantErr string
	}{
		{
			name: "bad sandbox name",
			mutate: func(in *NewInput) {
				in.ID = SandboxID{Name: "My_App"}
			},
			wantErr: "invalid",
		},
		{
			name: "missing control repo",
			mutate: func(in *NewInput) {
				in.ControlRepo = ControlRepository{}
			},
			wantErr: "control repository",
		},
		{
			name: "bad port",
			mutate: func(in *NewInput) {
				in.Endpoint.Port = 0
			},
			wantErr: "ssh port",
		},
		{
			name: "bad cpus",
			mutate: func(in *NewInput) {
				in.Resources.CPUs = 0
			},
			wantErr: "cpus",
		},
		{
			name: "missing arch",
			mutate: func(in *NewInput) {
				in.Resources.Arch = ""
			},
			wantErr: "arch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			_, err := New(in)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestFromForkRegeneratesIdentityFields(t *testing.T) {
	src, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	target, err := ParseSandboxID("my-app-copy")
	if err != nil {
		t.Fatal(err)
	}
	got := FromFork(src, target, Endpoint{Port: 6023})
	if err := Validate(got); err != nil {
		t.Fatal(err)
	}
	if got.ID.Name != "my-app-copy" || got.Provider.ProfileName != "my-app-copy" {
		t.Fatalf("target identity: %+v", got)
	}
	if got.Endpoint.Port != 6023 || got.Endpoint.Host != DefaultSSHHost {
		t.Fatalf("endpoint: %+v", got.Endpoint)
	}
	if got.Resources != src.Resources || got.Workspace != src.Workspace || got.ControlRepo != src.ControlRepo {
		t.Fatalf("copied fields mismatch: %+v vs %+v", got, src)
	}
	if got.Endpoint.IdentityFile != "" || got.Endpoint.User != "" {
		t.Fatalf("fork endpoint should not copy source SSH user/key: %+v", got.Endpoint)
	}
}

func TestWithSSHConfigReconcilesEndpoint(t *testing.T) {
	id, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	got, err := WithSSHConfig(id, SSHConfig{
		Host:         "127.0.0.1",
		Port:         6100,
		User:         "alice",
		IdentityFile: "/path/to/key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint.Port != 6100 || got.Endpoint.User != "alice" || got.Endpoint.IdentityFile != "/path/to/key" {
		t.Fatalf("endpoint: %+v", got.Endpoint)
	}
}

func TestWithSSHConfigRejectsInvalidPort(t *testing.T) {
	id, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	_, err = WithSSHConfig(id, SSHConfig{Port: -1})
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestValidateRejectsProviderMismatch(t *testing.T) {
	id, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	id.Provider.ProfileName = "other"
	if err := Validate(id); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("got %v", err)
	}
}
