package sandbox

import (
	"fmt"
	"sort"
)

// NextPort returns the next free SSH port at or above base, avoiding used.
func NextPort(base int, used []int) (int, error) {
	if base < 1 || base > 65535 {
		return 0, fmt.Errorf("invalid base port: %d", base)
	}
	occupied := make(map[int]struct{}, len(used))
	for _, p := range used {
		occupied[p] = struct{}{}
	}
	for p := base; p <= 65535; p++ {
		if _, taken := occupied[p]; !taken {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free SSH port available from %d", base)
}

// EnsurePortFree returns port if unused, or an error if taken.
func EnsurePortFree(port int, used []int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	for _, p := range used {
		if p == port {
			return fmt.Errorf("ssh port %d already in use", port)
		}
	}
	return nil
}

// MergeUsed unions integer port slices (deduped, sorted).
func MergeUsed(sets ...[]int) []int {
	seen := map[int]struct{}{}
	for _, set := range sets {
		for _, p := range set {
			if p > 0 {
				seen[p] = struct{}{}
			}
		}
	}
	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}
