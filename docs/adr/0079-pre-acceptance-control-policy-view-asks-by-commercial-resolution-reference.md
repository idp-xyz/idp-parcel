# ADR-0079: 接受前控制策略视图凭商业解析回指提问——消费方只回显标识，提供方从已固定闭包取合同

Status: Accepted
Date: 2026-08-26

## Context

[ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md) 给 `PreAcceptanceControlPolicyView` 补了`未配置`格，同时明说「本记录不解决提供方表面」。那半边现在到期了：`party-commercial` 已有 `PreAcceptanceControlDeclaration` 领域类型、`PreAcceptanceControlDeclarationView` 只读端口与真库适配器，种子里也发布了真声明（`SYN-FIN-CONTROL-01`，客户合同正文绑定）。缺的不再是数据，是一道键形裁决。

**两侧的键不同维。** SA 这一口按资金作用域提问：

```go
LoadControlPolicy(ctx, tenant, scope) (domain.PreAcceptanceControlPolicy, bool, error)
```

`SettlementScope` 是（责任法人 / 结算账户 / 币种）。而 PC 的声明按（租户 + 客户合同版本）键入——`PAR-COM-15` 列在合同版本下，库上 `object_kind` 的 CHECK 钉在客户合同那一格。**签名装不下这次翻译**：适配器拿到的东西问不出答案。

同时，两条既有约束把显而易见的补法都堵死了：

- SA 端口第一句写着「本上下文只消费它，绝不自行推导」——SA 不得自己从资金作用域反查合同。
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：只有跨上下文适配器可以同时导入两个上下文，且翻译必须是全函数。SA→PC 适配器因此够不到 PS 的商业解析器。

另一侧有一件现成的东西：PS 的商业依据快照**刻意不持有任何商业版本内容**，但持有 `ResolutionID`；PC 把已固定的解析闭包按标识落库，并开有只读口 `CommercialResolutionView.LoadResolution(tenant, resolutionID)`，注释明写「消费方只要回指标识」（[ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) / [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)）。裁决要做的，是判定这条既有形状是否适用于 SA↔PC 这道缝，以及适用时签名与语义各是什么。

裁决经用户 2026-08-26 队列委托（「作为业务和系统专家自决」）；三案对比与事实链取证记于 [sa-preacceptance-policy-view/01](../../.scratch/sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md)，出处是[第二十六轮重盘](../../.scratch/mechanism-reinventory-r26/report.md)第四节判据 B。

## Decision

**一、控制策略视图的签名再扩一格：商业解析回指。**

```go
LoadControlPolicy(ctx, tenant, scope, resolution) (domain.PreAcceptanceControlPolicy, bool, error)
```

`resolution` 是 SA 侧的新值类型 `CommercialResolutionReference`——**只是一个标识**，不是任何商业内容。SA 域内不解释它，只原样递给提供方适配器。

**二、`scope` 留在签名上，但不参与提问。** 它是资金维，商业侧的声明不按它键入。留着是因为端口的其余实现（含未配置桩）按它作答，且它日后可能参与一致性核对。**提供方适配器不得拿它去过滤**：那是「用一个装不下答案的键去找答案」，恰好答对时也不是因为它对。

**三、空回指在编排入口即`未受理`，不落成`未配置`。** `ApplyPreAcceptanceControlCommand` 同扩一格，且回指列入 `minimumIdentityEstablished`：缺回指时**一次也不问策略**。

两种缺口的补法相反，所以必须分格。放过去的话，视图只能答 `found=false`，编排落成 `CONTROL_POLICY_NOT_CONFIGURED`，于是租户被支去补一份其实早就存在的声明——而真正缺的是调用方少给了键。

**四、续办摘要纳入回指。** 换了回指就是换了一次问答；共用一条续办引用，会让续办方接回另一次商业依据下的停摆。

**五、回指与资金作用域同出一次解析，同进同出。** PS→SA 适配器的 `ControlScopeSource` 交回 `ControlScope{Settlement, Resolution}` 整体，而不是裸作用域加一个另取的引用。资金作用域本就派生自那次解析回显的结算政策；**分两口取就给出了两次解析的机会**，而「施加与释放两径同引用」正靠同源——一旦分头取，同源退化成一条谁也没在证的约定。

释放命令不带回指：释放不问控制策略，它按原请求身份在冻结账本上认领。回指在释放路径上取到了却不用，正是同源的代价，也是它的证据。

**六、提供方适配器分三段：回指换闭包 → 闭包取已采用的客户合同 → 合同读声明。** 落点是 ADR-0054 预留的 `internal/settlementaccounting/adapters/partycommercial/`。

