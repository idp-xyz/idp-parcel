# ADR-0221：权威交接作参与依据时由交接键铸——对象、范围、版本三节长度前缀，另起一个起头；只带版本的存量照读、按版本加时刻认，NO 的转出引用不随之换

Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/111，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `76686b7b`，2026-10-19）」停点一与文末「另记一处观察」。**部分停用** [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 决定一两句与决定六「响亮失败」下一条，被停条款与适用场景见「对已接受记录的作用」。另部分停用 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md)「补记：票 66 落决定五时的补充裁定」里「参与的入场依据只带交接版本、不带交接范围。」一句（决定一、五；指针 2026-10-21 补），被停原句与适用场景见 ADR-0213 的 Status 行。越权风险点见文末，待 TF owner 复核，第 3 点同时待 NO owner 会签。）<!-- 索引注记 -->｜**铸法**：`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本各一节「字节长度:原串」，用于进段、结束与重派生长出的各版；只有`已交接`给得出；另起起头，新旧串集合不相交。**存量**照读不改写：逐字对上的一定是那一次，旧拼法按版本加按它立下的时刻认。**自动形成**：逐字对上的优先，多于一条仍响亮失败；旧拼法对上多于一条答`结束的参与认不出`、入账。**交付门禁与重派生**同一套认法，`ENTRY_HANDOVER_AMBIGUOUS` 自此只出自存量。**NO 的转出引用不变**，换不换归票 112。别的控制事实（场外揽收、实际承运商首次有效收寄、有效交付）的版本由本上下文按全局序列铸、本身唯一，依据只带版本不在此列。越权风险点含存量歧义仍在、旧拼法恰一条也要时刻对上、同一次交接两串、派送任务引用变长
Date: 2026-10-19

## Context

- 演示动线重走（钉 `76686b7b`，只记 `S`）：一件包裹照动线登五次`已交接`，每次各起一个范围，版本都写 `v1`；深圳口岸交进与交出同用范围 `SZX`，版本 `v1`、`v2`。
  - 段登记册上，自营段那条参与的终点依据（`SZX`/`v1` 写下）与干线段那条的终点依据（`SIN`/`v1` 写下）都是 `TRANSPORT-HANDOVER/v1`。
  - `SIN`/`v1` 与 `SINLM`/`v1` 两封交接信封，实际履约发生项自动形成那一路都对上两段，答 `ErrParticipationEndedInSeveralSegments`，信封烧完重投预算落 `ABANDONED`。干线发生项没有形成，BUY 要人工登发生项才走得下去。
- 交接的身份是（租户，对象，范围，版本）。版本只在（对象，范围）之内唯一，同一件包裹在两个范围各登一版 `v1` 是合法的；TF 的首登口也不拒（ADR-0220 决定四）。
- 参与的入场与终点依据都取 `TransportHandover.TransferOutBasis`，拼成 `TRANSPORT-HANDOVER/<版本>`，不带范围，指不回是哪一次交接。同一个缺口落在几处：
  - 自动形成找「这件事实结束了哪一条参与」：ADR-0218 决定一把多于一条当库面不一致，演示这组数据却合法。
  - 交付门禁读数找「对象凭哪一次交接进的段」：ADR-0213 文末补记（票 66）已认这一格，先按业务时间再分，仍多于一版答 `ENTRY_HANDOVER_AMBIGUOUS`。
  - 交接更正重派生找「被替代的是哪一条」：按版本串比，另一个范围用了同一版本号的更正会对上不相干的参与。
- 这串还被原样抄到别处：委托的运输开始依据、段采用的开始依据（ADR-0204），以及派送发起的任务引用（ADR-0216，票 tf-face-algebra/09 的铸法）。查阅时这几处都指不回交接。
- 别的控制事实作依据也只带版本，却不在此列：场外揽收、实际承运商首次有效收寄与有效交付的版本由本上下文按全局序列铸（PG 适配器 `ResultVersions` 的 `NextPickupResultVersion`、`NextCarrierFirstEffectivePickupVersion`、`NextDeliveryResultVersion`），本身就唯一，`OFFSITE-PICKUP/`、`CARRIER-FIRST-EFFECTIVE-PICKUP/`、`EFFECTIVE-DELIVERY/` 加版本指得回那一件事实。只有交接版本是登记方给的，缺口只在这一种。
- NO 的转出引用也是同一个串（`TransferOutBasis` 包装成 `TransferOutReference`）。ADR-0214 越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」已经说过它不够；NO 转入一侧自铸 `HANDOVER-IN/` 加交接键三节长度前缀。
- 先例：NO 的转入依据（ADR-0214 文末「转入立起的控制依据」）与派送任务引用（票 tf-face-algebra/09 裁定「单射铸法：取长度前缀，带形状标记」），都把不透明的几件写成「字节长度 + 冒号 + 原串」。
- 不是 ADR-0096 那一类推导：段的身份仍由登记方声明。这里定的是参与上记的「依据」怎样指名那一件控制事实，交接键本身就是那件事实的身份。

## Decision

**一、`已交接`作参与依据时，由交接键铸。**（机制）

- 拼法：`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本三节，每节写成「字节长度 + 冒号 + 原串」，以斜杠相连。例：对象 `parcel-1`、范围 `handover-scope-1`、版本 `handover-result/parcel-1/v1` 铸出 `TRANSPORT-HANDOVER-KEY/8:parcel-1/16:handover-scope-1/27:handover-result/parcel-1/v1`。
- 用在：凭交接进段的入场依据，下一次交接结束参与的终点依据，交接更正长出的替代版本与失效版本的入场依据（失效版本取撤回控制那一版的键）。
- 只有`已交接`给得出（`TransportHandover.ParticipationBasis`）；拒收与待确认不转出控制，立不起也结束不了参与。
- 租户不进串：参与与它的依据都在租户之内。对象进串：委托与段采用的开始依据、派送任务引用只抄这一串，不带对象就指不回交接。
- 写长度而不直拼：三件都是只拒空白的不透明串，可以含斜杠，直拼不单射。不取摘要：依据出现在答复与查阅上，人要读得出它是哪一次交接。

