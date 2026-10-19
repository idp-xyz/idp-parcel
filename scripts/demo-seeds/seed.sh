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
# 干净库上全程零报错；各登记 CLI 的非零退出码会经 set -e 中止脚本并如实透出。
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

echo "== 编译登记 CLI 与迁移助手 =="
go build -o "$BIN/" \
  ./cmd/parcel-pricing-register \
  ./cmd/parcel-network-register \
  ./cmd/parcel-customs-register \
  ./cmd/parcel-commercial \
  ./cmd/parcel-ve-register \
  ./cmd/parcel-collection-register \
  ./cmd/parcel-governance-register \
  ./cmd/parcel-access-register \
  ./cmd/parcel-settlement-register \
  ./cmd/parcel-transport-register \
  ./scripts/demo-seeds/migrate

echo "== 施加迁移计划（${RESET_FLAG:-不重置}） =="
"$BIN/migrate" $RESET_FLAG

echo "== 商业权威发布（party-commercial：发布批正文）与消费方解析键登记 =="
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
# 第二客户 SYN-ACCOUNT-02 的键：形状照上一行，三维换成它自己的相对方 SYN-PARTY-SHIPPER-02，闭包解出它自己的合同
# SYN-CONTRACT-02、再与 SYN-SETTLEMENT-PREPAID-02 的六维命中。两户的必需依据都列 CUSTOMER_SERVICE_RULE：
# 不列，闭包就不采用任何客户服务规则版本，索赔的首次索赔期限与最低材料两维恒答未登记。
# 两户的必需依据也都列 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY：不列，闭包就不采用合同上绑的
# SYN-FIN-CONTROL-01，受理前财务控制答策略未配置（票 product-strategy-boundary/26 实跑停在这一格）。
# 同一范围并存两版客户服务规则：挂产品的 SYN-SERVICE-RULE-01（首次索赔 14 天、LOSS 要 POD）与壳指名合同 02 的
# SYN-SERVICE-RULE-CONTRACT-02（只登首次索赔 7 天）。闭包合同优先（ADR-0176 决定一）：账户 02 采纳合同版，
# 期限答 7 天、材料行继承产品版的 POD（决定二）；账户 01 的合同层零候选，回落产品版，答复与没有合同版时相同。
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-02.json"
# 注册号类型目录（票 legal-entity-profile/01、product-strategy-boundary/03）：身份层显式采用 CN、SG 两份
# 参考配置（ADR-0147），资料层各一类合成类型，另一类登记后停用。
# 排在参与方身份之前：责任法人身份登记按注册国家 / 地区在这份目录里判号（ADR-0145 决定一）。
"$BIN/parcel-commercial" register-registration-number-types \
  -input "$SEEDS/commercial/register-registration-number-types.json"
# 参与方身份四册（票 admin-remainder-mechanism-batch/01）：SYN-LE-01 与 SYN-ACCOUNT-01
# 在此获得身份册登记，与上面发布批里的同名引用同指一物。停用批单独一笔，让法人页
# 与参与方页的身份状态三格（已登记/已生效/已停用）都有真实例可显。
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-parties.json"
"$BIN/parcel-commercial" deactivate-party-identity \
  -input "$SEEDS/commercial/deactivate-party-retired-01.json"
# 法人资料（票 legal-entity-profile/03）：SYN-LE-01 两笔修订，先不带开票资料、后补抬头，让资料页的「资料不全」与
# 修订切换都有真实例可显。排在参与方身份之后：资料引用已登记的责任法人，地址国家对着它的身份层判。
"$BIN/parcel-commercial" register-legal-entity-profiles \
  -input "$SEEDS/commercial/register-legal-entity-profiles.json"
# 服务形态与产品—渠道映射（票 admin-remainder-mechanism-batch/02）：EXPRESS 配两个
# 渠道引用，ECON 显式登记「未配置」——批级判据点名实例格显式未配置要有真实例可显；
# 渠道本体不预造（ADR-0072），引用等 PAR-INT-01 的接入证据。
"$BIN/parcel-commercial" register-products -input "$SEEDS/commercial/register-products.json"

