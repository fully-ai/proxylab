package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fully-ai/proxylab/internal/dashboard"
	"github.com/fully-ai/proxylab/internal/proxy"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "judge":
		runJudge(os.Args[2:])
	case "version":
		fmt.Println("proxylab", version)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: proxylab <check|serve|judge|version> [flags]")
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	input := fs.String("input", "", "Path to proxy list file")
	output := fs.String("output", "", "Optional output file path")
	format := fs.String("format", "json", "Output format: json, csv, txt")
	judgeURL := fs.String("judge", "", "Authorized echo judge URL")
	proto := fs.String("proto", "http", "Default protocol when omitted")
	concurrency := fs.Int("concurrency", 32, "Worker concurrency")
	rps := fs.Float64("rps", 0, "Max requests per second (0 = unlimited)")
	timeout := fs.Duration("timeout", 5*time.Second, "Per-request timeout")
	attempts := fs.Int("attempts", 1, "Attempts per proxy")
	aliveOnly := fs.Bool("alive-only", false, "Filter to alive proxies only")
	_ = fs.Parse(args)

	if *input == "" || *judgeURL == "" {
		fmt.Fprintln(os.Stderr, "error: -input and -judge are required")
		os.Exit(2)
	}

	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening input: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	specs, dupes := proxy.ParseReader(f, proxy.Protocol(*proto))
	fmt.Fprintf(os.Stderr, "Loaded %d unique proxies (%d duplicates removed)\n", len(specs), dupes)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	checker := proxy.NewChecker(proxy.CheckConfig{
		JudgeURL:     *judgeURL,
		Timeout:      *timeout,
		Concurrency:  *concurrency,
		RateLimitRPS: *rps,
		Attempts:     *attempts,
	})
	results := checker.CheckAll(ctx, specs)
	filtered := proxy.FilterResults(results, proxy.FilterOptions{AliveOnly: *aliveOnly})

	out := os.Stdout
	if *output != "" {
		out, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating output: %v\n", err)
			os.Exit(1)
		}
		defer out.Close()
	}

	switch strings.ToLower(*format) {
	case "csv":
		err = proxy.ExportCSV(out, filtered)
	case "txt":
		err = proxy.ExportTXT(out, filtered)
	default:
		err = proxy.ExportJSON(out, filtered)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "export error: %v\n", err)
		os.Exit(1)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8085", "Dashboard listen address")
	judgeURL := fs.String("judge", "", "Default authorized echo judge URL")
	_ = fs.Parse(args)

	srv := dashboard.NewServer(nil, proxy.CheckConfig{
		JudgeURL:    *judgeURL,
		Timeout:     5 * time.Second,
		Concurrency: 16,
		Attempts:    1,
	})
	fmt.Printf("ProxyLab dashboard listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func runJudge(args []string) {
	fs := flag.NewFlagSet("judge", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8086", "Echo judge listen address")
	_ = fs.Parse(args)
	fmt.Printf("ProxyLab echo judge listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, proxy.NewEchoJudgeHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "judge error: %v\n", err)
		os.Exit(1)
	}
}