**二、另起一个起头，不在旧起头下加形状标记。**（机制）

- 旧拼法的版本是任意非空串，凡以 `TRANSPORT-HANDOVER/` 起头的串都可能是一条旧依据。新铸法若还在这个起头之下，就构造得出与某条旧串逐字相同的新串（票 tf-face-algebra/09 的「已知局限」就是这一格）。
- `TRANSPORT-HANDOVER-KEY/` 不是旧起头的前缀延伸，新旧两种拼法的串集合不相交，认的时候不另设防线。
- 不进标识前缀门禁：它是依据串的起头，与 `OFFSITE-PICKUP/`、`DELIVERY-DISPATCH/` 同类，带横线与斜杠，不是签发器的类别前缀。

**三、存量照读，不改写。**（机制）

- 本记录之前写下的 `TRANSPORT-HANDOVER/<版本>` 留在段登记册上，也留在委托与段采用的开始依据里，不迁移、不回填。依据是事实的一部分；改写它，从它派生的派送任务引用也就在册上找不回了。
- 认一条记下的依据是不是指某一版交接，分两格：
  - 逐字等于那一版按键铸的串：一定是那一次。
  - 是旧拼法、版本相同，且按它立下的时刻正是那一版交接的业务时间（入场比起点，终点比终点时刻，按登记册精度）：算对上，但认不全——同一版本号、同一业务时间、不同范围的两次交接仍分不开。
- 旧拼法加了时刻这一条，是交付门禁读数早已有的认法（票 66），这里推到结束参与与重派生两处：进段、结束都按交接的业务时间立时刻，替代版本照抄前版的终点，更正不改业务时间，这一条对存量成立。

**四、自动形成找结束的参与：逐字对上的优先。**（机制）

- 逐字对上的多于一条：同一次交接在两个段里结束了参与，是库面不一致，照旧响亮失败。
- 逐字对上恰一条：就是它，旧拼法那几条不看。
- 没有逐字对上的，旧拼法对上恰一条：就是它。
- 旧拼法对上多于一条：答`结束的参与认不出`（`ENDED_PARTICIPATION_AMBIGUOUS`），入账，不挑一段交给登记用例。重投不会变；那一段的发生项照 ADR-0207 决定六人工登记。
- 都没对上：照旧答没有参与被这件事实结束。

**五、交付门禁读数与重派生用同一套认法。**（机制）

