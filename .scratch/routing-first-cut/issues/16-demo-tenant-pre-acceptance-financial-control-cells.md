# 16 演示租户受理前财务控制两格：时点采用产品参考配置、种子登结算账户

Category: enhancement
Status: ready-for-agent——2026-10-10 22:2x 通道 1 分诊（用户授权自决）：收为「两格登上、停点前移」，越过格 5 不是本票判据，见「分诊裁定」。此前 needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证：演示动线越过 psb/05 格 4 之后停在格 5
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它挡在 11 判据一的路上。
地盘：`scripts/demo-seeds`（演示租户的采用行与账户登记）；另含随种子失真的注释——`cmd/parcel-dispatch/assemble.go` 与 `internal/parcelshipment/adapters/partycommercial/submission_receipt_as_of.go` 里说「演示种子的财务控制格没采用」的那几句（按这句引文搜得到）。执行器不动（分诊已核）。
出处：[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「格 5 · 受理前财务控制」两次取证，都写「没找到点名这一格的票」；AGENTS.md「演示租户就是 SYN-TENANT-01，代码按真实租户对待它」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)。

## 现象（通道 2 实测，代码钉 rfc/11 的 `1bbd0f09`，种子即 `6857e599`）

- 演示委托的可达性答 REACHABLE、越过格 4 之后，停在格 5：`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`。委托不会被接受，初始路由也就不会被触发。
- 时点一半：种子规则包里 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 那一格的语义是合成串 `SYN-ASOF-ACCEPT-TIME`，没采用产品参考配置；`SubmissionReceiptAsOf.FormAsOfValue` 只认 `submission-receipt@1` 那一版引用。
- 账户一半：账户目录登记册已有，种子没登演示租户的结算账户；越过时点之后会答 `CONTROL_SCOPE_NOT_CONFIGURED`（psb/05 按代码判）。
- `Amounts` 的估价方法在 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项，needs-triage。

## 待分诊

- 两格都是演示租户的租户取值（采用哪一种时点形态、登哪个账户）：按 ADR-0146 经参考配置采用、用 `SYN-` 数据灌。先核两格是否都只需改种子、不碰执行器。
- `Amounts` 留空时财务控制答什么：若它挡在两格之后，本票 Blocked by psb/06 第 2 项；不挡就不挂。
- 走通之后的下一停点：受理决定形成、初始路由触发后停在成本来源，见 [17](17-initial-route-pricing-input-from-customer-declaration.md)。

## 分诊裁定（通道 1 · 2026-10-10 22:2x · 钉 main `1253c768`）

取证：通道 5 只读取证（`task-2a6b94e0`，读码与 `git grep`，未跑用例）。

- 两格都只动种子，执行器零改。时点：`SubmissionReceiptAsOf.FormAsOfValue` 只比 `Declared.Semantics()` 与 `SubmissionReceiptCitation()`，不看判断类别；`scripts/demo-seeds/data/commercial/publish-batch.json` 里 `SYN-RULEPKG-01` 的 `declarations.asOfPolicies`，可达性那一项已是 `REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1`，财务控制那一项是 `SYN-ASOF-ACCEPT-TIME`。账户：`acceptanceFinancialControl` → `PolicyBackedControlScopeSource` → `RegisteredAccountDirectory.FindSettlementAccount`，键为（法人、相对方、RECEIVABLE、币种、结算政策）；登记口 `parcel-settlement-register settlement-account` 已在，`seed.sh` 既不编译也不调用它。
- `Amounts` 留空时：`ApplyPreAcceptanceFinancialControl` 先作用域后金额，`Amounts` 为 nil 即答 `CONTROL_AMOUNT_NOT_CONFIGURED`，发生在问 SA 要不要控制之前——挡在两格之后。
- 所以两格登上之后的下一停点是 `CONTROL_AMOUNT_NOT_CONFIGURED`，不是「待分诊」末条写的 17 的成本来源：受理决定形不成，初始路由不触发。

裁定：

1. 本票收为「两格登上、停点前移」，转 ready-for-agent，不挂 Blocked by。越过格 5 要等 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项（`Amounts` 的估价方法）定下来——那是 psb/06 的事，不是本票的判据；其估价基里的区域与 [18](18-leg-endpoints-folded-into-price-card-zones.md) 同题，宜一起定。
2. 两格都是演示租户的租户取值（[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 三类之三）：时点经参考配置采用产品内置的 `submission-receipt@1`，账户用 `SYN-` 数据灌；证据记 `S`。
3. 种子一改，「地盘」里列的注释即失真，随种子在本票改，不另立票。
4. 另记，不在本票：`AdvanceFinancialControlJudgmentHandler.Handle` 不把 `assessment.Reason` 带进续办，作用域、金额两停在任务上都记 `FINANCIAL_CONTROL_NOT_FORMED`，演示动线上看不出停在哪一格。要不要立票，留到下一次走动线时定。

## 做什么

1. `publish-batch.json` 里 `SYN-RULEPKG-01` 的 `asOfPolicies` 中判断类别为 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 的那一项，`semantics` 换成 `REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1`，重算该项 `contentDigest`。
2. 种子登演示租户的结算账户：`tenantId` `SYN-TENANT-01`、`legalEntityId` `SYN-LE-01`、`counterpartyId` `SYN-PARTY-SHIPPER-01`、`direction` `RECEIVABLE`、`currency` `CNY`、`settlementPolicyId` `SYN-SETTLEMENT-PREPAID-01`，`accountId` 与 `responsibilityBasis` 取 `SYN-` 值（取值照种子里已登的法人、货主与结算政策，开工时核）；`seed.sh` 编译并调用 `parcel-settlement-register settlement-account`。
3. 「地盘」里列的注释改成现行说法。

## 完成判据

- [ ] `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 在新种子上绿。
- [ ] 灌种子后走演示委托（或带 DSN 的现成用例），停点从 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` 前移到 `CONTROL_AMOUNT_NOT_CONFIGURED`，途中不出现 `CONTROL_SCOPE_NOT_CONFIGURED`；写明怎么证的。证据层级 `S`。
- [ ] 注释与种子一致，不再说演示种子的财务控制格没采用。

## 不做

- 不碰执行器；`Amounts` 的估价方法与越过格 5 不在本票。
- 不改 `AdvanceFinancialControlJudgmentHandler`。
