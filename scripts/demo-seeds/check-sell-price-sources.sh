#!/usr/bin/env bash
# 灌完演示库之后核对销售价格来源这条链（票 commercial-four-module-requirements/21）：两位新客户的合同各自声明的来源、
# 旧合同没有约定、采用册带不带计价范围，以及受理前估价与正式 SELL 评价两条编排对同一件包裹选到同一张卡、算出同一个金额。
# 核的内容在 sellpricewalk 包里，本脚本只把它指到那只库上。
#
# 不进 seed.sh：种子是主数据，这一步是核对。只读库，评价不入册。
#
# 用法：
#   IDP_PARCEL_POSTGRES_DSN=postgres://…/<刚灌过的库>?sslmode=disable scripts/demo-seeds/check-sell-price-sources.sh
set -euo pipefail

: "${IDP_PARCEL_POSTGRES_DSN:?IDP_PARCEL_POSTGRES_DSN 未设置——核对脚本不猜连接串}"

cd "$(dirname "$0")/../.."
if ! command -v go >/dev/null 2>&1 && [[ -x /usr/local/go/bin/go ]]; then
  export PATH="/usr/local/go/bin:${PATH}"
fi
IDP_PARCEL_DEMO_CHECK=1 go test -count=1 -v -run 'TestDemoSellPriceSources' ./scripts/demo-seeds/sellpricewalk/
