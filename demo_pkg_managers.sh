#!/usr/bin/env bash
# demo_pkg_managers.sh — Fetch packages through CacheProxyfy using native package managers.
# Each ecosystem uses its own tool: npm pack, pip download, mvn dependency:get, go get.
# Missing tools are skipped automatically.
#
# Usage:
#   bash demo_pkg_managers.sh                                      # 1 round, all ecosystems
#   ROUNDS=3 bash demo_pkg_managers.sh                            # 3 rounds to build bytes_saved
#   PROXY_HOST=http://127.0.0.1:8080 bash demo_pkg_managers.sh   # explicit host
#   PROXY_USER=alice PROXY_PASS=secret bash demo_pkg_managers.sh # custom credentials
#   ECOSYSTEM=npm bash demo_pkg_managers.sh                       # npm only
#   ECOSYSTEM=pypi bash demo_pkg_managers.sh                      # pypi only
#   ECOSYSTEM=maven bash demo_pkg_managers.sh                     # maven only
#   ECOSYSTEM=go bash demo_pkg_managers.sh                        # go only
#
# Notes:
#   - npm: npm ≥ 11.5.1 enforces HTTPS even when --registry specifies HTTP.
#     Use HTTPS or run an older npm version if you see ERR_SSL_WRONG_VERSION_NUMBER.
#   - go: requires go ≥ 1.16. GONOSUMDB=* disables the checksum database (demo only).
#   - All downloaded artifacts are written to a temp directory and removed on exit.

HOST="${PROXY_HOST:-http://localhost}"
PROXY_USER="${PROXY_USER:?'Error: set PROXY_USER before running. Example: PROXY_USER=alice PROXY_PASS=secret bash demo_pkg_managers.sh'}"
PROXY_PASS="${PROXY_PASS:?'Error: set PROXY_PASS before running. Example: PROXY_USER=alice PROXY_PASS=secret bash demo_pkg_managers.sh'}"
ROUNDS="${ROUNDS:-1}"
ECOSYSTEM="${ECOSYSTEM:-}"   # if set, only this ecosystem runs (npm|pypi|maven|go)
WORKDIR="$(mktemp -d)"
PASS=0
FAIL=0
COUNT=0

cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

# ── Helpers ───────────────────────────────────────────────────────────────────

has() { command -v "$1" >/dev/null 2>&1; }

# Returns 0 if the given ecosystem should run (ECOSYSTEM unset, or matches).
runs() { [[ -z "$ECOSYSTEM" || "$ECOSYSTEM" == "$1" ]]; }

# Inserts user:pass after the URL scheme so package managers can send Basic Auth.
auth_url() {
    local url="$1"
    local scheme="${url%%://*}"
    local rest="${url#*://}"
    printf '%s://%s:%s@%s' "$scheme" "$PROXY_USER" "$PROXY_PASS" "$rest"
}

# Extracts the bare hostname from a URL (no scheme, no port, no path).
url_host() {
    printf '%s' "${1#*://}" | cut -d: -f1 | cut -d/ -f1
}

_result() {
    local ok="$1" ecosystem="$2" label="$3"
    COUNT=$((COUNT + 1))
    if [[ "$ok" -eq 0 ]]; then
        printf "  [%3d] %-6s  %-52s  OK\n"     "$COUNT" "$ecosystem" "$label"
        PASS=$((PASS + 1))
    else
        printf "  [%3d] %-6s  %-52s  FAILED\n" "$COUNT" "$ecosystem" "$label"
        if [[ -s "$WORKDIR/out.log" ]]; then
            grep -v '^$' "$WORKDIR/out.log" | tail -10 | while IFS= read -r line; do
                printf "        | %s\n" "$line"
            done
        fi
        FAIL=$((FAIL + 1))
    fi
}

# ── Connectivity check ────────────────────────────────────────────────────────

printf "\nCacheProxyfy — Package Manager Demo\n"
printf "Target: %s  |  Rounds: %s\n" "$HOST" "$ROUNDS"
printf "Checking connectivity..."
health=$(curl -sk --max-time 5 --connect-timeout 3 "${HOST}/healthz" 2>/dev/null) || true
if [[ -z "$health" ]]; then
    printf " FAILED\n\nERROR: Could not reach %s/healthz\n  Start the stack: docker compose up -d\n\n" "$HOST"
    exit 1
fi
printf " OK\n%s\n\n" "$health"
printf "%.0s─" {1..78}; printf "\n"

# ── Per-ecosystem setup ───────────────────────────────────────────────────────