- 交付门禁读数：入场依据读得回版本（两种拼法都读），按（租户、对象、版本）找回各版，由同一套认法挑进段那一次。新依据只认得出范围相同的那一版；`ENTRY_HANDOVER_AMBIGUOUS` 自此只出自存量。
- 交接更正重派生：被替代的那一条按被更正版本的键认，存量按版本加起点认；回指写那一条在册的入场依据原串，两种拼法各回指各的。

**六、NO 的转出引用暂不换。**（机制）

- `TransferOutBasis` 照旧交回 `TRANSPORT-HANDOVER/<版本>`，只给 NO 包装成转出引用。NO 要不要换成按键铸的那一串，是 NO 的事，归票 demo-chain-gaps/112。
- 同一次交接于是有两串：TF 参与上的按键铸的依据，NO 转出引用的旧串。查阅按交接键对，不按串对（ADR-0214 越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」）。

三类：六条都是**机制**；没有产品策略与租户取值。演示租户证据只记 `S`。

## 对已接受记录的作用

- **ADR-0218 决定一，部分停用。**
  - 被停条款：「是`已交接`的，终点依据取交接自己交出的转出依据」；「多于一条：库面不一致，响亮失败，不挑一段去计费」。
  - 适用场景：前一句全部停用，终点依据改由交接键铸、存量按版本加时刻认（决定一、三）。后一句只停在旧拼法存量那一格：旧拼法对上多于一条答`结束的参与认不出`、入账（决定四）；逐字对上多于一条仍是库面不一致、响亮失败。
- **ADR-0218 决定六，部分停用。**
  - 被停条款：「响亮失败」下「同一件事实在多个段里都结束了参与」一条。
  - 适用场景：同上，只停旧拼法存量那一格；它改落「入账」。
- ADR-0213 文末补记（票 66）的停点 `ENTRY_HANDOVER_AMBIGUOUS` 不停用，词表不变；它只会出自本记录之前的存量入场依据。
- ADR-0214 决定四「转出依据是交接给出的引用（`TRANSPORT-HANDOVER/<版本>`）」不变（决定六）。
- ADR-0112 决定四「撤回控制的版本没有入场依据可立（`TransferOutBasis` 不给）」的意思不变：`ParticipationBasis` 同样不给，失效版本取的是撤回控制那一版的交接键。
- 其余各条不变，正文不改。

## 候选与反方

- **只在自动形成那一路按业务时间再分，依据不动。** 否决。
  - 同一版本号、同一业务时间、不同范围的两次交接仍分不开。
  - 依据仍指不回交接：委托与段采用的开始依据、派送任务引用照旧只剩一个版本号，psb/05 那条观察不解决。
  - 重派生那一处要另修一遍，同一条认法多写一份。
- **依据写成 `TRANSPORT-HANDOVER/<范围>/<版本>`。** 否决。范围与版本都可以含斜杠，直拼不单射；也不带对象，委托与段采用的开始依据仍指不回交接。
- **沿用旧起头，加形状标记（`TRANSPORT-HANDOVER/v2/…`），照票 tf-face-algebra/09。** 否决。见决定二：旧串可以构造得与新串逐字相同，认的时候要另设防线，仍留一格已知局限。那一票沿用旧起头，是因为手工建任务口要拦保留起头；参与依据没有这个需要。
- **取摘要。** 否决：依据出现在答复与查阅上，人要读得出是哪一次交接。
- **迁移改写存量。** 否决：改写一条已发生事实的依据，从它派生的派送任务引用在册上就找不回了；演示库与开发库按一次性库用，生产没有存量。
- **连 NO 的转出引用一起换。** 暂不。NO 自铸转入依据，转出引用在 NO 那一侧只作查阅；换不换、怎样兼认已在册的转出行，是 NO 的取舍，且那一片当下有别的会话在改。归票 112。
- **首登口拒同一对象在不同范围用同一版本号。** 否决：交接的键里有范围，版本只在（对象，范围）之内唯一；拒它就是替登记方改编号口径。ADR-0220 决定四同一理由。

## Consequences

