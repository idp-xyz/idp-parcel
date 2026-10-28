# 22 结算其余编排的装配、入口与触发面

Category: enhancement
Status: in-progress——第 1 笔（确认）2026-10-28 进 main（`53c514602`，见文末「第 1 笔进 main 记录」）；第 2 笔（运营决定口）完工待评审，分支 claude-psb22（见文末「第 2 笔完成记录」）；第 3、4 笔待做。此前 ready-for-agent——2026-10-27 用户同意由冻结 R3 上调为「限量生产前必须」（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「一之二」，理由引 PILOT-SCOPE）：费用确认、截单发布、供应商账单接收与审核及确认与截单的触发上调；费用调整、成本分摊、赔付金额仍冻结；排在 tops-26 名下 6 张 code 票进 main 之后，先 10 后 22。此前 ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；接单时按编排拆笔
Blocked by: 无；赔付金额文法与分摊形态归[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md) 第 1、2 项，结算四口的登记册与读口归[票 16](./16-mechanism-gaps-without-a-ticket.md) 第 3 项
地盘：`internal/settlementaccounting` 的入口与触发、`cmd/` 装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 20 的装配与触发面那一半。

## 现象

`settlementaccounting/application` 的 `NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewAuditSupplierBillHandler`、`NewSettleClaimAmountsHandler` 在 `cmd/` 零引用。这一类不在生产接线棘轮的网里（棘轮排除 `New*` 构造，见开发主线「2026-09-02 裁决」一段）。

## 做什么

- **机制半边**：各编排进生产装配，各有入口（运营命令面、受控 CLI 或消费门，按编排定）。
- **产品策略半边**：何时确认费用、何时截单发布的内置触发策略；账期是租户取值，经参考配置在演示租户上采用。

## 要裁的

1. 逐编排定入口形态：人发起的给命令面（生产形态等操作者渠道），事件驱动的给消费门。
2. 费用确认与截单的内置触发：到点批量、事件逐项，还是两者都给由租户显式采用。
3. 费用调整（`RecordChargeAdjustmentHandler`）开入口之前，先裁 [docs-consistency/code/46](../../docs-consistency/code/46-settlement-application-and-reversal-have-no-process-entry.md) 越权风险点「潜伏卡死：确认后的贷记调整让作用域账不平之后，撤销也撤不动」：入口一开，一笔确认后的贷记调整就能让作用域已确认应收低于已核销，此后该作用域的撤销、映射、核销全被重算拒绝。思路两条：放行不让账变得更坏的撤销，或允许一次撤多笔。（2026-10-26 文档自洽 code/46 第三轮轻评审 N-b 补）

## 完成判据

- 每份编排在生产进程里有入口；触发策略有内置执行器；票 05 格 20 的装配与触发面那一半越过（隔离形态实测只记 `S`）。

## 拆笔（2026-10-28，Claude Code 会话；用户 2026-10-27「按你的建议开始吧」）

1. **确认**：七项确认事实随预估客户费用同笔落下；「有效交付已终局」作内置确认依据，从终局信封与费用形成两头记；依据到了而这一户登了确认时点就当场确认；确认条件目录开受控登记口。裁决见 [ADR-0336](../../../docs/adr/0336-confirmation-facts-land-with-the-estimated-charge-and-an-effective-delivery-final-is-the-built-in-confirmation-basis.md)。
2. **运营决定口**：运营确认口（ADR-0336 决定三）与截单发布口。截单的内置触发另开 ADR：SA CONTEXT「对账周期的日历不在这本册里，本期边界由截单命令点名」，到点自动截单要先有周期册，第 2 笔里重新定。
3. **供应商账单**：接收与审核两个运营决定口。
4. **演示种子与重走**：演示租户登确认条件、两个结算时点、对账安排；重走动线到`已确认`与对账单发布。

## 第 1 笔完成记录（分支 claude-psb22，基于 main `c2a771cbb`）

- **做了什么**：`FormEstimatedCustomerChargeHandler` 的必备依赖 `FollowUp`（`FormedChargeConfirmationFollowUp`）在费用落下之后、记结论之前同笔登七项事实，PS 当前终局为已终局的有效交付就补记依据并按时点确认；新编排 `ConfirmChargeOnBasisHandler`；SA 新开第二路终局消费 `settlement-accounting/record-confirmation-basis-on-final-outcome`（`cmd/parcel-dispatch/routes_final_outcome.go`，名次 `positionBilling + 10`）；账户册 `FindByID`；依据册与条件目录的端口形写口；CLI 新命令 `charge-confirmation-condition`。无迁移。
- **文档**：ADR-0336；SA `CONTEXT.md`「费用形成与证据」两句；UC-SA-002 `AT-SA-217`–`220` 与交接一行（运营确认口归第 2 笔）。作者不重生成 ADR 索引、AT 盘点与机制清点。
- **自验**（受影响范围，真库 `idp-parcel-postgres-gate`）：`go test ./internal/settlementaccounting/... ./cmd/parcel-dispatch/ ./cmd/parcel-settlement-register/ ./cmd/parcel-api/ ./internal/architecture/... -count=1` 全过；`go vet ./...` 干净；`gofmt -l` 空。真库路由表用例 `TestAFormedChargeIsConfirmedOnTheEffectiveDeliveryFinalInEitherOrder` 两种先后都走到`已确认`、交出一封费用确认信封。全量与种子灌库留给推送方。
- **改动的既有用例**：dispatch 三条用例。两条直写 SELL 请求的补登了请求指名的结算账户（`sellRequestAccount`），因为形成之后要按标识读回那一户；终局扇出用例的 inbox 期望由两路改成三路。
- **代决与越权风险点**：见 ADR-0336「越权风险点」，待 SA owner 复核。

