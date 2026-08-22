package egress

import (
	"errors"
	"testing"
)

type fakeTopology struct{ err error }

func (f fakeTopology) Name() string              { return "test" }
func (f fakeTopology) Enforce(string, int) error { return nil }
func (f fakeTopology) Verify() error             { return f.err }
func TestProxyOnlyFailsClosedWithoutVerifiedBackend(t *testing.T) {
	if err := RequireEnforced("proxy_only", nil); err == nil {
		t.Fatal("accepted unavailable topology")
	}
	if err := RequireEnforced("proxy_only", fakeTopology{}); err != nil {
		t.Fatal(err)
	}
	if err := RequireEnforced("proxy_only", fakeTopology{errors.New("bypass works")}); err == nil {
		t.Fatal("accepted failed verification")
	}
}
