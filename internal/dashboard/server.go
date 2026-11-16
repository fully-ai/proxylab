package dashboard

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fully-ai/proxylab/internal/proxy"
)

//go:embed index.html
var indexHTML []byte

type Server struct {
	mu      sync.RWMutex
	results []proxy.CheckResult
	cfg     proxy.CheckConfig
}

func NewServer(initial []proxy.CheckResult, cfg proxy.CheckConfig) *Server {
	return &Server{results: initial, cfg: cfg}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.Handle("/judge", proxy.NewEchoJudgeHandler())
	mux.HandleFunc("/api/results", s.handleResults)
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/check", s.handleCheck)
	return mux
}

func (s *Server) filtered(r *http.Request) []proxy.CheckResult {
	q := r.URL.Query()
	opts := proxy.FilterOptions{
		AliveOnly: q.Get("alive") == "true",
		Protocol:  proxy.Protocol(q.Get("protocol")),
		Anonymity: proxy.Anonymity(q.Get("anonymity")),
		Country:   q.Get("country"),
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return proxy.FilterResults(s.results, opts)
}

func (s *Server) handleResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": s.filtered(r)})
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	report := proxy.Summarize(s.results)
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	items := s.filtered(r)
	format := strings.ToLower(r.URL.Query().Get("format"))
	var buf bytes.Buffer
	switch format {
	case "csv":
		_ = proxy.ExportCSV(&buf, items)
		w.Header().Set("Content-Type", "text/csv")
	case "txt":
		_ = proxy.ExportTXT(&buf, items)
		w.Header().Set("Content-Type", "text/plain")
	default:
		_ = proxy.ExportJSON(&buf, items)
		w.Header().Set("Content-Type", "application/json")
	}
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Proxies  string `json:"proxies"`
		JudgeURL string `json:"judge_url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	specs, _ := proxy.ParseReader(strings.NewReader(req.Proxies), proxy.ProtoHTTP)
	cfg := s.cfg
	if req.JudgeURL != "" {
		cfg.JudgeURL = req.JudgeURL
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	results := proxy.NewChecker(cfg).CheckAll(ctx, specs)
	s.mu.Lock()
	s.results = results
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"checked": len(results), "report": proxy.Summarize(results)})
}
