#!/usr/bin/env bash
# 演示驱动脚本的共用部分（票 demo-2026-10-30/03），由各分段脚本 source。**仅限本机隔离环境**：全部是 SYN- 合成值，证据只记 S。
#
# 约定：运行态都在 .run/ 下——env.sh 是 up.sh 写的进程与发行方坐标，state.env 是各分段写、后段读的状态（委托号、包裹号、案号……）。
# 每步一行「步骤 → 关键答复字段」；状态码或 outcome 不在可接受集合里就停，并打印该步原始响应。不绕过、不写库。
set -euo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SEEDS_DIR="$(cd "$DEMO_DIR/.." && pwd)"
REPO_DIR="$(cd "$SEEDS_DIR/../.." && pwd)"
RUN_DIR="$DEMO_DIR/.run"
STATE_FILE="$RUN_DIR/state.env"
export PATH="/usr/local/go/bin:$PATH"

load_env() {
  if [[ ! -f "$RUN_DIR/env.sh" ]]; then
    printf '没有 %s：先跑 up.sh <库名>\n' "$RUN_DIR/env.sh" >&2
    exit 1
  fi
  # shellcheck disable=SC1091
  source "$RUN_DIR/env.sh"
  [[ -f "$STATE_FILE" ]] && source "$STATE_FILE"
  return 0
}

# state_set KEY VALUE：写（或覆盖）state.env 里的一项，并立刻导出给本进程。
state_set() {
  mkdir -p "$RUN_DIR"
  touch "$STATE_FILE"
  grep -v "^$1=" "$STATE_FILE" > "$STATE_FILE.tmp" || true
  printf '%s=%q\n' "$1" "$2" >> "$STATE_FILE.tmp"
  mv "$STATE_FILE.tmp" "$STATE_FILE"
  export "$1=$2"
}

need_state() {
  local key
  for key in "$@"; do
    if [[ -z "${!key:-}" ]]; then
      printf '缺状态 %s：先跑产生它的前一段（见 README 的顺序）\n' "$key" >&2
      exit 1
    fi
  done
}

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
# 班次日期取当天（上海当地日；班次身份是 模板/日期，物化按当地日）。
today_sha() { TZ=Asia/Shanghai date +%Y-%m-%d; }
today_sin() { TZ=Asia/Singapore date +%Y-%m-%d; }

operator_token() {
  "$SEEDS_DIR/operator-channel/operator-token.sh" "${1:-syn-operator-config-01@syn-tenant-01.example.invalid}"
}
integration_token() { "$SEEDS_DIR/operator-channel/integration-token.sh" "$1" "$2"; }
customer_token() { "$SEEDS_DIR/operator-channel/customer-token.sh"; }

# jget FILE 点路径：从 JSON 文件取一个值（数组下标用数字），缺失打空串。
jget() {
  python3 - "$1" "$2" <<'PY'
import json, sys
try:
    cur = json.load(open(sys.argv[1], encoding="utf-8"))
    for part in sys.argv[2].split("."):
        if part == "":
            continue
        cur = cur[int(part)] if isinstance(cur, list) else cur[part]
    print(cur if not isinstance(cur, (dict, list)) else json.dumps(cur, ensure_ascii=False))
except Exception:
    print("")
PY
}

# pj k=v … ：组装 JSON 对象。k=v 是字符串；k:=<json> 是原样 JSON 值。
pj() {
  python3 - "$@" <<'PY'
import json, sys
out = {}
for arg in sys.argv[1:]:
    head = arg.split(":=", 1)[0]
    if ":=" in arg and "=" not in head:
        out[head] = json.loads(arg.split(":=", 1)[1])
    else:
        k, v = arg.split("=", 1)
        out[k] = v
print(json.dumps(out, ensure_ascii=False))
PY
}

LAST="$(mktemp)"
trap 'rm -f "$LAST"' EXIT

