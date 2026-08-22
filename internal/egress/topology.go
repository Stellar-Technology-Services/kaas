// Package egress exposes topology capabilities. KaaS never silently treats
// proxy environment variables as proxy-only enforcement.
package egress

import "fmt"

// Topology is the enforcement backend contract. A platform implementation must
// prove every non-proxy egress path is blocked before proxy_only is enabled.
type Topology interface {
	Name() string
	Enforce(proxyHost string, proxyPort int) error
	Verify() error
}

// RequireEnforced rejects proxy_only unless an independently verified backend
// is supplied. This fail-closed guard prevents a configuration typo or a mere
// HTTP_PROXY setting from becoming a false security claim.
func RequireEnforced(mode string, backend Topology) error {
	if mode != "proxy_only" {
		return nil
	}
	if backend == nil {
		return fmt.Errorf("egress.mode proxy_only is unavailable: no verified Colima/Lima topology backend")
	}
	if err := backend.Verify(); err != nil {
		return fmt.Errorf("egress.mode proxy_only topology verification: %w", err)
	}
	return nil
}
