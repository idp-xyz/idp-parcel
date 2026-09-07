# ADR-0125：`parcel-shipment` 的接受前财务控制采用结果长成「逐项控制项结果 + 共同通过条件 + 推导出的接受侧结论」——按共同通过条件折叠从消费适配器搬进领域，结论由构造期推导而不由适配器交入；释放按「有没有成立的项」发，不再按结论；失败处置与责任引用留给授权处置流程，读口与流程同票

Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 自决」口径，由通道 3 代裁票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md) 三问并实施。裁决能力边界：读过该票全文、[ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 全文、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)、PS CONTEXT 关于接受前财务控制的那一句、SA CONTEXT 关于组合控制的两句、`psdomain.FinancialControlResult` 与 `FinancialControlCheckFor`、`AdvanceFinancialControlJudgmentHandler`、`FormAcceptanceDecisionHandler` 的 `assembleChecks` / `releaseIfRejected`、`RejectShipmentRequestHandler` 与 `WithdrawShipmentRequestHandler` 的 `releaseFreeze`、PS→SA 适配器 `pre_acceptance_control.go` 全文与测试、`AcceptanceJudgments` 读写与迁移 `0005` / `0018`、SA 侧 `ApplyPreAcceptanceControlResult` 与 `ReleasePreAcceptanceControlHandler`（只读）、PC `PreAcceptanceFinancialControlPolicy` 与 `PreAcceptanceFinancialControlPolicyContentView`、SA→PC 适配器、UC-PS-001 校验组行与 `AT-PS-035`、BD-PS-003 简报、HTTP 读面 `query_acceptance_review_queue.go`；**未读** admin-web 前端。因此本记录只裁 PS 域结果的形状、折叠的位置、释放的触发、失败处置的去处与「未执行」分不分五件；授权处置流程本身另立票）
Date: 2026-09-07

## Context

ADR-0122 起 `settlement-accounting` 对一次接受前控制请求按策略正文逐项执行，交回 `ExecutedControls()`（每项的种类、判断顺序、有没有形成`业务限制`）、可同时在场的 `Freeze()` 与 `Exposure()`、以及 `JointPassCondition()`；它明写「多项结果怎么合起来看由 `parcel-shipment` 按共同通过条件折」，并把 PS 域对逐项结果的表达留给本票。SA CONTEXT 硬句：「合同明确组合多项接受前控制时，每项结果必须保持独立依据和有效性，由 `parcel-shipment` 按策略的共同通过条件形成接受判断。」PS CONTEXT 硬句：「`parcel-shipment` 只采用预付冻结、信用校验、明确无控制或合同明确组合所形成的权威结果与依据，不拥有价格、余额、冻结或信用暴露。」

改之前 PS 这一侧是这样的（取证锚 `b24ccccf`）：

- `psdomain.FinancialControlResult` 是**一个请求一个结论**：`FinancialControlOutcome` 四格（`HELD` / `RESTRICTED` / `NOT_APPLICABLE` / `CREDIT_EXPOSED`）加一条依据引用与时点，由四参构造器 `NewFinancialControlResult` 直接收结论。逐项结果与共同通过条件装不进去。
- 折叠发生在 PS→SA 适配器的 `appliedControlAssessment`：任一项受限即 `RESTRICTED`（带那一项自己的原因），全部成立时有冻结即 `HELD`、只有暴露即 `CREDIT_EXPOSED`。位置在消费方是对的（判接受的是 PS），但它是**判断而不是翻译**——哪个组合算通过是本上下文的接受语言，ADR-0025 说适配器只翻译不判断；同一条理由已经把 `FinancialControlCheckFor` 放在了领域层。
- 释放在三处（`FormAcceptanceDecisionHandler.releaseIfRejected`、`RejectShipmentRequestHandler.releaseFreeze`、`WithdrawShipmentRequestHandler.releaseFreeze`）都按 `Outcome() == HELD` 发。于是 `CREDIT_EXPOSED` 从不释放——账期额度占用在拒绝 / 撤回后成孤儿；组合策略下第一项冻结成立、第二项受限折成 `RESTRICTED` 也不释放——适配器自己的用例 `TestACombinedControlFoldsByTheJointPassCondition` 断言那笔冻结「留在账本上等释放」，而没有任何一条路会去释放它。两处孤儿都是「把三件事压成一格」的直接后果：结论把「有没有占用」盖住了。
- 接受编排读控制结果只经 `FinancialControlCheckFor` 与 `assembleChecks`（零值判未形成）两处；HTTP 读面透出结论一格，看不到逐项。UC-PS-001 全文不含 `HELD` / `CREDIT_EXPOSED` 字面，`AT-PS-035` 用的是「信用校验和预付冻结…两项」的业务措辞。
- 失败处置（`REJECT` / `AUTHORIZED_DISPOSITION`）与责任引用按 ADR-0122 决定四不随 SA 答复走；PS 今天没有任何一处读它们，`RESTRICTED` 一律译确定性不通过。

