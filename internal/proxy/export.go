package proxy

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

func ExportJSON(w io.Writer, results []CheckResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func ExportCSV(w io.Writer, results []CheckResult) error {
	cw := csv.NewWriter(w)
	header := []string{"protocol", "host", "port", "country", "alive", "latency_ms", "success_rate", "anonymity", "exit_ip", "error"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range results {
		row := []string{
			string(r.Proxy.Protocol),
			r.Proxy.Host,
			r.Proxy.Port,
			r.Proxy.Country,
			strconv.FormatBool(r.Alive),
			strconv.FormatInt(r.LatencyMs, 10),
			fmt.Sprintf("%.2f", r.SuccessRate),
			string(r.Anonymity),
			r.ExitIP,
			r.Error,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func ExportTXT(w io.Writer, results []CheckResult) error {
	for _, r := range results {
		if _, err := fmt.Fprintln(w, r.Proxy.Canonical()); err != nil {
			return err
		}
	}
	return nil
}
