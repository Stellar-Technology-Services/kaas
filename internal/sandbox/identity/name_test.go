package identity

import (
	"strings"
	"testing"
)

func TestParseSandboxIDValid(t *testing.T) {
	cases := []string{
		"a",
		"ab",
		"my-app",
		"app1",
		"a1b2-c3",
		strings.Repeat("a", MaxNameLength),
	}
	for _, name := range cases {
		id, err := ParseSandboxID(name)
		if err != nil {
			t.Fatalf("ParseSandboxID(%q): %v", name, err)
		}
		if id.Name != name {
			t.Fatalf("ParseSandboxID(%q).Name = %q", name, id.Name)
		}
	}
}

func TestValidateNameRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"", "required"},
		{"MyApp", "invalid"},
		{"my_app", "invalid"},
		{"my.app", "invalid"},
		{"-myapp", "invalid"},
		{"myapp-", "invalid"},
		{"my app", "whitespace"},
		{"my/app", "path separators"},
		{`my\app`, "path separators"},
		{"../victim", "path separators"},
		{"..", "reserved"},
		{".", "reserved"},
		{"default", "reserved"},
		{strings.Repeat("a", MaxNameLength+1), "at most"},
	}
	for _, tc := range cases {
		err := ValidateName(tc.name)
		if err == nil {
			t.Fatalf("ValidateName(%q): expected error containing %q", tc.name, tc.want)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("ValidateName(%q): error %q does not contain %q", tc.name, err.Error(), tc.want)
		}
	}
}