## 第 1 笔评审记录与修订轮（2026-10-28）

**评审**：非作者隔离评审（子代理，隔离检出，钉 `935cf7fb2`，对 main `c2a771cbb`）。结论：可合入，无阻断。核过的有：事务边界、重投不重复确认也不重发信封（真库探针：终局先到并确认之后，再投两封同键终局，确认信封始终一封）、租户取值缺席的几格（真库探针：删条件、删时点各只记依据、不重投；删账户则整笔回滚、重投）、`Retryable` 分格、`SaveConfirmationCondition` 的读回、与 parcel-api 装配逐项一致、改动的既有用例。建议七条，本轮这样处理：

- **N1 矛盾被记成待重投**：已修。跟进只把等依赖的几格包上 `ErrConfirmationFollowUpUnavailable`，外层据它答未决；请求指名的账户不在册上等矛盾原样上抛。
- **N2 两头并发漏记**：已修。新增 `ParcelConfirmationSerializer` / `ParcelConfirmationLocks`，按（租户、包裹）取事务级咨询锁；终局那一头读请求之前取，形成那一头读终局之前取。ADR-0336 决定二第 4 项与越权风险点 3 相应改写。
- **N3 确认信封没交出去不重投**：已修。确认交接引用非空时 `Retryable` 为真。
- **N4 注释指了一个不存在的符号**：已修。
- **N5 跨文件的序数与计数**：已修，改为按名字指。
- **N6 一条注释的理由不成立**：已修，改写了理由。
- **N7 确认装配在两个进程里各写一份**：本笔不动。第 2 笔给 parcel-api 开运营确认口时会再碰这一段，到时抽成共用构造或加比对用例。
- **待核（终局重派生之后旧信封迟到）**：与既有 `RequestOnFinalOutcomeAdapter` 的读法同形，归 ADR-0336 越权风险点 4。

**修订后自验**：受影响各包加 `internal/architecture` 全过。新增用例：两头取锁（单元）、确认信封交接失败要重投（单元）、包裹确认锁只让同一件包裹排队（真库并发）、锁在事务外被拒（架构门禁要求）。`go vet ./...` 干净，`gofmt -l` 为空。

**复核**（同一评审，钉 `9fa63de61`）：阻断清零。N1–N6 修法成立。N2 有修前、修后两次并发探针取证：形成事务读终局时终局尚未提交、终局那一路随后到达；修后终局那一路等形成提交之后再读，费用落到`已确认`、确认信封一封；修前（`935cf7fb2`）没有等待，费用停在`预估`、确认信封零封。两只旧探针在新 tip 上结果不变，只有「无账户」那一格的失败码按 N1 由 `consumer_undecided` 变为 `publish_failed`。新提一条 N8（措辞）：`AT-SA-217` 没写失败码的口径，已补。

## 第 1 笔进 main 记录（2026-10-28）

- **main**：`53c514602`。分支 `claude-psb22` 在 main `2afdd0b1e` 之上重放（`git rebase origin/main`，无冲突），共六笔：实现 `64b9eab7d`、修订 `c75085b24`、N8 与复核 `c905238df`，以及三份生成物各一笔（ADR 索引 `50aff4cbf`、AT 盘点补行与重算 `5f71d70bd`、机制清点 `53c514602`）。
- **评审**：非作者隔离评审两轮，阻断清零，见上面「评审记录与修订轮」。
- **生成物**：`go run ./tools/adr-index -check` 与 `go run ./tools/at-coverage-counts -check` 都退 0；机制清点重生成之后 `git status --porcelain` 为空。
- **全量真库**：`scripts/test-full.sh` 在推送的 tip 上退 0，160 个包 ok，0 FAIL。
- **种子**：`seed.sh` 灌一次性库 `psb22_seed` 退 0，用完已删。
- **推送**：`git push origin 53c514602:main`，`origin/main` 等于该 SHA。

## 第 2 笔完成记录（分支 claude-psb22，rebase 于 main `2d0193336`）