## Decision

**一、`FinancialControlResult` 长成一个对象：逐项控制项结果 + 共同通过条件 + 按条件推导出的接受侧结论；不并列第二个对象。** 新值对象 `ControlItemResult` 是本上下文对 SA 一项已执行控制的采用引用：控制种类（`ControlItemKind`，PS 自有封闭集 `PREPAID_FREEZE` / `CREDIT_CHECK`，与 SA / PC 同名不 import，纪律同 `SettlementMethodEcho` 不建枚举的反面——这里要按种类判「成立的项里有没有冻结」，所以要封闭集）× 判断顺序 × 结论（`成立` / `业务限制`）× 受限原因引用（只在`业务限制`时必带）。`JointPassCondition` 同为 PS 自有封闭集，首发一值 `ALL_CONTROLS_PASS`，集外报错不吸收（ADR-0025）。两个构造入口：`NewExecutedFinancialControlResult`（已执行：至少一项、判断顺序唯一、种类唯一、条件在集内，按顺序排定）与 `NewInapplicableFinancialControlResult`（明确无控制：无项、无条件、必带商业不适用依据、不带结果标识）。旧的四参构造器移除：留着它，就留着「结论可以与逐项不一致」的入口，而两处重建（适配器与登记册）都会顺手走它。不并列新对象的理由：并列意味着折叠仍在适配器、逐项只是写进去没人读的装饰，且两个对象要靠约定保持一致——ADR-0122 决定四留的正是「PS 域里没有一格能装按哪个条件判的」这个缺口，装饰填不上它。

**二、接受侧结论由构造期按共同通过条件推导；`FinancialControlOutcome` 四格不改词，`HELD` / `CREDIT_EXPOSED` 降为「成立」的投影。** 「全部通过」之下：任一项`业务限制`即 `RESTRICTED`，依据取判断顺序最靠前的受限项自己的原因（SA 今天停在第一处限制，所以至多一项受限，但本上下文不把这条假设写进构造期——多项受限时取最靠前的，其余都在逐项里）；全部成立时，成立的项里有预付冻结即 `HELD`、否则 `CREDIT_EXPOSED`——沿 ADR-0122 决定四「两者并存时以资金已被占用那一句为准」。保留两格是为了不动 ADR-0047 决定三的词汇、迁移 `0005` 的 CHECK 与既有读面；代价是两格与逐项之间有一处冗余。冗余的处置是**只有一个来源**：此刻发生的结论只能由 `NewExecutedFinancialControlResult` 从逐项推出；登记册重建走另一扇门 `RehydrateFinancialControlResult`——把库里记的结论当数据收下、只校验它与逐项在所记条件下一致，不一致按坏数据拒绝，不用今天的推导覆盖当时记下的结论（重建只校验不重算，ADR-0028）。**读的人要问「执行了什么、有没有暴露」去看逐项，结论只答「按条件成没成立」。**

