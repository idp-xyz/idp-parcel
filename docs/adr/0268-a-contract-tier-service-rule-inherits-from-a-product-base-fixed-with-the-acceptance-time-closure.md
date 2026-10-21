# ADR-0268：合同版客户服务规则继承的产品底座随接受时的闭包一并固定，已接受委托读底座不再按锚点现选；VE 的合同点核比闭包采用的客户合同，不比查询携带的合同

Status: Accepted（2026-10-21，Claude Code 会话代 PC、VE owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/11，出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B3·2、B3·3。部分停用 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定二一句与决定四一句，被停原句与适用场景见「对已接受记录的作用」；ADR-0176 正文不改，只在其 Status 行与 Links 加前向指针。[ADR-0136](./0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md)、[ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md) 无条款停用。代码随票 [docs-consistency/code/61](../../.scratch/docs-consistency/code/61-a-contract-tier-service-rule-reads-its-product-base-at-claim-time.md)。越权风险点见文末，待 PC、VE owner 复核。）<!-- 索引注记 -->｜**底座固定**：闭包采纳挂合同的那一版客户服务规则时，同一次解析按同一份键与锚点选出它的继承底座（同范围、壳上不指名客户合同、容得下声明的服务产品、锚点生效），恰一版即随闭包一并固定；零版记没有底座，合同版没写的行如实未登记；多版是客户服务规则这一项的`适用冲突`，在解析时就答。底座不是第二个已采用依据，闭包对这一类仍只采纳一版。底座与采用的成员同受指名引用核对。已接受委托读底座只读闭包上固定的那一版，底座此后被截止、退役或有效性更正都不改口。**点核**：VE 对拼进来的每份正文（采纳的那一版与底座）核其所挂合同等于同一份闭包采用的客户合同，不比 `EligibilityQuery.Contract`；挂产品的不核。**存量**：本记录之前形成、没有底座一格的闭包快照，照旧按锚点现选

Date: 2026-10-21

## Context