# npm: write a scoped .npmrc so Basic Auth is sent to the proxy registry.
# Format: //host:port/npm/:_auth=base64(user:pass)
# A minimal package.json is required so npm install has a project root.
mkdir -p "$WORKDIR/npm"
printf '{}' > "$WORKDIR/npm/package.json"
NPMRC="$WORKDIR/.npmrc"
NPM_HOST="${HOST#*://}"   # strip scheme → "127.0.0.1:8080"
NPM_AUTH=$(printf '%s:%s' "$PROXY_USER" "$PROXY_PASS" | base64 | tr -d '\n')
printf '//%s/npm/:_auth=%s\nalways-auth=true\nstrict-ssl=false\n' \
    "$NPM_HOST" "$NPM_AUTH" > "$NPMRC"
chmod 0600 "$NPMRC"
printf "\n  .npmrc (%s):\n" "$NPMRC"
cat "$NPMRC"

# pip: embed credentials directly in the index URL.
# --trusted-host takes the bare hostname (no port) for HTTP connections.
PIP_INDEX="$(auth_url "$HOST")/pypi/simple/"
PIP_TRUSTED_HOST="$(url_host "$HOST")"
mkdir -p "$WORKDIR/pip"

# maven: temp settings.xml with a mirror that redirects Maven Central to the
# proxy and a <server> block that provides the Basic Auth credentials.
MVN_SETTINGS="$WORKDIR/mvn-settings.xml"
cat > "$MVN_SETTINGS" <<XML
<settings>
  <servers>
    <server>
      <id>cacheproxyfy</id>
      <username>${PROXY_USER}</username>
      <password>${PROXY_PASS}</password>
    </server>
  </servers>
</settings>
XML
printf "\n  mvn-settings.xml (%s):\n" "$MVN_SETTINGS"
cat "$MVN_SETTINGS"

# go: embed credentials in GOPROXY and use an isolated module cache.
# go get requires a go.mod context; a shared temp module is initialised once.
GO_PROXY_URL="$(auth_url "$HOST")/go"
GO_MODULE_DIR="$WORKDIR/gomod"
mkdir -p "$GO_MODULE_DIR" "$WORKDIR/gopath"

# ── Ecosystem request functions ───────────────────────────────────────────────

npm_req() {
    local pkg="$1"
    # npm install downloads the package tarball through the proxy into node_modules.
    # --prefix routes the install into the temp workdir instead of the current directory.
    # --no-save skips writing to package.json; --no-package-lock skips package-lock.json.
    printf "  cmd: npm install %s --registry %s/npm --prefix %s --userconfig %s --no-save --no-package-lock\n" \
        "$pkg" "$HOST" "$WORKDIR/npm" "$NPMRC"
    npm install "$pkg" \
        --registry "${HOST}/npm" \
        --prefix "$WORKDIR/npm" \
        --userconfig "$NPMRC" \
        --no-save \
        --no-package-lock \
        --quiet > "$WORKDIR/out.log" 2>&1
    _result $? "npm" "$pkg"
}

pip_req() {
    local pkg="$1"
    local pip_bin="pip"
    has pip3 && pip_bin="pip3"
    # pip download fetches the wheel/sdist without installing it.
    printf "  cmd: %s download %s --index-url %s --trusted-host %s --no-deps -d %s\n" \
        "$pip_bin" "$pkg" "$PIP_INDEX" "$PIP_TRUSTED_HOST" "$WORKDIR/pip"
    "$pip_bin" download "$pkg" \
        --index-url "$PIP_INDEX" \
        --trusted-host "$PIP_TRUSTED_HOST" \
        --no-deps \
        --quiet \
        -d "$WORKDIR/pip" > "$WORKDIR/out.log" 2>&1
    _result $? "pypi" "$pkg"
}

mvn_req() {
    local artifact="$1" label="$2"
    # Fully-qualified goal bypasses Maven's plugin prefix resolution, which reads
    # plugin group metadata from the repository. Using the short form "dependency:get"
    # requires Maven to discover the plugin via metadata requests first — if those
    # fail (SSL, auth, or proxy gap), Maven errors before the artifact download starts.
    # The <mirrors> block is intentionally absent from settings.xml so that plugin
    # resolution goes straight to Maven Central; only artifact traffic is routed
    # through the proxy via -DremoteRepositories with matching server credentials.
    # SSL insecure flags allow self-signed certificates; wagon transport is forced
    # because Maven 3.9+ switched to a native HTTP client that ignores those flags.
    printf "  cmd: mvn org.apache.maven.plugins:maven-dependency-plugin:3.7.0:get -Dartifact=%s -DremoteRepositories=cacheproxyfy::default::%s/maven -s %s -Dmaven.resolver.transport=wagon -Dmaven.wagon.http.ssl.insecure=true -Dmaven.wagon.http.ssl.allowall=true\n" \
        "$artifact" "$HOST" "$MVN_SETTINGS"
    mvn org.apache.maven.plugins:maven-dependency-plugin:3.7.0:get \
        -Dartifact="$artifact" \
        -DremoteRepositories="cacheproxyfy::default::${HOST}/maven" \
        -s "$MVN_SETTINGS" \
        -Dmaven.resolver.transport=wagon \
        -Dmaven.wagon.http.ssl.insecure=true \
        -Dmaven.wagon.http.ssl.allowall=true \
        -q > "$WORKDIR/out.log" 2>&1
    _result $? "maven" "$label"
}

