package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestWritePublishedURLsPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	writePublishedURLsFooter(&buf, "my-app", []publishedURLLine{
		{Label: "api", URL: "http://api.my-app.kaas.test"},
		{Label: "frontend", URL: "http://frontend.my-app.kaas.test"},
	}, "http://kaas.test/", "run kaas dns enable so browsers resolve *.kaas.test")

	got := buf.String()
	for _, want := range []string{
		"Published URLs (my-app)",
		"api",
		"http://api.my-app.kaas.test",
		"frontend",
		"http://frontend.my-app.kaas.test",
		"catalog",
		"http://kaas.test/",
		"Hint:",
		"kaas dns enable",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain output missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI escapes with NO_COLOR, got %q", got)
	}
}

func TestWritePublishedURLsOmitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	writePublishedURLsFooter(&buf, "x", nil, "", "")
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got %q", buf.String())
	}
}

func TestPreferPlainTermNOCOLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if !preferPlainTerm(os.Stdout) {
		t.Fatal("expected plain term when NO_COLOR is set")
	}
}

func TestPreferPlainTermDumb(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if !preferPlainTerm(os.Stdout) {
		t.Fatal("expected plain term when TERM=dumb")
	}
}
