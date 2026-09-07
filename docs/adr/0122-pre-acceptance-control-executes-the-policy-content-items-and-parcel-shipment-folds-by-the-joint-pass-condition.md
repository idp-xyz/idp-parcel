# ADR-0122：接受前财务控制按策略正文的控制项逐项执行——`settlement-accounting` 的控制策略答复长成「控制项集合 + 共同通过条件 + 采用的控制策略版本」，控制种类选路、判断顺序定序、结算方式只作保存；费用范围取自已采用结算政策的适用范围；释放两本账各认领；多项结果怎么合起来看由 `parcel-shipment` 按共同通过条件折

Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 授权自决」口径由本会话代裁票 [sa-preacceptance-policy-view/02](../../.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md) 五问并实施。裁决能力边界：读过该票全文、票 01、[ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 全文、`settlement-accounting` `CONTEXT.md` 词条「接受前财务控制结果」与 Rules 中关于接受前控制的四句、`internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`、`application/apply_pre_acceptance_control.go`、`application/release_pre_acceptance_control.go`、`domain/pre_acceptance_control.go`、`domain/funds_freeze.go` 的 `Freeze` / `FindByRequest`、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`、PC 侧 `domain/pre_acceptance_financial_control_policy.go`、`domain/reference_closure.go` 的 `ResolveCommercialClosure` / `AdoptedBasis`、`domain/settlement_policy.go` 的 `SettlementApplicability` / `SettlementSelector`、`cmd/parcel-dispatch/assemble.go` 的 SA 装配段；**未重读** `UC-SA-001` 全文、PS 接受编排读取控制结果的那一段与 SA 的 HTTP 读面。因此本记录只裁 SA 结果形状、执行顺序、费用范围来源、释放认领与 PS 消费适配器怎么折五件；PS 域对逐项结果的表达不在此，另立票）
Date: 2026-09-07

## Context

ADR-0115 让接受前财务控制策略版本有了正文——控制项逐行（种类 × 适用范围 × 判断顺序 × 失败处置 × 责任），父行持共同通过条件——并明写 Decision 五：「`settlement-accounting` 的 `LoadControlPolicy` 一字不改……改为经本点读口读『要执行哪些控制项』属 SA 地盘，另立票。那一票落地前，接受前控制链在生产上的行为一字不变。」本记录就是那一票的决策。

改之前 SA 这一侧是这样的（取证锚 `61344989`）：

- `sadomain.PreAcceptanceControlPolicy` 的`要求`形状是**一种结算方式 + 一份结算政策引用**；`ApplyPreAcceptanceControlHandler.Handle` 按 `policy.Method()` 分支——预付走冻结账本、账期走暴露账本——**方式即选路**（ADR-0047）。
- 适配器 `policyFrom` 从闭包里已采用的结算政策取方式，从不读任何策略版本正文；它自己的注释写着「由本适配器从声明反推方式，就会从账期倒推出无需信用校验，那是 `pn-02-w03` 明禁的那条推导」，然后从结算方式推了控制方式——结算方式与控制方式是两件事，那句禁令管的正是把前者当后者。
- 于是 CONTEXT 允许的组合（「合同明确组合多项接受前控制时，每项结果必须保持独立依据和有效性」）说不出话：一份账期合同同时要求预付保证金冻结、一份预付合同同时要求信用校验，旧形装不下。
- `ReleasePreAcceptanceControlHandler` 在冻结账本认领到就停，注释写「一次控制按方式只走了一条路」；PS 消费适配器 `assessmentFor` 见到冻结就译、不再看暴露，注释写「冻结与暴露不会同时在场」。两处都建在「一次请求只走一条路」上，正文一旦允许组合，两处都会把另一本账上的占用漏掉。

正文那侧还留了两个 SA 消费方要答的问题（ADR-0115 Decision 三、四）：控制项按费用范围挂，SA 的键是资金维，费用范围从哪来；控制项只说「做信用校验」不说按哪一版信用政策，那一版由谁取。

## Decision

**一、SA 的`要求`答复长成「控制项集合 + 共同通过条件 + 采用的控制策略版本」，结算方式与结算政策仍随答复带回但不再选路。** `sadomain.ControlKind` 封闭两值 `PREPAID_FREEZE` / `CREDIT_CHECK`（与 PC 正文词汇同名、不 import，纪律同 `SettlementMethod`），集合里刻意没有「明确无控制」——那由合同带依据声明（ADR-0115 决定一），进了这里就是给「默认通过」开一条路。`ControlItem` 只有种类 × 判断顺序；`JointPassCondition` 首发一值 `ALL_CONTROLS_PASS`；`ControlPolicyReference` 指名正文所出自的策略版本。`NewRequiredControlPolicy` 核至少一项、种类唯一、顺序唯一（与 PC 正文的行级约束同形），按判断顺序交回。结算方式与结算政策引用保留在答复上，是因为 CONTEXT 硬句「每项费用、冻结、信用暴露和核销必须保存实际采用的结算政策、预付/账期方式及其适用范围」——它们是结果上要保存的一格，不再是选路的开关。**费用范围不另收一格，取已采用结算政策的 `Applicability().ChargeScope()`**：资金作用域本就是从这份结算政策派生的，它适用的费用范围就是这次解析对上的那一个；再让调用方送一格进来，两处就可以不一致。正文里挂在别的范围上的控制项不进本次答复；本范围一项都没有时答 found=false——合同绑定把范围指到了这份策略、策略却没为它写控制项，那要租户补，不是「无控制」。

**二、编排按判断顺序逐项执行，控制种类决定走哪本账；一次请求共用一个控制时刻；「全部通过」之下一项形成`业务限制`后后续项不再执行。** `PREPAID_FREEZE` 读运营余额、在冻结账本占金额；`CREDIT_CHECK` 读信用状况、在暴露账本占额度——ADR-0047 的两本账互不借用不变，变的只是选路开关。共同通过条件在执行前先穷举（ADR-0025 全函数），集外组合子报错不吸收：一个本上下文还不认识的组合子若等到出现限制那一刻才发现，前面的项已经占了资金。停在第一处`业务限制`的理由是：「全部通过」之下后面的项无论结果如何都改不了共同通过条件的答案，继续执行只会为一笔多半不会接受的委托占更多资金、给释放路径多一处要认领——这是判断顺序存在的意义，不是本上下文在汇总：每一项已执行的结果原样交回，`ExecutedControls` 记录哪几项跑了、各自有没有受限。依赖不可用、请求冲突、未受理时整个请求停在那一步，已执行的项**不回滚**：两本账对同一请求身份幂等（重放交回原冻结 / 原暴露），续办从头重走一遍不会二次占用，回滚反而要发明一条本上下文没有的补偿路径。

**三、释放两本账各认领一次。** 同一请求身份在两本账上都可能有占用，认领到一本就停会把另一本上的占用留成永远释放不掉的孤儿；两本都没有才是`无可释放`。冻结那本已落库的释放在暴露账本读不回时不撤回：续办重放，冻结账本按幂等交回原释放答案，暴露账本再认领一次——两本各自收口，不必同事务。

**四、失败处置与责任不随 SA 答复走；多项结果怎么合起来看，由 `parcel-shipment` 在自己的消费适配器里按共同通过条件折。** ADR-0115 说失败处置「只答委托去向，不拥有拒绝决定」——去向是 PS 的事，SA 执行控制不需要它，SA 带着它就成了 PC 内容的转手站（第二条读路）；PS 要用时经自己的商业缝读。CONTEXT 写死「由 `parcel-shipment` 按策略的共同通过条件形成接受判断」，所以 SA 只把每项结果与条件原样交出，折叠在 PS 的 `adapters/settlementaccounting`：「全部通过」之下任一项受限即 `RESTRICTED`（带那一项自己的原因），全部成立时有冻结即 `HELD`、只有暴露即 `CREDIT_EXPOSED`——两者并存时以资金已被占用那一句为准，暴露那一项仍留在提供方账本、由同一请求身份释放；集外组合子同样报错不吸收。PS 域的 `FinancialControlResult` 今天仍是一个请求一个结果，逐项结果在 PS 的表达是另一张票（[sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)），本记录不替它发明第二种结果形状。

**五、信用政策版本本记录不取。** `CREDIT_CHECK` 今天照旧对 `CreditStandingView` 交回的额度与逾期状态执行；哪一版信用政策决定那份额度，是信用状况那一口提供方（SA→PC 信用缝，今天未接）的事，不在控制项上带一个没人读的引用。ADR-0115 Decision 四留的「闭包解析 vs 版本壳指名引用」两条路，到接那条缝时再选。

## Consequences

- `sa-preacceptance-policy-view/02` 落地：领域形状、编排逐项执行、释放双账认领、SA→PC 适配器改读正文（装配收第三个只读半边，`parcel-dispatch` 接 `NewPreAcceptanceFinancialControlPolicyContents`）、PS→SA 适配器按条件折叠；真库用例覆盖正文读回、组合按序、只取本范围、正文缺失与闭包未采用两向、三半缺一装配即拒。
- `pn-02-w03`「结算模式不等于接受前财务控制策略」自此被结构守住：`要求`格里没有任何一处从结算方式派生控制方式；`0007` 说要求而正文未登记停 `CONTROL_POLICY_NOT_CONFIGURED`，**不留**「沿用结算政策推方式」的过渡——恢复动作是租户补正文，与 ADR-0054 的`未配置`格同一恢复方向。
- 生产路径行为：租户登记的解析键若未要求 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`、或合同未绑、或正文未登记，接受前控制停在`未配置`——首发没有租户时这是唯一走得到的分支，与改前「停在 `CONTROL_SCOPE_NOT_CONFIGURED`」同属实例半边未齐的诚实停点。
- 后继：PS 域逐项结果的表达（票 03）；SA→PC 信用缝接通时再定信用政策版本取法；共同通过条件放宽出第二种组合子时，SA 编排的穷举与 PS 适配器的折叠两处都会先炸，各按新组合子另判、不得默认按全部通过。
- SA CONTEXT 各句不改：「只执行适用合同策略明确要求的控制项」「不得把多个结果汇总成委托接受或拒绝决定」「由 `parcel-shipment` 按策略的共同通过条件形成接受判断」正是本记录逐条落成代码的那三句。

## Alternatives considered

- **SA 结果形状不改，适配器把多项控制折成一种方式交给旧编排。** 否决：折叠发生在提供方侧，等于 SA 替 PS 按共同通过条件判了；且旧编排一次只走一本账，组合里的第二项永远执行不到。
- **让调用方（PS）在命令上多送一格费用范围。** 否决：费用范围已在闭包里已采用结算政策的适用范围上，与资金作用域同源；多送一格就允许两处不一致，而适配器没有任何依据裁哪个对。
- **一项受限后继续执行余下各项，把全部结果交回。** 否决：「全部通过」之下余项改不了答案，只多占资金、多留释放认领点；判断顺序的意义就在于此。若将来有「任一通过」组合子，那一格由它自己的分支判，穷举已保证不会静默沿用本条。
- **第二项停下时回滚第一项。** 否决：账本对同一请求身份幂等，续办重走即可；回滚要一条本上下文没有的补偿路径，且释放语义（`已释放`留审计）与「当没发生过」不是一回事。
- **SA 答复携带失败处置与责任引用。** 否决：它们答委托去向，SA 执行控制不需要；带上就是 PC 内容的第二条读路，PS 经自己的商业缝读。
- **控制项上带信用政策版本引用（按闭包解析或壳指名取）。** 否决：今天没有任何消费形状读它——`CREDIT_CHECK` 对 `CreditStandingView` 执行，那一口提供方未接；进了没人读，与 ADR-0104 / 0115「不进首发」同一判据。
- **释放仍认领到一本就停。** 否决：组合策略下另一本上的占用成孤儿；两本各认领一次、幂等各自收口的代价只是多读一本账。

## Links

- 票 [sa-preacceptance-policy-view/02](../../.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md)：五问、改前读路径与提供方形状的取证——本记录的出处；[01](../../.scratch/sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md)：同一条缝的回指半边
- 票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md)：决定四留给 PS 域的后继
- [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：正文形状与 Decision 三、四、五——本记录答的正是它留给 SA 消费票的三件
- [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：两本账互不借用——决定二保留的那一半；「方式即选路」是被本记录换掉的那一半
- [ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：`未配置`格与恢复方向——正文未登记停在同一格
- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md) / [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：结算方式是解析输出、SA 凭回指问闭包——决定一保留方式与结算政策引用、从闭包取费用范围的依据
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：两侧封闭集全函数翻译、集外报错不吸收——决定一、二、四三处穷举的依据
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「只执行适用合同策略明确要求的控制项」「不得把多个结果汇总成委托接受或拒绝决定」「由 `parcel-shipment` 按策略的共同通过条件形成接受判断」「每项冻结与信用暴露必须保存实际采用的结算政策、预付/账期方式及其适用范围」
- [`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md)：「结算模式不等于接受前财务控制策略」——本记录 Consequences 第二条所指
- `internal/settlementaccounting/domain/pre_acceptance_control.go`、`application/apply_pre_acceptance_control.go`、`application/release_pre_acceptance_control.go`、`adapters/partycommercial/pre_acceptance_control_policy.go`、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`：决定一至四的落点
- 来源：IDP 队列通道 1 的授权（2026-09-07）
