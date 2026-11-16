package proxy

import "strings"

// FilterResults returns only results matching the given FilterOptions.
func FilterResults(results []CheckResult, opts FilterOptions) []CheckResult {
	var out []CheckResult
	for _, r := range results {
		if opts.AliveOnly && !r.Alive {
			continue
		}
		if opts.Protocol != "" && r.Proxy.Protocol != opts.Protocol {
			continue
		}
		if opts.Anonymity != "" && r.Anonymity != opts.Anonymity {
			continue
		}
		if opts.MaxLatencyMs > 0 && r.LatencyMs > opts.MaxLatencyMs {
			continue
		}
		if opts.MinSuccessRate > 0 && r.SuccessRate < opts.MinSuccessRate {
			continue
		}
		if opts.Country != "" && !strings.EqualFold(r.Proxy.Country, opts.Country) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Summarize computes aggregate statistics for a slice of CheckResults.
func Summarize(results []CheckResult) SummaryReport {
	report := SummaryReport{
		TotalChecked: len(results),
		ByProtocol:   make(map[string]int),
		ByAnonymity:  make(map[string]int),
	}
	var totalLatency int64
	for _, r := range results {
		report.ByProtocol[string(r.Proxy.Protocol)]++
		if r.Alive {
			report.AliveCount++
			totalLatency += r.LatencyMs
			report.ByAnonymity[string(r.Anonymity)]++
		} else {
			report.DeadCount++
		}
	}
	if report.AliveCount > 0 {
		report.AvgLatencyMs = float64(totalLatency) / float64(report.AliveCount)
	}
	return report
}