echo "== 计价登记（parcel-pricing：价卡 + 参考序列 + 序列复核 + 参考目录） =="
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg.json"
"$BIN/parcel-pricing-register" -kind price-card -file "$SEEDS/pricing/price-card-cn-sg-cost.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fuel.json"
"$BIN/parcel-pricing-register" -kind reference-series -file "$SEEDS/pricing/reference-series-fx-cny-sgd.json"
# 序列版本经复核通过才在用（ADR-0099）：先登记再复核，复核责任方与登记责任方不同。
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fuel-review.json"
"$BIN/parcel-pricing-register" -kind reference-series-review -file "$SEEDS/pricing/reference-series-fx-cny-sgd-review.json"
# 参考目录同一条纪律（ADR-0109）：先登记再复核。售价卡绑这本目录，分区由委托申报的邮编路线经它解析。
"$BIN/parcel-pricing-register" -kind reference-catalogue -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg.json"
"$BIN/parcel-pricing-register" -kind reference-catalogue-review -file "$SEEDS/pricing/reference-catalogue-zone-cn-sg-review.json"

echo "== 网络目录登记（network-routing：演示租户采用参考配置 network-routing/network-catalog/SYN-CN-SG@1） =="
# 演示网络随产品发布为参考配置（票 routing-first-cut/11，ADR-0147）。下面每一行稳定定义都是一笔采用：
# 登记行只给身份、修订号与生效时点，内容取自那一版参考配置，依据格写成 REFCFG-1 引用串。登记行不点名
# adopt，参考配置就不进任何租户的目录。日历是四个节点各一份（处理时长与截单）加三条连接各一份（衔接
# 缓冲）——候选经过的每个节点都要有处理时长才得到时间投影（ADR-0175）。
# 上海枢纽的修订 2 不是采用：它演示「采用后再改就是租户取值」，依据换成演示租户自己的 SYN- 依据。
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
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/12-calendar-node-sha-hub-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/13-calendar-node-szx-gate-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/14-calendar-node-sin-hub-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/15-calendar-node-sin-lm-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/16-calendar-conn-sha-szx-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/17-calendar-conn-szx-sin-v1.json"
"$BIN/parcel-network-register" -kind service-calendar -file "$SEEDS/network/18-calendar-conn-sin-lm-v1.json"
# 台风停运是一条带来源的临时调整陈述，不随参考配置发，照旧直接登记。
"$BIN/parcel-network-register" -kind availability-adjustment -file "$SEEDS/network/19-adjustment-typhoon-v1.json"
"$BIN/parcel-network-register" -kind route-strategy -file "$SEEDS/network/20-route-strategy-cn-sg-v1.json"
# 自动改路事实是登记方折算完的陈述（0009），不是初始路由跑出来的计划。判断键对齐
# SYN-ACCOUNT-01 与 SYN-RS-CN-SG-01，不指向一笔已经受理的委托。同键再登答退出码 2，
# 已灌过的库重跑本节请用 --reset。
"$BIN/parcel-network-register" -kind auto-reroute-facts -file "$SEEDS/network/21-auto-reroute-facts-cn-sg-v1.json"
# 段计价区域（ADR-0189 决定二）：演示线路各段共引那张成本卡，卡没绑分区目录、只有 Z1，每段都登 Z1——这是演示租户的
# 租户取值，不是产品默认。没登时候选成本停在待判断。区域挂在已在册的线路版本上，所以排在线路之后。
"$BIN/parcel-network-register" -kind leg-pricing-zone -file "$SEEDS/network/22-leg-pricing-zones-cn-sg-v1.json"
# 路由策略修订 2 是采用之后的租户修订（同上海枢纽修订 2）：声明冻结形态「剩余段数」，剩余不超过一段即冻结。采用行装不下
# 这一格——冻结段数是租户取值，参考配置不带。没有它，收寄后的路由复核在计划仍适用时答冻结形态未配置（票
# product-strategy-boundary/34）。
"$BIN/parcel-network-register" -kind route-strategy -file "$SEEDS/network/23-route-strategy-cn-sg-v2.json"

