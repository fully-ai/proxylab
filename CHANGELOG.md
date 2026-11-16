# Changelog

## [0.2.0] - 2026-11-16

### Added
- Dashboard HTTP API integration tests (`internal/dashboard/server_test.go`) verifying `/api/check`, `/api/results`, `/api/export`, and `/judge`.
- Complete maintainer README with Mermaid diagram, API table, CLI flag reference, and known limitations.

## [0.1.0] - 2026-11-16

### Added
- Concurrent HTTP, HTTPS, and SOCKS5 proxy checker with token-bucket rate limiting.
- Anonymity level classification (`elite`, `anonymous`, `transparent`) via authorized echo judge.
- Deduplication, filtering, and CSV/JSON/TXT exporters.
- Embedded web dashboard and JSON API (`proxylab serve`).
