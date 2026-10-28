#!/usr/bin/env bash
# 申报发送走一遍（票 product-strategy-boundary/10，ADR-0342）：declare-one-parcel.sh 形成一版提交之后，dispatch 经通道把它发给
# 本机的合成报关服务商（declaration-broker/），服务商同步答已收并给一个消息号。本脚本扮服务商的另一半：拿集成客户端令牌把一份
# 异步技术回执送到 `POST /customs/technical-receipts`，凭消息号关联回那一次尝试，末行打出尝试此刻的技术判断。
#
# 用法：
#   IDP_PARCEL_API_BASE=http://127.0.0.1:19080 scripts/demo-seeds/send-one-declaration.sh <提交版本号>
# 提交版本号取 declare-one-parcel.sh 末行打出的那一个。要先起：合成服务商（`go run ./scripts/demo-seeds/declaration-broker`）、
# cmd/parcel-dispatch、带集成客户端族三件参数的 parcel-api（README「集成客户端渠道」）。
#
# 消息号由服务商按幂等键 `<租户>/<版本>/1` 派生（见 declaration-broker 头注），这里同法算出，不读服务商的输出。dispatch 还没发出去
# 时回执答 UNATTRIBUTABLE，本脚本每两秒再送一次、至多一分钟；同一份回执重送答 EXISTING_RECEIPT，不多落一行。证据只记 S。
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf '用法：%s <提交版本号>\n' "$0" >&2
  exit 1
fi
version="$1"
base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'send-one-declaration: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

here="$(cd "$(dirname "$0")" && pwd)"
if ! token="$("$here/operator-channel/integration-token.sh" syn-integration-customs-broker-01 syn-customs-broker-secret)"; then
  printf 'send-one-declaration: 取不到集成客户端令牌——集成发行方起了没有（README「集成客户端渠道」）\n' >&2
  exit 1
fi

message="$(python3 -c 'import hashlib,sys; print("SYN-MSG-" + hashlib.sha256(sys.argv[1].encode()).hexdigest()[:16].upper())' \
  "SYN-TENANT-01/${version}/1")"
body="$(python3 - "$message" <<'PY'
import json, sys
message = sys.argv[1]
print(json.dumps({"sourceId": "SYN-RCPT-" + message, "messageId": message, "outcome": "ACKNOWLEDGED",
                  "occurredAt": "2026-10-28T10:00:00Z"}))
PY
)"
answer_file="$(mktemp)"
trap 'rm -f "$answer_file"' EXIT

for _ in $(seq 30); do
  status="$(curl -sS -o "$answer_file" -w '%{http_code}' -X POST -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" --data "$body" "${base}/customs/technical-receipts")"
  outcome="$(python3 - "$status" "$answer_file" <<'PY'
import json, sys
status, path = sys.argv[1:]
raw = open(path, encoding="utf-8").read()
if status in ("401", "403"):
    sys.exit(f"/customs/technical-receipts {status}：令牌或授予没过。本脚本不绕过。应答：\n" + raw)
print(f"{status}:{json.loads(raw).get('outcome')}")
PY
)"
  case "$outcome" in
    200:UNATTRIBUTABLE) sleep 2 ;;
    201:RECEIPT_RECORDED|200:EXISTING_RECEIPT)
      python3 -c 'import json,sys; b=json.load(open(sys.argv[1])); print(f"消息号 {sys.argv[2]}；尝试 {b[\"version\"]}/{b[\"sequence\"]}；回执 {b[\"outcome\"]}；当前技术判断 {b[\"current\"]}")' \
        "$answer_file" "$message"
      exit 0 ;;
    *) printf 'send-one-declaration: 回执答 %s，应答：\n%s\n' "$outcome" "$(cat "$answer_file")" >&2; exit 1 ;;
  esac
done
printf 'send-one-declaration: 一分钟内消息号 %s 都关联不上——dispatch 起了没有、合成服务商起了没有、通道登了没有\n' "$message" >&2
exit 1
