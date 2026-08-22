package ingress

import "testing"

func TestHostname(t *testing.T) {
	got := Hostname("Frontend", "My-App", "kaas.test")
	want := "frontend.my-app.kaas.test"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"API.myapp.kaas.test:18000": "api.myapp.kaas.test",
		"kaas.test":                 "kaas.test",
		"127.0.0.1:18000":           "127.0.0.1",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestURLFor(t *testing.T) {
	u := URLFor("http", "api.demo.kaas.test", "127.0.0.1:18000", true)
	if u != "http://api.demo.kaas.test" {
		t.Fatalf("dns ready: %q", u)
	}
	u = URLFor("http", "api.demo.kaas.test", "127.0.0.1:18000", false)
	if u != "http://api.demo.kaas.test:18000" {
		t.Fatalf("dns not ready: %q", u)
	}
}
