package proxy

import "time"

type Protocol string

const (
	ProtoHTTP   Protocol = "http"
	ProtoHTTPS  Protocol = "https"
	ProtoSOCKS5 Protocol = "socks5"
)

type Anonymity string

const (
	AnonTransparent Anonymity = "transparent"
	AnonAnonymous   Anonymity = "anonymous"
	AnonElite       Anonymity = "elite"
	AnonUnknown     Anonymity = "unknown"
)

type ProxySpec struct {
	Protocol Protocol `json:"protocol"`
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	Country  string   `json:"country,omitempty"`
}

func (p ProxySpec) Address() string {
	return p.Host + ":" + p.Port
}

func (p ProxySpec) Canonical() string {
	return string(p.Protocol) + "://" + p.Address()
}

type CheckResult struct {
	Proxy       ProxySpec     `json:"proxy"`
	Alive       bool          `json:"alive"`
	LatencyMs   int64         `json:"latency_ms"`
	Attempts    int           `json:"attempts"`
	Successes   int           `json:"successes"`
	SuccessRate float64       `json:"success_rate"`
	Anonymity   Anonymity     `json:"anonymity"`
	ExitIP      string        `json:"exit_ip,omitempty"`
	Error       string        `json:"error,omitempty"`
	CheckedAt   time.Time     `json:"checked_at"`
}

type FilterOptions struct {
	AliveOnly      bool
	Protocol       Protocol
	Anonymity      Anonymity
	MaxLatencyMs   int64
	MinSuccessRate float64
	Country        string
}

type SummaryReport struct {
	TotalChecked int            `json:"total_checked"`
	AliveCount   int            `json:"alive_count"`
	DeadCount    int            `json:"dead_count"`
	AvgLatencyMs float64        `json:"avg_latency_ms"`
	ByProtocol   map[string]int `json:"by_protocol"`
	ByAnonymity  map[string]int `json:"by_anonymity"`
}
