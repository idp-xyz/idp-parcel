#!/usr/bin/env bash
# 起一套演示环境（票 demo-2026-10-30/03）：演示发行方、灌种子到指名库、parcel-api(:19080)、parcel-dispatch，并把演示账户余额补到能连发多件。
# **仅限本机隔离环境**，拒绝指向 postgres 库。证据只记 S。
#
# 用法（在仓库任意处）：
#   scripts/demo-seeds/demo-run/up.sh <库名> [--reset]
#   库名     55432 上的库；不存在就建。已存在而不带 --reset：沿用库里已灌的种子，不重灌。
#   --reset  先 DROP 全部 parcel schema 再重迁重灌（seed.sh --reset），演示前复灌用。
# 环境变量可改：IDP_PARCEL_DEMO_PG_CONTAINER（缺省 idp-parcel-postgres-gate）、IDP_PARCEL_DEMO_PG_HOSTPORT（缺省 127.0.0.1:55432）、
# IDP_PARCEL_DEMO_API_ADDR（缺省 :19080）。
#
# 运行态落 .run/：bin/ 二进制、*.log 与 *.pid 进程日志与 PID、env.sh 坐标、state.env 各分段的状态（每次 up 清空）。
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

db="${1:-}"
reset=""
[[ "${2:-}" == "--reset" ]] && reset="--reset"
if [[ ! "$db" =~ ^[a-z][a-z0-9_]*$ || "$db" == "postgres" || "$db" == template* ]]; then
  printf '用法：%s <库名> [--reset]（库名小写字母起头；不得是 postgres）\n' "$0" >&2
  exit 1
fi

pg_container="${IDP_PARCEL_DEMO_PG_CONTAINER:-idp-parcel-postgres-gate}"
pg_hostport="${IDP_PARCEL_DEMO_PG_HOSTPORT:-127.0.0.1:55432}"
api_addr="${IDP_PARCEL_DEMO_API_ADDR:-:19080}"
api_port="${api_addr##*:}"
dsn="postgres://parcel:parcel@${pg_hostport}/${db}?sslmode=disable"

if [[ -f "$RUN_DIR/env.sh" ]]; then
  printf '已有一套在记的演示环境（%s）：先跑 down.sh 收掉再 up\n' "$RUN_DIR/env.sh" >&2
  exit 1
fi
if ss -ltn 2>/dev/null | grep -q ":${api_port} "; then
  printf '端口 %s 已被占用（上一次没收干净？）。先 down.sh，或换 IDP_PARCEL_DEMO_API_ADDR\n' "$api_port" >&2
  exit 1
fi
mkdir -p "$RUN_DIR/bin"
BIN_DIR="$RUN_DIR/bin"
: > "$STATE_FILE"

section "库 ${db}"
exists="$(docker exec "$pg_container" psql -U parcel -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='${db}'")"
if [[ -z "$exists" ]]; then
  docker exec "$pg_container" psql -U parcel -d postgres -c "CREATE DATABASE ${db}" >/dev/null
  echo "新建库 ${db}"
  need_seed=1
elif [[ -n "$reset" ]]; then
  echo "库已存在，--reset 复灌"
  need_seed=1
else
  echo "库已存在，沿用已灌的种子（要复灌带 --reset）"
  need_seed=0
fi
created_db=0
[[ -z "$exists" ]] && created_db=1

section "发行方"
# 两只发行方都已答 /keys 就不碰 compose：compose 的配置哈希含挂载的绝对路径，换一棵 worktree 去 up 会把别的会话
# 正在用的共享发行方重建掉（手里的令牌全部作废）。只有缺哪只才 up。
if curl -sf http://127.0.0.1:5556/dex/keys >/dev/null 2>&1 && curl -sf http://127.0.0.1:5558/dex/keys >/dev/null 2>&1; then
  echo "共享发行方 5556、5558 已在跑，不动"
else
  docker compose -f "$SEEDS_DIR/operator-channel/compose.yaml" up -d --wait >/dev/null
