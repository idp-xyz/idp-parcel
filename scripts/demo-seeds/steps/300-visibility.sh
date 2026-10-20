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
# 出发门禁阻断信号规则一租户一条（ADR-0205 决定五与文末补记）：出发时关务判阻断的那一件形成信号；分诊规则不登这一类，信号进人工复核。
"$BIN/parcel-ve-register" departure-gate-signal-rule -input "$SEEDS/visibility/13-departure-gate-signal-rule-v1.json"
# 披露策略第二版（票 demo-chain-gaps/97）：账户 01 的终局改为展示，账户 02 照抄 v1 留作对照。必须登在 v1 之后——
# 不带终点的新版接续闭合前版，v1 原样在册、区间止于 v2 生效时点；先登 v2 再登 v1 会撞区间重叠。
"$BIN/parcel-ve-register" disclosure-policy -input "$SEEDS/visibility/14-disclosure-policy-v2.json"
