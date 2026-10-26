# ADR-0301：小包托运开「受理基线地理投影」读口——受理后不由小包托运信封触发的复核经它只取判断键引用、接受时刻与地理维，不再读回整份委托聚合

Status: Accepted（2026-10-25，Claude Code 会话代 PS owner 决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」；[代决复核报告](../../.scratch/owner-review-20261020/report.md) R6 裁「是，作为过渡」并立票 [demo-chain-gaps/143](../../.scratch/demo-chain-gaps/issues/143-parcel-shipment-offers-an-acceptance-baseline-geography-projection.md)，本记录收掉那段过渡。部分停用 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二三句，被停原句与适用场景见「对已接受记录的作用」；ADR-0215 正文不改，只在其 Status 行与 Links 加前向指针；同一处回答 ADR-0215 越权风险点 1。越权风险点见文末，待 PS owner 复核，NR owner 会签。）<!-- 索引注记 -->｜**读口**：小包托运按（租户，包裹）答当前已接受委托接受基线那一版的地理投影，只交委托与客户账户引用、基线所指那一版、基线固定时刻与那一版的寄收两端国家与邮编；不是当前已接受成员答「不是」，歧义（ADR-0060）与接受基线自相矛盾各自具名，其余是读不动。**实现**：一条 SQL 从快照只取接受基线与那一版的地址要素子段，与重建同一组文档类型解码，不过重建门、自守三格自相矛盾；无迁移。**适配器**：NR `AcceptedCarriedGeoSource` 只换协作者，判断键、接受时刻与投影逐格不变，交接与收寄采用两路共用；自相矛盾不进未决哨兵。**不动**：初始路由仍按来源身份读委托聚合。**否决**：读口内整份读回再取投影、维持过渡、投影与判断键分两口取、拓宽反查口

Date: 2026-10-25

## Context

- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二：受理之后、不由小包托运信封触发的复核（交接进入节点）没有请求可随带投影，由网络与路由的消费侧适配器读取。适配器只用小包托运已有的两个读法：按包裹反查当前已接受委托，再按来源身份把委托聚合整份读回，取受理基线那一版提交的投影。
- [ADR-0279](./0279-a-reassessment-trigger-earlier-than-the-shipment-acceptance-is-settled-as-not-judged.md) 决定四让收寄采用一路按同一读法取接受时刻。两路共用同一个适配器（`AcceptedCarriedGeoSource`）。
- ADR-0215 越权风险点 1 把这一读法记为越权：小包托运的委托仓储多了一个跨上下文读者。它的候选里留了一条路——小包托运新开「受理基线地理投影」端口，适配器只换一个协作者。
- 代决复核 R6（2026-10-20）裁「是，作为过渡」，立票 143：小包托运开这个端口，网络与路由不再为取地理维而经手整份委托。
- 委托快照里有什么（取证于 main `f8b25837b`）：
  - 不带地址文本与收件人身份。寄收件条目只进载荷摘要，与地址有关的只有各版提交上的地址要素子段（国家 / 地区码、邮编）。
  - 整份读回时网络与路由经手的是：各版申报测量画像、接受决定与商业依据、结算条款、客户包裹件号、接受后的资料修订与它们的要素内容。取地理维用不着其中任何一样。聚合以后再添的任何一格，也会随整份读回进到网络与路由。
- 两个读法之间委托可能在动：反查命中了，聚合却读不回或已不在`已接受`。适配器只能按读不动重投。

## Decision

**一、小包托运开「受理基线地理投影」读口（机制）。**

- 端口 `AcceptanceBaselineGeoProjectionView`，按（租户，声明包裹）问。不要求消费方持有委托、接受基线或提交版本（[ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md) 决定二），不过授权查询作用域，待遇同其余进程内按包裹答问的读口。
- 答一份 `AcceptanceBaselineGeoProjection`，只有这些：
  - 委托引用与客户账户引用；
  - 接受基线所指的那一版提交；
  - 基线固定的时刻，即委托接受的时刻；
  - 那一版提交的地理解析投影：寄、收两端的国家 / 地区码与邮编，与接受决定、收寄采用两封信封所携同一取法。
