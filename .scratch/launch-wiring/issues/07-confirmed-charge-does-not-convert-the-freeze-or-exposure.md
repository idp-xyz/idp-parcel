# 07 费用确认后冻结与信用暴露不转正式扣费、不释放

Category: bug
Status: in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-occ（代码最后一笔 8158554a8）。此前：in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领：psb/05 收口重走停点三，协调方按冻结清单「上线线」口径上调为挡上线；分支 claude-lw-occ
Blocked by: 无

## 现象

psb/05「收口重走（钉 `d78fd1fbf`，2026-10-28）」：演示账户每件委托接受前占用 9600；第一件终局、客户费用自动 `CONFIRMED`、对账单 `STATEMENT_PUBLISHED` 之后，占用（`EXP-0001`）仍在，第三件委托答 `AVAILABLE_BALANCE_INSUFFICIENT`。代码里费用确认（psb/22，ADR-0336）与对账单发布（ADR-0339）都不读写接受前控制的冻结或信用暴露；`release_pre_acceptance_control.go` 只服务接受失败、拒绝、撤回一类释放。

## 为什么挡

SA CONTEXT：「冻结后必须转为正式扣费或显式释放」「费用确认后将适用冻结转为正式扣费；取消或失败时显式释放」。今天占用只增不减：预付客户的可用余额只靠冻结额充当扣减、估价与确认额的差永远不对账；授信客户付清账单（UC-SA-005 核销）也不放暴露，额度在「额度 ÷ 每件估价」件后耗尽，新委托全拒，没有任何生产绕法。

## 做什么

先读 SA CONTEXT（冻结、信用暴露、运营余额、已确认未结各词条与不变量）、UC-SA-002、UC-SA-005、ADR-0047、ADR-0127、ADR-0179、ADR-0181、ADR-0336、ADR-0339，以及 PS CONTEXT「控制项结果」。按现行规则实现：

1. 预付：费用确认时，按原业务关联把该委托的适用冻结转为正式扣费——冻结收口、入账余额（或其登记）按确认额扣减，估价与确认额的差按 CONTEXT 处理；同一确认重放不重复扣、不重复释放。
2. 授信：费用确认时把信用暴露转入已确认未结（或按 CONTEXT 写明的口径收口），核销时减已确认未结，额度随之恢复；同样幂等。
3. 一份委托多件、部分件取消或失败的情形照 CONTEXT「按原业务关联显式释放」处理，写明本票覆盖到哪。
4. CONTEXT 没写死、要裁的格（例如差额、转换时点是确认还是对账单发布）写成越权风险点；难逆转的取舍立 ADR（会话代 SA owner 接受、待复核）。

## 完成判据

- 真库进程或编排用例：演示租户预付与授信各一条——连续 N+1 件（N 为额度 ÷ 估价）在费用确认（与授信下核销）之后不再被拒；占用、扣费、已确认未结的读数与账一致；重放不重复。
- 释放未受理册、撤回释放等既有路径不回归。

## 完成记录（2026-10-28 作者）

起点 `489db2936`；分支 `claude-lw-occ`。

**改了什么**
- 设计与代决：[ADR-0343](../../../docs/adr/0343-a-confirmed-charge-converts-its-parcels-pre-acceptance-occupation-in-the-confirming-transaction.md)（会话代 SA owner 接受、待复核）。SA CONTEXT 补转出与授信额度占用的现行规则一条；UC-SA-002「给开发的交接」记包裹取消/失败份额无触发口一行。
- 迁移 `0051_pre_acceptance_control_conversion.sql`：冻结/暴露行加 `converted_minor`、`converted_at`，状态加 `4=CONVERTED`，新表转出册。
- 领域：`FreezeLedger.Convert`、`CreditExposureLedger.Convert`、`FreezeConverted`/`ExposureConverted`、`RemainingMinor`；`HeldMinor`/`ExposedMinor` 求「原金额 − 已转出」；重建门核转出两格。
- 应用：`ConvertOccupationOnChargeConfirmationHandler`（锁控制作用域 → 按原业务关联认领 → 整份转出并记转出册 → `recomputeCustomerScope` 重算入账余额与已确认未结）；`ConfirmChargeHandler` 在定格（含重放、并发二确落败）后同事务调它，失败错误交回、事务回滚。`recomputeScope` 抽成 `recomputeCustomerScope` 供映射与确认共用（ADR-0181 决定三「函数只有一个」）。释放编排对已转满的答`无可释放`。
- 适配器：`PreAcceptanceConversions`（费用 → 包裹 → 估价留痕份额；转出册）；两本占用账读写 `converted_*`；`LoadCreditStanding` 的已占用额度 = 暴露占用 + 已确认未结。PS 一侧适配器把 `CONVERTED` 译为成立。parcel-api 与 parcel-dispatch 两处装配。

