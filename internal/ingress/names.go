package ingress

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const DefaultDomain = "kaas.test"

// Hostname builds service.sandbox.domain (lowercase DNS labels).
func Hostname(service, sandbox, domain string) string {
	service = strings.ToLower(strings.TrimSpace(service))
	sandbox = strings.ToLower(strings.TrimSpace(sandbox))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		domain = DefaultDomain
	}
	return fmt.Sprintf("%s.%s.%s", service, sandbox, domain)
}

// CatalogHostname is the apex domain used for the service list page.
func CatalogHostname(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		domain = DefaultDomain
	}
	return domain
}

// NormalizeHost strips an optional port and lowercases the host.
func NormalizeHost(hostport string) string {
	host := strings.TrimSpace(hostport)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasSuffix(host, "]") {
		// bare IPv6 in brackets without port
		host = strings.Trim(host, "[]")
	} else if i := strings.LastIndex(host, ":"); i > 0 {
		// host:port without brackets (IPv4 or name)
		if _, err := strconv.Atoi(host[i+1:]); err == nil {
			host = host[:i]
		}
	}
	return strings.ToLower(host)
}

// URLFor builds an absolute URL for a published service via the ingress listen address.
// When dnsReady is true, use the hostname; otherwise use listen with Host implied by path docs.
func URLFor(scheme, hostname, listen string, dnsReady bool) string {
	if scheme == "" {
		scheme = "http"
	}
	if dnsReady {
		return scheme + "://" + hostname
	}
	// Without local DNS, browsers still need Host routing; surface the listen URL
	// and document curl --resolve / Host header usage in CLI output.
	host, port, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		return scheme + "://" + hostname
	}
	if port == "80" && scheme == "http" {
		return scheme + "://" + hostname
	}
	return fmt.Sprintf("%s://%s:%s", scheme, hostname, port)
}
