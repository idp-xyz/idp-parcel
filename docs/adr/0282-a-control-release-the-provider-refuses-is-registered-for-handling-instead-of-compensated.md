# ADR-0282：提供方确定不受理的控制释放落「释放未受理册」待人处理，不进补偿续办——释放端口答已收口、可重试未形成、确定未受理三格，接受链遇拒收响亮失败，释放按施加时存下的作用域认领

Status: Accepted（2026-10-26，Claude Code 会话代 PS owner 裁定：用户原话「你作为业务和系统专家自决」；协调方据此裁定撤回、拒绝、处置几路另立待人处理格，接受链照 ADR-0081 决定三响亮上抛，作用域随控制结果存下；作者实施时停下回报这几条与已接受记录相抵，2026-10-26 协调方裁定 (A)，走本记录；票 [docs-consistency/code/71](../../.scratch/docs-consistency/code/71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md)，出处票 code/20 隔离评审越权风险点 1 与 N6。部分停用（收窄）[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一一句与决定四一句，部分停用（扩成）[ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定三一句，被停原句与适用场景见「对已接受记录的作用」；两份正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 PS owner 复核，SA owner 会签。）<!-- 索引注记 -->｜**三格**：释放答已收口（已释放、无可释放）、可重试未形成（账本读不回或存不回、依赖答不出）、确定未受理（提供方拒收）；错误只留给形不成三格之一。**终局与暂停**：撤回、主动拒绝、授权处置两个去向、自动拒绝、停进等待受控补充遇确定未受理，业务事实照常成立，那一笔记进释放未受理册、结果带出这一格、不留补偿引用；形不成三格之一同落此格；记不下册改留补偿续办。**接受链**：放被取代各版遇确定未受理响亮上抛、落 `publish_failed`。**作用域**：施加时随控制结果存下，释放按它认领、不问作用域源；存量无作用域行交零值，由提供方答未受理。**控制端口**：译不动的答复上抛，不再折成依赖不可用
Date: 2026-10-26

## Context

- `parcel-shipment` 的释放端口（`ports.PreAcceptanceControlRelease`）只交回一个 `error`。消费侧适配器把 `settlement-accounting` 的`未受理`（`ReleaseRequestNotAccepted`，提供方在读任何账本之前就拒收了请求）译成编程错误，把作用域没形成译成普通错误；到了编排都是 `err != nil`，一律折成 `ControlReleasePending`。
- 这一格的恢复动作不是重试。撤回、拒绝、处置这几路交回的补偿续办引用没有持久记录，也没有东西会按它重放；被拒收的那一笔重放一万次仍是同一个答复。接受链开头放被取代各版（票 code/20）时，它让新版本停在内部重试，消费门回滚重投到失败预算耗尽、信封死信。
- 接受前财务控制那一口的业务答复已是封闭集，但适配器译不动的答复（提供方答出词汇表外的东西，或刚施加的控制交回已释放的冻结之类阶段契约被打破）同样经 `error` 交回，财务控制那一步一律折成 `FINANCIAL_CONTROL_UNAVAILABLE` 内部重试。
- 释放那一刻，适配器经作用域源从商业解析回显与结算账户登记重新推一次资金作用域（`FormControlScope`），而不是取施加时用过的那一份。施加之后结算政策回显或账户登记变了，推出来的就是另一份作用域：提供方在那里找不到这一笔、答`无可释放`，本上下文当成已收口，原占用静默成孤儿（code/20 评审 N6）。`cmd/parcel-api` 的撤回、主动拒绝、授权处置三处释放口根本没接作用域源，有占用要放时必然留补偿。
- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一与决定四把授权处置的释放失败一律交给补偿续办（内部重试）。[ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定三列了接受链里保持 `publish_failed` 响亮的三格。UC-PS-005「给开发的交接」写着：撤回提交和财务释放「不得通过分布式事务绑成一个可阻塞决定，也不得把释放失败改写为撤回失败」。

## Decision

**一、释放端口答封闭三格；错误只留给形不成三格之一。**（机制）

- 三格按调用方的恢复动作分（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）：`已收口`——已释放，或这一关联下无可释放；`可重试未形成`——账本读不回或存不回、依赖答不出，重放同一请求可能放得掉；`确定未受理`——提供方拒收了请求（身份或作用域立不起来），重放得到同一个答复。非`已收口`时带提供方的原因引用。
- 适配器逐格翻译，不留兜底；提供方答出集合之外的值、或它自己没在封闭集合里作答，交回错误，包着端口级哨兵 `ErrUntranslatableProviderAnswer`。依赖答不出不作错误，在`可重试未形成`里答。
- 接受前财务控制那一口同此：译不动的答复交回的错误包着同一个哨兵，财务控制那一步原样上抛，不折成 `FINANCIAL_CONTROL_UNAVAILABLE`；依赖调不通照旧落那一格。
- 释放口外那一层 savepoint 对非`已收口`的答复与错误都退回，开、退、收 savepoint 本身失败按`可重试未形成`交回。

**二、终局各路与暂停那一路遇`确定未受理`：业务事实照常成立，那一笔记进释放未受理册待人处理，结果带出这一格，不留补偿续办引用。**（机制）

- 适用的路：撤回、主动拒绝、授权处置的`拒绝`与`交客户补充`两个去向、自动拒绝、停进`等待受控补充`。各路同一判法，经同一处释放定义；ADR-0132 决定一「不因去向不同而不同」的本意照旧。
- 不回滚、不答 5xx：撤回或拒绝本身已成立，答 5xx 会让调用方以为没成立而重试（UC-PS-005「不得把释放失败改写为撤回失败」）。
- 释放未受理册只追加，一笔占用一行，主键是（租户，委托，提交版本，控制结果）；记客户账户、施加时存下的作用域（没存下即空）、原因与登记时刻。同一笔再被拒收答已有记录。登记与那件业务事实同笔落地。
- 结果带出这一格：撤回、主动拒绝、授权处置三口的 HTTP 答复加 `controlReleaseNotAccepted`，与补偿续办引用分开——那一笔重放推不动，留引用只会让调用方以为系统会把它补上。
- 端口形不成三格之一时这几路同落此格，原因是本上下文自有的 `CONTROL_RELEASE_ANSWER_UNTRANSLATABLE`：上抛会让已成立的业务事实随整笔回滚。
- `可重试未形成`照旧走 `ControlReleasePending`、留补偿续办引用。
- 记不下册（没接上或写不进）时那一笔改留补偿续办，原因 `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`，与`释放未收口`各派各的引用。

**三、接受链开头放被取代各版遇`确定未受理`（或形不成三格之一）：响亮上抛，落 `publish_failed`。**（机制）

- 不折成内部重试：重投推不动它，新版本却一直被挡着。上抛 `ErrControlReleaseNotAccepted`（或包着端口哨兵的原错误），不进 `acceptanceChainUndecidedSentinels`，消费门不入账。
- 这一步不记册：这一轮整笔回滚，记下的会随之撤回。可续办的缺口与拒收同时出现时也上抛。

**四、释放按施加时存下的作用域认领，不在释放时重新推导。**（机制）

- 施加时用的那一份资金作用域（责任法人、结算账户、币种三段）随控制结果交回、随结果落库；释放请求带上它，适配器不再问作用域源。`明确无控制`没有占用，也没有作用域。
- 存量结果（施加时还不存作用域）读回作用域为空，释放照样交零值给提供方，由它短路答`未受理`，落进决定二或决定三的格。不回退到现查：现查正是要堵的那一格。

## 候选与反方

- **撤回、拒绝、处置几路遇拒收响亮上抛。** 否决：业务事实已经越过提交边界，上抛会让它随整笔回滚、答 5xx，调用方据此重试一个已经成立的撤回，释放失败改写了业务结果。
- **拒收照旧留补偿续办引用。** 否决：那条引用没有持久记录，也没有东西会按它重放；被拒收的请求重放得到同一个答复，补偿永远收不了口，这正是本票要修的缺口。
- **接受链遇拒收也记册、再按未决入账。** 否决：入账之后新版本照判，旧占用还压着新版本的控制，受限即可能按策略拒单——拒绝不可覆盖。停在内部重试又推不动。响亮失败让人先来处理那一笔。
- **释放时现查作用域，查到的与施加时不同就答确定未受理。** 否决：要比就得先存下施加时那一份；存下了，直接按它认领更直接，也不必等两份不同时才发现。
- **存量无作用域行回退到现查。** 否决：现查就是 N6 那一格；首发前只有演示租户，存量随 `--reset` 重灌即消，回退分支会让释放路径保留两种认领法。
- **释放端口不留错误，答复落不进三格就算确定未受理。** 否决：适配器的 default 不得吸收（ADR-0025）；是否把它落进待人处理格是编排对各路的判断，不是翻译。

## 对已接受记录的作用

- **部分停用（收窄）** ADR-0132 决定一「释放失败按既有补偿续办（`ControlReleasePending`，内部重试），不因去向不同而不同。」一句。**适用场景**：提供方确定不受理的释放，与释放口形不成三格之一的那一格——两个去向都记进释放未受理册待人处理，不进补偿续办；可重试的释放失败照原句。「不因去向不同而不同」照旧成立，决定一其余各句不变。
- **部分停用（收窄）** ADR-0132 决定四「补偿（释放失败）续办**不用**它：释放按原业务关联请求向 `settlement-accounting` 发、续办方是系统（内部重试），责任方是谁不改变谁来重试；本上下文也不据它裁任何费用、追偿或通知——那些各归 `settlement-accounting` 与商业侧。」一句里「续办方是系统（内部重试）」那一半。**适用场景**：同上，那一笔交人处理；补偿续办不用责任引用、本上下文不据它裁费用追偿通知与决定四其余各句不变。
- **部分停用（扩成）** ADR-0081 决定三「消费门的失败分格按 [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 第三条的既有纪律：链的未决哨兵（`psinbox.ErrAcceptanceChainUndecided`）在路由条目处翻译成 `dispatch.consumer_undecided`；封闭集合外、装配缺件、空成员清单三格保持 `publish_failed` 响亮——它们重投不自愈，折进未决会重投到失败预算耗尽。」一句。**适用场景**：保持 `publish_failed` 响亮的格加两种——放被取代各版时提供方确定不受理（`ErrControlReleaseNotAccepted`），与接受前财务控制、释放两口答出译不动的东西（包着 `ErrUntranslatableProviderAnswer`）；理由同原句「重投不自愈」，决定三其余各句不变。
- **不停用** [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md) 决定五：「回指与资金作用域同出一次解析，同进同出」与「释放命令不带回指」照旧；末句「回指在释放路径上取到了却不用，正是同源的代价，也是它的证据。」描述的是写下时释放路径现查作用域的现状，本记录决定四之后不再成立，在其索引注记挂读法注。

## Consequences

- 代码随票 code/71：端口答复改形、适配器逐格翻译与释放按存下作用域认领、`occupationRelease` 逐格分派、财务控制那一步与接受链开头那一步上抛、释放未受理册的端口与真库适配器、三口 HTTP 答复加一格、管理台撤回页与处置摘要显示这一格；迁移 `parcel_shipment/0030`（控制结果加三列作用域，新表 `control_release_refusal`）。
- `cmd/parcel-api` 的撤回、主动拒绝、授权处置三处释放口不再需要作用域源：占用落在存下的作用域里，今天就放得掉，不再必然留补偿。
- `FINANCIAL_CONTROL_UNAVAILABLE` 只剩依赖调不通一种来由。停在`等待受控补充`、占用已释放的那一版被再驱一拍时（控制那一步重施加，提供方交回已释放的冻结），链改为响亮失败，不再内部重试到失败预算耗尽。
- 释放未受理册只有按委托的读口，没有运营查阅面，也没有处理动作；人怎么处理那一笔（核对后在结算侧释放、补登作用域重放）不在本记录内。
- 未决原因集合多一格 `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`，只出现在补偿引用里。

## 越权风险点

1. **册上的积压无人看见。** 今天只有按委托的读口；没有队列、没有告警，也没有处理之后的收口动作。一笔被拒收的占用会一直占着客户的余额或额度，直到有人查到它。
2. **存量无作用域行一律交人。** 施加时还不存作用域的结果，终局各路落册，接受链开头那一步响亮失败、那份委托的新版本判不了。首发前只有演示租户，靠 `--reset`；有了真实租户再换口径，要一条补登作用域的运维路径，今天没有。
3. **终局各路把「端口形不成三格之一」也落册而不上抛。** 这与「破坏不变量的失败仍然上抛」的通行纪律相反一格，依据是 UC-PS-005「不得把释放失败改写为撤回失败」。编程错误因此不会让撤回答 5xx，只会在册上多一行 `CONTROL_RELEASE_ANSWER_UNTRANSLATABLE`，要有人去看册才发现。
4. **接受链响亮失败之后怎么续。** 那一封信封耗尽失败预算后死信；人处理完那一笔之后，新版本要靠重投死信或另一封信封重驱，本记录没有为它立续办触发。
5. **记不下册时留的补偿引用仍无人推。** 终局各路的补偿引用本就不持久（code/72 越权风险点、code/73 越权风险点 1），记不下册退回这一格，只比静默好一点。
6. **认领依赖提供方不跨作用域迁移占用。** 存下的作用域是施加那一刻的；结算侧日后若合并账户、把冻结迁到另一个作用域，按存下那一份认领会答`无可释放`，那笔占用照样成孤儿——方向与 N6 相反。今天结算侧没有这类迁移。
7. **控制端口译不动改为响亮。** 提供方一次形状变化会让每一份走到控制那一步的委托都落 `publish_failed`，而不是安静地内部重试；运维看到的是成片的失败，不是一条一直在等的依赖。

## Links

- 票 `.scratch/docs-consistency/code/71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md`；出处票 `.scratch/docs-consistency/code/20-superseded-submission-version-keeps-its-control-occupation.md`（评审记录 N6 与越权风险点 1）
- [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md)：部分停用（收窄）决定一一句与决定四一句的一半，可重试的释放失败照旧补偿续办
- [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md)：部分停用（扩成）决定三一句，`publish_failed` 响亮格加两种
- [ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：决定五末句的现状随本记录决定四不再成立，无条款停用
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：翻译是全函数，default 不吸收
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：已形成的业务答案答 2xx，业务判别进响应体
- [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)：决定三释放两本账各认领、幂等归提供方账本
- [小包托运上下文](../domain/parcel-shipment/CONTEXT.md)：词条「控制项结果」
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)、[UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)：给开发的交接