fi
integration_issuer="http://127.0.0.1:5558/dex"
integration_jwks="http://127.0.0.1:5558/dex/keys"
private_container=""
probe_ok() { # 四台集成/客户端凭据都取得到才算共享发行方可用
  integration_token syn-integration-customs-broker-01 syn-customs-broker-secret >/dev/null 2>&1 &&
    integration_token syn-integration-carrier-trunk-01 syn-carrier-trunk-secret >/dev/null 2>&1 &&
    integration_token syn-integration-bank-01 syn-bank-secret >/dev/null 2>&1 &&
    customer_token >/dev/null 2>&1
}
if probe_ok; then
  echo "共享集成客户端发行方可用（5558）"
else
  # 共享那只起得早于配置里后加的客户端会答 invalid_client；不 --force-recreate 它（会让别人手里的令牌作废），另起私有的。
  private_container="idp-parcel-demo-integration-issuer-${db}"
  docker rm -f "$private_container" >/dev/null 2>&1 || true
  docker run -d --name "$private_container" -p 127.0.0.1:5559:5556 \
    -v "$SEEDS_DIR/operator-channel/integration-dex.yaml:/etc/dex/dex.yaml:ro" \
    ghcr.io/dexidp/dex:v2.46.0 dex serve /etc/dex/dex.yaml >/dev/null
  export IDP_PARCEL_DEMO_INTEGRATION_ISSUER="http://127.0.0.1:5559/dex"
  export IDP_PARCEL_DEMO_CUSTOMER_ISSUER="http://127.0.0.1:5559/dex"
  integration_jwks="http://127.0.0.1:5559/dex/keys"
  for _ in $(seq 30); do curl -sf "$integration_jwks" >/dev/null 2>&1 && break; sleep 1; done
  probe_ok || { printf '私有集成发行方也取不到令牌，停\n' >&2; docker rm -f "$private_container" >/dev/null; exit 1; }
  echo "共享发行方答 invalid_client，改用私有的 ${private_container}（5559，iss 串不变）"
fi

# 到这里起才写 env.sh：之后 down.sh 才认得这套环境。
{
  printf 'export IDP_PARCEL_DEMO_DB=%q\n' "$db"
  printf 'export IDP_PARCEL_DEMO_DB_CREATED=%q\n' "$created_db"
  printf 'export IDP_PARCEL_DEMO_PG_CONTAINER=%q\n' "$pg_container"
  printf 'export IDP_PARCEL_POSTGRES_DSN=%q\n' "$dsn"
  printf 'export API_BASE=%q\n' "http://127.0.0.1:${api_port}"
  printf 'export IDP_PARCEL_API_BASE=%q\n' "http://127.0.0.1:${api_port}"
  printf 'export BIN_DIR=%q\n' "$BIN_DIR"
  printf 'export IDP_PARCEL_DEMO_PRIVATE_ISSUER_CONTAINER=%q\n' "$private_container"
  if [[ -n "$private_container" ]]; then
    printf 'export IDP_PARCEL_DEMO_INTEGRATION_ISSUER=%q\n' "$IDP_PARCEL_DEMO_INTEGRATION_ISSUER"
    printf 'export IDP_PARCEL_DEMO_CUSTOMER_ISSUER=%q\n' "$IDP_PARCEL_DEMO_CUSTOMER_ISSUER"
  fi
} > "$RUN_DIR/env.sh"

section "灌种子"
if [[ "$need_seed" == 1 ]]; then
  if ! (cd "$REPO_DIR" && IDP_PARCEL_POSTGRES_DSN="$dsn" ./scripts/demo-seeds/seed.sh ${reset:+--reset} >"$RUN_DIR/seed.log" 2>&1); then
    printf 'seed.sh 失败，日志尾部：\n' >&2; tail -20 "$RUN_DIR/seed.log" >&2; exit 1
  fi
  echo "seed.sh 退 0（日志 .run/seed.log）"
else
  echo "沿用库里已灌的种子，不重灌"
fi

section "编译"
(cd "$REPO_DIR" && go build -o "$BIN_DIR/" ./cmd/parcel-api ./cmd/parcel-dispatch ./cmd/parcel-settlement-register \
  && go build -o "$BIN_DIR/declaration-broker" ./scripts/demo-seeds/declaration-broker)
echo "二进制在 .run/bin/"