- 读的是接受基线所指的那一版。不是当前提交版本，也不是接受之后经资料修订采用的版本（ADR-0215 越权风险点 5 照旧）。
- 答格：
  - 「不是」（found=false）：没有当前已接受委托声明它，或那一份的接受基线里没有它。不区分不存在、他租户与仅`已提交`。
  - 歧义：两份已接受委托同时声明它，具名上抛，不挑一份（[ADR-0060](./0060-parcel-lookup-uses-current-snapshot-projection.md)）。
  - 接受基线自相矛盾：没有基线、基线没有固定时刻、基线所指的版本不在这份委托里。新哨兵 `ErrAcceptanceBaselineInconsistent`，重投不会变（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)）。
  - 其余错误是读不动。
- 另立一口，不拓宽既有的反查口：一口一问（[ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md) Consequences 的先例）。

**二、实现只取快照的两段（机制）。**

- 一条 SQL：取行与其余按包裹读口同一个谓词、同一个部分索引；从快照只取接受基线，与基线所指那一版提交的地址要素子段。
- 两段用重建那条路上的同一组文档类型解码，这两段怎么读只有一处定义。
- 不过重建门（[ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)）。整份聚合完好与否由仓储读回时的重建门守；本口只守它读的两段上那三格自相矛盾。
- 反查与读取在同一条 SQL 里，两次读之间委托在动的窗口不再有。
- 不加列，不加迁移。

**三、网络与路由的适配器只换协作者（机制）。**

- `AcceptedCarriedGeoSource` 只收这一个读口。判断键各维、接受时刻与投影的取值逐格不变，复核触发指纹不变。交接与收寄采用两路共用它，派发装配两处随之只接一个协作者。
- 答格对应：
  - 「不是」照旧答「不是」；
  - 歧义照旧响亮上抛；
  - 接受基线自相矛盾归 `ErrAcceptedRequestInconsistent`，不进未决哨兵；
  - 其余归 `ErrAcceptedParcelUnreadable`，进本路未决哨兵、重投。
- 网络与路由的领域、复核编排与两路未决哨兵名单都不变。

**四、原读法留着（机制）。**

- 按包裹反查当前已接受委托、按来源身份读委托聚合两个读法照旧在：小包托运内部与别的上下文在用。
- 网络与路由的初始路由（接受决定信封那一路）照旧按来源身份读委托聚合，取接受基线、接受决定与商业依据解析引用。那一路要的不是地理维，R6 没问到它，本记录不动（越权风险点 3）。

## 候选与反方

- **读口内部整份读回、再从聚合上取投影**（复用其余按包裹读口共用的取行重建那一段）。否决：委托照旧整份读进网络与路由所在的进程，只是换到小包托运的适配器里读；R6 要的是取地理维时不经手其余部分。
- **维持过渡，不开读口。** 否决：R6 裁的就是过渡。
- **读口只交投影，判断键各维与接受时刻另经反查口取。** 否决：两次读之间委托在动的窗口又回来；判断键与投影必须出自同一份基线。
- **拓宽按包裹反查的那一口，让它连投影一起交。** 否决：那一口还有别的读者，拓宽等于让它们也拿到地理维；一口一问。
- **投影随交接信封携带，或网络与路由留存一份投影。** 否决，理由同 ADR-0215 的同名候选。

## 对已接受记录的作用

- **部分停用** ADR-0215 决定二三句：
  - 「适配器按（租户，包裹）反查当前已接受委托，读委托聚合，取受理基线那一版提交的地理投影。」改为：适配器按（租户，包裹）问小包托运的受理基线地理投影读口，取受理基线那一版提交的地理投影（决定一、三）。
  - 「委托聚合按小包托运的读法整份读回，地址本体与身份内容不取用、不落库，投影本体也不落库。」改为：读口只交地理维、接受时刻与判断键要的引用，委托聚合不读回；投影本体照旧不落库。
  - 「适配器只用小包托运已有的两个读法：按包裹反查当前已接受委托，按来源身份读委托聚合。小包托运不为此新开端口。」改为：适配器只用这一个读口，小包托运为此开了它。
  - **适用场景**：受理之后、不由小包托运信封触发的复核取所携地理投影，今天是交接进入节点这一种；收寄采用一路按 ADR-0279 决定四的「同一读法」取接受时刻，随之同走这一口。ADR-0215 决定二其余各句（只取用地理维、与两封信封同一份投影、进触发指纹、基线引用同源）与其余各条不变。
