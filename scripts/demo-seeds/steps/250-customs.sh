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
# 「税费付款」那一道的规则行（票 launch-wiring/06）：演示租户在 SYN-PROC-CN-EXPORT 下取「不构成前置条件」——这是演示采用的取值（证据 S），
# 不是产品默认，也不是任何真实程序的读法；真实租户的规则取值属租户上线时的实施，未登即答 DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED。
# 与上面的目录、逐项认定同一把键，放行门禁核对（486 那一节授予）据此可答出结论。
"$BIN/parcel-customs-register" duty-payment-gate-rule -input "$SEEDS/customs/32-duty-payment-gate-rule-not-a-precondition.json"
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