- **做了什么**：
  - **两种运营决定**：`CHARGE_CONFIRMATION`、`STATEMENT_PUBLICATION`（`accessidentity`，迁移 `access_identity/0034`；CHECK 全集以 main 上 0029 为底）。
  - **SA 两口**：`NewConfirmChargeEndpoint`、`NewPublishStatementEndpoint`，以及 `OperatorDecisionIntake` 上的两个 Intake。parcel-api 特性文件 `endpoints_settlement_decisions.go` 挂 `/settlement-charge-confirmations` 与 `/settlement-statement-publications`。
  - **客户费用读口**：多 `attributionDate`、`billedOn` 两格。
  - **到点截单**（用户 2026-10-28 答「自动截单」）：
    - 领域 `StatementCycle`（日截、周截、月截与起算日）；
    - 迁移 `settlement_accounting/0049`，即对账周期册与截止记录；
    - 编排 `CutOffStatementsOnCycleHandler`；
    - dispatch 的 `assemble_cycle_cut.go`：派发器外包一层，每拍之后逐户截；
    - CLI 命令 `statement-cycle`。
- **裁决**：ADR-0339（原拟 0337、0338，号都被别的票先占，改号）。决定三改为到点截单。CONTEXT「对账安排」与「结算账户、对账与争议」两处、GLOSSARY「对账安排」各补一句；ADR-0269 那句「本期边界由截单命令点名」照旧成立，不停用。
- **文档**：UC-SA-002 加 `AT-SA-221`，删掉「运营确认口归第 2 笔」那行交接；UC-SA-003 加 `AT-SA-222`–`224`。
- **自验**（真库）：受影响各包加 `internal/architecture`、`migrations` 全过。真库用例：
  - parcel-api 两口的纵向用例；
  - dispatch `TestTheDispatchBeatCutsADueCycleIntoAPublishedStatement`：确认那一拍归属日没关上不出单；归属日拨到前天后一拍出一张单，收这一笔、合计 10500；之后的截止日只记无费用；再一拍不多出单。
- **评审第一轮**（钉 `8178355da`，非作者隔离评审）：两条阻断，都已修。
  - **B1**：误把 `cmd/parcel-api/parcel-api` 二进制提交进来。已从那一笔拿掉、改写了提交；`.gitignore` 补了 `/cmd/*/parcel-*`。
  - **B2**：迁移 CHECK 漏了 main 上新加的决定种类。rebase 后改号 0030，以 0029 为底。
  - **建议 N1–N5**：已修。截单时刻固定一次；发布口各格在 Intake 校验，申报合计只收不带符号与前导零的整数串；`billedOn` 排序与出账锁一致；删掉 `main.go` 里多余的那次构造和计数注释；补读口 JSON 与发布口载荷的用例。
  - **N6**：票面完成判据的措辞，收票时对齐。
- **评审 N7 没收**：确认与截单编排在两个进程各装一份，到点截单又多了一处。仓里没有给多个 cmd 共用的装配包，新开一个是架构取舍，不在本票范围。

**第二轮评审**（钉 `5ffe1187c`）：B1、B2、N1–N5 复核已修。新增两条阻断，都已修：

- **B3**：确认时判不出归属日的费用，到点截单永远收不到（候选 SQL 的 NULL 比较）。修法：截之前先给这一户这类费用当场补判、判出即落库，与截单编排判同一只；判不出这一户停住、不记截止日，报 `ErrCycleCutUnconfigured`。dispatch 真库用例改走这一格：清空归属日、确认时刻拨回两天，执行器补判之后截进那一期。
- **B4**：「对账周期是不是对账安排的一部分」，文档前后不一。按 (a) 处理：周期是对账安排之下的第二本册，同步改了领域头注、迁移头注和 ADR-0339 决定三之 1；ADR-0339 写明回答了 ADR-0269 越权风险点 5。CONTEXT 与 GLOSSARY 原有的「属于对账安排」照旧成立，不停用任何 ADR。

建议这样处理：

- **N8** 改注释：起算日只定第一个截止日从哪天之后算起，不是费用的下限。
- **N9** 截单时点门挪到最前面，没登就什么都不记。
- **N10** 日志按等依赖、未配置、矛盾分开记（`cycleCutFailureKind`）；没有退避，记进越权风险点 4。
- **N11** 记进越权风险点 3。
- **N13** `ClosingInstant` 改按墙钟取截单时刻。
- **N14** 补了 CLI 两条命令的译装用例。候选 SQL 各排除条件（已出账、币种不符）与跨进程并发截，仍没有专门用例。
- **N15** ADR 的取证 SHA 已更新。
- **N12** 同 N7，不收。

**第三轮评审**（钉 `023a3bc83`）：阻断清零，可合入。B3、B4 修法成立；dispatch 真库用例确实走到补判那一格，上海 18 点前后两种情形期望都自洽。新提三条建议：

- **N16**（一笔判不出归属日让整户停截）：记进 ADR-0339 越权风险点 5。
- **N17**（缺来源时点的费用被静默跳过）：记进越权风险点 6。
- **N18**（时点门判停时，没登对账安排的户每拍报一条日志）：与 N10 同类，不另收。