- **回答** ADR-0215 越权风险点 1：网络与路由不再经手委托聚合取地理维，小包托运的委托仓储少了这一个跨上下文读者，换成一个只交投影的读口。初始路由那一处读者照旧（越权风险点 3）。
- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：无条款停用。决定一由 ADR-0215 部分停用的适用场景与理由都不变，本记录只收窄那一场景下网络与路由读得到的面；决定二「地理维过界、身份维不过界」自此在读口这一层就成立。
- ADR-0279：无条款停用。决定四定的是取什么（接受基线固定的时刻）、与谁同源、读不动与读不着各归哪一格，读口换了之后三者都不变。它的候选「在 PS 读口上加一法取接受时刻」未采纳，理由是不必为接受时刻单独动小包托运；本记录为收窄可读面开口，接受时刻随之一并交。

## Consequences

- 代码随票 demo-chain-gaps/143：
  - 小包托运：领域 `AcceptanceBaselineGeoProjection` 与 `ErrAcceptanceBaselineInconsistent`，端口 `AcceptanceBaselineGeoProjectionView`，委托仓储上的 `LoadAcceptanceBaselineGeoProjection`。
  - 网络与路由：`AcceptedCarriedGeoSource` 的构造只收这一读口；派发进程交接与收寄采用两处装配随之改接。
- 无迁移。
- CONTEXT、CONTEXT-MAP 与 UC-NR-003 不改。各处写的是网络与路由的消费侧适配器按（租户，包裹）取受理基线那一版提交的同一份投影、只取用地理维，读口换成小包托运的投影口之后照旧成立。

## 越权风险点

1. **读口不过重建门**（PS owner）。只取两段：委托别处坏到仓储读不回时，读口照答，复核照跑。缓解：同一份委托的其他读者（初始路由、小包托运自己的写侧）读回时撞重建门，坏行看得见；本口守它读的两段上那三格自相矛盾。owner 若要读口连带担保整份聚合，只能改回整份读回（候选第一条），网络与路由所在进程就又经手整份委托。
2. **读口的 SQL 按名取快照的几个键**（PS owner）。接受基线、当前版本、历史版本、版本标识与地址要素子段的键名写在 SQL 里，快照改形时要跟着改。缓解：两段用同一组文档类型解码；真库用例比的是「与在整份聚合上取的同一份」，键名漂移时先红。
3. **初始路由一路仍读整份委托聚合**（NR、PS owner）。见决定四。网络与路由所在的进程因此仍会经手整份委托，只是不再为取地理维而经手。要收窄，照本记录的形状另开一口交接受基线与接受决定，另立票。
4. **坏行仍按读不动重投**（NR owner）。快照解码失败、标识过不了构造这类坏行，读口按读不动交出，网络与路由落本路未决哨兵、重投到预算耗尽——与整份读回时重建失败的落法相同。要把坏行分出来响亮失败，得在小包托运那一侧给坏行另立哨兵，那是所有按包裹读口共有的事，不在本记录。

## Links

- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)：前驱——部分停用决定二三句，回答越权风险点 1，见「对已接受记录的作用」
- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：地址随请求携带；决定一由 ADR-0215 部分停用，本记录无条款停用
- [ADR-0279](./0279-a-reassessment-trigger-earlier-than-the-shipment-acceptance-is-settled-as-not-judged.md)：接受时刻经同一读法取，本记录无条款停用
- [ADR-0130](./0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md)、[ADR-0133](./0133-delivery-condition-reference-is-the-acceptance-time-commercial-resolution-reference.md)：小包托运按（租户，包裹）答、一口一问的先例
- [ADR-0060](./0060-parcel-lookup-uses-current-snapshot-projection.md)：按包裹反查当前已接受委托，歧义不挑一份
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：重建门；本读口不过它的理由见决定二
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：读不动与自相矛盾按恢复动作分格
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：跨上下文适配器落在消费侧
- 票 `.scratch/demo-chain-gaps/issues/143-parcel-shipment-offers-an-acceptance-baseline-geography-projection.md`；出处为代决复核报告 `.scratch/owner-review-20261020/report.md` R6
