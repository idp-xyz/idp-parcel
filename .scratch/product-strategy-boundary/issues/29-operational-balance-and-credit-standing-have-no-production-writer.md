# 29 运营余额与信用状况没有生产写入方：演示委托的预付冻结停在`余额不可用`

Category: enhancement
Status: needs-info——指向 [ADR-0181](../../../docs/adr/0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md)（Proposed，接受归用户）。2026-10-14 17:48 通道 2 分诊认领（派单本卡 ← 通道 1），分支 mcp2-psb29t，基 0715ace33a942261c13821405962c6a0945ab660。此前：needs-triage——2026-10-13 22:1x 通道 1 立，出自 [26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md) 判据 1：通道 5 开工取证报「冻结要读运营余额、而它没有生产写入方」，通道 1 在 main `b3c8bd46` 上核实属实。是机制缺口，不是租户取值；要裁的是写入方的形状，见「要裁的问题」，裁完照「做什么」拆笔转 ready-for-agent。
Blocked by: 无——要裁的是本票自己的形状
挡着：26 判据 1 的冻结那一半（26 照实标 ◑，不扩地盘）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)墙三之后的预付冻结一格，以及账期委托的信用暴露一格。
地盘（裁完按形状收窄）：`internal/settlementaccounting`——余额与信用状况的写端口、UC-SA-005 映射编排（`MapExternalFundsHandler`）与问题二、三裁到的那几处编排、postgres 适配器 `operational_position.go`；它们的生产装配（`cmd/parcel-api` 的 `buildSettlementRegistrationOrchestration`、`cmd/parcel-settlement-register`、`cmd/parcel-dispatch`）；演示种子 `scripts/demo-seeds/data/settlement/` 与 `seed.sh` 结算段。
出处：[SA `CONTEXT`](../../../docs/domain/settlement-accounting/CONTEXT.md)「运营余额、收付款与坏账」；[UC-SA-005](../../../docs/application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md) 主流程「形成运营余额、账龄和信用恢复/限制依据」那一步与结果「运营余额已更新」；`migrations/settlement_accounting/0010_operational_position.sql` 头注；[ADR-0047](../../../docs/adr/0047-terms-control-forms-credit-exposure-not-a-freeze.md)；[ADR-0127](../../../docs/adr/0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md)；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)；[bento-gate-reeval/01](../../bento-gate-reeval/issues/01-reeval-verdict-bento-gate-stays-blocked.md) 的 settlementaccounting 特名方法清单。

## 现状（钉 main `b3c8bd46`，只作此刻取证）

- **读口齐、写口悬空。** `ports.OperationalBalanceView.LoadBalance` 与 `ports.CreditStandingView.LoadCreditStanding` 由 `cmd/parcel-dispatch` 装配（`sapostgres.NewOperationalBalances`、`NewCreditStandings`），`ApplyPreAcceptanceControlHandler` 的 `freezeFunds` 与 `exposeCredit` 各读其一。写只有适配器方法 `OperationalBalances.RecordBalance`（`BalancePosting`：入账余额、授信额度、已确认未结、截至时点）与 `CreditStandings.RecordStanding`（`CreditPosition`：额度、逾期、截至时点），同键只准往前；`ports` 里没有对应的写端口，非测试代码里没有调用者（`git grep` 只命中适配器自身与测试）。
- **无行即`余额不可用`。** 没登记过的作用域，`LoadBalance` 答 `ErrBalanceNotRegistered`，`freezeFunds` 折成 `BalanceUnavailable` 并保持未决；`exposeCredit` 在读到授信依据之后读信用状况，无行同样折成 `CreditStandingUnavailable`。所以任何租户的预付冻结与信用暴露今天都停在这两格，不只演示租户。
- **来源写在注释里、没接上。** `BalancePosting` 头注：入账余额来自 UC-SA-005 映射的真实收付，授信额度来自商业侧信用政策，已确认未结来自已出账未核销的应收。而 `MapExternalFundsDeps` 只有事实、映射、采用、下游、交接与时钟，没有余额那一只；UC-SA-005 状态行写「运营余额……已形成」，与代码不符。
- **额度已经改道。** `exposeCredit` 的额度一律取 PC 交来的授信依据（ADR-0127 决定五），`credit_standing.limit_minor` 自此只是一列登记值；信用状况里还非它不可的是逾期与那一行本身的存在。
- **外部收款已经进得来。** `parcel-settlement-register` 与 `/settlement-external-funds-fact-registrations`（[sa-cc/27](../../sa-cc-funds-and-credential-seams/issues/27-sa-external-funds-fact-adoption-and-correction-registration-face.md)、[sa-cc/31](../../sa-cc-funds-and-credential-seams/issues/31-sa-external-funds-fact-registration-endpoints-and-admin-write-face.md)）都经 `MapExternalFundsHandler` 采用并映射外部收款事实——缺的只是映射之后推进余额那一步。

