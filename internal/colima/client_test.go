package colima

import (
	"encoding/json"
	"testing"
)

func TestParseListNDJSON(t *testing.T) {
	line := `{"name":"kaas","status":"Running","arch":"aarch64","cpus":4}`
	var p Profile
	if err := json.Unmarshal([]byte(line), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "kaas" || p.Status != "Running" {
		t.Fatalf("got %+v", p)
	}
}
