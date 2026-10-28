#!/usr/bin/env bash
# 段四·异常（票 demo-2026-10-30/03），两件事，各用自己的包裹：
#   甲 收寄后处置：一件新包裹走到收寄，运营发起退运决定（RETURN）→ 待人工处理清单列 NR、NO 两项 → 登记 HANDLED、UNABLE →
#      原样重登 ALREADY_REGISTERED → 读回两项带结果与处理人、未登记清单清空。（管理台 `post-intake-disposition` 页也能点，见 README。）
#   乙 客户渠道命令：walk-customer-command-faces.sh 现提两份委托，走补充、撤回、修订、索赔、取消各一次成立一次重放，
#      外加两条反例（管理台的撤回、取消页发的是同一批口但带操作者令牌，必 401，所以现场用终端）。
# 乙会再现提两份委托（各占一次冻结余额）。证据只记 S。
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
load_env
export IDP_PARCEL_API_BASE
source "$DEMO_DIR/chain.sh"

STAMP="$(date -u +%H%M%S)"
section "异常甲·收寄后处置（退运）"
ch_submit
ch_intake
step "退运决定（运营）" "$OP" POST /post-intake-disposition-decisions \
  "$(pj parcel="$PARCEL" supersedes:=0 direction=RETURN basis=SYN-CHECKED-PRODUCT-AND-CONTRACT)" 201:DECIDED \
  decision.version decision.ownerRequests
step "待人工处理清单" "$OP" GET "/post-intake-disposition-manual-items?parcel=${PARCEL}" - 200:* pendingCount items.0.owner items.0.action items.1.owner items.1.action
python3 - "$LAST" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))["items"]
owners = sorted(i["owner"] for i in items)
if owners != ["NETWORK_ROUTING", "NODE_OPERATIONS"]:
    sys.exit(f"[失败] 待人工清单要 NETWORK_ROUTING、NODE_OPERATIONS 两项，读到 {owners}")
PY
nr_action="$(jsel "$LAST" items owner NETWORK_ROUTING action)"; no_action="$(jsel "$LAST" items owner NODE_OPERATIONS action)"
dv="$(jget "$LAST" items.0.decisionVersion)"
reg() { # 属主 动作 结果 备注 可接受
  step "登记 $1 → $3" "$OP" POST /post-intake-disposition-manual-item-results \
    "$(pj parcel="$PARCEL" decisionVersion:="$dv" owner="$1" action="$2" outcome="$3" note="$4")" "$5" outcome
}
reg NETWORK_ROUTING "$nr_action" HANDLED "SYN-NOTE 退运路线已人工规划" 201:REGISTERED
reg NODE_OPERATIONS "$no_action" UNABLE "SYN-NOTE 节点无法装退运" 201:REGISTERED
reg NETWORK_ROUTING "$nr_action" HANDLED "SYN-NOTE 退运路线已人工规划" 200:ALREADY_REGISTERED
step "读回清单（带结果与处理人）" "$OP" GET "/post-intake-disposition-manual-items?parcel=${PARCEL}" - 200:* pendingCount items.0.result.outcome items.0.result.handledBy items.1.result.outcome
python3 - "$LAST" <<'PY'
import json, sys
body = json.load(open(sys.argv[1]))
if any(not i.get("result") for i in body["items"]):
    sys.exit("[失败] 读回的清单里还有未登记结果的项")
PY
state_set EXC_PARCEL "$PARCEL"

section "异常乙·客户渠道命令（补充、撤回、修订、索赔、取消）"
walk_out="$(bash "$SEEDS_DIR/walk-customer-command-faces.sh" 2>&1)" || { printf '[失败] walk-customer-command-faces.sh：\n%s\n' "$walk_out" >&2; exit 1; }
printf '%s\n' "$walk_out" | sed 's/^/  /'
printf '\n异常段完成\n'
