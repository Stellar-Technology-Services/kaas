package cli

import (
	"strings"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func stateSandbox(workspaceDir string) state.Sandbox {
	return state.Sandbox{Name: "demo", WorkspaceDirectory: workspaceDir}
}

func TestDetachScriptShape(t *testing.T) {
	s := detachScript("ab12cd34", "~/workspace/app", "npm run build && echo 'done'")
	for _, want := range []string{
		"mkdir -p $HOME/.kaas-runs",
		"cd $HOME/'workspace/app'",
		"nohup setsid bash -lc",
		"ab12cd34.exit",
		"> $HOME/.kaas-runs/ab12cd34.log 2>&1 < /dev/null & echo $!",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("detach script missing %q:\n%s", want, s)
		}
	}
	// The user's single quotes must survive shellQuote so the inner bash sees them.
	if !strings.Contains(s, `echo '"'"'done'"'"'`) {
		t.Errorf("inner quotes not escaped: %s", s)
	}
}

func TestStatusScriptAndParsePs(t *testing.T) {
	s := statusScript("ab12cd34", 4242, 10)
	if !strings.Contains(s, "kill -0 4242") || !strings.Contains(s, "tail -n 10") || !strings.Contains(s, "echo ---") {
		t.Fatalf("status script: %s", s)
	}
	rows := parsePs("  PID %CPU   RSS USER     ELAPSED COMMAND\n 2210 98.0 831897 user   01:02:03 node build.js --watch\n")
	if len(rows) != 1 || rows[0]["pid"] != 2210 || rows[0]["command"] != "node build.js --watch" || rows[0]["memory_mb"] != 812 {
		t.Fatalf("parsePs: %+v", rows)
	}
}

func TestDefaultCwd(t *testing.T) {
	if got := defaultCwd(stateSandbox("~/workspace/x"), ""); got != "~/workspace/x" {
		t.Fatalf("workspace default: %q", got)
	}
	if got := defaultCwd(stateSandbox(""), ""); got != "~" {
		t.Fatalf("home default: %q", got)
	}
	if got := defaultCwd(stateSandbox("~/workspace/x"), "/tmp"); got != "/tmp" {
		t.Fatalf("explicit wins: %q", got)
	}
}
