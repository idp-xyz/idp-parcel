# 16 演示租户受理前财务控制两格：时点采用产品参考配置、种子登结算账户

Category: enhancement
Status: 完工，待评审与重放——2026-10-10 通道 2 收口（派单 `task-7536ef53` ← 通道 1）：分支 `mcp2-rfc16` 基 `5e6cd4b6`，代码 tip `128239dc`，完成记录见文末；三条判据 ✅，执行器零改；非作者评审另派，作者不自评；重放进 main 由通道 1 做。此前 in-progress——2026-10-10 通道 2 认领，分支 `mcp2-rfc16`、基 `5e6cd4b6`。此前 ready-for-agent——2026-10-10 22:2x 通道 1 分诊（用户授权自决）：收为「两格登上、停点前移」，越过格 5 不是本票判据，见「分诊裁定」。此前 needs-triage——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 的判据一取证：演示动线越过 psb/05 格 4 之后停在格 5
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

- [x] `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 在新种子上绿。
- [x] 灌种子后走演示委托（或带 DSN 的现成用例），停点从 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` 前移到 `CONTROL_AMOUNT_NOT_CONFIGURED`，途中不出现 `CONTROL_SCOPE_NOT_CONFIGURED`；写明怎么证的。证据层级 `S`。
- [x] 注释与种子一致，不再说演示种子的财务控制格没采用。

## 不做

- 不碰执行器；`Amounts` 的估价方法与越过格 5 不在本票。
- 不改 `AdvanceFinancialControlJudgmentHandler`。

## 完成记录（2026-10-10，通道 2，派单 `task-7536ef53` ← 通道 1；分支 `mcp2-rfc16` 基 `5e6cd4b6`；待非作者评审与重放）

**落点**

| 笔 | 内容 |
|---|---|
| `41102335` | 票面：认领 |
| `d8e72d35` | 时点一格：`publish-batch.json` 里 `SYN-RULEPKG-01` 的 `asOfPolicies` 中 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 那一项，语义换成 `REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1`，规则包 `contentDigest` 重钉为 `PCC-1:17569a7f…1136`；`SubmissionReceiptAsOf` 类型注释与 `assemble.go` 里 `acceptanceChainConsumers`、`acceptanceCommercialBasis` 两处头注改为机制说法 |
| `128239dc` | 账户一格：`data/settlement/01-settlement-account-shipper-01-receivable-cny.json`；`seed.sh` 编译 `./cmd/parcel-settlement-register`，在末尾另起一段调 `settlement-account`；README 组成表补一行、数据故事补第 7 条 |
| 本笔 | 票面：判据、完成记录、Status |

代码 tip `128239dc`。执行器零改：`git diff -U0 5e6cd4b6 128239dc -- '*.go'` 的增删行去掉注释行后为空（两份文件 6 增 6 删，全是注释）。

**对完成判据**

