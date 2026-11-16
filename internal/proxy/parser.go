package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

// ParseLine parses a single proxy line in `protocol://host:port` or `host:port` format,
// optionally followed by `,COUNTRY`.
func ParseLine(line string, defaultProto Protocol) (ProxySpec, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ProxySpec{}, fmt.Errorf("empty or comment line")
	}

	var country string
	if idx := strings.Index(trimmed, ","); idx != -1 {
		country = strings.TrimSpace(trimmed[idx+1:])
		trimmed = strings.TrimSpace(trimmed[:idx])
	}

	proto := defaultProto
	if !strings.Contains(trimmed, "://") {
		trimmed = string(defaultProto) + "://" + trimmed
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return ProxySpec{}, fmt.Errorf("invalid proxy URL %q: %w", line, err)
	}
	switch Protocol(strings.ToLower(u.Scheme)) {
	case ProtoHTTP:
		proto = ProtoHTTP
	case ProtoHTTPS:
		proto = ProtoHTTPS
	case ProtoSOCKS5:
		proto = ProtoSOCKS5
	default:
		return ProxySpec{}, fmt.Errorf("unsupported protocol %q", u.Scheme)
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" || port == "" {
		return ProxySpec{}, fmt.Errorf("invalid host:port in %q", line)
	}

	return ProxySpec{
		Protocol: proto,
		Host:     host,
		Port:     port,
		Country:  strings.ToUpper(country),
	}, nil
}

// ParseReader parses and deduplicates proxy entries from a reader.
func ParseReader(r io.Reader, defaultProto Protocol) ([]ProxySpec, int) {
	scanner := bufio.NewScanner(r)
	seen := make(map[string]struct{})
	var specs []ProxySpec
	duplicates := 0

	for scanner.Scan() {
		spec, err := ParseLine(scanner.Text(), defaultProto)
		if err != nil {
			continue
		}
		key := spec.Canonical()
		if _, exists := seen[key]; exists {
			duplicates++
			continue
		}
		seen[key] = struct{}{}
		specs = append(specs, spec)
	}
	return specs, duplicates
}
