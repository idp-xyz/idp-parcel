#!/usr/bin/env bash
# 页面层取证：对 Edge 无头 --dump-dom 的产物核字符串。
# 命中集分两组——**该在的**（SYN 实例值、计数摘要、状态词）与**不该在的**
# （未配置码与骨架期占位语），两组一起核，因为「有数据」与「没混进未配置态」
# 是两件事，只核前一组会漏掉页面同时渲染两套说法的情形。
#
# needle 逐页写死在下面的 case 里：这是取证脚本，通用化会让它答不出「这一页
# 到底该看见什么」。接新页时加一段 case，不要改成从参数读。
set -uo pipefail

TMP=${1:-/tmp/parcel-dom}

check() {
  local file="$1"; shift
  local sense="$1"; shift
  for needle in "$@"; do
    if grep -qF -- "$needle" "$file"; then
      echo "  [$sense] HIT  $needle"
    else
      echo "  [$sense] miss $needle"
    fi
  done
}

for id in "${@:2}"; do
  f="$TMP/$id.html"
  [ -f "$f" ] || { echo "=== $id (缺 DOM 产物，先跑 dom-dump.sh) ==="; continue; }
  echo "=== $id ($(wc -c <"$f") bytes) ==="
  case "$id" in
    party-contracts)
      check "$f" want "SYN-CONTRACT-01" "SYN-RULEPKG-01" "SYN-CHARGE-PREPAID" \
        "SYN-FIN-CONTROL-01" "SYN-CHARGE-COD" "不适用" "已生效" "份合同版本"
      ;;
    supplier-agreements)
      check "$f" want "SYN-SUPPLIER-TRUNK-CN-SG-01" "SYN-SUPPLIER-LASTMILE-SG-01" \
        "已生效" "份协议版本"
      ;;
    commercial-policies)
      # 票 07：接单规则包页签上新增的两族阶段内容声明。四个终局结果与两个收寄来源
      # 都要在，且规则引用仍是 5 条——三族若互相翻倍，条数会一起变。
      check "$f" want "SYN-RULEPKG-01" "SYN-QUAL-REALNAME-01" \
        "节点收寄" "场外揽收" "有效送达" "退回完成" "服务终止" "监管处置" \
        "SYN-FINAL-DELIVERY-01" "接单规则包 1 条"
      check "$f" forbid "未声明" "已声明,正文为空"
      ;;
  esac
  check "$f" forbid "ACCESS_CHANNEL_NOT_CONFIGURED" "尚未接线" "访问通道尚未配置" "0 份"
done
