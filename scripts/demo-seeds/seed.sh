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
# 干净库上全程零报错；五个登记 CLI 的非零退出码会经 set -e 中止脚本并如实透出。
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

echo "== 0/6 编译登记 CLI 与迁移助手 =="
go build -o "$BIN/" \
  ./cmd/parcel-pricing-register \
  ./cmd/parcel-network-register \
  ./cmd/parcel-customs-register \
  ./cmd/parcel-commercial \
  ./cmd/parcel-ve-register \
  ./scripts/demo-seeds/migrate

echo "== 1/6 施加迁移计划（${RESET_FLAG:-不重置}） =="
"$BIN/migrate" $RESET_FLAG

echo "== 2/6 商业权威发布（party-commercial：服务产品与五策略）与消费方解析键登记 =="
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-batch.json"
# 解析键属消费方（parcel-shipment）的实例半边，与上一行的商业权威发布不是同一件事，
# 只因同一个 CLI 承两个入口才挨在一起。键上要结算依据、因而也要客户合同——结算政策按
# 哪一版合同选由闭包解出（ADR-0080），登记面结构上没有合同维。
#
# 上一行的发布批里 `SYN-SETTLEMENT-PREPAID-01` 的六维正是照这行键的三维（相对方、费用
# 范围、币种）加闭包会解出的合同版本配的，闭包因此解得开。六维差一维就命不中：这不是
# 巧合要维护，而是本上下文明禁借宽泛客户关系跨维归集。
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-01.json"
# 参与方身份四册（票 admin-remainder-mechanism-batch/01）：SYN-LE-01 与 SYN-ACCOUNT-01
# 在此获得身份册登记，与上面发布批里的同名引用同指一物。停用批单独一笔，让法人页
# 与参与方页的身份状态三格（已登记/已生效/已停用）都有真实例可显。
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-parties.json"
"$BIN/parcel-commercial" deactivate-party-identity \
  -input "$SEEDS/commercial/deactivate-party-retired-01.json"

echo "== 3/6 计价登记（parcel-pricing：价卡 + 参考序列） =="
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-cost.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fx-cny-sgd.json"

echo "== 4/6 网络目录登记（network-routing：七族版本行） =="
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

echo "== 5/6 关务案件配置登记（customs-compliance：六册） =="
"$BIN/parcel-customs-register" readiness-register -input "$SEEDS/customs/01-readiness-cn-export.json"
"$BIN/parcel-customs-register" authority-grant -input "$SEEDS/customs/02-authority-grant.json"
# 第二单元灌出「就绪仍有效、授权已撤销」——0006 迁移自注点名必须表达得出的那一格，
# 案件页提交前件两栏据它展示各自独立的撤销态（票 admin-web-page-wiring-frontier/05）。
"$BIN/parcel-customs-register" readiness-register -input "$SEEDS/customs/12-readiness-cn-export-02.json"
"$BIN/parcel-customs-register" authority-grant -input "$SEEDS/customs/13-authority-grant-02.json"
"$BIN/parcel-customs-register" authority-revoke -input "$SEEDS/customs/14-authority-revoke-02.json"
"$BIN/parcel-customs-register" interpretation-rule -input "$SEEDS/customs/03-interpretation-rule-v1.json"
"$BIN/parcel-customs-register" interpretation-rule -input "$SEEDS/customs/04-interpretation-rule-v2.json"
"$BIN/parcel-customs-register" obligation-catalog -input "$SEEDS/customs/05-obligation-catalog.json"
"$BIN/parcel-customs-register" obligation-item -input "$SEEDS/customs/06-obligation-item-concluded.json"
"$BIN/parcel-customs-register" obligation-item -input "$SEEDS/customs/07-obligation-item-handed.json"
"$BIN/parcel-customs-register" gate-catalog -input "$SEEDS/customs/08-gate-catalog.json"
"$BIN/parcel-customs-register" gate-finding -input "$SEEDS/customs/09-gate-finding-met.json"
# 第二份门禁只登目录、不登任何前置条件——0008 自注点名必须与「目录未登记」分得开的
# 那一格：登记了空清单是「此动作在此边界本就不受门禁」（领域折为不适用），未登记才是
# 无从复核。合规限制页据它展示两态各说各话（票 admin-web-page-wiring-frontier/06）。
"$BIN/parcel-customs-register" gate-catalog -input "$SEEDS/customs/15-gate-catalog-unguarded.json"
"$BIN/parcel-customs-register" case-requirement -input "$SEEDS/customs/10-case-requirement-cn-export.json"
"$BIN/parcel-customs-register" case-requirement -input "$SEEDS/customs/11-case-requirement-sg-import.json"

echo "== 6/6 追踪与异常目录登记（visibility-exception：六类七笔） =="
"$BIN/parcel-ve-register" milestone-mapping -input "$SEEDS/visibility/01-milestone-mapping-v1.json"
"$BIN/parcel-ve-register" triage-rules -input "$SEEDS/visibility/02-triage-rules-v1.json"
"$BIN/parcel-ve-register" notification-policy -input "$SEEDS/visibility/03-notification-policy-disclose-v1.json"
"$BIN/parcel-ve-register" claim-eligibility -input "$SEEDS/visibility/04-claim-eligibility-contract-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/05-claim-authorization-account-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/06-claim-authorization-account-02-empty.json"
"$BIN/parcel-ve-register" disclosure-policy -input "$SEEDS/visibility/07-disclosure-policy-v1.json"

echo "种子灌入完成：租户 SYN-TENANT-01，五上下文全部落库。"