go_req() {
    local mod="$1"
    # go get requires a module root (go.mod). The shared temp module is reused
    # across requests so subsequent go get calls only add to go.mod/go.sum.
    # GONOSUMDB=* skips the checksum database — acceptable for local demo use.
    printf "  cmd: GOPROXY=%s GONOSUMDB=* GOPATH=%s go get %s  (cwd: %s)\n" \
        "$GO_PROXY_URL" "$WORKDIR/gopath" "$mod" "$GO_MODULE_DIR"
    (
        cd "$GO_MODULE_DIR"
        GOPROXY="$GO_PROXY_URL" GONOSUMDB="*" GOPATH="$WORKDIR/gopath" \
            go get "$mod" > "$WORKDIR/out.log" 2>&1
    )
    _result $? "go" "$mod"
}

# ── Initialise the Go module once (outside the round loop) ───────────────────

if runs go && has go; then
    (cd "$GO_MODULE_DIR" && go mod init cpf_demo > /dev/null 2>&1) || true
fi

# ── Round function ────────────────────────────────────────────────────────────

run_round() {
    local round="$1"
    printf "\nRound %d / %d\n" "$round" "$ROUNDS"
    printf "%.0s─" {1..78}; printf "\n"

    # ── npm ──────────────────────────────────────────────────────────────────
    if runs npm && has npm; then
        printf "\n  npm\n"
        npm_req "lodash@4.17.21"
        npm_req "express@4.18.2"
        npm_req "axios@1.6.7"
        npm_req "chalk@5.3.0"
        npm_req "commander@12.0.0"
        npm_req "dotenv@16.4.5"
        npm_req "uuid@9.0.0"
        npm_req "typescript@5.4.2"
        npm_req "prettier@3.2.5"
        npm_req "semver@7.6.0"
    elif runs npm; then
        printf "\n  npm  (skipped — npm not found)\n"
    fi

    # ── pypi ─────────────────────────────────────────────────────────────────
    if runs pypi && { has pip3 || has pip; }; then
        printf "\n  pypi\n"
        pip_req "requests==2.31.0"
        pip_req "flask==3.0.2"
        pip_req "click==8.1.7"
        pip_req "httpx==0.27.0"
        pip_req "pytest==8.1.1"
        pip_req "fastapi==0.110.0"
        pip_req "pydantic==2.6.4"
        pip_req "uvicorn==0.29.0"
    elif runs pypi; then
        printf "\n  pypi  (skipped — pip not found)\n"
    fi

    # ── maven ────────────────────────────────────────────────────────────────
    if runs maven && has mvn; then
        printf "\n  maven\n"
        mvn_req "com.google.guava:guava:33.0.0-jre"                          "guava-33.0.0-jre"
        mvn_req "com.google.code.gson:gson:2.10.1"                           "gson-2.10.1"
        mvn_req "com.fasterxml.jackson.core:jackson-databind:2.16.1"         "jackson-databind-2.16.1"
        mvn_req "org.apache.commons:commons-lang3:3.14.0"                    "commons-lang3-3.14.0"
        mvn_req "org.slf4j:slf4j-api:2.0.12"                                "slf4j-api-2.0.12"
        mvn_req "org.postgresql:postgresql:42.7.2"                           "postgresql-42.7.2"
        mvn_req "org.mockito:mockito-core:5.10.0"                            "mockito-core-5.10.0"
    elif runs maven; then
        printf "\n  maven  (skipped — mvn not found)\n"
    fi

    # ── go ───────────────────────────────────────────────────────────────────
    if runs go && has go; then
        printf "\n  go\n"
        go_req "github.com/gin-gonic/gin@v1.9.1"
        go_req "github.com/gorilla/mux@v1.8.1"
        go_req "github.com/sirupsen/logrus@v1.9.3"
        go_req "github.com/spf13/cobra@v1.8.0"
        go_req "go.uber.org/zap@v1.27.0"
        go_req "github.com/google/uuid@v1.6.0"
        go_req "github.com/redis/go-redis/v9@v9.5.1"
    elif runs go; then
        printf "\n  go  (skipped — go not found)\n"
    fi
}

# ── Run ───────────────────────────────────────────────────────────────────────

for round in $(seq 1 "$ROUNDS"); do
    run_round "$round"
done

# ── Summary ───────────────────────────────────────────────────────────────────
printf "\n"
printf "%.0s─" {1..78}; printf "\n"
printf "Requests: %d total  |  %d succeeded  |  %d failed\n\n" \
    "$COUNT" "$PASS" "$FAIL"