**三、折叠从 PS→SA 适配器搬进 PS 领域。** 适配器 `appliedControlAssessment` 改为全函数翻译：SA `ExecutedControls` 逐项译成 `ControlItemResult`（种类 × 顺序 × 有没有受限；受限原因从同一答复的 `Freeze()` / `Exposure()` 按种类取），`JointPassCondition` 译成本上下文的条件，交构造期推结论。ADR-0122 决定四写的「折叠在 PS 的 `adapters/settlementaccounting`」自此位置变、归属不变——仍是 `parcel-shipment` 折，只是折在它的领域层而不是适配器里。ADR-0122 正文不改。

**四、释放按「有没有成立的项」发，不再按结论。** `FinancialControlResult.OccupationFormed()` 在任一项`成立`时为真；三处释放改按它发。依据：SA 释放对同一请求身份在两本账各认领一次、限制不入账本（ADR-0122 决定三），所以「有成立项就发」既不会漏掉另一本账上的占用，也不会对着一笔不存在的占用重试。Context 里两处孤儿由此修掉；`AT-PS-035`「接受提交未成立时按原关联释放冻结」的判据补上「已成立的项不因接受侧结论而免释放」。

**五、失败处置与责任引用：经 PS 自己的 PC 消费缝读、只读绑定到本次委托费用范围的那几行、只在有项受限时读——但本记录不建这条读路。** 不随 SA 走已由 ADR-0122 决定四钉死；读的缝是 PC 的 `PreAcceptanceFinancialControlPolicyContentView`（形状已在）；取项口径与 SA 执行时同源——闭包里已采用结算政策的 `Applicability().ChargeScope()` → `ItemsFor`，别的范围上的项没执行过、管不了这份委托的去向；通过的控制没有去向问题。处置的消费方是 UC-PS-001 说的「进入授权处置」那条流程，本上下文今天没有：授权角色能对不通过的控制做什么（CONTEXT「人工处理不得绕过硬规则或把缺少的权威结果改成通过」）、它是不是一种新的等待态、读面与续办都未裁。没有消费方的读路进了没人读——与 ADR-0122 决定五不带信用政策版本引用同一判据。读口与流程同票落地（[sa-preacceptance-policy-view/04](../../.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md)）。

**六、「未执行」与「执行且成立」不分。** 「未执行」不是一项结果：没有结论、没有依据、没有占用；把它记成一项，就得让 PS 读策略正文来列全项，为的是一个在「全部通过」之下改不了接受判断的信息。PS 只记已执行的项（SA `ExecutedControls` 原样译回，含顺序）；「哪些没跑」由读的人算：`RecordedJudgments.AdoptedCommercialResolution` → 闭包 → 采用的控制策略版本 → 正文项 − 已执行项，这条路今天就是通的。放宽出第二种组合子、「未执行」开始影响结论时再议。

## Consequences

- `sa-preacceptance-policy-view/03` 落地：领域形状与推导、适配器改翻译、三处释放改 `OccupationFormed`、登记册父表加共同通过条件列 + 子表 `acceptance_financial_control_item`（迁移 `0019`）、HTTP 读面加 `jointPassCondition` 与 `items`、PS CONTEXT 加词条「接受前财务控制采用结果」与「控制项结果」、UC-PS-001 校验组行与 `AT-PS-035` 改口。
- **过渡状态明记**：票 04 落地前 `RESTRICTED` 仍译确定性不通过 → 拒绝，等于替每份合同按 `REJECT` 处置；租户若在正文里登记 `AUTHORIZED_DISPOSITION`，委托会被自动拒绝而不是进入授权处置。这是机制半边未齐，不是产品口径；首发没有租户，风险停在纸面，但它不该被读成「已确认 `RESTRICTED` 即拒」。
- 共同通过条件放宽出第二种组合子时，会先炸的地方从两处变成三处：SA 编排的穷举、PS 适配器的翻译、PS 领域的推导——都按新组合子另判，不得默认按全部通过。
- 生产路径行为不变：租户配置未齐时接受前控制仍停在`未配置`（ADR-0054 / ADR-0122），走不到本记录改的任何一格。
- 越权风险点单列在票 03「裁决」节，供 owner 复核：ADR-0047 两格未 supersede；ADR-0122 决定四折叠位置那句字面过时；「未执行可算」依赖 SA 停在首处限制与闭包可查；释放触发以 SA 账本语义为依据。

