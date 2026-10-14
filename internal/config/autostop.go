package config

import (
	"fmt"
	"strings"
	"time"
)

// MaxAutoStop caps the TTL (boat.dev caps at 30 days too).
const MaxAutoStop = 30 * 24 * time.Hour

// ParseAutoStop turns an `--auto-stop` / `defaults.auto_stop` value into a
// TTL. "", "never", "0", and "off" mean no auto-stop (zero duration). Plain
// hours like "4h" and "90m" are Go durations; a bare "2d" means two days.
func ParseAutoStop(value string) (time.Duration, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "never", "0", "off", "none":
		return 0, nil
	}
	if strings.HasSuffix(v, "d") {
		var days float64
		if _, err := fmt.Sscanf(strings.TrimSuffix(v, "d"), "%g", &days); err == nil && days > 0 {
			d := time.Duration(days * float64(24*time.Hour))
			if d > MaxAutoStop {
				return 0, fmt.Errorf("%q exceeds the 30d maximum", value)
			}
			return d, nil
		}
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration (use 1h, 90m, 2d, or never)", value)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%q must be positive or never", value)
	}
	if d < time.Minute {
		return 0, fmt.Errorf("%q is shorter than the 1m minimum", value)
	}
	if d > MaxAutoStop {
		return 0, fmt.Errorf("%q exceeds the 30d maximum", value)
	}
	return d, nil
}

// FormatAutoStop renders a TTL the way users type it.
func FormatAutoStop(d time.Duration) string {
	if d <= 0 {
		return "never"
	}
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return d.String()
}
