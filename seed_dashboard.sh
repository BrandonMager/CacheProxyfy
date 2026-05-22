#!/usr/bin/env bash
# seed_dashboard.sh — Seed the dashboard with cached artifact traffic.
# All requests are actual artifacts (.tgz / .whl / .zip / .jar) — no metadata.
# First run fills the cache (misses); subsequent runs accumulate bytes_saved.
#
# Usage:
#   bash seed_dashboard.sh               # 1 round (~80 artifact requests)
#   ROUNDS=5 bash seed_dashboard.sh      # 5 rounds to build up bytes_saved
#   PROXY_HOST=https://myhost bash seed_dashboard.sh

HOST="${PROXY_HOST:-https://localhost}"
PROXY_USER="${PROXY_USER:?'Error: set PROXY_USER before running. Example: PROXY_USER=alice PROXY_PASS=secret bash seed_dashboard.sh'}"
PROXY_PASS="${PROXY_PASS:?'Error: set PROXY_PASS before running. Example: PROXY_USER=alice PROXY_PASS=secret bash seed_dashboard.sh'}"
ROUNDS="${ROUNDS:-1}"
PASS=0
FAIL=0
COUNT=0

req() {
    local ecosystem="$1" label="$2" path="$3"
    COUNT=$((COUNT + 1))

    response=$(curl -ski --max-time 60 --connect-timeout 5 -u "${PROXY_USER}:${PROXY_PASS}" "${HOST}${path}" 2>/dev/null) || true
    http_code=$(printf '%s' "$response" | grep -E "^HTTP" | tail -1 | awk '{print $2}')
    xcache=$(printf '%s' "$response" | grep -i "^x-cache:" | awk '{print $2}' | tr -d '\r\n')

    if [[ "$http_code" =~ ^(200|206)$ ]]; then
        printf "  [%3d] %-6s  %-54s  %s  cache=%s\n" \
            "$COUNT" "$ecosystem" "$label" "$http_code" "${xcache:-n/a}"
        PASS=$((PASS + 1))
    elif [[ "$http_code" =~ ^(404|410)$ ]]; then
        printf "  [%3d] %-6s  %-54s  %s  (upstream 404)\n" \
            "$COUNT" "$ecosystem" "$label" "$http_code"
        FAIL=$((FAIL + 1))
    else
        printf "  [%3d] %-6s  %-54s  %s  FAILED\n" \
            "$COUNT" "$ecosystem" "$label" "${http_code:-ERR}"
        FAIL=$((FAIL + 1))
    fi
}

# ── Connectivity check ────────────────────────────────────────────────────────
printf "\nCacheProxyfy Dashboard Seed Script\n"
printf "Target: %s  |  Rounds: %s\n" "$HOST" "$ROUNDS"
printf "Checking connectivity..."
health=$(curl -sk --max-time 5 --connect-timeout 3 -u "${PROXY_USER}:${PROXY_PASS}" "${HOST}/healthz" 2>/dev/null) || true
if [[ -z "$health" ]]; then
    printf " FAILED\n\nERROR: Could not reach %s/healthz\n  Start the stack: docker compose up -d\n\n" "$HOST"
    exit 1
fi
printf " OK\n%s\n\n" "$health"
printf "%.0s─" {1..78}; printf "\n"

# ── Artifact requests (all cached — no metadata) ──────────────────────────────