## Alternatives considered

- **并列一个新对象装逐项与条件，既有编排继续读旧 `FinancialControlResult`。** 否决：折叠仍在适配器（判断留在了只该翻译的地方），逐项成了没人读的装饰，两个对象靠约定保持一致——而 ADR-0122 留的缺口正是「PS 域里装不下按哪个条件判」。
- **结论合成一格「通过」，去掉 `HELD` / `CREDIT_EXPOSED`。** 否决于本票：它改 ADR-0047 决定三的词汇、迁移 `0005` 的 CHECK 与所有读面，而两格降为投影、结论只从逐项推出之后，冗余已经没有第二个来源；合不合是 ADR-0047 的改动，单列为越权风险点。
- **保留四参构造器作兼容。** 否决：它就是「结论与逐项可以不一致」的入口，且从结论反推逐项（`HELD` → 一项预付冻结）是在发明事实。
- **释放仍按 `HELD` 发，另给 `CREDIT_EXPOSED` 加一条。** 否决：组合策略下 `RESTRICTED` 前面的成立项仍是孤儿；「有没有占用」是逐项的事实，不是结论的事实。
- **本票就建 PS→PC 读处置的适配器，`AUTHORIZED_DISPOSITION` 先停未决。** 否决：停在哪一格、谁续办、读面在哪都未裁，一个没有续办方的未决与一个没人读的读口同样是「进了没人用」；且它会让今天所有 `RESTRICTED` 的确定性拒绝（`AT-PS-035`「确定失败时不接受」）在处置读不到时也停下。流程与读口同票（04）。
- **记「未执行」项。** 否决：没有结论与依据的「项」；要列全项得读正文，为的是「全部通过」之下改不了结论的信息；可由已采用解析算出。

## Links

- 票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)：三问、取证与越权风险点——本记录的出处；[04](../../.scratch/sa-preacceptance-policy-view/issues/04-authorized-disposition-flow-and-failure-disposition-read.md)：决定五留给授权处置流程的后继
- [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)：SA 逐项交回、释放两本账各认领、折叠归 PS——本记录承接它的决定四，把折叠位置从适配器搬进领域
- [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：失败处置「只答委托去向」与共同通过条件在父行——决定五、六的依据
- [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：`CREDIT_EXPOSED` 第四格与两本账——决定二保留其词汇、决定四换掉「释放编排按结论找账本」那一半
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：适配器只翻译不判断、封闭集全函数、集外不吸收——决定一、三的依据
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建与构造分属两扇门、重建只校验不重算——决定二「重建门校验所记结论与逐项一致而不覆盖」的依据
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「只采用预付冻结、信用校验、明确无控制或合同明确组合所形成的权威结果与依据」；[settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「由 `parcel-shipment` 按策略的共同通过条件形成接受判断」
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：校验组「接受前财务控制」行与 `AT-PS-035`——本记录改口的两处
- `internal/parcelshipment/domain/acceptance_basis.go`、`domain/judgment_translation.go`、`application/form_acceptance_decision.go`、`application/reject_shipment_request.go`、`application/withdraw_shipment_request.go`、`adapters/settlementaccounting/pre_acceptance_control.go`、`adapters/postgres/acceptance_judgments.go`、`adapters/http/query_acceptance_review_queue.go`、`migrations/parcel_shipment/0019_acceptance_financial_control_items.sql`：决定一至四的落点
- 来源：IDP 队列通道 1 的授权（2026-09-07）
