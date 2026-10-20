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
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/23-authority-interval-operation-decision-pending-identity-recheck.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/27-authority-interval-operation-decision-identity-conflict-disposition.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/24-authority-interval-operation-decision-transport-commission-submission.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/25-authority-interval-operation-decision-transport-charge-occurrence-registration.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/28-authority-interval-operation-decision-capacity-reservation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/29-authority-interval-operation-decision-capacity-release.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/30-authority-interval-operation-decision-booking-request-submission.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/31-authority-interval-external-fact-carrier-booking-answer.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/32-authority-interval-operation-decision-segment-commission-adoption.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/33-authority-interval-operation-decision-transport-commission-cancellation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/34-authority-interval-operation-decision-booking-request-cancellation.json"
# 送站预报两口（ADR-0212 决定二；票 ps-external-mark-relations/09）是首方客户族的生产写，准入范围在铸造侧判（ADR-0139 决定八）：
# 能力 CUSTOMER_COMMAND，事实类型取客户面能力词，对象范围另起一个客户面的合成范围。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/35-authority-interval-customer-command-delivery-pre-advice-submission.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/36-authority-interval-customer-command-delivery-pre-advice-withdrawal.json"
# 送站交接单（ADR-0212 决定三；票 ps-external-mark-relations/12）：运营决定的送站交接确认、客户面的送站交接确认，与作业事实登记面的
# 开立、归入、关闭三种——作业事实登记面的能力词是 WORK_FACT_REGISTRATION，事实类型逐种取，对象范围另起一个作业事实的合成范围。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/37-authority-interval-operation-decision-delivery-handover-confirmation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/38-authority-interval-customer-command-delivery-handover-confirmation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/39-authority-interval-work-fact-delivery-handover-note-opening.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/40-authority-interval-work-fact-delivery-handover-note-assignment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/41-authority-interval-work-fact-delivery-handover-note-closure.json"
