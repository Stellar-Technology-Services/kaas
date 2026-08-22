// Package identity owns secret-free sandbox identity value types and validation.
package identity

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxNameLength is the maximum length of a sandbox name.
// It matches the DNS label limit and keeps names safe as directory and profile keys.
const MaxNameLength = 63

// reservedNames cannot be used as sandbox identities because they collide with
// provider defaults or path semantics.
var reservedNames = map[string]struct{}{
	"default": {},
	".":       {},
	"..":      {},
}

// nameRE is the canonical sandbox name grammar:
// lowercase alphanumeric, optional internal hyphens, no leading/trailing hyphen.
var nameRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// SandboxID is a validated user-facing sandbox name.
type SandboxID struct {
	Name string
}

// ParseSandboxID validates name and returns a SandboxID.
func ParseSandboxID(name string) (SandboxID, error) {
	if err := ValidateName(name); err != nil {
		return SandboxID{}, err
	}
	return SandboxID{Name: name}, nil
}

// ValidateName enforces the strict sandbox name invariant.
//
// Rules:
//   - non-empty
//   - at most MaxNameLength characters
//   - lowercase ASCII letters, digits, and internal hyphens only
//   - no leading or trailing hyphen
//   - no whitespace, path separators, dots, underscores, or uppercase
//   - not a reserved name (for example "default")
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("sandbox name is required")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("sandbox name must be at most %d characters", MaxNameLength)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("sandbox name %q is invalid: path separators are not allowed", name)
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return fmt.Errorf("sandbox name %q is invalid: whitespace is not allowed", name)
	}
	if _, reserved := reservedNames[name]; reserved {
		return fmt.Errorf("sandbox name %q is reserved", name)
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("sandbox name %q is invalid: use lowercase letters, digits, and internal hyphens only (e.g. my-app)", name)
	}
	return nil
}
