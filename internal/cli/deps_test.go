package cli

import (
	"runtime"
	"strings"
	"testing"
)

func TestHostDependenciesDarwin(t *testing.T) {
	deps := hostDependencies("darwin")
	if len(deps) == 0 {
		t.Fatal("expected darwin deps")
	}
	byName := map[string]hostDep{}
	for _, d := range deps {
		byName[d.Name] = d
		if !d.Required && d.Name != "herdr" {
			t.Fatalf("unexpected optional dep %q", d.Name)
		}
		if len(d.BrewPackages) == 0 {
			t.Fatalf("%s: expected brew packages", d.Name)
		}
		if !autoInstallable(d, "darwin") {
			t.Fatalf("%s should be auto-installable on darwin", d.Name)
		}
	}
	for _, name := range []string{"ansible", "colima", "lima", "secretspec"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing required dep %s", name)
		}
	}
	if _, ok := byName["herdr"]; !ok {
		t.Fatal("missing optional herdr")
	}
}

func TestHostDependenciesLinux(t *testing.T) {
	deps := hostDependencies("linux")
	if len(deps) == 0 {
		t.Fatal("expected linux deps")
	}
	byName := map[string]hostDep{}
	for _, d := range deps {
		byName[d.Name] = d
	}
	for _, name := range []string{"kvm", "ansible", "curl", "lima", "colima", "secretspec"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing required dep %s", name)
		}
	}
	kvm := byName["kvm"]
	if len(kvm.AptPackages) < 3 {
		t.Fatalf("kvm should list apt packages, got %v", kvm.AptPackages)
	}
	if !autoInstallable(kvm, "linux") {
		t.Fatal("kvm apt packages should be auto-installable")
	}
	if !autoInstallable(byName["lima"], "linux") {
		t.Fatal("lima installer should be auto-installable")
	}
	if hint := installHint(byName["ansible"], "linux"); !strings.Contains(hint, "apt") {
		t.Fatalf("ansible hint: %q", hint)
	}
}

func TestHostDependenciesUnsupported(t *testing.T) {
	if deps := hostDependencies("windows"); deps != nil {
		t.Fatalf("expected nil, got %#v", deps)
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings(nil, "a", "b", "a", "", "c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("got %v", got)
	}
}

func TestLinuxReleaseArch(t *testing.T) {
	arch, err := linuxReleaseArch()
	switch runtime.GOARCH {
	case "amd64":
		if err != nil || arch.lima != "x86_64" || arch.colima != "x86_64" {
			t.Fatalf("amd64: %+v %v", arch, err)
		}
	case "arm64":
		if err != nil || arch.lima != "aarch64" || arch.colima != "arm64" {
			t.Fatalf("arm64: %+v %v", arch, err)
		}
	default:
		if err == nil {
			t.Fatalf("expected error for arch %s", runtime.GOARCH)
		}
	}
}

func TestMissingAutoFiltersOptional(t *testing.T) {
	statuses := []hostDepStatus{
		{hostDep: hostDep{Name: "ansible", Required: true, BrewPackages: []string{"ansible"}}, Status: "missing"},
		{hostDep: hostDep{Name: "herdr", Required: false, BrewPackages: []string{"herdr"}}, Status: "missing"},
		{hostDep: hostDep{Name: "colima", Required: true, BrewPackages: []string{"colima"}}, Status: "ok"},
	}
	// Force GOOS-sensitive helper by testing filter logic inline for darwin packages.
	got := missingAutoInstallableFor(statuses, false, "darwin")
	if len(got) != 1 || got[0].Name != "ansible" {
		t.Fatalf("without optional: %v", namesOf(got))
	}
	got = missingAutoInstallableFor(statuses, true, "darwin")
	if len(got) != 2 {
		t.Fatalf("with optional: %v", namesOf(got))
	}
}

func missingAutoInstallableFor(statuses []hostDepStatus, includeOptional bool, goos string) []hostDep {
	var out []hostDep
	for _, st := range statuses {
		if st.Status == "ok" {
			continue
		}
		if !st.Required && !includeOptional {
			continue
		}
		if autoInstallable(st.hostDep, goos) {
			out = append(out, st.hostDep)
		}
	}
	return out
}

func namesOf(deps []hostDep) []string {
	out := make([]string, len(deps))
	for i, d := range deps {
		out[i] = d.Name
	}
	return out
}
