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
# 干净库上全程零报错；六个登记 CLI 的非零退出码会经 set -e 中止脚本并如实透出。
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

echo "== 0/7 编译登记 CLI 与迁移助手 =="
go build -o "$BIN/" \
  ./cmd/parcel-pricing-register \
  ./cmd/parcel-network-register \
  ./cmd/parcel-customs-register \
  ./cmd/parcel-commercial \
  ./cmd/parcel-ve-register \
  ./cmd/parcel-collection-register \
  ./cmd/parcel-governance-register \
  ./scripts/demo-seeds/migrate

echo "== 1/7 施加迁移计划（${RESET_FLAG:-不重置}） =="
"$BIN/migrate" $RESET_FLAG

echo "== 2/7 商业权威发布（party-commercial：服务产品与五策略）与消费方解析键登记 =="
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
# 服务形态与产品—渠道映射（票 admin-remainder-mechanism-batch/02）：EXPRESS 配两个
# 渠道引用，ECON 显式登记「未配置」——批级判据点名实例格显式未配置要有真实例可显；
# 渠道本体不预造（ADR-0072），引用等 PAR-INT-01 的接入证据。
"$BIN/parcel-commercial" register-products -input "$SEEDS/commercial/register-products.json"

echo "== 3/7 计价登记（parcel-pricing：价卡 + 参考序列 + 序列复核） =="
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-cost.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fx-cny-sgd.json"
# 序列版本经复核通过才在用（ADR-0099）：先登记再复核，复核责任方与登记责任方不同。
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fuel-review.json"
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fx-cny-sgd-review.json"

echo "== 4/7 网络目录登记（network-routing：七族版本行） =="
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

echo "== 5/7 关务案件配置登记（customs-compliance：八册） =="
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
# 口岸目录与申报路径目录（票 admin-remainder-mechanism-batch/03）：SZX 口岸 v1→v2 换版
# 展示版本轴（v1 终点由 v2 登记落定），两条路径三维（口岸、方向、申报模式）齐全。
# 路径以标识引用口岸、不查在册性——两册对照是读侧判断，裁量记在票 03。
"$BIN/parcel-customs-register" candidate-port -input "$SEEDS/customs/16-candidate-port-szx-v1.json"
"$BIN/parcel-customs-register" candidate-port -input "$SEEDS/customs/17-candidate-port-szx-v2.json"
"$BIN/parcel-customs-register" candidate-port -input "$SEEDS/customs/18-candidate-port-sin-v1.json"
"$BIN/parcel-customs-register" declaration-path -input "$SEEDS/customs/19-declaration-path-cn-export.json"
"$BIN/parcel-customs-register" declaration-path -input "$SEEDS/customs/20-declaration-path-sg-import.json"

echo "== 6/7 追踪与异常目录登记（visibility-exception：六类七笔） =="
"$BIN/parcel-ve-register" milestone-mapping -input "$SEEDS/visibility/01-milestone-mapping-v1.json"
"$BIN/parcel-ve-register" triage-rules -input "$SEEDS/visibility/02-triage-rules-v1.json"
"$BIN/parcel-ve-register" notification-policy -input "$SEEDS/visibility/03-notification-policy-disclose-v1.json"
"$BIN/parcel-ve-register" claim-eligibility -input "$SEEDS/visibility/04-claim-eligibility-contract-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/05-claim-authorization-account-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/06-claim-authorization-account-02-empty.json"
"$BIN/parcel-ve-register" disclosure-policy -input "$SEEDS/visibility/07-disclosure-policy-v1.json"

echo "== 7/7 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="
# 一条 COD 指令走全程（票 admin-remainder-mechanism-batch/04）：渠道报收全额入账
# `渠道在途`；银行实际到账少于渠道报收，到账部分凭该到账事实搬进`待清分`，差额先登
# 短款差异事项、再凭它落进`短款`——差额不自动落账；到账部分凭 ALLOCATION（指向代收
# 指令）清分进`应付客户`，进应付客户不得凭代收事实（第四条依据门）。SGD 分户账只
# 开立不记账，让「已开立但当期无记账」与「未开立」分得开；回汇批次一笔不造——批次
# 属实例半边，页面上那格要显式展示未配置。
#
# 已灌过的库上重放本节会在记账处以 UNDERFUNDED（退出码 4）中止：写口的重放判定排在
# 余额守卫之后，来源位置被原记账清空后守卫先答余额不足，轮不到「已在册」（实测于
# d340014 对演示库重放）。非干净库复灌一律走 --reset，别指望本节像前六步那样幂等。
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/01-subledger-account-01-cny.json"
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/02-subledger-account-01-sgd.json"
"$BIN/parcel-collection-register" instruction -input "$SEEDS/collection/03-instruction-parcel-01.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/04-fact-channel-report.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/05-posting-intake-in-transit.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/06-fact-bank-credit.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/07-posting-settle-awaiting-allocation.json"
"$BIN/parcel-collection-register" discrepancy -input "$SEEDS/collection/08-discrepancy-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/09-posting-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/10-posting-allocation-payable.json"

echo "== 试点治理登记（pilot-governance：权威区间、暂停、恢复；票 admin-skeleton-closure-batch/02） =="
# 治理是产品级机制，登记无租户维（ADR-0083）；登记走受控 CLI（票 syn-wall-door-audit/12），
# 首批只开三类——阶段评审与接管第二批，故 stage-admission 页那两格如实说明未开，不造数。
# 权威区间讲一次交接：旧引擎区间已闭、试点引擎接棒开放区间，相邻不重叠（冲突预检半开区间语义）。
# 暂停两笔一笔已恢复：恢复四件（解除证据、一致性核对、在途盘点、决定人）由领域门把守，缺一不可。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/01-authority-interval-routing-legacy.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/02-authority-interval-routing-pilot.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/03-authority-interval-pricing-shadow.json"
"$BIN/parcel-governance-register" suspend -input "$SEEDS/governance/04-suspension-routing-scope.json"
"$BIN/parcel-governance-register" resume -input "$SEEDS/governance/05-resumption-routing-scope.json"
"$BIN/parcel-governance-register" suspend -input "$SEEDS/governance/06-suspension-pricing-scope.json"
# 委托受理维的权威区间（ADR-0091）：隔离写路径准入启用时，生产归属就是拿这一行作答。
# 四维必须与 cmd/parcel-api 的 isolatedGovernance* 常量逐字相同——对不上的后果不是报错，
# 是查不到这一行，答出来的`权威未确定`与「压根没登记」一模一样。本行的试点范围版本与
# 上面两笔暂停各不相同，因此不受它们影响，准入控制为 OPEN。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/07-authority-interval-shipment-intake.json"

echo "种子灌入完成：租户 SYN-TENANT-01，七上下文全部落库。"
