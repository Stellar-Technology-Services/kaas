package state

import (
	"path/filepath"
	"testing"
)

func TestServiceRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.CreateSandbox(Sandbox{
		Name: "demo", RepoPath: "/repo", RepoName: "repo", SSHPort: 6022,
	}); err != nil {
		t.Fatal(err)
	}
	svc := Service{
		Sandbox: "demo", Service: "api", GuestPort: 8080, HostPort: 19001,
		Scheme: "http", Hostname: "api.demo.kaas.test", Status: "registered",
	}
	if err := s.UpsertService(svc); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetService("demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if got.HostPort != 19001 || got.Hostname != "api.demo.kaas.test" {
		t.Fatalf("unexpected service: %+v", got)
	}
	ports, err := s.UsedServicePorts()
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 1 || ports[0] != 19001 {
		t.Fatalf("ports: %v", ports)
	}
	list, err := s.ListServices("demo")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if err := s.DeleteSandbox("demo"); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListServices("")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected cascade delete, got %v", list)
	}
}
