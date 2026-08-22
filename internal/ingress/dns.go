package ingress

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
)

// DNSServer answers A/AAAA for names under Domain with Address / Address6.
type DNSServer struct {
	Domain   string
	Address  net.IP // IPv4 answer (usually 127.0.0.1)
	Address6 net.IP // optional IPv6 (::1)
	// Allowed, when non-nil, restricts answers to registered hostnames (lowercase).
	Allowed func(hostname string) bool

	mu   sync.Mutex
	conn *net.UDPConn
}

// ListenAndServe binds UDP DNS on addr (e.g. 127.0.0.1:5353) until ctx is done.
func (s *DNSServer) ListenAndServe(ctx context.Context, addr string) error {
	if s.Domain == "" {
		s.Domain = DefaultDomain
	}
	s.Domain = strings.ToLower(strings.TrimSuffix(s.Domain, "."))
	if s.Address == nil {
		s.Address = net.IPv4(127, 0, 0, 1)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen DNS %s: %w", addr, err)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, 512)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		resp, ok := s.answer(buf[:n])
		if !ok {
			continue
		}
		_, _ = conn.WriteToUDP(resp, remote)
	}
}

func (s *DNSServer) answer(req []byte) ([]byte, bool) {
	if len(req) < 12 {
		return nil, false
	}
	// Only standard queries.
	if req[2]&0x80 != 0 {
		return nil, false
	}
	qdCount := binary.BigEndian.Uint16(req[4:6])
	if qdCount == 0 {
		return nil, false
	}
	name, qtype, qclass, off, ok := parseQuestion(req, 12)
	if !ok {
		return nil, false
	}
	_ = off
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	domain := s.Domain
	if !(name == domain || strings.HasSuffix(name, "."+domain)) {
		return buildDNSResponse(req, name, qtype, qclass, nil, nil, true), true
	}
	if s.Allowed != nil && !s.Allowed(name) && name != domain {
		return buildDNSResponse(req, name, qtype, qclass, nil, nil, true), true
	}
	var a, aaaa net.IP
	switch qtype {
	case 1: // A
		a = s.Address.To4()
	case 28: // AAAA
		if s.Address6 != nil {
			aaaa = s.Address6
		}
	case 255: // ANY
		a = s.Address.To4()
		aaaa = s.Address6
	default:
		return buildDNSResponse(req, name, qtype, qclass, nil, nil, true), true
	}
	return buildDNSResponse(req, name, qtype, qclass, a, aaaa, false), true
}

func parseQuestion(msg []byte, off int) (name string, qtype, qclass uint16, next int, ok bool) {
	var labels []string
	for {
		if off >= len(msg) {
			return "", 0, 0, 0, false
		}
		l := int(msg[off])
		off++
		if l == 0 {
			break
		}
		if l&0xC0 != 0 {
			return "", 0, 0, 0, false // compression in question not expected from stub
		}
		if off+l > len(msg) {
			return "", 0, 0, 0, false
		}
		labels = append(labels, string(msg[off:off+l]))
		off += l
	}
	if off+4 > len(msg) {
		return "", 0, 0, 0, false
	}
	qtype = binary.BigEndian.Uint16(msg[off : off+2])
	qclass = binary.BigEndian.Uint16(msg[off+2 : off+4])
	return strings.Join(labels, "."), qtype, qclass, off + 4, true
}

func buildDNSResponse(req []byte, name string, qtype, qclass uint16, a, aaaa net.IP, nx bool) []byte {
	// Echo question from request when possible; rebuild otherwise.
	out := make([]byte, 0, 128)
	out = append(out, req[0], req[1]) // ID
	flags := uint16(0x8180)           // response, recursion available
	if nx {
		flags = 0x8183 // NXDOMAIN
	}
	var hdr [10]byte
	binary.BigEndian.PutUint16(hdr[0:2], flags)
	binary.BigEndian.PutUint16(hdr[2:4], 1) // QDCOUNT
	ancount := 0
	if a != nil {
		ancount++
	}
	if aaaa != nil {
		ancount++
	}
	if nx {
		ancount = 0
	}
	binary.BigEndian.PutUint16(hdr[4:6], uint16(ancount))
	out = append(out, hdr[:]...)

	out = append(out, encodeName(name)...)
	var qtail [4]byte
	binary.BigEndian.PutUint16(qtail[0:2], qtype)
	binary.BigEndian.PutUint16(qtail[2:4], qclass)
	out = append(out, qtail[:]...)

	if !nx {
		if a != nil {
			out = appendRR(out, name, 1, qclass, a.To4())
		}
		if aaaa != nil {
			ip := aaaa.To16()
			out = appendRR(out, name, 28, qclass, ip)
		}
	}
	return out
}

func encodeName(name string) []byte {
	if name == "" {
		return []byte{0}
	}
	parts := strings.Split(name, ".")
	out := make([]byte, 0, len(name)+2)
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, byte(len(p)))
		out = append(out, p...)
	}
	out = append(out, 0)
	return out
}

func appendRR(out []byte, name string, typ, class uint16, rdata []byte) []byte {
	out = append(out, encodeName(name)...)
	var mid [10]byte
	binary.BigEndian.PutUint16(mid[0:2], typ)
	binary.BigEndian.PutUint16(mid[2:4], class)
	binary.BigEndian.PutUint32(mid[4:8], 30) // TTL
	binary.BigEndian.PutUint16(mid[8:10], uint16(len(rdata)))
	out = append(out, mid[:]...)
	out = append(out, rdata...)
	return out
}