## 为什么是机制

按[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」与 [ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：某客户付了多少、给了多少授信、逾期没有，是租户取值；把已采用的外部收付、已确认费用与核销变成余额登记的**写入方**，是产品必须自带的机制。缺了它，判断方法再全也只会答`余额不可用`。演示租户照 ADR-0150 按真实租户对待，只能走同一条写入路径——经 UC-SA-005 采用一笔 `SYN-` 外部收款事实，不在种子里直写 `operational_balance` 或 `credit_standing` 的行。

## 要裁的问题

1. **入账余额由谁推进、何时推进。** 候选：
   - 甲 · UC-SA-005 编排在采用、映射、核销、撤销各自的事务里重算该作用域的入账余额并推进登记——与事实同笔落库，没有中间态；代价是 `MapExternalFundsHandler` 多一只依赖，余额的算法落在编排里。
   - 乙 · 一个余额投影消费 SA 自己的映射与核销事件，异步推进快照——编排不动；代价是冻结可能读到落后于已采用事实的余额，要定滞后怎么答。
   - 丙 · 外部财务系统交来的余额快照作为一种外部事实采用——要先核它与 SA `CONTEXT`「本上下文只有 `UC-SA-005` 可以在外部事实已经确认后形成真实收付映射、相应运营余额和核销关系」的读法合不合。
   不论取哪个，都要定「入账余额」按 SA 自己的哪几本账求出来（已采用收款、未分配、已核销各进出哪一项），写进 SA `CONTEXT` 或裁定 ADR。
2. **已确认未结由谁推进。** 它在 UC-SA-002 确认费用时增、在 UC-SA-005 核销时减；两处各推一次，还是归问题一那个写入方统一求。
3. **信用状况那一行由谁写、逾期谁算。** 额度已取授信依据，那一行还挡在 `exposeCredit` 前面：要么由账龄判断（UC-SA-005「账龄和信用恢复/限制依据」）推进逾期与那一行，要么改成无行时按「未逾期」判——后者是改判断语义，要另出决定，不在本票顺手改。`limit_minor` 那一列留不留，一并定。
4. **首切范围。** 演示动线越过墙三只要预付冻结一支（入账余额）；已确认未结、信用状况与逾期同票做还是拆子票。

难逆转的那几格（余额的求法、写入方落在编排还是投影）起草 ADR 为 Proposed，接受归用户。

## 分诊

2026-10-14 通道 2。四问都没有落在已接受 ADR 的原话里：ADR-0007 只说不与法定账共同维护可覆盖余额，ADR-0047 只说预付读运营余额、账期读信用状况，ADR-0127 只把额度从信用状况列移到信用依据。谁在哪一笔事务里推进、入账余额按哪几本账求、无行能不能当未逾期，都还没有裁定。因此不转 ready-for-agent。裁定在 [ADR-0181](../../../docs/adr/0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md)，Status 为 Proposed，接受归用户。

分界按开发主线「切片的机制半边与实例半边」与 ADR-0146 决定一、决定二。某一笔收了多少、确认了多少、哪一天到期，是租户取值。下面每问的归属是「系统怎样把已经存在的账收成登记」，不看任何一个租户的客户或合同。

1. **入账余额由谁推进、何时推进。机制。** 推荐甲：`UC-SA-005` 在采用并映射、核销、核销撤销各自的事务里重算并推进，与事实同一笔落库。理由：CONTEXT 把运营余额的形成放在外部事实确认之后的这一次用例里；冻结读到的必须是已采用事实已经蕴含的余额。乙否决：异步投影会让冻结读到落后于已采用事实的余额，而今天没有一格回答这种滞后。丙否决：外部余额快照是银行或法定账上的数，ADR-0007 禁止把它当成可覆盖的运营余额采用，它也答不出下面这几本账。
   入账余额按该作用域的两本账求，作用域是租户、责任法人、结算账户、币种。已采用并已映射的客户方向真实收付记为 P（收款进、退回与撤销出，COD 代收本金不进）。已核销分配净额记为 W（分配出、核销撤销回）。入账余额 = P − W。未分配不是再加的第三本：P − W 就是尚未分配的已映射收付。已核销离开入账余额，并等额离开已确认未结，可用余额在核销前后不变。
2. **已确认未结由谁推进。机制。** 推荐归问题一那个重算函数统一求：当前有效的已确认客户运营应收减去指向它们的核销分配。不由确认费用与核销两处各推一次增量。调用点仍是两处事务（确认费用，以及问题一的映射与核销），函数只有一个。某一笔应收是多少仍是租户取值。
3. **信用状况那一行由谁写、逾期谁算。** 谁写那一行是机制：本上下文写，不采用外部状况快照；无行保持`信用状况不可用`，不改成未逾期。逾期谁算是产品策略：截至时点仍有未核销、且到期日早于截至时点，即为逾期；不设宽限日。到期日和未核销金额是租户取值。到期日今天没有字段，所以在它有登记处之前不插入信用状况行——插入并把逾期写成假，就是没算却回答未逾期。`limit_minor` 留列、不当额度依据（ADR-0127 已把额度移走）；将来真写行时该列只为非空写 0。
4. **首切范围。** 推荐只做问题一在客户收款映射及其撤销上的那一支，外加同一次重算写出的已确认未结（账上没有应收则求和为 0，不写死）。演示动线越过预付冻结靠这一支：经既有登记面采用一笔 `SYN-` 外部收款并映射，不直写表。确认费用上的调用点拆出去，否则有历史应收的账户会把未结停在旧值。信用状况与逾期再拆一张，并且阻塞于到期日有处可读；`SYN-FIN-CONTROL-01` 两项都要过，这一支只解开冻结那一半，信用检查仍停在信用状况不可用。

`operational_balance` 的授信额度列不由这个写入方抄入政策额度。新行写 0，表示这一行不承载额度；之后的推进不覆盖该列。抄进去会让预付冻结占用账期额度。

## 做什么（裁完照此拆笔）

1. 写端口：余额与信用状况的写端口进 `ports`，适配器方法挂上去；写入方照问题一至三的裁定接进对应编排。
2. 生产装配：写入方随编排进 `cmd/parcel-api`、`cmd/parcel-settlement-register` 与 `cmd/parcel-dispatch` 的装配；读口不变。
3. 演示租户：`SYN-SETTLEMENT-ACCOUNT-01` 经普通登记采用一笔 `SYN-` 外部收款事实并映射成入账余额，金额覆盖 26 那一件的试算合计；种子走既有登记面，不直写表；证据只记 `S`。
4. 重走 26 判据 1：演示委托越过格 5、冻结一笔，金额等于这一件的试算合计；动线墙三补记写新的下一停点。

## 不做

- 任何真实租户的余额、额度与逾期取值。
- 冻结金额与已占用暴露进登记表——它们由冻结账本与暴露账本当场求和（`HeldMinor`、`ExposedMinor`），迁移头注的理由不变。
- COD 代收本金进普通运营余额（UC-SA-005 原话禁）。

## 完成判据

- 已采用并映射的外部收款推进该作用域的入账余额，迟到的快照不盖新值；核销与撤销各推进一格（带测试，真库带 DSN）。
- 受理前控制对有入账余额的作用域冻结成立，对从未推进过的作用域照旧答`余额不可用`（带测试）。
- 演示环境灌完种子后，演示委托越过格 5、冻结一笔，金额等于这一件的试算合计（`S`）。
