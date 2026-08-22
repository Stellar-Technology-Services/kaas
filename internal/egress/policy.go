// Package egress evaluates KaaS's value-free, project-owned egress policy.
package egress

import (
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

type Decision struct {
	Allowed bool
	Rule    *config.EgressRule
}

// Decide determines reachability. Credential use is deliberately independent:
// an allowed host has no credential unless its matching rule selects one.
func Decide(policy config.Egress, host string) Decision {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for i := range policy.Rules {
		rule := &policy.Rules[i]
		if matches(rule.Host, host) {
			return Decision{Allowed: true, Rule: rule}
		}
	}
	return Decision{Allowed: policy.Default == "allow"}
}

func matches(pattern, host string) bool {
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*") // retains dot
		return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
	}
	return host == pattern
}

// RedactedPlan is safe for CLI, JSONL, events, and persisted metadata. It
// expresses key names and injection methods but cannot contain resolved values.
type RedactedPlan struct {
	SchemaVersion int            `json:"schema_version" yaml:"schema_version"`
	Mode          string         `json:"mode" yaml:"mode"`
	Default       string         `json:"default" yaml:"default"`
	Rules         []RedactedRule `json:"rules" yaml:"rules"`
}
type RedactedRule struct {
	Host       string `json:"host" yaml:"host"`
	Credential string `json:"credential,omitempty" yaml:"credential,omitempty"`
	Injection  string `json:"injection,omitempty" yaml:"injection,omitempty"`
	Header     string `json:"header,omitempty" yaml:"header,omitempty"`
}

func Plan(policy config.Egress) RedactedPlan {
	plan := RedactedPlan{SchemaVersion: policy.SchemaVersion, Mode: policy.Mode, Default: policy.Default}
	for _, rule := range policy.Rules {
		out := RedactedRule{Host: rule.Host}
		if rule.InjectSecret != nil {
			out.Credential, out.Injection, out.Header = rule.InjectSecret.Key, rule.InjectSecret.Type, rule.InjectSecret.Header
		}
		plan.Rules = append(plan.Rules, out)
	}
	return plan
}