`要求`那一格的方式与采用政策**取自同一份闭包里的结算政策**，不由声明反推（ADR-0044 的分工，[`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md) 的禁推导句）。

**七、坏回指是 error，不是`未登记`。** ADR-0054 的三格在提供方这一侧逐格落定：

| 情形 | 答复 | 恢复动作 |
|---|---|---|
| 合同在、声明没写 | `found=false` | 租户去补合同正文（`PAR-COM-15`） |
| 声明在场 | `found=true` | 按声明走：`不适用`带合同给的依据，`要求`带闭包里的方式与采用政策 |
| 回指译不动 / 闭包查无 / 闭包不是唯一已解析 / 闭包没采用客户合同 | `error` | 修调用方或提供方，不是等登记 |

第三行整行都不是「没人登记过」：答成 `found=false` 会把租户支去补一份其实已经存在的声明，而真正坏的是那条回指。他租户拿本租户的回指同理——那是一次跨租户读取，不得因为「反正读不到」就折成未配置。

**八、装配期拒 nil 半边。** 适配器一旦被装上就是要真去问商业侧的；缺一只读口而静默答`未配置`，会让一次接线疏漏与租户没登记长得一模一样。

**九、推论：已固定的闭包快照必须原样读得回消费方要读的每一样东西。** 本裁决把「回指换闭包」变成一条生产路径，闭包快照因此从「写下就好」升级为**读写两侧都受约束**。落库时丢掉解析键上的任何一维，那份闭包写得进、读不回，而两侧都不报错。

政策正文整份留在快照里，不回登记册按版本重读：登记册那份改一次，一次已固定的解析就会改口说自己当初采用的是别的方式——快照的全部意义就是不许它改口（[ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)）。

## Consequences

- 这是一次跨上下文签名改动：SA 端口、SA 应用命令、编排、PS→SA 适配器与两侧测试替身同笔跟随。跟随部分只改签名不改语义（同 ADR-0054 当年的跟随口径）。
- ADR-0054 的`未配置`格从此有了真实的提供方：在此之前，无论商业侧登没登记过声明，控制策略一律答`未配置`——「租户还没写合同正文」与「这条路根本没接」长得一模一样。
- **Decision 九当场兑现了一个实例**：解析闭包快照原先不落解析键上的结算选择器，也不落已采用结算政策的正文。采用了结算政策的闭包因此写成功、读必失败（最小身份立不起来，重建门整份拒掉），而 `Save` 答 `SAVED`、`Load` 报一句像是快照坏了的话。生产今天撞不到——PS 的解析键登记面明拒结算政策依据，闭包目前形不成——但下游正是凭这份方式决定冻不冻款，缺席会被读成「商业侧没登记过控制」。同笔修复，配对用例在 `commercial_resolution_test.go`。
- **价格政策的同处缺席不随本记录一并解决。** `NewCommercialPricePolicy` 要方案方向与跨向转换两个入参才立得起来，而 `CommercialPricePolicy` 并不留存它们；照结算政策的形状重建，就得跳过那道绑定校验，或者在快照里再存一份只为过校验的输入。两条路都要先决定「已固定的价格政策还要不要重验绑定」——那是一道决定，不是一段代码，留给它自己的票。缺口记在 `RehydrateAdoptedBasisSpec` 的注释里。
- **本裁决不让接受前控制链在生产上走通。** 上游 PS 的解析键登记面明拒结算政策依据（那一面不承载选择器与价格方向两组维度），闭包目前形不成；账户映射与估价两道缝也仍是实例半边，编排会停在 `CONTROL_SCOPE_NOT_CONFIGURED`。适配器因此暂不装进 `cmd/parcel-api`——装上去也走不到`要求`那一格。接线与那道登记面同票。
- 隔离合成 `S` 环境下，凭真库用例已可逐格取证；证据层级记 `S`，不因适配器到位而升格。

## Alternatives considered

- **甲：SA→PC 适配器自行反查（资金作用域 → 客户合同）。** 否决：反查目录是一份新的实例数据，无处取，等于发明账户映射的第二处定义；经 PC 解析应用重解则需要商业查询入参（客户账户、服务产品），`SettlementScope` 装不下；SA 适配器也不得导入 PS 的商业解析器（ADR-0025）。
- **乙：消费方（PS 侧）拼好策略再喂给 SA。** 否决：「本上下文只消费它，绝不自行推导」约束的正是 SA 与商业侧之间的那次问答；在 PS 拼答案等于把 SA↔PC 的缝搬进 PS，一决策两处定义。
- **命令直接携带客户合同版本，而不是解析回指。** 否决：那是让消费方持有商业内容，ADR-0027 / ADR-0062 明拒的形状；且合同版本是这次解析的**结论**，由消费方转手，就允许它与解析当时采用的那一份不一致。回指指向已固定的闭包，取回的合同必然是当初采用的那一份。
- **让 `SettlementScope` 长出一个合同维。** 否决：资金作用域是 SA 自己的语言（一个结算账户固定一个责任法人、结算相对方、收付方向与结算币种），塞进一个商业维会让 SA 的作用域随商业侧的键形变化，且那一维在 SA 的其余用途上永远为空。
- **回指缺席时答 `found=false`。** 否决：见 Decision 三。恢复动作相反是分格的唯一判据（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。
- **闭包重建时回登记册按版本重读政策正文。** 否决：见 Decision 九。那让一次已固定的解析可以改口。

## Links

- [ADR-0054：接受前财务控制策略视图增设「未配置」格](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：三格代数与「本记录不解决提供方表面」的出处，本记录补上那半边
- [ADR-0044：结算依据经结算政策采用](./0044-settlement-basis-adopts-via-settlement-policy.md)：`要求`格的方式与采用政策由它提供
- [ADR-0047：账期控制形成信用暴露而非冻结](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)：两种方式各自的下游形状
- [ADR-0027：跨上下文多步协议的中间状态由提供方按解析标识保留](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)、[ADR-0062：采用规则版本从已接受解析标识回指提供方持有的闭包](./0062-adopted-stage-owner-from-accepted-resolution.md)：回指形状的通例来源
- [ADR-0028：重建只校验不重算](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：Decision 九不回登记册重读的判据
- [ADR-0029：结果代数按消费方的恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：Decision 三与 Decision 七分格的判据
- [ADR-0025：跨上下文调用的适配器落在消费侧，翻译职责由它独占](./0025-cross-context-adapters-live-on-the-consumer-side.md)：甲案否决理由与全函数纪律
- [`pn-02-w03`](../design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md)：「结算模式不等于接受前财务控制策略」的禁推导句
- [sa-preacceptance-policy-view/01](../../.scratch/sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md)：三案对比、事实链取证与实现范围
