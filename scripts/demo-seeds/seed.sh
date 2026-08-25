#!/usr/bin/env bash
# 合成 S 主数据种子包一键灌入（票 master-data-wiring/08）。**仅限隔离环境**：
# 种子全部是 SYN- 前缀的合成实例（证据层级 S），不影射任何真实企业；本脚本
# 绝不能指向生产库——它信任 IDP_PARCEL_POSTGRES_DSN，不做「这是不是演示库」的猜测。
#
# 用法：
#   IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable \
#     ./scripts/demo-seeds/seed.sh [--reset]
#
#   --reset  先 DROP 全部 parcel schema 再重迁重灌（干净库复灌用，破坏性，仅限演示库）。
#
# 干净库上全程零报错；四个登记 CLI 的非零退出码会经 set -e 中止脚本并如实透出。
set -euo pipefail

cd "$(dirname "$0")/../.."

: "${IDP_PARCEL_POSTGRES_DSN:?IDP_PARCEL_POSTGRES_DSN 未设置——种子脚本不猜连接串}"

RESET_FLAG=""
if [[ "${1:-}" == "--reset" ]]; then
  RESET_FLAG="-reset"
fi

SEEDS=scripts/demo-seeds/data
BIN="$(mktemp -d)"
trap 'rm -rf "$BIN"' EXIT

echo "== 0/5 编译登记 CLI 与迁移助手 =="
go build -o "$BIN/" \
  ./cmd/parcel-pricing-register \
  ./cmd/parcel-network-register \
  ./cmd/parcel-customs-register \
  ./cmd/parcel-commercial \
  ./scripts/demo-seeds/migrate

echo "== 1/5 施加迁移计划（${RESET_FLAG:-不重置}） =="
"$BIN/migrate" $RESET_FLAG

echo "== 2/5 商业权威发布（party-commercial：服务产品与五策略） =="
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-batch.json"

echo "== 3/5 计价登记（parcel-pricing：价卡 + 参考序列） =="
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-cost.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fx-cny-sgd.json"

echo "== 4/5 网络目录登记（network-routing：七族版本行） =="
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/01-node-sha-hub-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/02-node-szx-gate-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/03-node-sin-hub-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/04-node-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind node -file "$SEEDS/network/05-node-sha-hub-v2.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/06-conn-sha-szx-v1.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/07-conn-szx-sin-v1.json"
"$BIN/parcel-network-register" -kind connection -file "$SEEDS/network/08-conn-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind line -file "$SEEDS/network/09-line-cn-sg-v1.json"
"$BIN/parcel-network-register" -kind service-area -file "$SEEDS/network/10-area-cn-east-v1.json"
"$BIN/parcel-network-register" -kind service-area -file "$SEEDS/network/11-area-sg-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/12-calendar-line-cn-sg-v1.json"
"$BIN/parcel-network-register" -kind availability-adjustment -file "$SEEDS/network/13-adjustment-typhoon-v1.json"
"$BIN/parcel-network-register" -kind route-strategy -file "$SEEDS/network/14-route-strategy-cn-sg-v1.json"

echo "== 5/5 关务案件配置登记（customs-compliance：六册） =="
"$BIN/parcel-customs-register" readiness-register -input "$SEEDS/customs/01-readiness-cn-export.json"
"$BIN/parcel-customs-register" authority-grant -input "$SEEDS/customs/02-authority-grant.json"
"$BIN/parcel-customs-register" interpretation-rule -input "$SEEDS/customs/03-interpretation-rule-v1.json"
"$BIN/parcel-customs-register" interpretation-rule -input "$SEEDS/customs/04-interpretation-rule-v2.json"
"$BIN/parcel-customs-register" obligation-catalog -input "$SEEDS/customs/05-obligation-catalog.json"
"$BIN/parcel-customs-register" obligation-item -input "$SEEDS/customs/06-obligation-item-concluded.json"
"$BIN/parcel-customs-register" obligation-item -input "$SEEDS/customs/07-obligation-item-handed.json"
"$BIN/parcel-customs-register" gate-catalog -input "$SEEDS/customs/08-gate-catalog.json"
"$BIN/parcel-customs-register" gate-finding -input "$SEEDS/customs/09-gate-finding-met.json"
"$BIN/parcel-customs-register" case-requirement -input "$SEEDS/customs/10-case-requirement-cn-export.json"
"$BIN/parcel-customs-register" case-requirement -input "$SEEDS/customs/11-case-requirement-sg-import.json"

echo "种子灌入完成：租户 SYN-TENANT-01，四上下文全部落库。"
