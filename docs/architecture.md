# Architecture — ProxyLab

## Components

- `cmd/proxylab/main.go`: CLI dispatcher providing `check`, `serve`, `judge`, and `version` subcommands.
- `internal/proxy/parser.go`: Parses `protocol://host:port[,COUNTRY]` input streams and deduplicates endpoints by canonical address.
- `internal/proxy/checker.go`: Bounded worker pool with token-bucket rate limiting (`golang.org/x/time/rate`) probing HTTP, HTTPS, and SOCKS5 (`golang.org/x/net/proxy`) targets against an authorized echo judge (`NewEchoJudgeHandler`).
- `internal/proxy/filter.go` & `export.go`: Result filtering, summary aggregation, and JSON/CSV/TXT serialization.
- `internal/dashboard/server.go`: Embedded HTTP dashboard and `/api/results`, `/api/report`, `/api/export`, `/api/check`, `/judge` endpoints.
