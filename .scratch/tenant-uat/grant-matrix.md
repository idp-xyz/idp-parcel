# 验收用的授予对照

这不是一套新的角色。产品只按人授能力面（ADR-0100、ADR-0363）：能登册的人和能给人授权的人是两格，不合成一个角色。

表里的「有 / 无」取自 `scripts/demo-seeds/data/access` 里已经写给演示租户的授予，证据记 `S`。验收照这张表登，不在这里新发明财务该拿哪些决定。

三列人：

| 列 | 是谁 | 验收库现在
|---|---|---|
| 演示主管 | 发行方里的 `syn-operator-config-01`，种子姓名「演示主管」 | 已在册。姓名是「演示主管」。已有操作者接入管理、主数据与运营查阅读、登记册配置写。运营决定和作业事实还没授 |
| 演示查阅员 | 发行方里的 `syn-operator-viewer-01`，种子姓名「演示查阅员」 | 还没进操作者册 |
| 财务 | 发行方里的 `syn-operator-finance-01` | 种子没有这个人的操作者，也没有授予。这一列保持未配置，不补默认 |

## 五格能力面

| 能力面 | 做什么 | 演示主管 | 演示查阅员 | 财务 |
|---|---|---|---|---|
| 登记册配置写 | 登公司、客户、价格、规则这些册，以及发布 | 有 | 无 | 未配置 |
| 主数据与运营查阅读 | 看各册和运单；没有这一格，自查授予的口也进不去 | 有 | 有 | 未配置 |
| 运营决定 | 按决定种类再分，下面单独一表 | 有 | 无 | 未配置 |
| 作业事实登记 | 按作业范围再分，下面单独一表 | 有 | 无 | 未配置 |
| 操作者接入管理 | 登记操作者、授与撤；首个管理员只有这一格 | 有 | 无 | 未配置 |

## 作业事实的范围

只挂在演示主管的「作业事实登记」上。查阅员没有这一面。

| 范围 | 演示主管 |
|---|---|
| NODE SYN-NODE-SHA-HUB | 有 |
| NODE SYN-NODE-SIN-HUB | 有 |
| NODE SYN-NODE-SIN-LM | 有 |
| NODE SYN-NODE-SZX-GATE | 有 |
| OFFSITE SYN-OFFSITE-CN-EAST | 有 |
| OFFSITE SYN-OFFSITE-SG-LM | 有 |
| WAREHOUSE SYN-WH-01 | 有 |

## 运营决定

演示主管持有 71 种，查阅员一种都没有。种类名是产品词表里的原词，管理台上「授予…」选「运营决定」之后按这个选。

