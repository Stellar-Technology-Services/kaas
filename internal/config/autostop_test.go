package config

import (
	"testing"
	"time"
)

func TestParseAutoStop(t *testing.T) {
	ok := map[string]time.Duration{
		"":      0,
		"never": 0,
		"off":   0,
		"1h":    time.Hour,
		"90m":   90 * time.Minute,
		"2d":    48 * time.Hour,
		"30d":   30 * 24 * time.Hour,
	}
	for in, want := range ok {
		got, err := ParseAutoStop(in)
		if err != nil || got != want {
			t.Errorf("ParseAutoStop(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"31d", "soon", "-1h", "30s", "0.5"} {
		if _, err := ParseAutoStop(bad); err == nil {
			t.Errorf("ParseAutoStop(%q): expected error", bad)
		}
	}
	if FormatAutoStop(0) != "never" || FormatAutoStop(4*time.Hour) != "4h" || FormatAutoStop(48*time.Hour) != "2d" || FormatAutoStop(90*time.Minute) != "1h30m0s" {
		t.Fatalf("FormatAutoStop mismatch: %q %q %q %q", FormatAutoStop(0), FormatAutoStop(4*time.Hour), FormatAutoStop(48*time.Hour), FormatAutoStop(90*time.Minute))
	}
}
