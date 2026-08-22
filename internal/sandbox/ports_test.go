package sandbox

import "testing"

func TestNextPort(t *testing.T) {
	p, err := NextPort(6022, nil)
	if err != nil || p != 6022 {
		t.Fatalf("got %d %v", p, err)
	}
	p, err = NextPort(6022, []int{6022, 6023})
	if err != nil || p != 6024 {
		t.Fatalf("got %d %v", p, err)
	}
}

func TestEnsurePortFree(t *testing.T) {
	if err := EnsurePortFree(6022, []int{6023}); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePortFree(6022, []int{6022}); err == nil {
		t.Fatal("expected conflict")
	}
}

func TestMergeUsed(t *testing.T) {
	got := MergeUsed([]int{6023, 6022}, []int{6022, 6024})
	want := []int{6022, 6023, 6024}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