- ✅ **判据一**：`TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 在新种子上绿。换语义后旧摘要先红：「第 3 项 ACCEPTANCE_RULE_PACKAGE：声明 `PCC-1:a7c2be09…8198` 算出 `PCC-1:17569a7f…1136`」；按算出值重钉后绿。同包拒收用例（未发布引用、坏形状引用）照旧绿。
- ✅ **判据二**：停点从 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` 前移到 `CONTROL_AMOUNT_NOT_CONFIGURED`，途中没出现 `CONTROL_SCOPE_NOT_CONFIGURED`。证据层级 `S`。
  - 怎么量的：55432 上三只一次性库，各按该笔检出的 `seed.sh` 干净灌，三次都退 0。照[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」起 `parcel-api`（读写两个隔离开关同取 `SYN-TENANT-01`，启动日志有 ADR-0078 与 ADR-0091 两行）与 `parcel-dispatch`（七个变量取脚本里的演示值）。`submit-one-shipment.sh` 提交一笔，答 `201 SUBMITTED`；等 50 秒后读库与 dispatch 日志。时段 14:40Z–14:49Z。
  - 动线分不出作用域停与金额停：两者在适配器里各有原因码，到 `AdvanceFinancialControlJudgmentHandler` 都折成 `FINANCIAL_CONTROL_NOT_FORMED`（分诊裁定第 4 条另记的那一格）。所以每只库再跑一次探针：在 `cmd/parcel-dispatch` 包内临时放一个用例文件，照生产装配调 `acceptanceCommercialBasis` 与 `acceptanceFinancialControl`，对库里那笔委托的当前提交版本直接调 `ApplyPreAcceptanceFinancialControl`，读回原因码，整笔在事务里回滚。探针未入库，用完已删。

  | 种子 | 检出 | 一次性库 | 委托 | 动线停点 | 探针 |
  |---|---|---|---|---|---|
  | 原样 | `41102335` | `idp_mcp2_rfc16_base` | `SHR-76N47FUCGV5BPBRWGPNE6GSKPM` | `acceptance_processing_attempt` 一行：`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`；可达性判断已落一行 | `NOT_FORMED` / `CONTROL_SCOPE_NOT_CONFIGURED` |
  | 只改时点 | `d8e72d35` | `idp_mcp2_rfc16_asof` | `SHR-S55SYVRNDAWYFUWAGJSQ4UZEJE` | 三次投递都是 `dispatch.consumer_undecided`：`stage FINANCIAL_CONTROL_JUDGMENT, reason FINANCIAL_CONTROL_NOT_FORMED`；未决整笔回滚，任务行与可达性判断都是零行 | `NOT_FORMED` / `CONTROL_SCOPE_NOT_CONFIGURED` |
  | 两格都登 | `128239dc` | `idp_mcp2_rfc16_final` | `SHR-V2FP3BLSK4S7XULHOGQX7K5JRE` | 同上一行；`settlement_accounting.settlement_account` 有 `SYN-SETTLEMENT-ACCOUNT-01` 一行，种子那一步答「已登记」 | `NOT_FORMED` / `CONTROL_AMOUNT_NOT_CONFIGURED` |

  - 第一行顺带把 psb/05「按代码判」的那一句量实了：旧种子越过时点之后确实停在作用域。中间一行说明两格各管一格：只改时点，链越过时点、停在作用域；再登账户，作用域形成、停在金额。
- ✅ **判据三**：`git grep -n '财务控制格没采用' 128239dc -- ':!.scratch'` 零命中。`SYN-ASOF-ACCEPT-TIME` 在 `.scratch` 之外只剩 `submission_receipt_as_of_test.go` 里当「非产品语义」样例的那一处，它用替身、不读种子，没改。

**判断项**

1. **注释改成机制说法，不写新的种子现状。** 三处原来写的是种子事实，种子一改即无声失真（psb/06 评审的非阻断项点过这一条）。换成「演示种子两格都采用了」会再埋一句同样的话，所以改成「没采用的格答`未配置`」，执行器注释里不再写种子。
2. **账户标识与责任依据。** 账户标识取 `SYN-SETTLEMENT-ACCOUNT-01`。责任依据取 `SYN-CONTRACT-01`：`ResponsibilityBasis` 的类型注释是「这个账户得以成立的合同或责任依据」，种子里结算政策 `SYN-SETTLEMENT-PREPAID-01` 引的正是这份客户合同。五维照种子核过：法人与相对方取结算政策正文，币种与解析键一致；结算政策取对象标识，因为 `RegisteredAccountDirectory` 按 `terms.Policy().Object()` 查。`payerId` 不给，即付款责任方就是相对方。
3. **`seed.sh` 新段放在末尾，不重编前七步的序号**，照治理段与操作者段的先例。头注「六个登记 CLI」与结尾「七上下文」加上这一段就更不对了，改成不计数的「各登记 CLI」「各上下文」。
4. **判据二用动线加探针取证，探针不入库。** 理由见判据二第二条。没把探针做成入库用例，因为本票地盘不含 `cmd/parcel-dispatch` 的用例文件。
5. **没有已有用例钉着旧停点。** `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` 与两个控制原因码在用例里的命中都用替身，不读演示种子；读演示种子的用例包都在下面那一跑里，全绿。

**验证**（钉 `128239dc`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp2-rfc16`，go 经 `/usr/local/go/bin/go`，带 `HTTPS_PROXY`）

- `go build ./...` 退 0；`go vet ./...` 退 0；`gofmt -l` 对两个改动包无输出；`git status` 干净。
- `go test -count=1 -p 1`，带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，退 0，零 `--- FAIL`。包：两个改动包；改动适配器的生产反向依赖（`go list` 反查得 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/postgres`）；读演示种子的用例包（`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`cmd/parcel-pricing-register`、`internal/parcelpricing/adapters/pricecardtemplate`）；种子新调用的 `cmd/parcel-settlement-register`；`./internal/architecture/...`。
- 同一环境 `-v` 单跑 `TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring`：`--- PASS`，不是 `SKIP`。
- 没动端点与迁移，没重生成机制清点。没跑全仓，留给推送方重放后那一跑。

**未做 / 边界**

- 越过格 5：`Amounts` 的估价方法在 [psb/06](../../product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) 第 2 项，不在本票。
- 两处带日期的文档补记写的仍是本票之前的种子状态，不在本票地盘，没改：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) 2026-09-30 补记（票 psb/06 第 1 项）末句「演示种子的可达性格采用了这一形态，财务控制格没有」；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「墙三」下 2026-10-10 那段补记（票 rfc/11）写的停点 `FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED`。
- 没碰共享树与 `main`。
