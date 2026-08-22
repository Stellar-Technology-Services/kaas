package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTablePlainNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	writeTable(&buf, []string{"NAME", "STATUS"}, [][]string{
		{"ansible", "OK"},
		{"colima", "MISSING"},
	}, tableOpts{StatusCols: []int{1}})
	got := buf.String()
	for _, want := range []string{"NAME", "STATUS", "ansible", "OK", "colima", "MISSING"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI with NO_COLOR, got %q", got)
	}
}

func TestWriteKVPlainNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	writeKV(&buf, []kvPair{{Key: "RUNNING", Value: "true"}, {Key: "LISTEN", Value: "127.0.0.1:18181"}})
	got := buf.String()
	if !strings.Contains(got, "RUNNING") || !strings.Contains(got, "true") {
		t.Fatalf("unexpected output %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI with NO_COLOR, got %q", got)
	}
}

func TestStatusStyleKinds(t *testing.T) {
	s := newTermStyles(&bytes.Buffer{})
	cases := []string{"ok", "running", "missing", "failed", "stopped", "warn", "other"}
	for _, c := range cases {
		_ = statusStyle(s, c).Render(c)
	}
}