- [ADR-0136](./0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md) 决定一：「索赔资格规则（首次索赔期限、最低材料两维）的适用规则版本，是该索赔项目标包裹所属委托被接受时固定的商业依据里已采用的客户服务规则版本」。决定二：「VE 不再走 PC 第一阶段解析」，以及「读一份已固定的闭包没有`适用冲突` / `解析未决` / `输入未受理`可答」。判据是 PC CONTEXT「委托被接受时固定其适用的…」：按索赔提交时刻选，客户会拿到一份他下单时不存在的规则。
- [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 在其后立了合同版与产品版的行级继承。决定二：「底座版在层次读口内选「同范围挂服务产品、锚点生效」的版本，多候选照`适用冲突`纪律答。」合同版没写的行，取自一份不在接受时闭包里、读的时候才选出来的版本，这一选还可能答`适用冲突`。它的 Consequences 又写「已接受的委托按接受时固定的版本读取，不受后续发布影响」。ADR-0176 全文不提 ADR-0136。
- ADR-0176 决定四：「VE 适配器点读后核「正文挂合同必须等于查询携带的」（原文折行续「合同」），以及「挂产品的版本不核」，理由是查询无产品维、由解析唯一性保证。ADR-0136 决定二写 `EligibilityQuery.Contract`「本记录不动它、不拿它去 PC 过滤」，越权风险点 5 写「`EligibilityQuery.Contract` 与闭包采用的合同版本不核一致」。[ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md) 决定四要求「消费方点读之后核「这一版挂的是不是我手上这份产品 / 合同」」；ADR-0176 只停用过 ADR-0104 的一个前提，没有碰决定四。
- 代码（取证于 main `b18f662bc`）：
  - `internal/partycommercial/application/customer_service_rule_layers.go` 的 `LoadCustomerServiceRuleLayers` 每次读都向权威视图 `LoadScope` 取此刻的登记册，经 `CommercialRegistry.CustomerServiceRuleProductBase` 现选底座；选法里的 `applicable` 只收状态为生效、选用区间（经有效性更正之后的区间）含锚点的版本。锚点与声明产品取冻结闭包上的，登记册取此刻的。ADR-0233 起旧版可被截止或同起点退役，退役的版本此后不参选。
  - 端口头注自认：「底座不随闭包冻结，按传入的锚点读时选；已接受委托的底座只靠锚点固定与版本不回溯生效保持不变。」VE 适配器 `internal/visibilityexception/adapters/partycommercial/claim_service_rules.go` 的 `productBase` 头注：「底座不随闭包冻结，按锚点读时选；有效性更正改了底座区间时这里会改口」；底座答`适用冲突`时它报 `ErrUntranslatableAnswer`。同一文件 `adoptedRuleVersion` 头注照写 ADR-0136 决定二「读一份已固定的闭包没有`适用冲突`」，两说并存。
  - 合同点核 `tiedToAdoptedContract(rule, adoption.contract)` 比的是同一份闭包 `AdoptedFor(CustomerContractObject)` 交出的客户合同对象，不是 `EligibilityQuery.Contract`；合同版、底座与单版点读各条路都过它。挂产品的正文不核。
  - 闭包解析 `resolveCustomerServiceRuleBasis` 只采纳一版；`namedReferencesConfirmed` 只核采用的成员，底座不在其中。
- 后果：包裹接受之后，产品底座那一版若被截止、同起点退役或有效性更正，客户合同继承来的首次索赔期限与最低材料会在事后改口，正是 ADR-0136 要防的那一种。

## Decision

**一、合同版继承的产品底座随接受时的闭包一并固定。**（机制）

- 闭包对客户服务规则这一类采纳挂合同的那一版（ADR-0176 决定一）时，同一次解析、按同一份键与同一个锚点选出它的继承底座：同范围、壳上不指名客户合同、容得下键上声明的服务产品、锚点生效——与 ADR-0176 决定二的选法与闭包回落层同一处。
- 恰一版：作为这一项采纳的继承底座随闭包一并固定，闭包快照记它的版本身份。零版：记没有底座，合同版没写的行如实答未登记。多版：客户服务规则这一项答`适用冲突`，与合同层、产品层多候选同一纪律；在接受时就挡，不留到索赔时报。
- 底座不是第二个已采用依据：对客户服务规则这一类，闭包照旧只采纳一版（`AdoptedFor` 交出的仍是合同版），底座是那一版的继承来源。ADR-0176 候选与反方否决的「闭包为服务规则开双槽」不在此。
- 底座与采用的成员同受指名引用核对（`namedReferencesConfirmed` 的判法）：它壳上指名的服务产品若不是闭包采用的那一个，这份闭包不唯一解析，与采用的成员指名错了同一格。
- 已接受委托读底座，只读闭包上固定的那一版正文，层次读口不再按锚点现选。那一版此后被截止、退役或有效性更正，都不改已接受委托所读的那一行；PC CONTEXT「委托被接受时固定其适用的…」与客户服务规则版本生命周期「既有案件、通知和索赔历史继续引用原版本」在继承来的行上也成立。

**二、VE 的合同点核比同一份闭包采用的客户合同。**（机制）

- VE 对它读到并拼接的每份客户服务规则正文——闭包采纳的那一版与固定的底座——核：正文挂客户合同时，挂的必须是同一份闭包采用的那份客户合同。不等即提供方契约被打破。
- 比较对象不是 `EligibilityQuery.Contract`：那是 VE 自己的册（合同责任范围、申请人授权目录）的键（ADR-0136 决定二），与 PC 客户合同对象不是一套词，两者不核一致（ADR-0136 越权风险点 5 照旧）。拿查询携带的一格去核，是把核对对象换成调用方可控的值。
- 挂产品的正文不核，照 ADR-0176 决定四。产品那一半由闭包解析保证：采用的成员与决定一之后的底座都受声明产品收窄与指名引用核对。

**三、存量快照。**（机制）

本记录之前形成的闭包快照没有底座一格。读到这种快照时照今天的做法按锚点现选，只出现在演示库与测试夹具里；演示库 `scripts/demo-seeds --reset` 重灌后不再出现。

## 候选与反方

- **认读时选，订正 ADR-0136 与 ADR-0176 Consequences。** 否决：那等于承认客户合同继承来的期限与材料可以在接受之后改口，与 PC CONTEXT「委托被接受时固定其适用的…」和 ADR-0136 决定一的判据相反；索赔期限是客户下单时买到的条款，有效性更正与退役是运营对登记册的动作，不该回头改一份已接受委托的索赔条款。
- **底座作为客户服务规则的第二个已采用依据。** 否决：与「每种必需商业依据必须唯一适用」正面冲突（ADR-0176 候选与反方同一理由）；底座记在采纳的那一版之下，只采纳一版的形不变。
- **底座多候选仍留到索赔时报错。** 否决：读一份已固定的闭包没有`适用冲突`可答（ADR-0136 决定二）；把冲突挪到接受时，运营在委托还没接受时就能更正登记，比索赔时一件件报错有用。
- **合同点核比查询携带的合同（ADR-0176 决定四的字面）。** 否决：理由见决定二。
- **产品那一半也在 VE 核。** 不采纳：采用的服务产品就在 VE 已经读到的闭包里，核起来不多一段跨上下文取数；但决定一之后闭包已经保证它，VE 再核一遍只多一个永远不触发的失败格。

## 对已接受记录的作用

- **部分停用** ADR-0176 决定二「底座版在层次读口内选「同范围挂服务产品、锚点生效」的版本，多候选照`适用冲突`纪律答。」一句。**适用场景**：闭包采纳挂合同那一版的已接受委托——底座在接受时的同一次解析里按同一选法选出、随闭包固定，层次读口按闭包上固定的那一版读，不再按锚点现选；多候选在解析时答`适用冲突`。合同版有行用合同版、无行取底座那一行的拼法，与决定二其余各句不变。
- **部分停用** ADR-0176 决定四「VE 适配器点读后核「正文挂合同必须等于查询携带的」（原文折行续「合同」）一句里比较对象那一半。**适用场景**：VE 对它读到并拼接的每份客户服务规则正文——比较对象是同一份闭包采用的客户合同对象，不是查询携带的那一格；「挂产品的版本不核」与「壳 ↔ 正文一致」那一道不变。
- ADR-0176 决定四「挂产品的版本不核」（理由是查询无产品维、由解析唯一性保证），是对 ADR-0104 决定四「消费方点读之后核「这一版挂的是不是我手上这份产品 / 合同」」里产品那一半的部分停用，ADR-0176 正文没写；本记录认定，指针补在 ADR-0176 的 Status 行前段（去向）与 ADR-0104 的索引注记（来向），适用场景限客户服务规则正文的产品那一半，合同那一半仍由消费方核。
- ADR-0176 Consequences「已接受的委托按接受时固定的版本读取，不受后续发布影响」：由决定一兑现到继承来的行，不改。
- ADR-0136：无条款停用。决定一、二由决定一在继承底座上兑现：底座也随接受时闭包固定，读一份已固定的闭包没有`适用冲突`可答。决定三「由 PC 闭包解析时的指名引用核对守，VE 不重核」说的是壳上的指名引用，决定一把底座也纳入这道核；VE 的合同点核核的是正文所挂的合同，是 ADR-0176 决定四另立的一层，不是重做这道核。越权风险点 5 照旧成立。

## Consequences

- 代码随票 [code/61](../../.scratch/docs-consistency/code/61-a-contract-tier-service-rule-reads-its-product-base-at-claim-time.md)：闭包解析在采纳合同版时选出并固定底座、底座过指名引用核对、底座多候选答`适用冲突`；闭包快照记底座一格；层次读口按快照上的那一版读；存量快照照旧现选；改端口与 VE 适配器头注里「底座不随闭包冻结」的几句。
- party-commercial CONTEXT「客户服务规则」那几条 Rules 补合同版行级继承与底座随接受固定一句；visibility-exception CONTEXT 索赔资格时点那一条补继承来的那一维同样取接受时固定的底座。UC-PC-002 与 UC-VE-007「给开发的交接」各记今天的缺口。
- 行为变化：今天同范围产品层多候选的合同版委托能接受、索赔时才报错；改后在接受时停在`适用冲突`，按既有接受链处置。

## 越权风险点

1. **底座多候选会挡接受**（PC owner）。改后一份合同版委托的接受，要求同范围产品层恰好一版或一版都没有；运营登了两版同期生效的产品版而没收尾，接受停在`适用冲突`。这是把一个今天藏到索赔时才露出的登记错误提前露出。
2. **有效性更正不回及已接受委托**（PC、VE owner）。若 owner 认为对底座的有效性更正（不是发新版）说的是登记本来就错了、应当追溯到已接受委托的索赔条款，推翻的是决定一；那时合同版与产品版一样要另立更正回及在途委托的规则，不只是底座。
3. **存量快照照旧现选**（PC owner）。决定三之下演示库与测试夹具里的旧闭包仍会漂；生产没有存量，演示库重灌即消。
4. **合同点核与 `EligibilityQuery.Contract` 仍不核一致**（VE owner）。ADR-0136 越权风险点 5 不因本记录消解：VE 的合同责任范围与申请人授权目录按查询携带的合同键入，PC 那一头按回指走，两者指向不同合同时今天没有一格会报。

## Links

- [ADR-0136](./0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md)：决定一、二由本记录在继承底座上兑现；决定三的指名引用核对扩到底座；越权风险点 5 照旧
- [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md)：部分停用决定二「底座版在层次读口内选……」一句与决定四比较对象那一半；认定决定四「挂产品的版本不核」部分停用 ADR-0104 决定四产品那一半
- [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：决定四「这一版挂的是不是我手上这份产品 / 合同」
- [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)：截止与同起点退役，底座事后离开新选择的来路
- [party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md)「客户服务规则」各条与 Rules「委托被接受时固定其适用的…」、[visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)「证据、客户索赔与追偿」索赔资格时点一条
- [UC-PC-002](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md)、[UC-VE-007](../application/visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md)「给开发的交接」
- 票 `.scratch/docs-consistency/issues/11-unclaimed-cross-adr-contradictions.md`；代码票 `.scratch/docs-consistency/code/61-a-contract-tier-service-rule-reads-its-product-base-at-claim-time.md`