echo "== 关务案件配置登记（customs-compliance：就绪/授权/规则/义务/门禁/建案/口岸/路径/凭证/协作） =="
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
# 第三申报单元先就绪再撤销：单元 01 仍有效、单元 02 只撤授权，就绪撤销要有自己的一格。
"$BIN/parcel-customs-register" readiness-register -input "$SEEDS/customs/21-readiness-cn-export-03.json"
"$BIN/parcel-customs-register" readiness-revoke -input "$SEEDS/customs/22-readiness-revoke-03.json"
# 监管凭证两版：01 写明次数额度，02 不写 uses（领域把缺席与 0 都记成来源未提供额度）。
"$BIN/parcel-customs-register" regulatory-credential -input "$SEEDS/customs/23-regulatory-credential-broker-01.json"
"$BIN/parcel-customs-register" regulatory-credential -input "$SEEDS/customs/24-regulatory-credential-quota-unstated.json"
# 税费协作两格：出口单元按核定税费形成，进口单元显式无需付款。核对命令不在这里——
# 它要已接收的资金事实版本和付款人规则，这两样本 CLI 都没有登记入口。
"$BIN/parcel-customs-register" duty-collaboration -input "$SEEDS/customs/25-duty-collaboration-assessed.json"
"$BIN/parcel-customs-register" duty-collaboration -input "$SEEDS/customs/26-duty-collaboration-not-required.json"
# 节点关务区域（ADR-0206）：演示网络四个节点，上海枢纽与深圳口岸在 CN、新加坡枢纽与末端在 SG——租户取值（PAR-CUS-08），
# 不是产品默认。准入判断据此判 SHA→SZX 同区只适用装载出发、SZX→SIN 跨区另加跨关务区域移动；不登时带跨区门的对象答未配置。
"$BIN/parcel-customs-register" node-zone -input "$SEEDS/customs/27-node-zone-sha-hub.json"
"$BIN/parcel-customs-register" node-zone -input "$SEEDS/customs/28-node-zone-szx-gate.json"
"$BIN/parcel-customs-register" node-zone -input "$SEEDS/customs/29-node-zone-sin-hub.json"
"$BIN/parcel-customs-register" node-zone -input "$SEEDS/customs/30-node-zone-sin-lm.json"

echo "== 追踪与异常目录登记（visibility-exception：目录册 + 材料收讫） =="
"$BIN/parcel-ve-register" milestone-mapping -input "$SEEDS/visibility/01-milestone-mapping-v1.json"
"$BIN/parcel-ve-register" triage-rules -input "$SEEDS/visibility/02-triage-rules-v1.json"
"$BIN/parcel-ve-register" notification-policy -input "$SEEDS/visibility/03-notification-policy-disclose-v1.json"
"$BIN/parcel-ve-register" claim-eligibility -input "$SEEDS/visibility/04-claim-eligibility-contract-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/05-claim-authorization-account-01.json"
"$BIN/parcel-ve-register" claim-authorization -input "$SEEDS/visibility/06-claim-authorization-account-02-empty.json"
"$BIN/parcel-ve-register" disclosure-policy -input "$SEEDS/visibility/07-disclosure-policy-v1.json"
# 异常披露规则与分诊用同一对信号（CUSTOMS_HOLD / ETA_GAP）：账户 01 可披露但不自动发布，
# 账户 02 的 ETA 缺口明确不披露。冲突信号规则一租户一条，钉在 CUSTOMS_HOLD 上。
"$BIN/parcel-ve-register" exception-disclosure-rules -input "$SEEDS/visibility/08-exception-disclosure-rules-v1.json"
"$BIN/parcel-ve-register" conflict-signal-rule -input "$SEEDS/visibility/09-conflict-signal-rule-v1.json"
# 同一索赔批次收两样材料：照片留下，发票收讫后撤销。管理台今天没有这一册的读页。
"$BIN/parcel-ve-register" claim-material-receipt -input "$SEEDS/visibility/10-claim-material-receipt-photo.json"
"$BIN/parcel-ve-register" claim-material-receipt -input "$SEEDS/visibility/11-claim-material-receipt-invoice.json"
"$BIN/parcel-ve-register" claim-material-receipt-revocation -input "$SEEDS/visibility/12-claim-material-receipt-invoice-revocation.json"

