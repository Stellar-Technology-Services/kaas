package cli

import "testing"

func TestParseSandboxPath(t *testing.T) {
	cases := []struct {
		value, name, path string
		remote            bool
	}{
		{"app:~/report.xml", "app", "~/report.xml", true},
		{"./patch.diff", "", "./patch.diff", false},
		{"/tmp/patch.diff", "", "/tmp/patch.diff", false},
		{"app:/tmp/report.xml", "app", "/tmp/report.xml", true},
	}
	for _, tc := range cases {
		name, path, remote := parseSandboxPath(tc.value)
		if name != tc.name || path != tc.path || remote != tc.remote {
			t.Fatalf("%q: got (%q, %q, %v)", tc.value, name, path, remote)
		}
	}
}

func TestShellEscapeRemotePath(t *testing.T) {
	if got := shellEscapeRemotePath("~/workspace/app/file"); got != "~/workspace/app/file" {
		t.Fatalf("home path: %q", got)
	}
	if got := shellEscapeRemotePath("/tmp/file with spaces"); got != "'/tmp/file with spaces'" {
		t.Fatalf("absolute path: %q", got)
	}
}
