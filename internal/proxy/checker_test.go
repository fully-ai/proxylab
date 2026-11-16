package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseAndDeduplicate(t *testing.T) {
	input := `
# comment
127.0.0.1:8080,US
http://127.0.0.1:8080,US
socks5://127.0.0.1:1080,DE
invalid-line
`
	specs, dupes := ParseReader(strings.NewReader(input), ProtoHTTP)
	if len(specs) != 2 {
		t.Fatalf("expected 2 unique proxies, got %d", len(specs))
	}
	if dupes != 1 {
		t.Fatalf("expected 1 duplicate, got %d", dupes)
	}
	if specs[0].Country != "US" || specs[1].Protocol != ProtoSOCKS5 {
		t.Fatalf("unexpected parsed specs: %+v", specs)
	}
}

func startTestHTTPProxy(t *testing.T, injectXFF string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if injectXFF != "" {
			req.Header.Set("X-Forwarded-For", injectXFF)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u.Host
}

func startTestSOCKS5(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleSOCKS5Conn(conn)
		}
	}()
	return ln.Addr().String()
}

func handleSOCKS5Conn(c net.Conn) {
	defer c.Close()
	buf := make([]byte, 260)
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(c, buf[:nMethods]); err != nil {
		return
	}
	_, _ = c.Write([]byte{0x05, 0x00}) // no auth

	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		return
	}
	var host string
	switch buf[3] {
	case 0x01: // IPv4
		if _, err := io.ReadFull(c, buf[:4]); err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 0x03: // Domain
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return
		}
		dlen := int(buf[0])
		if _, err := io.ReadFull(c, buf[:dlen]); err != nil {
			return
		}
		host = string(buf[:dlen])
	default:
		return
	}
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	target, err := net.DialTimeout("tcp", net.JoinHostPort(host, itoa(port)), 2*time.Second)
	if err != nil {
		return
	}
	defer target.Close()
	_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	go func() { _, _ = io.Copy(target, c) }()
	_, _ = io.Copy(c, target)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestCheckerHTTPAndSOCKS5AndExports(t *testing.T) {
	judge := httptest.NewServer(NewEchoJudgeHandler())
	defer judge.Close()

	eliteHTTP := startTestHTTPProxy(t, "")
	transHTTP := startTestHTTPProxy(t, "203.0.113.9")
	socksAddr := startTestSOCKS5(t)

	specs, _ := ParseReader(strings.NewReader(
		"http://"+eliteHTTP+",US\n"+
			"http://"+transHTTP+",DE\n"+
			"socks5://"+socksAddr+",NL\n"+
			"http://127.0.0.1:1,FR\n",
	), ProtoHTTP)

	checker := NewChecker(CheckConfig{
		JudgeURL:       judge.URL,
		ClientPublicIP: "203.0.113.9",
		Timeout:        2 * time.Second,
		Concurrency:    4,
		RateLimitRPS:   100,
		Attempts:       2,
	})

	results := checker.CheckAll(context.Background(), specs)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	if !results[0].Alive || results[0].Anonymity != AnonElite || results[0].SuccessRate != 100 {
		t.Fatalf("expected elite alive HTTP proxy, got %+v", results[0])
	}
	if !results[1].Alive || results[1].Anonymity != AnonTransparent {
		t.Fatalf("expected transparent HTTP proxy, got %+v", results[1])
	}
	if !results[2].Alive || results[2].Proxy.Protocol != ProtoSOCKS5 {
		t.Fatalf("expected alive SOCKS5 proxy, got %+v", results[2])
	}
	if results[3].Alive {
		t.Fatalf("expected dead proxy for closed port")
	}

	alive := FilterResults(results, FilterOptions{AliveOnly: true})
	if len(alive) != 3 {
		t.Fatalf("expected 3 alive proxies, got %d", len(alive))
	}

	var jsonBuf, csvBuf, txtBuf bytes.Buffer
	if err := ExportJSON(&jsonBuf, alive); err != nil {
		t.Fatal(err)
	}
	var decoded []CheckResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &decoded); err != nil || len(decoded) != 3 {
		t.Fatalf("unexpected JSON export: %v", err)
	}
	if err := ExportCSV(&csvBuf, alive); err != nil || !strings.Contains(csvBuf.String(), "socks5") {
		t.Fatalf("unexpected CSV export: %v", err)
	}
	if err := ExportTXT(&txtBuf, alive); err != nil || len(strings.Split(strings.TrimSpace(txtBuf.String()), "\n")) != 3 {
		t.Fatalf("unexpected TXT export: %v", err)
	}
}