- TF：
  - 领域：新文件 `domain/handover_participation_basis.go`，拼、拆、认三件都在那一处——`TransportHandover.ParticipationBasis`、`FulfillmentParticipation.EntryHandoverVersion`（从交付门禁读数那一份搬来）、`EnteredByHandover`（同上）、`EndedByHandover`、`ControlFactMatch`；`ActualFulfillmentSegment.ParticipationEndedByHandover`。进段、结束、重派生三道门改用它；`TransferOutBasis` 只给 NO。
  - 应用：`FormActualFulfillmentOnParticipationEndHandler` 按决定四找结束的参与，结果多一格 `FormationEndedParticipationAmbiguous`（`ENDED_PARTICIPATION_AMBIGUOUS`，入账）。
  - 不加迁移，不加信封，不改派发路由、inbox 名与哨兵名单。
- NO：不改。
- 派送任务引用：新参与的入场依据变长，新任务的引用随之变长，对象在引用里出现两次（一次在任务键，一次在依据里）。存量参与的依据不变，它们的任务引用也不变。
- 文档：
  - [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)「载运对象、计划与实际履约」一条；
  - [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md) 步 9 一处引用与 `AT-TF-266`–`270`、`AT-TF-272`；[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md) `AT-TF-271`；
  - [演示动线](../design/synthetic-demo-journey-script.md)补记一则；
  - ADR-0218 的 Status 行与 Links 加前向指针，README 那一行写部分停用；ADR-0213 的 Links 加一条。
- 演示（证据 `S`）：照动线各次交接都写 `v1`、范围各异，干线段在 SIN-HUB 交接之后实际履约发生项自动形成、BUY 请求发出，五封交接信封都 PUBLISHED。

## 越权风险点

1. **存量歧义仍在**（TF owner）。本记录之前写下的旧拼法依据，同一对象在两个范围用了同一版本号、业务时间也相同时，仍认不出是哪一次：
   - 自动形成答`结束的参与认不出`、入账，那一段的发生项不自动形成，要人工登记；入账之外没有别的信号，与 ADR-0218 越权风险点 6 同一去处（票 demo-chain-gaps/88 的「该有而没有」）。
   - 交付门禁读数停在 `ENTRY_HANDOVER_AMBIGUOUS`。
   - 交接更正重派生在这一格上可能对上不相干的参与。
   - 演示库与开发库按一次性库用，生产没有存量。
2. **旧拼法恰一条对上也要时刻对上**（TF owner）。本记录之前，旧拼法逐字对上一条就算；现在还要按它立下的时刻等于那一版交接的业务时间。生产路径上两者一律相等（进段、结束按业务时间立时刻，替代版本照抄前版终点，更正不改业务时间），据此判定没有存量会因此从「对上」变成「没对上」。若有人手工改过参与的时刻，那一条会答没有参与被结束、入账。
3. **同一次交接两串**（TF、NO owner）。TF 参与上是按键铸的依据，NO 转出引用是只带版本的旧串；拿串去对两本册对不上，要按交接键对。去处票 demo-chain-gaps/112。
4. **派送任务引用变长**（TF owner）。新任务引用里嵌着按键铸的入场依据，对象出现两次。引用仍单射、仍读得出，手工建任务口拦保留起头的那一道不受影响；只是更长。

## Links

- [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)：部分停用其决定一两句与决定六「响亮失败」下一条（适用场景见「对已接受记录的作用」）
- [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md)：文末补记（票 66）「那次交接」的认法与停点 `ENTRY_HANDOVER_AMBIGUOUS`；按业务时间再分的先例
- [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md)：NO 转出引用不变；转入依据的长度前缀铸法是先例；越权风险点「转入依据由本上下文铸、不复用 TF 的转出引用」
- [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md)：派送任务引用由入场依据派生
- [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md)：委托与段采用的开始依据抄参与的入场依据
- [ADR-0112](./0112-source-correction-rederives-participation-as-a-superseding-version-on-the-same-segment.md)：替代与失效版本回指被替代参与的入场依据
- [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md)：决定四「TF 的首登口不改」
- [ADR-0096](./0096-a-fulfillment-segment-identity-is-declared-not-derived.md)：段身份声明、不推导，本记录不动它
- [UC-TF-005](../application/transport-fulfillment/UC-TF-005-ESTABLISH-HANDOVER-AND-ACTUAL-FULFILLMENT.md)、[UC-TF-006](../application/transport-fulfillment/UC-TF-006-PERFORM-DELIVERY-AND-CAPTURE-POD.md)
- 票 `.scratch/demo-chain-gaps/issues/111-actual-fulfillment-formation-matches-participation-ends-by-version-only.md`；后续票 `112`；先例票 `.scratch/tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md`
