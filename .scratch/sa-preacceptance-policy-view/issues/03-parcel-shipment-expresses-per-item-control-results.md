# `parcel-shipment` 的 `FinancialControlResult` 一个请求只装一个结果——组合控制的逐项结果与共同通过条件在 PS 域里无处表达，今天靠消费适配器折叠

Category: enhancement
Status: resolved——由 [ADR-0122](../../../docs/adr/0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定四拆出（2026-09-07，通道 1，票 02 落地时）；PS 地盘。2026-09-07 通道 3 认领并落地（task-0ec68b23，基线 `b24ccccf`，分支 `mcp3-sa03`），三问裁决与完成记录见下；第 2 问的后继在票 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md)
Blocked by: 无（SA 侧已逐项交回，见票 [02](./02-load-control-policy-reads-policy-content-items.md)）

## 缺口

ADR-0122 起，`settlement-accounting` 对一次接受前控制请求按策略正文逐项执行，`ApplyPreAcceptanceControlResult` 交回
`ExecutedControls()`（每项的种类、判断顺序、有没有形成`业务限制`）、`Freeze()` 与 `Exposure()`（可同时在场）与
`JointPassCondition()`。CONTEXT 写「合同明确组合多项接受前控制时，每项结果必须保持独立依据和有效性，由 `parcel-shipment`
按策略的共同通过条件形成接受判断」。

`parcel-shipment` 今天的 `psdomain.FinancialControlResult` 是**一个请求一个结果**：`HELD` / `CREDIT_EXPOSED` / `RESTRICTED` /
`NOT_APPLICABLE` 四格加一条依据。逐项结果装不进去，于是 `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`
的 `appliedControlAssessment` 在适配器里按共同通过条件折成一个：任一项受限即 `RESTRICTED`（带那一项自己的原因），全部成立时有冻结即
`HELD`、只有暴露即 `CREDIT_EXPOSED`。

折叠的位置对（判接受的是 PS），但它把三件事压成了一格：

- 哪几项执行了、各自的结论——事后看接受判断时只见一个 `HELD`，看不见同一请求还记了一笔信用暴露。
- 共同通过条件本身——今天只有 `ALL_CONTROLS_PASS`，折叠等价于「任一受限即拒」；放宽出第二种组合子时适配器会先炸（全函数），
  但 PS 域里没有一格能装「按哪个条件判的」。
- 「成立但有多项」与「成立只一项」在 PS 结果上分不开——释放与追溯都按同一请求身份认领，今天没出事，是因为 SA 那侧释放
  已改为两本账各认领一次（ADR-0122 决定三），不是因为 PS 知道有两项。

## 要裁的

1. `FinancialControlResult` 长成「逐项结果集合 + 共同通过条件 + 按条件判出的接受侧结论」，还是并列一个新对象让既有接受编排继续读旧形状？
   UC-PS-001「接受前财务控制」那一行与 `AT-PS-*` 里引用 `HELD` / `CREDIT_EXPOSED` 的判据要跟着改口的有哪些。
