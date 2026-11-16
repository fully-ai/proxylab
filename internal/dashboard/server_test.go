package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fully-ai/proxylab/internal/proxy"
)

func TestDashboardAPICheckReportAndExports(t *testing.T) {
	judge := httptest.NewServer(proxy.NewEchoJudgeHandler())
	defer judge.Close()

	// Local test HTTP proxy forwarding to authorized judge
	testProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = bytes.NewBuffer(nil).ReadFrom(resp.Body)
	}))
	defer testProxy.Close()

	// Better proxy handler that actually copies body back:
	testProxy.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.RequestURI, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(buf.Bytes())
	})

	u, _ := url.Parse(testProxy.URL)
	srv := NewServer(nil, proxy.CheckConfig{
		JudgeURL:    judge.URL,
		Timeout:     2 * time.Second,
		Concurrency: 4,
		Attempts:    1,
	})
	h := srv.Handler()

	// Trigger live check via /api/check
	payload := `{"proxies":"http://` + u.Host + `,US\nhttp://127.0.0.1:1,DE"}`
	wCheck := httptest.NewRecorder()
	h.ServeHTTP(wCheck, httptest.NewRequest(http.MethodPost, "/api/check", strings.NewReader(payload)))
	if wCheck.Code != http.StatusOK {
		t.Fatalf("expected 200 from /api/check, got %d", wCheck.Code)
	}

	var checkResp struct {
		Checked int                 `json:"checked"`
		Report  proxy.SummaryReport `json:"report"`
	}
	if err := json.Unmarshal(wCheck.Body.Bytes(), &checkResp); err != nil {
		t.Fatal(err)
	}
	if checkResp.Checked != 2 || checkResp.Report.AliveCount != 1 {
		t.Fatalf("unexpected check summary: %+v", checkResp)
	}

	// Filter alive only via /api/results?alive=true
	wRes := httptest.NewRecorder()
	h.ServeHTTP(wRes, httptest.NewRequest(http.MethodGet, "/api/results?alive=true", nil))
	if wRes.Code != http.StatusOK || !strings.Contains(wRes.Body.String(), u.Port()) {
		t.Fatalf("unexpected /api/results body: %s", wRes.Body.String())
	}

	// CSV export
	wCSV := httptest.NewRecorder()
	h.ServeHTTP(wCSV, httptest.NewRequest(http.MethodGet, "/api/export?format=csv&alive=true", nil))
	if wCSV.Code != http.StatusOK || !strings.Contains(wCSV.Body.String(), "protocol,host,port") {
		t.Fatalf("unexpected CSV export: %s", wCSV.Body.String())
	}
}