# step 标签 令牌 方法 路径 载荷 可接受集合 展示字段… [-- 额外头…]
#   可接受集合：逗号分隔的「状态码:outcome」，outcome 写 * 表示不看；GET 载荷写 -。
#   展示字段：答复里的点路径，逐个打印 名=值。失败即停并打印原始响应。
#   额外头走全局数组 EXTRA_HEADERS（用后清空），设备三头用 device_headers 设。
EXTRA_HEADERS=()
device_headers() {
  EXTRA_HEADERS=(-H "Parcel-Device-Id: $1" -H "Parcel-Work-Scope-Kind: $2" -H "Parcel-Work-Scope-Reference: $3")
}
step() {
  local label=$1 token=$2 method=$3 path=$4 body=$5 accept=$6
  shift 6
  local code
  if [[ "$body" == "-" ]]; then
    code="$(curl -sS -o "$LAST" -w '%{http_code}' -X "$method" -H "Authorization: Bearer ${token}" \
      "${EXTRA_HEADERS[@]}" "${API_BASE}${path}")"
  else
    code="$(curl -sS -o "$LAST" -w '%{http_code}' -X "$method" -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${token}" "${EXTRA_HEADERS[@]}" --data "$body" "${API_BASE}${path}")"
  fi
  EXTRA_HEADERS=()
  local line
  if ! line="$(python3 - "$code" "$LAST" "$accept" "$@" <<'PY'
import json, sys
code, path, accept = sys.argv[1:4]
fields = sys.argv[4:]
raw = open(path, encoding="utf-8").read()
try:
    body = json.loads(raw)
except Exception:
    body = {}
outcome = body.get("outcome", "") if isinstance(body, dict) else ""
ok = False
for item in accept.split(","):
    want_code, _, want_outcome = item.partition(":")
    if want_code == code and (want_outcome in ("", "*") or want_outcome == outcome):
        ok = True
if not ok:
    sys.exit(1)
parts = [f"{code} {outcome}".strip()]
for f in fields:
    cur = body
    try:
        for p in f.split("."):
            cur = cur[int(p)] if isinstance(cur, list) else cur[p]
    except Exception:
        cur = ""
    if isinstance(cur, (dict, list)):
        cur = json.dumps(cur, ensure_ascii=False)
    parts.append(f"{f}={cur}")
print(" ".join(parts))
PY
  )"; then
    printf '\n[失败] %s\n  %s %s 答 HTTP %s，可接受 %s。原始响应：\n' "$label" "$method" "$path" "$code" "$accept" >&2
    cat "$LAST" >&2
    printf '\n' >&2
    exit 1
  fi
  printf '%-34s → %s\n' "$label" "$line"
}

section() { printf '\n== %s ==\n' "$1"; }

# jsel FILE 列表路径 键 值 取值路径：在 JSON 列表里找 键==值 的第一行，打印它的「取值路径」；找不到打空串。
jsel() {
  python3 - "$@" <<'PY'
import json, sys
path, listpath, key, value, out = sys.argv[1:6]
try:
    cur = json.load(open(path, encoding="utf-8"))
    for part in listpath.split("."):
        if part:
            cur = cur[part]
    for row in cur:
        if row.get(key) == value:
            for part in out.split("."):
                if part:
                    row = row[int(part)] if isinstance(row, list) else row[part]
            print(row if not isinstance(row, (dict, list)) else json.dumps(row, ensure_ascii=False))
            break
    else:
        print("")
except Exception:
    print("")
PY
}

# poll 标签 秒数 命令…：每两秒跑一次命令，成功（退 0）即止；超时打印该标签并失败。
poll() {
  local label=$1 secs=$2
  shift 2
  local end=$((SECONDS + secs))
  while ((SECONDS < end)); do
    if "$@"; then return 0; fi
    sleep 2
  done
  printf '\n[失败] 等 %s 超过 %s 秒——dispatch 在跑吗？日志 %s/parcel-dispatch.log\n' "$label" "$secs" "$RUN_DIR" >&2
  exit 1
}