2. 失败处置（`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用今天不随 SA 答复走（ADR-0122 决定四）：PS 形成接受判断时要用它们，
   经自己的 PC 消费缝读策略正文，还是只读绑定到本次委托费用范围的那几行？读口形状归 PC（`PreAcceptanceFinancialControlPolicyContentView`
   已在），PS 侧适配器归本票。
3. 一项受限即停后续项（ADR-0122 决定二）之下，`ExecutedControls` 短于策略控制项——PS 结果要不要区分「未执行」与「执行且成立」。

## 边界

- 不动 SA 任何一格；不改 `ApplyPreAcceptanceControlResult` 的形状（要加格回 SA 票）。
- 裁前 `appliedControlAssessment` 的折叠照旧；裁决若改 PS 结果形状随实现落 ADR（引本票与 ADR-0122），编号取当时下一号。

## 裁决

2026-09-07，通道 3（task-0ec68b23），用户经 IDP 队列授权「owner 自决」口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点。三问的答落在 [ADR-0125](../../../docs/adr/0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md)，这里只记结论与取证；理由与备选见 ADR。

**能力边界**：读过本票全文、ADR-0122 全文、ADR-0047、PS CONTEXT「适用客户合同必须提供版本化接受前财务控制策略」那一句、SA CONTEXT 关于组合控制的两句、`psdomain.FinancialControlResult` 与 `FinancialControlCheckFor`、`AdvanceFinancialControlJudgmentHandler`、`FormAcceptanceDecisionHandler` 的 `assembleChecks` / `releaseIfRejected`、`RejectShipmentRequestHandler` 与 `WithdrawShipmentRequestHandler` 的 `releaseFreeze`、PS→SA 适配器 `pre_acceptance_control.go` 全文与测试、`AcceptanceJudgments` 的读写与迁移 `0005` / `0018`、SA 侧 `ApplyPreAcceptanceControlResult` 与 `ReleasePreAcceptanceControlHandler`（只读）、PC `PreAcceptanceFinancialControlPolicy` 与 `PreAcceptanceFinancialControlPolicyContentView`、SA→PC 适配器、UC-PS-001 校验组行与 `AT-PS-035`、BD-PS-003 简报、HTTP 读面 `query_acceptance_review_queue.go`。未读 admin-web 前端。

**取证（锚 `b24ccccf`，只作当时取证）**：

1. 三处释放（`releaseIfRejected` 与两处 `releaseFreeze`）都只在结论为 `HELD` 时发释放。`CREDIT_EXPOSED` 从不释放——账期额度占用在拒绝 / 撤回后成孤儿；组合策略下第一项冻结成立、第二项受限折成 `RESTRICTED` 也不释放——而适配器自己的用例 `TestACombinedControlFoldsByTheJointPassCondition` 断言那笔冻结「留在账本上等释放」。两处都是「压成一格」直接造成的：结论把「有没有占用」盖住了。
2. 接受编排读控制结果只经两处：`FinancialControlCheckFor`（`HELD` / `CREDIT_EXPOSED` / `NOT_APPLICABLE` 译通过，`RESTRICTED` 译不通过）与 `assembleChecks`（零值判未形成）。UC-PS-001 全文不含 `HELD` / `CREDIT_EXPOSED` 字面——票面猜的「AT-PS-* 里引用 HELD / CREDIT_EXPOSED 的判据」并不存在，`AT-PS-035` 用的是「信用校验和预付冻结…两项」的业务措辞。
3. HTTP 读面只透出结论、结果标识、依据与时点一格，看不到逐项。

**第 1 问：长成一个对象，不并列。** `FinancialControlResult` 长成「逐项控制项结果 + 共同通过条件 + 按条件推导出的接受侧结论」，结论由领域构造期从前两者推导，适配器不再交入结论。新值对象 `ControlItemResult`（控制种类 × 判断顺序 × 结论 × 受限原因引用），`ControlItemKind` 与 `JointPassCondition` 是 PS 自有封闭集、镜像不 import、集外报错不吸收。两个构造入口：已执行（≥1 项、顺序唯一、种类唯一、条件在集内）与明确无控制（无项、无条件、必带商业不适用依据）；旧的四参构造器移除——「结论可以与逐项不一致」的入口不能留。接受侧结论沿用 `FinancialControlOutcome` 四值不改词：`RESTRICTED` = 共同通过条件下不成立、依据取判断顺序最靠前的受限项自己的原因；`HELD` / `CREDIT_EXPOSED` 是「成立」的两种拼法，只区分成立的项里有没有预付冻结——逐项结果才是「执行了什么」的权威，两格只是投影。折叠从适配器搬进领域；新增 `OccupationFormed()`（任一项成立即真），三处释放改按它发。影响面：UC-PS-001 校验组「接受前财务控制」行的「确定性不通过时」一格改写为按共同通过条件；`AT-PS-035` 判据补「已成立的项不因接受侧结论而免释放」半句；PS CONTEXT 加词条；迁移 `0019`；HTTP 读面加 `jointPassCondition` 与 `items`。

**第 2 问：经自己的 PC 消费缝读，只读绑定到本次委托费用范围的那几行，且只在有项受限时读——但本票不建这条读路。** 不随 SA 走已由 ADR-0122 决定四钉死；读的缝是 `PreAcceptanceFinancialControlPolicyContentView`；取项口径与 SA 执行时同源（闭包里已采用结算政策的 `Applicability().ChargeScope()` → `ItemsFor`），别的范围上的项没执行过、管不了这份委托的去向；通过的控制没有去向问题。处置的消费方是「进入授权处置」那条流程，本上下文今天没有——授权角色能对不通过的控制做什么、等待态与续办路径、读面都未裁；没有消费方的读路进了没人读。读口与流程同票落地，另立 draft 票 [04](./04-authorized-disposition-flow-and-failure-disposition-read.md)。**过渡状态**：在那一票落地前 `RESTRICTED` 仍译确定性不通过 → 拒绝，等于按 `REJECT` 处置；租户若登记 `AUTHORIZED_DISPOSITION` 今天会被自动拒绝——这是机制半边未齐，不是产品口径，ADR-0125 Consequences 点名。

**第 3 问：不分。** 「未执行」不是一项结果——没有结论、没有依据、没有占用；把它记成一项就得让 PS 读策略正文来列全项，为的是一个在「全部通过」之下改不了接受判断的信息。PS 只记已执行的项（SA `ExecutedControls` 原样译回，含顺序）；「哪些没跑」由读的人算：已采用解析（`RecordedJudgments.AdoptedCommercialResolution`）→ 闭包 → 采用的控制策略版本 → 正文项 − 已执行项，这条路今天就是通的。放宽出第二种组合子、「未执行」开始影响结论时再议。

**越权风险点**（供 owner 复核）：

1. ADR-0047 决定三的 `HELD` / `CREDIT_EXPOSED` 两格降为「成立」的投影而保留，未 supersede——若 owner 认为该合成一格，那是 ADR-0047 的改动，不在本票。
2. ADR-0122 决定四写的折叠位置「PS 的 `adapters/settlementaccounting`」搬进了 PS 领域——归属未变（仍是 PS 折），那句字面过时；本票不改 ADR-0122 正文，由 ADR-0125 引它说明。
3. 「未执行可算」依赖 SA 停在首个受限项（ADR-0122 决定二）与闭包可查；SA 若改为继续执行，本票结论不变，只多几项已执行结果。
4. 释放改按 `OccupationFormed` 发，依据是 SA 释放对同一请求身份两本账幂等各认领、限制不入账本（ADR-0122 决定三）——「有成立项就发」不会对着一笔不存在的占用重试；SA 改账本语义时要回头看这一格。
5. 过渡状态「`RESTRICTED` 一律拒绝」等于替租户按 `REJECT` 处置；票 04 落地前无租户，风险停在纸面。

## 完成记录

2026-09-07，通道 3，分支 `mcp3-sa03`（隔离树，基 `origin/main = b24ccccf`）。逐笔：

| SHA | 内容 |
|---|---|
| `63dce12a` | docs(adr)：ADR-0125 + README 一行；本票「裁决」节、转 in-progress；PS CONTEXT 词条「接受前财务控制采用结果」「控制项结果」；立票 04 draft |
| `a0153884` | feat(parcel-shipment)：领域 `ControlItemResult` / `ControlItemKind` / `ControlItemConclusion` / `JointPassCondition`，`FinancialControlResult` 三入口（`NewExecutedFinancialControlResult` 推导结论、`NewInapplicableFinancialControlResult`、重建门 `RehydrateFinancialControlResult`），旧四参构造器移除，`OccupationFormed`；PS→SA 适配器 `appliedControlAssessment` 改全函数翻译；迁移 `0019`（父表 `joint_pass_condition` + 子表 `acceptance_financial_control_item`）与 `AcceptanceJudgments` 读写；HTTP 读面 `financialControl` 加 `jointPassCondition` / `occupationFormed` / `items`；PS domain / application / http / postgres 与 `cmd/parcel-dispatch` 两件测试夹具跟随 |
| `d1a878aa` | fix(parcel-shipment)：三处释放改按 `OccupationFormed`（`releaseIfRejected`、两处 `releaseOccupation`）；UC-PS-001 校验组行与 `AT-PS-035` 改口 |
| `af7dd29a` | docs(product)：清点在 `d1a878aa` 干净检出重生成（parcelshipment 生产 139→140 / 测试 138→139；迁移 147→148，parcel_shipment 18→19） |
| `e7c44522` | test(architecture)：重建面门禁登记两个新入口为受限（只对 `internal/parcelshipment/adapters/postgres` 开放） |

**三问的答**见「裁决」节与 ADR-0125；第 2 问只裁未建，读口与授权处置流程同票 04。

**触及文件**（除票面与 ADR）：`internal/parcelshipment/domain/{financial_control_result.go,acceptance_basis.go}`、`adapters/settlementaccounting/pre_acceptance_control.go`、`adapters/postgres/acceptance_judgments.go`、`adapters/http/query_acceptance_review_queue.go`、`application/{form_acceptance_decision.go,reject_shipment_request.go,withdraw_shipment_request.go}`、`migrations/parcel_shipment/0019_acceptance_financial_control_items.sql`、`internal/architecture/rehydration_gate_test.go`（门禁登记）、`cmd/parcel-dispatch/{synthetic_v0_test.go,manual_review_resume_loop_test.go}`（纯夹具签名跟随）、`docs/domain/parcel-shipment/CONTEXT.md`、`docs/application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md`、`docs/product/MECHANISM-INVENTORY.md`。**不动**：SA 任何一格、`internal/parcelshipment/adapters/partycommercial`、`cmd/parcel-api`（装配未变——端口签名未变，适配器构造签名未变）。

**验证**（隔离 detached 树 `idp-parcel-mcp3-verify` 钉 `e7c44522`，验后拆）：`gofmt -l .` 空；`go build ./...`、`go vet ./...` 退 0；`tools/mechanism-inventory` vet/test 退 0、在 tip 重生成零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（548s，21:36:37–21:45:45，接续会话跑，见 Comments 末条）——上一会话在 `af7dd29a` 上跑得 99 ok / 1 FAIL，红的只有 `internal/architecture` 重建面门禁登记那一处，`e7c44522` 补登后本次重跑绿；探针 `internal/parcelshipment/adapters/postgres -run 'ACombinedControlRoundTrips|AStoredConclusionThatContradicts|AcceptanceJudgmentShapesArePinned'`：无 DSN exit 0 SKIP 3 / PASS 0，含 DSN exit 0 SKIP 0 / PASS 3（两会话各跑一次同数）。未跑 `-race`（本机 Windows 无 cgo，CI 四片各带）。

**要 MCP-1 落的共享行**：`docs/adr/README.md` 的 0125 一行（与 main 上他人后落的行相邻，重放时按行合并）；`docs/product/MECHANISM-INVENTORY.md` 分支清点笔 `af7dd29a` 不必重放，由推送方在 tip 重生成兑底。`migrations/parcel_shipment/0019` 在 `fc90622a` 的 main 上仍空号（21:2x `git ls-tree` 量得，只作此刻取证）。进 main 后的 SHA 见下节。

## 进 main 记录

2026-09-07 22:3x，MCP-1 重放到 `f69b35b9` 之上（cherry-pick 零冲突；`docs/adr/README.md` 0125 行自动合并落 0124 之后），远端 `main = 9379c716`（推前 `ls-remote = f69b35b9`）。分支上的 SHA 作封存出处，与 main 上的并列：

| 分支 `mcp3-sa03` | main | 内容 |
|---|---|---|
| `63dce12a` | `0ca996d6` | ADR-0125 + README 行；票 03 裁决节；PS CONTEXT 两词条；立票 04 draft |
| `a0153884` | `096f1eae` | 领域 / PS→SA 适配器 / 迁移 `parcel_shipment/0019` / PG 读写 / HTTP 读面 / dispatch 夹具 |
| `d1a878aa` | `0848a3f0` | 三处释放改按 `OccupationFormed`；UC-PS-001 与 AT-PS-035 改口 |
| `af7dd29a` | —— | 分支清点笔，不重放（量的是 `b24ccccf` 上的数） |
| `e7c44522` | `2e582020` | 重建面门禁登记两个受限入口 |
| `5ffdd3f3` | `d93dfddf` | 票 03 转 resolved + 完成记录 |
| —— | `9379c716` | 清点在 `d93dfddf` 干净检出重生成（parcelshipment 生产 139→140 / 测试 138→139；迁移 `parcel_shipment` 18→19，合计 150→151；端口声明、端点数不变） |

MCP-1 钉 `9379c716` 的验证（隔离 detached 树）：gofmt 空；build/vet 退 0；清点门零差；含 DSN `go test -p 1 -count=1 ./...` 退 0，100 ok / 0 FAIL / 16 无测试 / 0 cached（552s）；探针 `parcelshipment/adapters/postgres` 三用例无 DSN SKIP 3 / 含 DSN PASS 3；改动 `.go` / `.sql` 全 `i/lf w/lf`。通道 3 侧比对（22:3x，`origin/main = 9379c716`）：分支自 `b24ccccf` 起触及的 28 件（除 README 与清点）`git diff origin/main mcp3-sa03 -- <files>` 为空，`git cherry` 只剩 `63dce12a`（README 合并改了 patch-id）与 `af7dd29a`（清点笔）两个 `+`，内容零差。`parcel_shipment` 迁移下一号 `0020`。

## Comments

- 2026-09-07 · 通道 1：由 ADR-0122 决定四拆出立票，只写票面，未动 PS 代码。能力边界：读过 PS 消费适配器全文与 SA 侧全部改动；
  **没读** PS 接受编排读取 `FinancialControlResult` 的那一段与 UC-PS-001 全文——第 1 问的措辞据 CONTEXT 与适配器代码写，开工者以代码为准。
- 2026-09-07 · 通道 3：认领，三问裁决落「裁决」节与 ADR-0125；票面第 1 问猜的「AT-PS-* 引用 HELD / CREDIT_EXPOSED」按代码与 UC 全文核过不存在，改口范围缩到 UC-PS-001 校验组一行与 `AT-PS-035` 半句。  取证到的两处释放孤儿（`CREDIT_EXPOSED` 从不释放、`RESTRICTED` 前的成立项不释放）随本票一并修，不另立票——它们就是「压成一格」的直接后果。
- 2026-09-07 · 通道 3（接续会话，21:2x）：上一会话在 `e7c44522` 上跑完 gofmt / build / vet / 清点零差 / 探针后崩溃（用户 21:2x 在队列报；台账 `0ec68b23` 停在 19:14 的 working 报，无完工报），本票完成记录写到一半未提交、含 DSN 全仓那一格没落字。本会话在同一 detached 树 `idp-parcel-mcp3-verify`（仍钉 `e7c44522`，`status --untracked-files=all` 零行）重跑含 DSN 全仓与探针，结果写进「验证」段；分支五笔一字未动，只补本 .md。`git cherry origin/main mcp3-sa03`（`origin/main = fc90622a`，21:2x fetch 后量得）五笔全 `+`，尚未进 main。
- **owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁）：越权风险点五条逐条认可（第 5 条认可为票 04 落地前的过渡态），理由写在 ADR-0125「owner 复核记录」；第 2 条已在 ADR-0122 加复核记录说明折叠位置搬进 PS 领域。