echo "== 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="
# 一条 COD 指令走全程（票 admin-remainder-mechanism-batch/04）：渠道报收全额入账
# `渠道在途`；银行实际到账少于渠道报收，到账部分凭该到账事实搬进`待清分`，差额先登
# 短款差异事项、再凭它落进`短款`——差额不自动落账；到账部分凭 ALLOCATION（指向代收
# 指令）清分进`应付客户`，进应付客户不得凭代收事实（第四条依据门）。SGD 分户账只
# 开立不记账，让「已开立但当期无记账」与「未开立」分得开。CNY 账上两笔回汇批次：
# 一笔停在已归集，一笔交出汇付主张（主张不是付款，账上位置不变）。SGD 账仍无批次，
# 分户账页那一格继续显「未配置」。批次是演示租户的合成实例，与本段其余 SYN- 行同一类。
#
# 已灌过的库上重放本节会在记账处以 UNDERFUNDED（退出码 4）中止：写口的重放判定排在
# 余额守卫之后，来源位置被原记账清空后守卫先答余额不足，轮不到「已在册」（实测于
# d340014 对演示库重放）。非干净库复灌一律走 --reset，别指望本节重放幂等。
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
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/11-batch-cny-open.json"
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/12-batch-cny-handed.json"
"$BIN/parcel-collection-register" hand-over -input "$SEEDS/collection/13-hand-over-cny-handed.json"

echo "== 试点治理登记（pilot-governance：权威区间、暂停、恢复、阶段评审；票 admin-skeleton-closure-batch/02） =="
# 治理是产品级机制，登记无租户维（ADR-0083）；登记走受控 CLI（票 syn-wall-door-audit/12）。
# 首批三类之外，第二批的阶段评审已开（票 demo-intake-admission-paused/01），接管仍未开；
# 阶段评审与接管都还没有读面，stage-admission 页那两格照旧如实说明未开，不造数。
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
# 是查不到这一行，答出来的`权威未确定`与「压根没登记」一模一样。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/07-authority-interval-shipment-intake.json"
# 委托受理的范围版本与上面两笔暂停的范围不同，但不同不等于互不影响：暂停按登记的覆盖关系读，
# 读不出就保守拦（domain.AdmissionSuspendedByUnreadableScopeRelation）。这一笔阶段评审随 Go 为
# shipment-intake@v1 与 routing@v3、pricing@v1 各登一条「互不相干」，准入控制因此为 OPEN；计价
# 范围那笔暂停对它自己的范围照旧立着，治理页上仍是「暂停中」。权威区间由上一行登记，本评审不再
# 授予第二条——同一区间再授一次会被冲突预检拦下。
"$BIN/parcel-governance-register" stage-review -input "$SEEDS/governance/08-stage-review-shipment-intake.json"
# 运营决定与外部事实各口的生产权威区间（ADR-0180 决定四；票 operator-channel/19）：操作者与集成客户端铸造器铸信封前
# 读的就是这几行。能力与事实类型照认证方的词登（OPERATION_DECISION + 决定种类、EXTERNAL_FACT + 外部事实类型），
# 不带 SYN-；带 SYN- 的是对象范围与权威串。权威串与 parcel-api 的 IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY 逐字相同，
# 对不上的后果是这几口照旧答 OUTSIDE_ADMISSION_SCOPE。只登今天已换口的那几种；余下的口换口时随那一笔补。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/09-authority-interval-operation-decision-manual-review-completion.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/10-authority-interval-operation-decision-active-rejection.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/11-authority-interval-operation-decision-authorized-disposition.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/12-authority-interval-operation-decision-load-assignment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/13-authority-interval-operation-decision-participation-termination.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/14-authority-interval-external-fact-customs-external-result.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/15-authority-interval-external-fact-regulatory-credential.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/16-authority-interval-external-fact-external-funds-fact.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/17-authority-interval-operation-decision-segment-closure.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/18-authority-interval-operation-decision-dispatch-task-registration.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/19-authority-interval-operation-decision-effective-time-judgment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/20-authority-interval-operation-decision-carrier-first-effective-pickup-judgment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/21-authority-interval-operation-decision-controlled-closure.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/22-authority-interval-operation-decision-controlled-reopening.json"