section "起进程"
(
  export IDP_PARCEL_POSTGRES_DSN="$dsn"
  export IDP_PARCEL_HTTP_ADDR="$api_addr"
  # 操作者渠道（ADR-0100）与治理自授权（ADR-0180）
  export IDP_PARCEL_OPERATOR_OIDC_ISSUER='http://127.0.0.1:5556/dex'
  export IDP_PARCEL_OPERATOR_OIDC_JWKS_URL='http://127.0.0.1:5556/dex/keys'
  export IDP_PARCEL_OPERATOR_OIDC_AUDIENCE='idp-parcel-admin-web'
  export IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY='SYN-AUTH/idp-parcel-pilot'
  # 集成客户端渠道（ADR-0149）与首方客户渠道（ADR-0139）：iss 串逐字比，JWKS 取实际可达的那只
  export IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER='http://127.0.0.1:5558/dex'
  export IDP_PARCEL_INTEGRATION_CLIENT_OIDC_JWKS_URL="$integration_jwks"
  export IDP_PARCEL_INTEGRATION_CLIENT_OIDC_AUDIENCE='idp-parcel-integration'
  export IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER='http://127.0.0.1:5558/dex'
  export IDP_PARCEL_CUSTOMER_API_OIDC_JWKS_URL="$integration_jwks"
  export IDP_PARCEL_CUSTOMER_API_OIDC_AUDIENCE='idp-parcel-customer-api'
  # 隔离读写两个开关同值（ADR-0078、ADR-0091）
  export IDP_PARCEL_ISOLATED_READ_TENANT='SYN-TENANT-01'
  export IDP_PARCEL_ISOLATED_WRITE_TENANT='SYN-TENANT-01'
  nohup "$BIN_DIR/parcel-api" >"$RUN_DIR/parcel-api.log" 2>&1 &
  echo $! > "$RUN_DIR/parcel-api.pid"
)
(
  export IDP_PARCEL_POSTGRES_DSN="$dsn"
  export IDP_PARCEL_ROUTE_SERVICE_PURPOSE='NETWORK_SERVICE'
  export IDP_PARCEL_DISPATCH_INTERVAL='2s'
  export IDP_PARCEL_DISPATCH_DELIVERY_TIMEOUT='5s'
  export IDP_PARCEL_DISPATCH_LIMIT='10'
  export IDP_PARCEL_DISPATCH_LEASE='1m'
  export IDP_PARCEL_DISPATCH_MAX_ATTEMPTS='3'
  export IDP_PARCEL_DISPATCH_RETRY_AFTER='10s'
  nohup "$BIN_DIR/parcel-dispatch" >"$RUN_DIR/parcel-dispatch.log" 2>&1 &
  echo $! > "$RUN_DIR/parcel-dispatch.pid"
)
load_env
for _ in $(seq 60); do
  if tok="$(operator_token 2>/dev/null)" && [[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $tok" "$API_BASE/network-catalog?family=node")" == 200 ]]; then
    ready=1; break
  fi
  sleep 1
done
if [[ -z "${ready:-}" ]]; then
  printf 'parcel-api 60 秒内没答 200，日志尾部：\n' >&2; tail -20 "$RUN_DIR/parcel-api.log" >&2; exit 1
fi
kill -0 "$(cat "$RUN_DIR/parcel-dispatch.pid")" 2>/dev/null || { printf 'parcel-dispatch 已退出，日志：\n' >&2; tail -20 "$RUN_DIR/parcel-dispatch.log" >&2; exit 1; }
echo "parcel-api ${API_BASE}（PID $(cat "$RUN_DIR/parcel-api.pid")）、parcel-dispatch（PID $(cat "$RUN_DIR/parcel-dispatch.pid")）已起"
grep -E 'Operator channel|Governance self|Isolated read|Isolated write|Integration client|Customer' "$RUN_DIR/parcel-api.log" | cut -c1-200 | sed 's/^/  日志 /' || true

"$DEMO_DIR/topup-funds.sh" 100000 01
printf '\n演示环境就绪：库 %s，API %s。接着按 README 的顺序跑分段。\n' "$db" "$API_BASE"
