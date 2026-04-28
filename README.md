# CacheProxyfy

A self-hosted caching proxy for package registries (npm, PyPI, Maven). It sits between your build environment and the public registries, caching artifacts locally so that repeated builds are faster, more reliable, and do not depend on external availability.

---

## Why This Exists

Build pipelines are fragile. Every `npm install`, `pip install`, or `mvn package` fires off dozens of HTTP requests to public registries. This creates several problems:

- **Slow builds** — cold fetches from public registries add seconds to every CI run
- **Flaky builds** — registry outages or rate limits fail builds for reasons unrelated to your code
- **Security blind spots** — packages with known CVEs are silently installed
- **No audit trail** — there is no record of what was fetched, when, or by whom

CacheProxyfy solves all of these by acting as a transparent caching layer in front of the registries.

---

## Goals

- Cache any package once and serve it locally on every subsequent request
- Support npm, PyPI, and Maven without changing client tooling
- Scan packages against the OSV vulnerability database and block or warn based on configurable severity thresholds
- Evict stale artifacts automatically based on a configurable TTL
- Expose cache statistics and security alerts through a REST API and a Next.js dashboard
- Run as a single binary or a fully containerised stack via Docker Compose

---

## Tech Stack

| Layer | Technology |
|---|---|
| Proxy server | Go (`net/http`) |
| Metadata store | PostgreSQL |
| Cache index | Redis |
| Artifact store | Local filesystem or S3-compatible object store |
| CVE scanning | OSV API (`api.osv.dev`) |
| Metrics | Prometheus + OpenMetrics |
| Dashboard | Next.js (App Router) |
| Containerisation | Docker, Docker Compose |

---

## Architecture

```
                        ┌─────────────────────────────────┐
  Build tool            │           CacheProxyfy           │
  (npm / pip / mvn) ───►│                                  │
                        │  Router → Ecosystem Handler      │
                        │       ↓                          │
                        │  Redis (L1 cache)                │
                        │       ↓ miss                     │
                        │  PostgreSQL (L2 metadata)        │
                        │       ↓ miss                     │
                        │  Security check (OSV API)        │
                        │       ↓ allowed                  │
                        │  Singleflight upstream fetch     │
                        │       ↓                          │
                        │  Storage (local FS / S3)         │
                        └─────────────────────────────────┘
                                    │
                        ┌───────────▼──────────┐
                        │  Internal API :9090  │
                        │  /metrics            │
                        │  /api/*              │
                        └───────────┬──────────┘
                                    │
                        ┌───────────▼──────────┐
                        │  Next.js Dashboard   │
                        │  :3000               │
                        └──────────────────────┘
```

### Request flow

1. A build tool sends a request to the proxy (e.g. `GET /npm/lodash/-/lodash-4.17.21.tgz`)
2. The router identifies the ecosystem and delegates to the appropriate handler
3. The proxy performs a three-tier cache lookup:
   - **L1 — Redis**: fast in-memory lookup by `cpf:<ecosystem>:<name>:<version>` key → returns checksum
   - **L2 — PostgreSQL**: authoritative metadata store → restores Redis entry on hit
   - **L3 — Upstream**: fetches from public registry on cache miss
4. On a cache miss, the package is scanned for CVEs via the OSV API
5. If allowed, the artifact is stored, indexed in Redis and PostgreSQL, and returned to the client
6. A background eviction worker periodically removes artifacts older than the configured TTL

### Singleflight deduplication

Concurrent requests for the same package version share a single upstream fetch. The first request executes the fetch; all subsequent requests for the same key block and receive the shared result. This prevents thundering-herd cache misses from overwhelming upstream registries.

---

## Artifact and Checksum Logic

Every artifact is stored by its **SHA-256 checksum**, not by its name or version. This is known as content-addressable storage.

### How it works

1. When an artifact is fetched from upstream, its SHA-256 checksum is computed while streaming the response body
2. The artifact is written to storage at path `<baseDir>/<first-2-chars-of-checksum>/<full-checksum>` (e.g. `data/artifacts/ab/abcdef1234...`)
3. The checksum is recorded in Redis and PostgreSQL alongside the ecosystem, name, and version
4. On subsequent requests, Redis or PostgreSQL returns the checksum, and the artifact is retrieved from storage by that key

### Why checksums

- **Deduplication**: two packages with identical content share one file on disk
- **Integrity**: serving by checksum guarantees the bytes returned match what was originally fetched
- **Decoupling**: storage is independent of ecosystem-specific naming conventions
- **Atomic writes**: artifacts are written to a `.tmp` file first, then renamed, preventing partial reads

---