| 决定种类 | 演示主管 | 演示查阅员 |
|---|---|---|
| ACTIVE_REJECTION | 有 | 无 |
| ALTERNATE_JOURNEY_START | 有 | 无 |
| AUTHORIZED_DISPOSITION | 有 | 无 |
| AUTHORIZED_REROUTE | 有 | 无 |
| BOOKING_REQUEST_CANCELLATION | 有 | 无 |
| BOOKING_REQUEST_SUBMISSION | 有 | 无 |
| CAPACITY_RELEASE | 有 | 无 |
| CAPACITY_RESERVATION | 有 | 无 |
| CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT | 有 | 无 |
| CHARGE_ADJUSTMENT_REGISTRATION | 有 | 无 |
| CHARGE_CONFIRMATION | 有 | 无 |
| CONDITION_RESTORE | 有 | 无 |
| CONTROLLED_CLOSURE | 有 | 无 |
| CONTROLLED_REOPENING | 有 | 无 |
| CONTROL_RELEASE_REFUSAL_HANDLING | 有 | 无 |
| CUSTOMS_CASE_CLOSURE | 有 | 无 |
| CUSTOMS_CASE_ESTABLISHMENT | 有 | 无 |
| CUSTOMS_DECLARATION_SUBMISSION | 有 | 无 |
| CUSTOMS_SUBMISSION_AUTHORIZATION | 有 | 无 |
| DELIVERY_ATTEMPT_RESULT_CONFIRMATION | 有 | 无 |
| DELIVERY_DISPATCH_REASK | 有 | 无 |
| DELIVERY_HANDOVER_CONFIRMATION | 有 | 无 |
| DISPATCH_TASK_REGISTRATION | 有 | 无 |
| EFFECTIVE_TIME_JUDGMENT | 有 | 无 |
| EXTERNAL_FUNDS_FACT_ADOPTION | 有 | 无 |
| FULFILLMENT_SEGMENT_COMMISSION_ADOPTION | 有 | 无 |
| FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL | 有 | 无 |
| GOODS_RECORD_ON_BEHALF | 有 | 无 |
| IDENTITY_CONFLICT_DISPOSITION | 有 | 无 |
| INBOUND_PRE_ADVICE_CLOSE | 有 | 无 |
| INBOUND_PRE_ADVICE_RECORD_ON_BEHALF | 有 | 无 |
| INTERNAL_RESTRICTION_DECISION | 有 | 无 |
| INTERNAL_RESTRICTION_ESTABLISHMENT | 有 | 无 |
| LOAD_ASSIGNMENT | 有 | 无 |
| MANUAL_MEASUREMENT_ENTRY | 有 | 无 |
| MANUAL_REVIEW_COMPLETION | 有 | 无 |
| NODE_COLLABORATION_ACCEPTANCE | 有 | 无 |
| OUTBOUND_CONFIRMATION_CORRECTION | 有 | 无 |
| OUTBOUND_REQUEST_RECORD_ON_BEHALF | 有 | 无 |
| PARTICIPATION_TERMINATION | 有 | 无 |
| PENDING_IDENTITY_RECHECK | 有 | 无 |
| PICKUP_ATTEMPT_RESULT_CONFIRMATION | 有 | 无 |
| POST_INTAKE_MANUAL_ITEM_HANDLING | 有 | 无 |
| POST_INTAKE_SERVICE_DISPOSITION | 有 | 无 |
| PROCESSING_INSTRUCTION | 有 | 无 |
| RECEIPT_DISCREPANCY_RECORD_ON_BEHALF | 有 | 无 |
| REGULATORY_CREDENTIAL_REGISTRATION | 有 | 无 |
| REGULATORY_DISPOSITION_ACCEPTANCE | 有 | 无 |
| RELABEL_INSTRUCTION | 有 | 无 |
| RELEASE_GATE_VERIFICATION | 有 | 无 |
| RETURN_RECORD_ON_BEHALF | 有 | 无 |
| SEGMENT_CLOSURE | 有 | 无 |
| STATEMENT_DISPUTE_REGISTRATION | 有 | 无 |
| STATEMENT_DISPUTE_RESOLUTION | 有 | 无 |
| STATEMENT_PUBLICATION | 有 | 无 |
| STOCK_ADJUSTMENT_APPROVAL | 有 | 无 |
| STOCK_ADJUSTMENT_SUBMISSION | 有 | 无 |
| STOCK_COUNT | 有 | 无 |
| STOCK_FREEZE | 有 | 无 |
| STOCK_IMPORT_SUBMISSION | 有 | 无 |
| SUPPLIER_BILL_AUDIT | 有 | 无 |
| SUPPLIER_BILL_DISPUTE_DECISION | 有 | 无 |
| SUPPLIER_BILL_RECEPTION | 有 | 无 |
| TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION | 有 | 无 |
| TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION | 有 | 无 |
| TRANSPORT_COMMISSION_AGREEMENT_REVISION | 有 | 无 |
| TRANSPORT_COMMISSION_CANCELLATION | 有 | 无 |
| TRANSPORT_COMMISSION_SUBMISSION | 有 | 无 |
| UNCLAIMED_ARRIVAL_CLAIM | 有 | 无 |
| UNCLAIMED_ARRIVAL_DISPOSITION | 有 | 无 |
| WAVE_RELEASE | 有 | 无 |

## 第 2 站照这张表还没做的

- 演示主管还差：运营决定（上表全部）、作业事实登记（上表七个范围）。登记册配置写已在页上用「按清单授予」登过一笔。
- 演示查阅员还没进册，进册后只授主数据与运营查阅读。
- 财务不在这张表里补授。
