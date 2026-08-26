#!/usr/bin/env bash
# 票 01 页面层取证的检查脚本：对 Edge 无头 --dump-dom 的产物核字符串。
# 命中集分两组——**该在的**（SYN 实例值、计数摘要、状态词）与**不该在的**
# （未配置码与骨架期占位语），两组一起核，因为「有数据」与「没混进未配置态」
# 是两件事，只核前一组会漏掉页面同时渲染两套说法的情形。
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

for id in party-contracts supplier-agreements; do
  f="$TMP/$id.html"
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
  esac
  check "$f" forbid "ACCESS_CHANNEL_NOT_CONFIGURED" "尚未接线" "访问通道尚未配置" "0 份"
done
