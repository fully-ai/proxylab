package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"
	"golang.org/x/time/rate"
)

type JudgeEcho struct {
	RemoteIP string            `json:"remote_ip"`
	Headers  map[string]string `json:"headers"`
}

// NewEchoJudgeHandler returns an HTTP handler that reports the caller's IP and proxy headers.
// Intended for authorized self-hosted proxy verification.
func NewEchoJudgeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		hdrs := make(map[string]string)
		for _, k := range []string{"X-Forwarded-For", "Via", "Forwarded", "X-Real-Ip", "Proxy-Connection"} {
			if v := r.Header.Get(k); v != "" {
				hdrs[strings.ToLower(k)] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JudgeEcho{RemoteIP: host, Headers: hdrs})
	})
}

type CheckConfig struct {
	JudgeURL       string
	ClientPublicIP string
	Timeout        time.Duration
	Concurrency    int
	RateLimitRPS   float64
	Attempts       int
	InsecureTLS    bool
}

type Checker struct {
	cfg     CheckConfig
	limiter *rate.Limiter
}

func NewChecker(cfg CheckConfig) *Checker {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 16
	}
	if cfg.Attempts <= 0 {
		cfg.Attempts = 1
	}
	var lim *rate.Limiter
	if cfg.RateLimitRPS > 0 {
		lim = rate.NewLimiter(rate.Limit(cfg.RateLimitRPS), max(1, int(cfg.RateLimitRPS)))
	}
	return &Checker{cfg: cfg, limiter: lim}
}

func (c *Checker) CheckAll(ctx context.Context, specs []ProxySpec) []CheckResult {
	results := make([]CheckResult, len(specs))
	jobs := make(chan int, len(specs))
	for i := range specs {
		jobs <- i
	}
	close(jobs)

	workers := min(c.cfg.Concurrency, max(1, len(specs)))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					results[idx] = CheckResult{
						Proxy:     specs[idx],
						Alive:     false,
						Attempts:  0,
						Anonymity: AnonUnknown,
						Error:     ctx.Err().Error(),
						CheckedAt: time.Now().UTC(),
					}
					continue
				}
				results[idx] = c.CheckOne(ctx, specs[idx])
			}
		}()
	}
	wg.Wait()
	return results
}

func (c *Checker) CheckOne(ctx context.Context, spec ProxySpec) CheckResult {
	res := CheckResult{
		Proxy:     spec,
		Attempts:  c.cfg.Attempts,
		Anonymity: AnonUnknown,
		CheckedAt: time.Now().UTC(),
	}

	var totalLatency int64
	var lastErr string

	for i := 0; i < c.cfg.Attempts; i++ {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				lastErr = err.Error()
				break
			}
		}
		latency, echo, err := c.probeOnce(ctx, spec)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		res.Successes++
		totalLatency += latency
		res.ExitIP = echo.RemoteIP
		res.Anonymity = classifyAnonymity(echo, c.cfg.ClientPublicIP)
	}

	if res.Successes > 0 {
		res.Alive = true
		res.LatencyMs = totalLatency / int64(res.Successes)
	} else {
		res.Error = lastErr
	}
	res.SuccessRate = (float64(res.Successes) / float64(res.Attempts)) * 100.0
	return res
}

func (c *Checker) probeOnce(ctx context.Context, spec ProxySpec) (int64, JudgeEcho, error) {
	client, err := c.buildClient(spec)
	if err != nil {
		return 0, JudgeEcho{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.JudgeURL, nil)
	if err != nil {
		return 0, JudgeEcho{}, err
	}
	req.Header.Set("User-Agent", "ProxyLab/0.1")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, JudgeEcho{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, JudgeEcho{}, fmt.Errorf("judge returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return 0, JudgeEcho{}, err
	}

	var echo JudgeEcho
	if err := json.Unmarshal(body, &echo); err != nil {
		return 0, JudgeEcho{}, fmt.Errorf("invalid judge JSON: %w", err)
	}
	return time.Since(start).Milliseconds(), echo, nil
}

func (c *Checker) buildClient(spec ProxySpec) (*http.Client, error) {
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: c.cfg.InsecureTLS},
	}

	switch spec.Protocol {
	case ProtoHTTP, ProtoHTTPS:
		proxyURL := &url.URL{Scheme: string(spec.Protocol), Host: spec.Address()}
		tr.Proxy = http.ProxyURL(proxyURL)
	case ProtoSOCKS5:
		dialer, err := xproxy.SOCKS5("tcp", spec.Address(), nil, &net.Dialer{Timeout: c.cfg.Timeout})
		if err != nil {
			return nil, err
		}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", spec.Protocol)
	}

	return &http.Client{
		Transport: tr,
		Timeout:   c.cfg.Timeout,
	}, nil
}

func classifyAnonymity(echo JudgeEcho, clientIP string) Anonymity {
	xff := echo.Headers["x-forwarded-for"]
	xri := echo.Headers["x-real-ip"]
	fwd := echo.Headers["forwarded"]
	via := echo.Headers["via"]

	if clientIP != "" && (strings.Contains(xff, clientIP) || strings.Contains(xri, clientIP) || strings.Contains(fwd, clientIP)) {
		return AnonTransparent
	}
	if xff != "" || xri != "" || fwd != "" || via != "" {
		return AnonAnonymous
	}
	return AnonElite
}