**代决与越权风险点**（ADR-0343 同号）：1 整份一次转出而非逐笔按额；2 授信把已确认未结算进额度占用；3 首次确认建立运营余额行，使从未登记的作用域从`余额不可用`变成可判；4 包裹取消/失败份额无触发口。另：入账余额一列确认时不动（ADR-0181 决定二），与本票「做什么」第 1 条「入账余额按确认额扣减」的字面不同——「扣费」在 CONTEXT 算式里就是进入已确认未结，已在 ADR 决定四写明。

**判据对用例**（`internal/settlementaccounting/adapters/postgres/pre_acceptance_conversion_test.go`，真库）
- 预付：`TestAConfirmedPrepaidChargeConvertsItsParcelsFreezeIntoConfirmedUnsettled`——两件各冻 9600、第三件被限；第一件确认（实际 9000）后该冻结 CONVERTED、未结 9000、可用 = 入账 − 剩余冻结 − 未结；重放不二转、转出册一行；释放已转满的答 NOTHING_TO_RELEASE、没转的照放，放后再来一件成立。
- 授信：`TestAConfirmedCreditChargeMovesTheExposureIntoUnsettledAndSettlementRestoresTheLimit`——额度 20000 ÷ 估价 9600 = 2 件，第三件被限；两件确认后暴露 CONVERTED、额度占用仍在（钱真用了，仍限）；经普通路径采用收款、映射、核销 9600 之后第三件成立。
- 其余：无占用可转的费用照常重算；转出册写口无事务被拒（PBC-08）；领域 Convert/Release/重建门用例；确认编排的失败回滚与重放用例。
- **与票面判据的差**：票面写预付「连续 N+1 件在确认之后不再被拒」。预付的钱确实被确认费用用掉，确认并不凭空恢复额度：确认后可用 = 入账 − 剩余冻结 − 实际确认额，第 N+1 件在钱不够时仍被限（直到再收款或估价高于实际时差额回升）。能恢复额度的是授信下的核销，已按此测。

**自验**：`go build ./...`、`go vet ./internal/... ./cmd/...`、`gofmt -l` 无输出；带 DSN（`postgres://parcel:parcel@127.0.0.1:55432/postgres`）`go test -count=1 -p 1` 过 `./internal/settlementaccounting/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-api`、`./internal/parcelshipment/...`、`./internal/architecture/...`、`./migrations/...`（architecture 的 PBC-08 门首跑红一条，补转出册写口的无事务负向证据后绿）。未跑演示种子灌库（本票不动种子）。

**留给协调方**
- 确认之后的调整（计价纠错、商业让利）改当前有效客户应收，ADR-0181 决定三点名它也该调重算；本票没挂，已确认未结在下一次映射、核销或确认之前停在旧值。不挡上线（核销与确认会重算），建议记冻结清单 R2。
- 包裹取消/失败时释放它份额的触发口（ADR-0343 决定八）：该份额一直占着；量小，不挡首发，建议记冻结清单 R2/R3。
- 演示动线文档里「第三件被拒」的停点三可改写为新行为（预付确认后可用 = 入账 − 剩余冻结 − 确认额）。
- `docs/adr/README.md` 索引与机制清点是生成物，未手改，进 main 时重生成。