## Supported Ecosystems

### npm

- Artifacts: `GET /npm/<name>/-/<name>-<version>.tgz`
- Scoped packages: `GET /npm/@<scope>/<name>/-/<name>-<version>.tgz`
- Metadata: `GET /npm/<name>` — proxies the package manifest from `registry.npmjs.org` and rewrites all tarball URLs to point back to the proxy

### PyPI

- Artifacts: `GET /pypi/packages/<hash1>/<hash2>/<dist>/<filename>`
- Metadata: `GET /pypi/simple/<name>` — proxies the simple index from `pypi.org` and rewrites download URLs to point back to the proxy
- Package names are normalised (lowercase, `-` canonical form)

### Maven

- Artifacts: `GET /maven/<groupPath>/<artifactId>/<version>/<filename>.jar`
- Metadata: `GET /maven/**/*-metadata.xml`, `.pom`, `.md5`, `.sha1`, `.sha256` — proxied transparently from Maven Central
- Group IDs are converted from dot notation to URL path segments (`com.example` → `com/example`)

---

## Security Scanning

When CVE scanning is enabled, every cache miss triggers a query to the [OSV API](https://osv.dev) before the artifact is served.

### Outcomes

| Outcome | Condition | Effect |
|---|---|---|
| Allow | No CVEs, or max severity below warn threshold | Artifact served normally |
| Warn | Max severity ≥ `warn_severity` (default: HIGH) | Artifact served, alert logged |
| Block | Max severity ≥ `block_severity` (default: CRITICAL) | Request rejected with 403 |

Scan results are recorded in PostgreSQL and exposed via `/api/cve-alerts`.

---

## API Reference

The internal API runs on port `9090` and is not exposed publicly.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/stats?since=24h` | Cache hits, misses, and bytes served for a time window |
| `GET` | `/api/packages` | List cached packages with optional ecosystem and name filters |
| `GET` | `/api/packages/summaries` | Unique packages with version count, size, and last hit time |
| `GET` | `/api/packages/cve-alerts` | CVE alerts for a specific package version |
| `GET` | `/api/cve-alerts?since=24h` | All CVE alerts within a time window |
| `GET` | `/api/config` | Active configuration with secrets omitted |
| `GET` | `/metrics` | Prometheus metrics (OpenMetrics format) |

---

## Cache Eviction

The eviction worker runs on a configurable interval (default: every hour) and removes packages whose last access time is older than the TTL (default: 30 days).

Deletion order is strict to prevent orphaned data:

1. **Storage** — delete the artifact file from disk or S3. If this fails, the package is skipped entirely.
2. **Redis** — remove the cache key. Failure is logged but does not abort.
3. **PostgreSQL** — remove the authoritative record.

This order ensures that if any step fails, the remaining layers can still locate and serve the artifact.

---

## Configuration

Copy `cacheproxyfy.example.yaml` to `cacheproxyfy.yaml` and adjust as needed. All fields can be overridden with environment variables using the `CACHEPROXYFY_` prefix (e.g. `CACHEPROXYFY_PROXY_PORT=9000`).

```yaml
proxy:
  port: 8080
  ecosystems: [npm, pypi, maven]

cache:
  backend: local          # "local" or "s3"
  local_dir: ./data/artifacts
  ttl_hours: 720          # 30 days
  eviction_interval_hours: 1

redis:
  addr: localhost:6379
  password: ""
  db: 0

database:
  host: localhost
  port: 5432
  user: postgres
  password: ""
  dbname: postgres
  sslmode: disable

security:
  cve_scanning: false
  block_severity: CRITICAL
  warn_severity: HIGH
```

---

## Running with Docker Compose

```bash
cp cacheproxyfy.example.yaml cacheproxyfy.yaml
docker compose up -d
```

| Service | Port | Description |
|---|---|---|
| proxy | `8080` | Package proxy |
| dashboard | `3000` | Next.js UI |
| postgres | internal | Metadata store |
| redis | internal | Cache index |

> **macOS note:** Use `127.0.0.1` instead of `localhost` when accessing the proxy from the host. On macOS, `localhost` resolves to `::1` (IPv6) but Docker publishes ports on `0.0.0.0` (IPv4).

---

## Client Configuration

### npm

```bash
npm install --registry http://127.0.0.1:8080/npm
```

### pip

```bash
pip install <package> --index-url http://127.0.0.1:8080/pypi/simple/
```

### Maven

Use the provided `cacheproxyfy.example.yaml` settings file:

```bash
mvn dependency:resolve -s maven-proxy-settings.xml
```

---

## License

MIT — see [LICENSE](LICENSE)