artifacts() {
    local round="$1"
    printf "\nRound %d / %d\n" "$round" "$ROUNDS"
    printf "%.0s─" {1..78}; printf "\n"

    # ── npm .tgz (25) ──────────────────────────────────────────────────────────
    printf "\n  npm\n"
    req npm "lodash-4.17.21.tgz"          "/npm/lodash/-/lodash-4.17.21.tgz"
    req npm "express-4.18.2.tgz"          "/npm/express/-/express-4.18.2.tgz"
    req npm "axios-1.6.7.tgz"             "/npm/axios/-/axios-1.6.7.tgz"
    req npm "chalk-5.3.0.tgz"             "/npm/chalk/-/chalk-5.3.0.tgz"
    req npm "commander-12.0.0.tgz"        "/npm/commander/-/commander-12.0.0.tgz"
    req npm "dotenv-16.4.5.tgz"           "/npm/dotenv/-/dotenv-16.4.5.tgz"
    req npm "uuid-9.0.0.tgz"              "/npm/uuid/-/uuid-9.0.0.tgz"
    req npm "moment-2.30.1.tgz"           "/npm/moment/-/moment-2.30.1.tgz"
    req npm "react-18.2.0.tgz"            "/npm/react/-/react-18.2.0.tgz"
    req npm "react-dom-18.2.0.tgz"        "/npm/react-dom/-/react-dom-18.2.0.tgz"
    req npm "typescript-5.4.2.tgz"        "/npm/typescript/-/typescript-5.4.2.tgz"
    req npm "jest-29.7.0.tgz"             "/npm/jest/-/jest-29.7.0.tgz"
    req npm "prettier-3.2.5.tgz"          "/npm/prettier/-/prettier-3.2.5.tgz"
    req npm "webpack-5.90.3.tgz"          "/npm/webpack/-/webpack-5.90.3.tgz"
    req npm "eslint-8.57.0.tgz"           "/npm/eslint/-/eslint-8.57.0.tgz"
    req npm "cors-2.8.5.tgz"              "/npm/cors/-/cors-2.8.5.tgz"
    req npm "body-parser-1.20.2.tgz"      "/npm/body-parser/-/body-parser-1.20.2.tgz"
    req npm "morgan-1.10.0.tgz"           "/npm/morgan/-/morgan-1.10.0.tgz"
    req npm "helmet-7.1.0.tgz"            "/npm/helmet/-/helmet-7.1.0.tgz"
    req npm "compression-1.7.4.tgz"       "/npm/compression/-/compression-1.7.4.tgz"
    req npm "rimraf-5.0.5.tgz"            "/npm/rimraf/-/rimraf-5.0.5.tgz"
    req npm "cross-env-7.0.3.tgz"         "/npm/cross-env/-/cross-env-7.0.3.tgz"
    req npm "debug-4.3.4.tgz"             "/npm/debug/-/debug-4.3.4.tgz"
    req npm "nodemon-3.1.0.tgz"           "/npm/nodemon/-/nodemon-3.1.0.tgz"
    req npm "semver-7.6.0.tgz"            "/npm/semver/-/semver-7.6.0.tgz"

    # ── pypi .whl (10) ─────────────────────────────────────────────────────────
    printf "\n  pypi\n"
    req pypi "requests-2.31.0.whl"   "/pypi/packages/70/8e/0e2d847013cb52cd35b38c009bb167a1a26b2ce6cd6965bf26b47bc0bf44/requests-2.31.0-py3-none-any.whl"
    req pypi "flask-3.0.2.whl"       "/pypi/packages/93/a6/aa98bfe0eb9b8b15d36cdfd03c8ca86a03968a87f27ce224fb4f766acb23/flask-3.0.2-py3-none-any.whl"
    req pypi "click-8.1.7.whl"       "/pypi/packages/00/2e/d53fa4befbf2cfa713304affc7ca780ce4fc1fd8710527771b58311a3229/click-8.1.7-py3-none-any.whl"
    req pypi "httpx-0.27.0.whl"      "/pypi/packages/41/7b/ddacf6dcebb42466abd03f368782142baa82e08fc0c1f8eaa05b4bae87d5/httpx-0.27.0-py3-none-any.whl"
    req pypi "pytest-8.1.1.whl"      "/pypi/packages/4d/7e/c79cecfdb6aa85c6c2e3cf63afc56d0f165f24f5c66c03c695c4d9b84756/pytest-8.1.1-py3-none-any.whl"
    req pypi "fastapi-0.110.0.whl"   "/pypi/packages/f0/f7/ea860cb8aa18e326f411e32ab537424690a53db20de6bad73d70da611fae/fastapi-0.110.0-py3-none-any.whl"
    req pypi "pydantic-2.6.4.whl"    "/pypi/packages/e5/f3/8296f550276194a58c5500d55b19a27ae0a5a3a51ffef66710c58544b32d/pydantic-2.6.4-py3-none-any.whl"
    req pypi "uvicorn-0.29.0.whl"    "/pypi/packages/73/f5/cbb16fcbe277c1e0b8b3ddd188f2df0e0947f545c49119b589643632d156/uvicorn-0.29.0-py3-none-any.whl"
    req pypi "starlette-0.37.2.whl"  "/pypi/packages/fd/18/31fa32ed6c68ba66220204ef0be798c349d0a20c1901f9d4a794e08c76d8/starlette-0.37.2-py3-none-any.whl"
    req pypi "pyyaml-6.0.1.tar.gz"   "/pypi/packages/cd/e5/af35f7ea75cf72f2cd079c95ee16797de7cd71f29ea7c68ae5ce7be1eda0/PyYAML-6.0.1.tar.gz"

    # ── go .zip (25) ───────────────────────────────────────────────────────────
    printf "\n  go\n"
    req go "gin-gonic/gin@v1.9.1"              "/go/github.com/gin-gonic/gin/@v/v1.9.1.zip"
    req go "gorilla/mux@v1.8.1"                "/go/github.com/gorilla/mux/@v/v1.8.1.zip"
    req go "sirupsen/logrus@v1.9.3"            "/go/github.com/sirupsen/logrus/@v/v1.9.3.zip"
    req go "stretchr/testify@v1.8.4"           "/go/github.com/stretchr/testify/@v/v1.8.4.zip"
    req go "pkg/errors@v0.9.1"                 "/go/github.com/pkg/errors/@v/v0.9.1.zip"
    req go "spf13/cobra@v1.8.0"                "/go/github.com/spf13/cobra/@v/v1.8.0.zip"
    req go "spf13/viper@v1.18.2"               "/go/github.com/spf13/viper/@v/v1.18.2.zip"
    req go "google/uuid@v1.6.0"                "/go/github.com/google/uuid/@v/v1.6.0.zip"
    req go "go-chi/chi/v5@v5.1.0"              "/go/github.com/go-chi/chi/v5/@v/v5.1.0.zip"
    req go "go.uber.org/zap@v1.27.0"           "/go/go.uber.org/zap/@v/v1.27.0.zip"
    req go "joho/godotenv@v1.5.1"              "/go/github.com/joho/godotenv/@v/v1.5.1.zip"
    req go "golang-jwt/jwt/v5@v5.2.1"          "/go/github.com/golang-jwt/jwt/v5/@v/v5.2.1.zip"
    req go "rs/cors@v1.10.1"                   "/go/github.com/rs/cors/@v/v1.10.1.zip"
    req go "cenkalti/backoff/v4@v4.3.0"        "/go/github.com/cenkalti/backoff/v4/@v/v4.3.0.zip"
    req go "mitchellh/mapstructure@v1.5.0"     "/go/github.com/mitchellh/mapstructure/@v/v1.5.0.zip"
    req go "go-playground/validator/v10@v10.19.0" "/go/github.com/go-playground/validator/v10/@v/v10.19.0.zip"
    req go "go.uber.org/mock@v0.4.0"           "/go/go.uber.org/mock/@v/v0.4.0.zip"
    req go "golang.org/x/sync@v0.6.0"          "/go/golang.org/x/sync/@v/v0.6.0.zip"
    req go "golang.org/x/text@v0.14.0"         "/go/golang.org/x/text/@v/v0.14.0.zip"
    req go "golang.org/x/crypto@v0.21.0"       "/go/golang.org/x/crypto/@v/v0.21.0.zip"
    req go "golang.org/x/net@v0.22.0"          "/go/golang.org/x/net/@v/v0.22.0.zip"
    req go "prometheus/client_golang@v1.19.0"  "/go/github.com/prometheus/client_golang/@v/v1.19.0.zip"
    req go "redis/go-redis/v9@v9.5.1"          "/go/github.com/redis/go-redis/v9/@v/v9.5.1.zip"
    req go "jackc/pgx/v5@v5.5.5"               "/go/github.com/jackc/pgx/v5/@v/v5.5.5.zip"
    req go "go-sql-driver/mysql@v1.8.0"        "/go/github.com/go-sql-driver/mysql/@v/v1.8.0.zip"

    # ── maven .jar (20) ────────────────────────────────────────────────────────
    printf "\n  maven\n"
    req maven "guava-33.0.0-jre.jar"            "/maven/com/google/guava/guava/33.0.0-jre/guava-33.0.0-jre.jar"
    req maven "gson-2.10.1.jar"                 "/maven/com/google/code/gson/gson/2.10.1/gson-2.10.1.jar"
    req maven "postgresql-42.7.2.jar"           "/maven/org/postgresql/postgresql/42.7.2/postgresql-42.7.2.jar"
    req maven "kafka-clients-3.6.1.jar"         "/maven/org/apache/kafka/kafka-clients/3.6.1/kafka-clients-3.6.1.jar"
    req maven "jackson-databind-2.16.1.jar"     "/maven/com/fasterxml/jackson/core/jackson-databind/2.16.1/jackson-databind-2.16.1.jar"
    req maven "logback-classic-1.5.3.jar"       "/maven/ch/qos/logback/logback-classic/1.5.3/logback-classic-1.5.3.jar"
    req maven "mockito-core-5.10.0.jar"         "/maven/org/mockito/mockito-core/5.10.0/mockito-core-5.10.0.jar"
    req maven "slf4j-api-2.0.12.jar"            "/maven/org/slf4j/slf4j-api/2.0.12/slf4j-api-2.0.12.jar"
    req maven "commons-lang3-3.14.0.jar"        "/maven/org/apache/commons/commons-lang3/3.14.0/commons-lang3-3.14.0.jar"
    req maven "h2-2.2.224.jar"                  "/maven/com/h2database/h2/2.2.224/h2-2.2.224.jar"
    req maven "micrometer-core-1.12.3.jar"      "/maven/io/micrometer/micrometer-core/1.12.3/micrometer-core-1.12.3.jar"
    req maven "HikariCP-5.1.0.jar"              "/maven/com/zaxxer/HikariCP/5.1.0/HikariCP-5.1.0.jar"
    req maven "netty-all-4.1.107.Final.jar"     "/maven/io/netty/netty-all/4.1.107.Final/netty-all-4.1.107.Final.jar"
    req maven "spring-core-6.1.4.jar"           "/maven/org/springframework/spring-core/6.1.4/spring-core-6.1.4.jar"
    req maven "junit-jupiter-api-5.10.2.jar"    "/maven/org/junit/jupiter/junit-jupiter-api/5.10.2/junit-jupiter-api-5.10.2.jar"
    req maven "assertj-core-3.25.3.jar"         "/maven/org/assertj/assertj-core/3.25.3/assertj-core-3.25.3.jar"
    req maven "jedis-5.1.0.jar"                 "/maven/redis/clients/jedis/5.1.0/jedis-5.1.0.jar"
    req maven "commons-io-2.15.1.jar"           "/maven/commons-io/commons-io/2.15.1/commons-io-2.15.1.jar"
    req maven "byte-buddy-1.14.12.jar"          "/maven/net/bytebuddy/byte-buddy/1.14.12/byte-buddy-1.14.12.jar"
    req maven "asm-9.6.jar"                     "/maven/org/ow2/asm/asm/9.6/asm-9.6.jar"
}

for round in $(seq 1 "$ROUNDS"); do
    artifacts "$round"
done

# ── Summary ───────────────────────────────────────────────────────────────────
printf "\n"
printf "%.0s─" {1..78}; printf "\n"
printf "Requests: %d total  |  %d succeeded  |  %d failed\n\n" \
    "$COUNT" "$PASS" "$FAIL"
