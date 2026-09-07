# ADR-0115：接受前财务控制策略版本的正文归 `party-commercial`，只登**要执行的控制项**——「明确无控制」独由客户合同声明，不进策略正文；控制项逐行成表（种类 × 适用范围 × 判断顺序 × 失败处置 × 责任），共同通过条件在父行、首发封闭为一值；`settlement-accounting` 的读路径不随本记录改

Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 派工并授权本会话「逐票代裁并实施」。裁决能力边界：读过票 [party-commercial-context-gaps/07](../../.scratch/party-commercial-context-gaps/issues/07-pre-acceptance-financial-control-policy-has-no-content-table.md) 全文（含四问与倾向）、票 [admin-write-faces/06](../../.scratch/admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 与 [admin-write-faces/07](../../.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md) 票面、`party-commercial` `CONTEXT.md`「接受前财务控制策略」词条与 Rules 中关于它的三句、`migrations/party_commercial/0007`、`0012`、`0020`、`0021`、`0023` 全文、`internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go` 全文、ADR-0042 / 0044 / 0054 / 0077 / 0079 / 0101 / 0104、`settlement-accounting` `CONTEXT.md`「接受前财务控制结果」词条与「只执行适用合同策略明确要求的控制项」那句、`UC-PS-001`「接受前财务控制」一行、[`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md) 的 `PAR-COM-15` 节、`internal/partycommercial/domain` 的 `customer_contract.go` 与 `customer_service_rule.go`；未重读 `UC-SA-001` 全文、SA 接受前控制编排与 PS 接受编排代码——本记录因此只裁**正文归谁、正文长什么样、与合同两层声明怎么分工、SA 读路径的分界**四件，不改 SA 任何一格、不动 `0007` 与 `0012`）
Date: 2026-09-07

## Context

`party-commercial` `CONTEXT.md` 把接受前财务控制策略判给本上下文，词条写明它「定义适用范围、共同通过条件和失败处置，但不拥有实际估价、余额、冻结或信用暴露结果」；Rules 写死了组合控制的结构：「策略必须明确每项控制的适用范围、判断顺序、共同通过条件和失败或补偿责任；某一范围明确无控制不能静默取消其他范围已经规定的控制」，并禁止「硬编码为永久互斥的三选一枚举」。

代码今天是这样的（票 07 取证锚 `08e62ec`，本记录复核了下列几处）：

- `CommercialObjectKind` 第 5 类 `PreAcceptanceFinancialControlPolicyObject` 在库里**只有 `commercial_version` 壳**：能发布、能被客户合同引用，`migrations/party_commercial/` 下没有它的正文表。
- 合同那一侧已经有**两层**关于控制的声明，都挂在客户合同版本（`object_kind=2`）下：`0007` 答合同版本级「这份合同**要不要**接受前财务控制」，`不适用`时必带 `not_applicable_basis`；`0012` 的 `customer_contract_control_binding` 按费用范围答「这个范围**适用哪份策略**（`policy_id`）还是**显式不适用**（`inapplicability_basis`）」，两列恰一非空。两处的「无控制」都带依据，都是合同层面的话。
- SA 的消费侧适配器 `LoadControlPolicy` 分三段：回指换闭包 → 闭包取已采用的客户合同 → 按合同读 `0007` 声明；`要求`那一格的**方式**取自同一份闭包里已采用的结算政策（ADR-0044 / ADR-0079 的分工），**从不读任何策略版本正文**——因为没有正文可读。

于是今天「控制怎么做」实际上是从结算方式派生的：预付 → 冻结、账期 → 信用暴露（ADR-0047）。这条派生对**单项控制**碰巧成立，对 CONTEXT 允许的**组合**说不出话：一份账期合同同时要求预付保证金冻结、一份预付合同同时要求信用校验，今天都写不进任何一张表；而 [`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md) 明写「结算模式不等于接受前财务控制策略：账期不能推导无需信用校验，预付不能推导冻结金额就是最终费用」。缺的不是数据，是承载「要执行哪些控制、按什么顺序、怎么算共同通过、失败了怎么办」的形状——正文表是缺口本体，票 07 立票时已经这样判，票 06 据此转 blocked。

与 [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md) 是同一形：CONTEXT 有语言、代码只有壳、形状由硬句推得出、取值属实例半边（`PAR-COM-15` 待提供）。机制半边现在就做。

## Decision

**一、正文归 `party-commercial`，这是重申 CONTEXT 不是新裁；正文只表达要执行的控制项，「明确无控制」不是正文的一项。** 控制种类首发封闭两值：`PREPAID_FREEZE`（预付冻结）与 `CREDIT_CHECK`（信用校验），逐字对应 CONTEXT 词条点名的前两种。第三种「明确接受前无财务控制」**独由客户合同版本声明**——合同版本级走 `0007`，按费用范围走 `0012`，两处都必须带不适用依据，且这条纪律不变。理由有两层：其一，依据的语义是「这份合同凭什么不控制」，那是合同层面的话，策略版本回答不了；其二也是要害——`0012` 让一个费用范围经 `policy_id` 指名一份策略而**不必**给不适用依据，若策略正文里存在一种「无控制」，一个范围就能通过指名它而绕开依据，那正是 CONTEXT 明禁的「用缺失结果或默认通过代替」。所以封闭集里不能有那一格；一版策略至少一项控制（零项照 `0014` / `0023` 的判据：不是显式空约定，是不登记）。控制种类是**行的键**而不是父行上的一个枚举列——「不得硬编码为永久互斥的三选一枚举」在这里的落法是：一版策略可以有多行，集合以迁移放宽 CHECK 而扩，永远不是三选一。

**二、组合以子表逐行表达，父行只持共同通过条件。** 每项控制一行：控制种类 × 适用范围引用 × 判断顺序 × 失败处置 × 失败或补偿责任引用。同一版策略内**判断顺序唯一**，**（种类 × 范围）唯一**——同一范围同一种控制出现两行，答不出该按哪条。选子表而不是一份结构化文档，理由与 `0023` 头注同一条：「判断顺序」「某一范围明确无控制不能静默取消其他范围已经规定的控制」两句要逐项可查、逐项可约束，顺序唯一与键唯一都是行级 CHECK 能守、文档列守不住的东西；本上下文反复否决过「一个数加一列标记」与整份 JSON 正文，不在这里破例。

**三、槽位的取值形态——机制只给槽，取值由 `PAR-COM-15` 供，任何一格不拟默认。**

| 槽 | 形态 | 出处 |
|---|---|---|
| 适用范围 | `ChargeScopeReference`，开放引用、不设外键 | 与 `0012` 的绑定键、结算政策的费用范围**同一词汇**——消费方按委托的费用范围经合同绑定找到策略，再在策略里找该范围下的控制项，三处必须说同一种话才对得上 |
| 判断顺序 | 正整数，版本内唯一 | CONTEXT「判断顺序」 |
| 失败处置 | 封闭两值：`REJECT`（按策略拒绝）、`AUTHORIZED_DISPOSITION`（进入授权处置） | `UC-PS-001`「任一必需控制不通过时按策略拒绝或进入授权处置；不得默认放行」——两格逐字对应，不多不少 |
| 失败或补偿责任 | 开放引用（非空白） | CONTEXT「失败或补偿责任」；谁承担属实例半边 |
| 共同通过条件 | 封闭集，首发**一值** `ALL_CONTROLS_PASS`，父行必填 | 见下 |

共同通过条件首发只有一值，却仍然成列、仍然必填，理由是**可逆性**：CONTEXT 要求策略「明确」它，`UC-PS-001` 的「任一必需控制不通过」正是全部通过的读法，而 `pn-02-w03` 禁止把「任一结果通过即可接受」当成可以**推导**的东西。今天没有任何消费形状读第二种组合子（SA 的控制策略答复只有一种方式一份采用政策，ADR-0104 「另四项不进首发」的判据同样适用），所以不发明 `ANY`；但若这一列不存在，将来加第二种组合子就得给既有行一个默认值——那正是红线禁的——或者加一列可空并把空读成「全部通过」，那是隐含默认。列在，条件就是租户说出来的；放宽只改 CHECK、既有行一字不动。消费方读它时必须穷举分派，集外取值报错不吸收（ADR-0025 的全函数纪律），所以一个尚未接的组合子不可能静默放行。

**四、正文里明确不放的，逐条写明。** 不放信用政策版本、结算账户、价格依据、金额与阈值——那些各有所有者：信用政策由本上下文的 `CREDIT_POLICY` 版本承载（`0020`）、账户与价格依据属 `PAR-SET-01/02`、金额与阈值属 SA 形成的结果。控制项只说「在这个范围上做信用校验」，**不说按哪一版信用政策**：那一版由消费侧按闭包解析或按版本壳的指名引用取得，两条路选哪条是 SA 消费票的问题，本记录不替它答。不放合同引用：合同 → 策略这层关系已由 `0012` 的 `policy_id` 拥有，正文再写一遍就是同一关系两处定义。不放有效区间：区间在版本壳上，照 `0023` 不抄第二份。

**五、解析走既有闭包，点读口新开一个；SA 的读路径不随本记录改。** `PreAcceptanceFinancialControlPolicyObject` 已在封闭集，`ResolveCommercialClosure` 对它一视同仁。选中之后按版本点读正文：新开 `PreAcceptanceFinancialControlPolicyContentView`，`found=false` 即正文未登记，有父行而零子行是坏数据、走 error 不折成未登记，与 `CustomerServiceRuleContentView` 同形。SA 的 `LoadControlPolicy` **一字不改**：它今天从 `0007` 答「要不要」、从结算政策答「方式」，那两格仍然对；改为经本点读口读「要执行哪些控制项」属 SA 地盘，另立票。**那一票落地前，接受前控制链在生产上的行为一字不变**——本记录只让「控制怎么做」有了一个能被读的地方。

**六、写口随发布同笔登记，形照其余正文册。** `PublicationRegistry` 加一个具名 `Save`、自己的落点类型（重放 / 内容冲突都不是 error、绝不覆盖，ADR-0031），发布用例加一条声明通道，受控 CLI 的批文翻译跟上；事后补正文等于改一份已固定的正文，那要发新版本。

## Consequences

- 票 07 转 ready-for-agent 并落地：一份迁移（父子两表）、领域正文对象与构造门（种类与处置封闭、顺序唯一、键唯一、至少一项）、`PublicationRegistry` 一个具名 Save、一个点读口、发布用例一条通道、受控 CLI 批文一节；**不动 `0007` / `0012`、不动 SA**。
- 票 [admin-write-faces/06](../../.scratch/admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 据此解阻：按 ADR-0077 通例在 `?kind=` 分派加一格，上列版本壳、正文左连接（壳在正文不在必须可见，判据同 `CustomerServiceRuleRow.HasContent`）。
- SA 侧另立一票（`settlement-accounting` 地盘）：`LoadControlPolicy` 的`要求`格改为经本点读口读控制项并翻译成 SA 自己的结果形状；`pn-02-w03` 那句禁推导从那一票起才真正被结构守住。
- `PAR-COM-15` 在参数登记册里的「控制项、共同通过条件、判断顺序、失败处置」半边，其登记面从此在本上下文的策略正文册；「账户和价格依据、金额或额度阈值、冻结释放与补偿条件」半边仍各归 `PAR-SET-01/02` 与 SA。登记册行由其所有者按本记录改口径，本记录不代改。
- `party-commercial` `CONTEXT.md` 词条补两句：策略正文只表达要执行的控制项、「明确无控制」独由合同声明；失败处置只答委托去向（拒绝或进入授权处置）并指名责任方，不拥有拒绝决定。其余各句不变。

## Alternatives considered

- **封闭集里给「明确无控制」留一格。** 否决：Decision 一的两层理由；尤其是 `0012` 让范围经 `policy_id` 绕开不适用依据那一条——放进去就是给「默认通过」开一条合法的路。
- **整份正文一个结构化文档（JSON 列）。** 否决：顺序唯一、（种类 × 范围）唯一都是行级约束，文档列守不住；与 `0023` 头注同一条否决理由。
- **共同通过条件不成列，由产品钉死为全部通过。** 否决：Decision 三——将来放宽要么给既有行默认值、要么把空读成全部通过，两条都是隐含默认；列在，放宽只改 CHECK。
- **共同通过条件首发就给两值（全部通过 / 任一通过）。** 否决：`ANY` 今天没有任何消费形状，进了没人读，与 ADR-0104 「另四项不进首发」同一判据；`pn-02-w03` 禁的是推导它，放宽 CHECK 那天由租户显式登记它，不必新 ADR。
- **控制项上带信用政策版本 / 账户 / 价格依据的引用。** 否决：各有所有者，写进来就是第二处定义；哪一版信用政策由解析或壳引用取得，选哪条是消费票的事。
- **正文里写合同引用。** 否决：合同 → 策略已由 `0012` 拥有；两处写就要第三处核一致。
- **同笔改 SA 的 `LoadControlPolicy` 读正文。** 否决：跨上下文签名与语义改动，且 SA 的结果形状（一种方式一份采用政策）装不下多项控制，得先在 SA 侧建模；本记录只保证正文有处可读。
- **把控制项并进 `0007` 或 `0012`。** 否决：两层答的是「要不要」与「哪个范围用哪份策略」，拥有对象是合同；控制怎么做拥有对象是策略版本，并表就是让合同拥有策略正文，与 CONTEXT「产品与合同只采用这些规则版本，不拥有其正文」相悖。

## Links

- 票 [party-commercial-context-gaps/07](../../.scratch/party-commercial-context-gaps/issues/07-pre-acceptance-financial-control-policy-has-no-content-table.md)：四问、两层分工表与「结算侧从结算政策方式推控制方式」一句的出处，本记录以代码复核了那一句
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)：「接受前财务控制策略」词条与 Rules 三句——本记录 Decision 一、二、三的全部依据
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「接受前财务控制结果」词条与「只执行适用合同策略明确要求的控制项……不得把多个结果汇总成委托接受或拒绝决定」——Decision 四、五的分界依据
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：「接受前财务控制」一行——失败处置两值的出处
- [`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md)：`PAR-COM-15` 节与「结算模式不等于接受前财务控制策略」——Context 里那条今天仍在发生的派生、Decision 三不发明 `ANY` 的依据
- [ADR-0042](./0042-acceptance-content-declarations-by-owning-object.md)：声明按拥有对象挂——`0007` / `0012` 归合同、本正文归策略版本的纪律来源
- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md)、[ADR-0079](./0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)：SA 今天的读路径与`要求`格取方式的分工——Decision 五「一字不改」所指的那段
- [ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：「本记录不解决提供方表面」——本记录补的是提供方表面的最后一块
- [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：同形先例——正文归所有者、首发只进有依据的项、子行至少一项、点读口新开、解析不另开口
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：写入落点是封闭代数，重放与冲突绝不覆盖
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧翻译必须是全函数——Decision 三「集外组合子报错不吸收」的依据
- `migrations/party_commercial/0007_pre_acceptance_control_declaration.sql`、`0012_customer_contract_content.sql`：合同两层声明——本记录不动的那两张表
- `migrations/party_commercial/0023_customer_service_rule.sql`：父子两表、「项类即表」与「有父无子是坏数据」的先例
- `internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`：`policyFrom` 从结算政策取方式——Context 取证点
- 来源：IDP 队列通道 1 的派工与授权（2026-09-07）
