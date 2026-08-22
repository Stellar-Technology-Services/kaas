package ingress

import (
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Route is one Host → upstream mapping.
type Route struct {
	Hostname  string `json:"hostname"`
	HostPort  int    `json:"host_port"`
	Scheme    string `json:"scheme"`
	Sandbox   string `json:"sandbox"`
	Service   string `json:"service"`
	GuestPort int    `json:"guest_port"`
	SSHHost   string `json:"ssh_host,omitempty"`
	SSHPort   int    `json:"ssh_port,omitempty"`
	SSHUser   string `json:"ssh_user,omitempty"`
	SSHKey    string `json:"ssh_key,omitempty"`
}

// Proxy is a Host-header reverse proxy with a catalog at the apex domain.
type Proxy struct {
	Domain string

	mu     sync.RWMutex
	routes map[string]Route // hostname → route
}

// NewProxy constructs an empty proxy for domain.
func NewProxy(domain string) *Proxy {
	if domain == "" {
		domain = DefaultDomain
	}
	return &Proxy{Domain: strings.ToLower(domain), routes: map[string]Route{}}
}

// SetRoutes replaces the route table.
func (p *Proxy) SetRoutes(routes []Route) {
	next := make(map[string]Route, len(routes))
	for _, r := range routes {
		host := NormalizeHost(r.Hostname)
		if host == "" {
			continue
		}
		if r.Scheme == "" {
			r.Scheme = "http"
		}
		r.Hostname = host
		next[host] = r
	}
	p.mu.Lock()
	p.routes = next
	p.mu.Unlock()
}

// Routes returns a snapshot of registered routes.
func (p *Proxy) Routes() []Route {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Route, 0, len(p.routes))
	for _, r := range p.routes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sandbox == out[j].Sandbox {
			return out[i].Service < out[j].Service
		}
		return out[i].Sandbox < out[j].Sandbox
	})
	return out
}

// HasHostname reports whether hostname is registered (or is the catalog apex).
func (p *Proxy) HasHostname(hostname string) bool {
	host := NormalizeHost(hostname)
	if host == CatalogHostname(p.Domain) {
		return true
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.routes[host]
	return ok
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := NormalizeHost(r.Host)
	apex := CatalogHostname(p.Domain)
	if host == apex || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		if r.URL.Path == "/" || r.URL.Path == "" {
			p.serveCatalog(w, r)
			return
		}
	}
	p.mu.RLock()
	route, ok := p.routes[host]
	p.mu.RUnlock()
	if !ok {
		http.Error(w, fmt.Sprintf("unknown service host %q", host), http.StatusNotFound)
		return
	}
	target, err := url.Parse(fmt.Sprintf("%s://127.0.0.1:%d", route.Scheme, route.HostPort))
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		http.Error(rw, fmt.Sprintf("upstream unreachable for %s: %v", route.Hostname, err), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func (p *Proxy) serveCatalog(w http.ResponseWriter, r *http.Request) {
	routes := p.Routes()
	apex := CatalogHostname(p.Domain)
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[")
		for i, route := range routes {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"sandbox":%q,"service":%q,"hostname":%q,"url":%q,"guest_port":%d,"host_port":%d,"scheme":%q}`,
				route.Sandbox, route.Service, route.Hostname,
				route.Scheme+"://"+route.Hostname, route.GuestPort, route.HostPort, route.Scheme)
		}
		fmt.Fprint(w, "]")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>KaaS services</title>")
	fmt.Fprint(w, "<style>body{font-family:ui-sans-serif,system-ui,sans-serif;margin:2rem;line-height:1.4}")
	fmt.Fprint(w, "table{border-collapse:collapse;width:100%;max-width:52rem}th,td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #ddd}")
	fmt.Fprint(w, "a{color:#0b57d0}</style></head><body>")
	fmt.Fprintf(w, "<h1>%s</h1><p>Registered sandbox services.</p>", html.EscapeString(apex))
	if len(routes) == 0 {
		fmt.Fprint(w, "<p><em>No services registered. Declare workspace.services in kaas.yaml and run <code>kaas services sync</code>.</em></p>")
	} else {
		fmt.Fprint(w, "<table><thead><tr><th>Sandbox</th><th>Service</th><th>URL</th><th>Guest</th><th>Host</th></tr></thead><tbody>")
		for _, route := range routes {
			u := route.Scheme + "://" + route.Hostname
			fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td><a href=%q>%s</a></td><td>%d</td><td>%d</td></tr>",
				html.EscapeString(route.Sandbox), html.EscapeString(route.Service),
				html.EscapeString(u), html.EscapeString(u), route.GuestPort, route.HostPort)
		}
		fmt.Fprint(w, "</tbody></table>")
	}
	fmt.Fprint(w, "</body></html>")
}

// ListenAndServeHTTP serves the proxy on addr until the listener is closed.
func ListenAndServeHTTP(addr string, handler http.Handler) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		_ = http.Serve(ln, handler)
	}()
	return ln, nil
}
