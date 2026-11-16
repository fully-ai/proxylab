# ProxyLab

High-concurrency HTTP, HTTPS, and SOCKS5 proxy verification, anonymity analysis, and reporting toolkit written in Go.

## Features

- **Multi-Protocol Probing**: HTTP, HTTPS, and SOCKS5 support with configurable timeouts and retry attempts.
- **Rate-Limited Worker Pool**: Configurable concurrency and requests-per-second token bucket (`golang.org/x/time/rate`).
- **Anonymity Classification**: Classifies proxies as `elite`, `anonymous`, or `transparent` against an authorized echo judge.
- **Deduplication & Filtering**: Strips duplicate endpoints and filters by status, protocol, anonymity, latency, success rate, and country code.
- **Exporters & Web Dashboard**: Export to JSON, CSV, or TXT from CLI or inspect results via the built-in web UI (`proxylab serve`).
- **Built-in Echo Judge**: Includes `proxylab judge` for isolated, authorized verification without external dependencies.

## Installation

```bash
git clone https://github.com/fully-ai/proxylab.git
cd proxylab
go build -o proxylab ./cmd/proxylab
```

## Usage

Start the local echo judge for authorized testing:

```bash
./proxylab judge -addr 127.0.0.1:8086
```

Run a proxy check:

```bash
./proxylab check -input proxies.txt -judge http://127.0.0.1:8086 -concurrency 32 -rps 50 -format csv -output results.csv
```

Launch the web dashboard:

```bash
./proxylab serve -addr 127.0.0.1:8085 -judge http://127.0.0.1:8086
```

## Testing

```bash
go vet ./...
go test -v -race ./...
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
