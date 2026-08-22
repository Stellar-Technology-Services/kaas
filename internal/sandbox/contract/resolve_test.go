package contract

import (
	"context"
	"testing"
)

func builtInLayer() Layer {
	return Layer{
		Source: "built-in",
		Resources: &Resources{
			CPUs: 4, MemoryGiB: 8, DiskGiB: 64, Arch: "aarch64",
			SSHPortPolicy: SSHPortPolicyNextFree,
		},
		Secrets: &SecretContract{Mode: SecretModeOpen},
	}
}

func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Resources
	}{
		{
			name: "built-in only",
			in:   Input{BuiltIn: builtInLayer()},
			want: Resources{CPUs: 4, MemoryGiB: 8, DiskGiB: 64, Arch: "aarch64", SSHPortPolicy: SSHPortPolicyNextFree},
		},
		{
			name: "user overrides built-in",
			in: Input{
				BuiltIn: builtInLayer(),
				User: Layer{
					Source:    "~/.kaas/kaas.yaml",
					Resources: &Resources{CPUs: 6, MemoryGiB: 16},
				},
			},
			want: Resources{CPUs: 6, MemoryGiB: 16, DiskGiB: 64, Arch: "aarch64", SSHPortPolicy: SSHPortPolicyNextFree},
		},
		{
			name: "project overrides user",
			in: Input{
				BuiltIn: builtInLayer(),
				User: Layer{
					Source:    "~/.kaas/kaas.yaml",
					Resources: &Resources{CPUs: 6, MemoryGiB: 16},
				},
				Project: Layer{
					Source:    "/proj/kaas.yaml",
					Resources: &Resources{CPUs: 2, DiskGiB: 40},
				},
			},
			want: Resources{CPUs: 2, MemoryGiB: 16, DiskGiB: 40, Arch: "aarch64", SSHPortPolicy: SSHPortPolicyNextFree},
		},
		{
			name: "env overrides project",
			in: Input{
				BuiltIn: builtInLayer(),
				Project: Layer{
					Source:    "/proj/kaas.yaml",
					Resources: &Resources{CPUs: 2},
				},
				Env: Layer{
					Source:    "env",
					Resources: &Resources{MemoryGiB: 32},
				},
			},
			want: Resources{CPUs: 2, MemoryGiB: 32, DiskGiB: 64, Arch: "aarch64", SSHPortPolicy: SSHPortPolicyNextFree},
		},
		{
			name: "flags win",
			in: Input{
				BuiltIn: builtInLayer(),
				User:    Layer{Source: "user", Resources: &Resources{CPUs: 6}},
				Project: Layer{Source: "project", Resources: &Resources{CPUs: 2}},
				Env:     Layer{Source: "env", Resources: &Resources{CPUs: 10}},
				Flags:   FlagsLayer(12, 0, 0, 6025, "x86_64", "flags"),
			},
			want: Resources{CPUs: 12, MemoryGiB: 8, DiskGiB: 64, Arch: "x86_64", SSHPort: 6025},
		},
	}

	var r Resolver
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Resolve(context.Background(), tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Contract.Resources != tt.want {
				t.Fatalf("resources: got %+v want %+v", got.Contract.Resources, tt.want)
			}
			if got.Contract.ContentHash == "" {
				t.Fatal("expected hash")
			}
		})
	}
}

func TestResolvePersistedWinsWithoutAdopt(t *testing.T) {
	persisted, err := sampleContract().WithHash()
	if err != nil {
		t.Fatal(err)
	}
	persisted.Resources.CPUs = 4
	persisted, err = persisted.WithHash()
	if err != nil {
		t.Fatal(err)
	}

	// Resume from another directory: project wants different resources/workspace.
	in := Input{
		BuiltIn: builtInLayer(),
		Project: Layer{
			Source: "/other-project/kaas.yaml",
			Resources: &Resources{
				CPUs: 99, MemoryGiB: 99, DiskGiB: 99, Arch: "x86_64",
			},
			Workspace: &WorkspaceContract{
				Sources: []WorkspaceSourceContract{{
					Repository: "git@github.com:other/repo.git",
					Directory:  "~/workspace/other",
				}},
			},
		},
		Persisted: &persisted,
	}

	got, err := (Resolver{}).Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Adopted {
		t.Fatal("must not adopt when persisted contract exists")
	}
	if got.Contract.Resources.CPUs != 4 {
		t.Fatalf("persisted authority violated: cpus=%d", got.Contract.Resources.CPUs)
	}
	if primary, _ := got.Contract.Workspace.Primary(); primary.Repository != "git@github.com:acme/app.git" {
		t.Fatalf("cwd must not override workspace: %q", primary.Repository)
	}
	if len(got.Changes) == 0 {
		t.Fatal("expected Changes to report drift vs current stack")
	}
}

func TestResolveAdoptsWhenNoPersistedContract(t *testing.T) {
	in := Input{
		BuiltIn: builtInLayer(),
		Project: Layer{
			Source: "/proj/kaas.yaml",
			Resources: &Resources{
				CPUs: 2, MemoryGiB: 8, DiskGiB: 40, Arch: "aarch64",
			},
			Workspace: &WorkspaceContract{
				Sources: []WorkspaceSourceContract{{
					Repository: "git@github.com:acme/app.git",
					Directory:  "~/workspace/app",
				}},
			},
		},
		Persisted: nil, // no persisted row yet
	}
	got, err := (Resolver{}).Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Adopted {
		t.Fatal("expected Adopted=true when no contract exists yet")
	}
	if got.Contract.Resources.CPUs != 2 {
		t.Fatalf("cpus: %d", got.Contract.Resources.CPUs)
	}
	if got.Contract.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema: %d", got.Contract.SchemaVersion)
	}
}

func TestResolveExplicitReconcile(t *testing.T) {
	persisted, err := sampleContract().WithHash()
	if err != nil {
		t.Fatal(err)
	}
	in := Input{
		BuiltIn: builtInLayer(),
		Flags:   FlagsLayer(16, 0, 0, 0, "", "flags"),
		Persisted: &persisted,
		Reconcile: true,
	}
	got, err := (Resolver{}).Resolve(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Adopted {
		t.Fatal("reconcile should mark adopted")
	}
	if got.Contract.Resources.CPUs != 16 {
		t.Fatalf("cpus: %d", got.Contract.Resources.CPUs)
	}
	if len(got.Changes) == 0 {
		t.Fatal("expected changes")
	}
}

func TestCompareDetectsChanges(t *testing.T) {
	a := sampleContract()
	b := sampleContract()
	b.Resources.MemoryGiB = 64
	changes := Compare(&a, &b)
	found := false
	for _, ch := range changes {
		if ch.Path == "resources.memory_gib" {
			found = true
			if ch.From != "8" || ch.To != "64" {
				t.Fatalf("change: %+v", ch)
			}
		}
	}
	if !found {
		t.Fatalf("missing memory change: %+v", changes)
	}
}