echo "== 操作者册登记（access-identity：合成操作者主体与能力面授予；票 operator-channel/01、17） =="
# 合成主体绑演示租户：配置员扮运营主管，持有全部运营决定（票 operator-channel/24「裁定」）——授登记册配置写、主数据与
# 运营查阅读，运营决定按 accessidentity 的 decisionKinds 逐种各授一行（各种名目见 README 授予表）；decisionKinds 多一种，
# data/access/02 的授予、03 的准入范围对照与 data/governance/ 的权威区间各补一行。授予只开门，不替任何人形成决定；
# 决定方从哪来逐口照 ADR-0151 决定二：复核完成、主动拒绝、授权处置取信封里的提交操作者作复核人、决定人、处置人，他有
# 没有权、凭哪一份授权由编排问 party-commercial；受控关闭与重开的命令里没有提交操作者，决定方、授权角色与授权依据快照
# 都由授权答复给出。委托侧各口越过 Intake 的前提与停点见 README「操作者渠道」一节授予表后的说明。查阅员只授查阅读，
# 继续证「授予不覆盖即拒」。
# 证据层级只记 S，不登任何真实操作者。发行方是演示发行方 Dex（operator-channel/compose.yaml），主体是
# 它给 dex.yaml 里静态用户真签出的令牌里的 sub（operator-token.sh … claims 读出），不按配置推算。
# 只登演示部署信任的那只发行方签得出的主体：iss 没有任何发行方签得出的主体，在册上与能登录的主体
# 长得一模一样，却永远登不进来。册上的主体不能改指到新发行方：换发行方就按新发行方的标识另登一批，
# 授予号与依据也另起。
"$BIN/parcel-access-register" operator-register -input "$SEEDS/access/01-operators.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/02-grants.json"
# 作业事实各口（收寄、揽收、交接、移动、派送、交付）要「操作者 × 设备 × 作业范围」三样都对上（ADR-0149 决定二）：
# 配置员的作业事实登记授予按作业范围给，每个范围登一台合成设备（票 product-strategy-boundary/30）。收寄只认节点
# 范围，给在演示网络的各节点上；运输履约各口只认场外范围，给在华东场外揽收与新加坡尾程派送两个范围上。场外范围是
# 租户自起的作业区名，不是网络目录对象，只比相等。作业事实不判准入范围，所以不登对照与区间。
"$BIN/parcel-access-register" device-register -input "$SEEDS/access/04-devices.json"
# 集成客户端族（ADR-0149 决定三）：外部结果、监管凭证与外部资金事实三类口要「客户端 × 授予」对上。合成报关服务商与
# 合成银行各一台，主体是演示的集成客户端发行方（operator-channel/integration-dex.yaml）签出的 sub；授予按事实类型给
# （票 product-strategy-boundary/31）。这三类事实的准入对照随下面的准入范围对照一起登。
"$BIN/parcel-access-register" integration-client-register -input "$SEEDS/access/05-integration-clients.json"
"$BIN/parcel-access-register" integration-client-grant -input "$SEEDS/access/06-integration-client-grants.json"
# 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）：演示租户在上面那几种（能力、事实类型）上的生产写
# 落在哪个治理对象范围。对照住接入身份、与操作者授予同处，不进 data/governance/；它只换坐标，区间才答「谁在写
# 生产」。演示租户越过准入范围只有这一条普通登记的路（ADR-0150 决定一）。配置员持有哪一种运营决定，这里就要有那一种
# 的对照，少了那一行，那一口授予齐备也答 OUTSIDE_ADMISSION_SCOPE；运营决定各口不论由管理台的页面调还是直接调 API 都经
# 它走，终止参与另供 probe-operator-channel.sh 取证。
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/03-admission-scope-mappings.json"

echo "== 结算账户与受理前估价采用登记（settlement-accounting：演示租户货主的应收结算账户与它采用的估价形态；票 routing-first-cut/16、product-strategy-boundary/26） =="
# 受理前财务控制的作用域从那一次商业解析回显的结算政策派生，账户按（法人、相对方、应收、币种、结算政策）
# 在结算账户登记册上找（cmd/parcel-dispatch 的 acceptanceFinancialControl）。这一行的五维照发布批里
# `SYN-SETTLEMENT-PREPAID-01` 的正文配，差一维就查不到，控制停在 `CONTROL_SCOPE_NOT_CONFIGURED`。账户标识与
# 责任依据是演示租户的合成取值：登记口与执行器都不代拟账户，没登的租户照旧停在那一格。
"$BIN/parcel-settlement-register" settlement-account \
  -input "$SEEDS/settlement/01-settlement-account-shipper-01-receivable-cny.json"
