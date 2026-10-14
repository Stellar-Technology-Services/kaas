package ui

import (
	"math/rand"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
)

// Short word lists for `boat new`-style auto-named sandboxes. Every pair
// satisfies the identity grammar (lowercase, digits, internal hyphens).
var (
	nameAdjectives = []string{"amber", "brisk", "calm", "crisp", "dusky", "eager", "fleet", "gentle", "hazel", "ivory", "jolly", "keen", "lucid", "mellow", "nimble", "olive", "plucky", "quiet", "rosy", "sunny", "tidy", "vivid", "witty", "zesty"}
	nameNouns      = []string{"otter", "falcon", "harbor", "meadow", "pebble", "quill", "ridge", "sparrow", "thicket", "willow", "anchor", "beacon", "canyon", "delta", "ember", "fjord", "glade", "heron", "island", "juniper", "kestrel", "lagoon", "marsh", "nook"}
)

// suggestName returns a random adjective-noun name that is not taken.
func suggestName(taken map[string]bool) string {
	for i := 0; i < 20; i++ {
		name := nameAdjectives[rand.Intn(len(nameAdjectives))] + "-" + nameNouns[rand.Intn(len(nameNouns))]
		if taken[name] {
			continue
		}
		if err := identity.ValidateName(name); err == nil {
			return name
		}
	}
	return ""
}

// normalizeName lowercases and swaps spaces/underscores for hyphens before the
// strict grammar check, so a form-typed "My App" becomes "my-app".
func normalizeName(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.NewReplacer(" ", "-", "_", "-", ".", "-").Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
