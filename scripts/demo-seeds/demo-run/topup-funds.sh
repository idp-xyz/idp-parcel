#!/usr/bin/env bash
# 给演示账户补可用余额（票 demo-2026-10-30/03）：合成银行客户端经生产口登一笔外部收款，再用受控 CLI 把它映射到账户。
# 映射没有在线口（映射只有受控登记，不走管理台），所以这一步必须用 parcel-settlement-register funds-mapping。
#
# 用法：topup-funds.sh [金额（最小币单位，缺省 100000）] [序号后缀（缺省 01）]
# 每件委托冻结 9600，种子余额 20000，缺省补 100000 后约可连发十二件。同一后缀只做一次（事实号固定，重放不另落）；
# 余额再用尽时换一个后缀再补。
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env

amount="${1:-100000}"
suffix="${2:-01}"
fact="SYN-FUNDS-DEMO-TOPUP-${suffix}"
occurred="$(now_utc)"

section "补演示账户余额（${fact}，${amount}）"
tok="$(integration_token syn-integration-bank-01 syn-bank-secret)"
body="$(pj factRef="$fact" sourceRef="SYN-FUNDS-SOURCE-DEMO-${suffix}" payerRef=SYN-PARTY-SHIPPER-01 kind=RECEIPT_CONFIRMED \
  currency=CNY "amountMinor:=${amount}" version=v1 occurredAt="${occurred}")"
# 重放时 occurredAt 不同会被当作冲突：已做过就不再登。
done_key="FUNDS_TOPUP_${suffix}_DONE"
if [[ -n "${!done_key:-}" ]]; then
  printf '后缀 %s 已补过，跳过（要再补就换一个后缀）\n' "$suffix"
  exit 0
fi
step "登外部收款（银行客户端）" "$tok" POST /settlement-external-funds-fact-registrations "$body" "201:*,200:*"

mapfile_json="$RUN_DIR/funds-mapping-${suffix}.json"
python3 - "$fact" "$occurred" "$suffix" > "$mapfile_json" <<'PY'
import json, sys
fact, at, suffix = sys.argv[1:]
print(json.dumps({"tenantId": "SYN-TENANT-01", "mappingRef": f"SYN-FUNDS-MAP-DEMO-{suffix}", "factRef": fact,
                  "targetKind": "ACCOUNT", "targetRef": "SYN-SETTLEMENT-ACCOUNT-01",
                  "basis": "SYN-PREPAY-INSTRUCTION-01", "mappedAt": at}))
PY
out="$("$BIN_DIR/parcel-settlement-register" funds-mapping -input "$mapfile_json" 2>&1)" || {
  printf '[失败] 映射 CLI 非零退出。输出：\n%s\n' "$out" >&2; exit 1; }
printf '%-34s → %s\n' "映射到账户（受控 CLI）" "$out"
state_set "$done_key" 1