# 账户级预收（ADR-0182 决定五）：第一张对账单之前没有可映射的对账单，收款映射到应收账户本身。
# 金额是这一件演示委托的合成事实，按售价卡粗算（1.50kg、Z1：首重 ¥55 + 两档续重 ¥36，再加约一成燃油）
# 后留出余量到 200.00 CNY；不够盖住试算合计时只调这一笔，不写进生产默认。
"$BIN/parcel-settlement-register" external-funds-fact \
  -input "$SEEDS/settlement/03-external-funds-receipt-account-01.json"
"$BIN/parcel-settlement-register" funds-mapping \
  -input "$SEEDS/settlement/04-funds-mapping-account-01.json"
# 受理前估价采用（ADR-0179 决定五）：这一户采用「逐件 SELL 试算合计」，计价范围与售价卡 SYN-PLAN-CN-SG-01 的
# 范围同值——估价按（租户、计价范围、SELL、客户费用、判断时点）解析在用售价卡（决定三），范围对不上就解不出卡。
# 采用册对账户册设外键，所以排在账户之后。没登这一行的户，控制停在金额未配置；形态与范围是演示租户的合成取值。
"$BIN/parcel-settlement-register" pre-acceptance-estimate-adoption \
  -input "$SEEDS/settlement/02-pre-acceptance-estimate-adoption-account-01.json"
# SELL 计费时点采用（ADR-0190 决定三）：这一户在「有效网络收寄已采用」那一拍发起 SELL 评价，计价范围与受理前估价
# 采用的同值，估价与随后的预估客户费用因此选同一张卡（ADR-0190 越权风险点 9）。没登这一行的户，收寄采用之后不计费。
"$BIN/parcel-settlement-register" sell-billing-moment-adoption \
  -input "$SEEDS/settlement/05-sell-billing-moment-adoption-account-01.json"
# 费用项目映射（ADR-0190 决定五）：售价卡出的基础运费与偏远附加都映到同一个合成费用项目——一份评价只映一个费用
# 项目才形成，把附加并进运费是 SYN 简化（ADR-0190 越权风险点 2）。
"$BIN/parcel-settlement-register" fee-item-mapping \
  -input "$SEEDS/settlement/06-fee-item-mapping-base-freight.json"
"$BIN/parcel-settlement-register" fee-item-mapping \
  -input "$SEEDS/settlement/07-fee-item-mapping-remote-area-surcharge.json"
# BUY 触发（ADR-0164、ADR-0208）：干线段的实际履约发生项一形成就发起 BUY 评价，费用项目记合成的干线成本项。只登这一个
# 原因——演示先走的是实际履约；失败尝试、订舱、取消不登，形成了也答未配置。主要范围不在这一行，取干线协议的商业范围。
"$BIN/parcel-settlement-register" buy-evaluation-trigger \
  -input "$SEEDS/settlement/08-buy-evaluation-trigger-actual-fulfillment.json"

echo "== 周期安排登记（transport-fulfillment：演示网络三条连接各一份按日运行的周期安排；票 demo-chain-gaps/16） =="
# 周期安排是 TF 自有登记册（ADR-0201 决定一）：方向写演示网络的连接代码与起讫节点，时区取那条连接的业务时区，
# 服务范围取演示的 SYN-SCOPE-01。出发时刻与历时是合成分钟数，不是任何线路的真实班期；准备评估按计划段窗口物化
# 候选，窗口对不上就调这里的时刻，不改匹配规则（票 demo-chain-gaps/18）。具体班次不在种子里预生成：准备评估时
# 物化，或经 schedule 命令 / 在线口按运行日期显式登记。
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/01-schedule-template-sha-szx.json"
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/02-schedule-template-szx-sin.json"
"$BIN/parcel-transport-register" schedule-template -input "$SEEDS/transport/03-schedule-template-sin-lm.json"

echo "种子灌入完成：租户 SYN-TENANT-01，各上下文全部落库。"
