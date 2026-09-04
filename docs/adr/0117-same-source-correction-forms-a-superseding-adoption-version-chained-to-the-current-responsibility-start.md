# ADR-0117: 同来源更正版本在采用口形成新的采用判断版本——采用记录按来源声明的更正关系回指前版成链，当前责任起点按链尾派生；另一来源的竞争照旧不采用

Status: Accepted
Date: 2026-09-04

## Context

`UC-PS-003`「一致性、幂等与并发」节把 `AT-PS-049` 与 `AT-PS-050` 的硬句并列写着：「客户送站与场外揽收都指向同一包裹时不能形成两个责任起点。先合法形成者保留」与「来源更正、撤销或身份关系变化形成新的采用判断版本。原责任判断和承诺历史不能删除；当前承诺影响通过带原因的新版本表达」。`parcel-shipment` CONTEXT 同义：「源事实更正或事件有效性变化可以按业务发生时间和适用时间形成新的派生判断版本，不能按消息到达顺序覆盖」；「正式承诺后发生允许调整的情况时，形成带原因的新版本」。

代码里只有第一条。票 [label-channel-service-first-release/24](../../.scratch/label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md) 在 `main = 4cc1bc34` 上量到的事实（逐符号名）：`transport-fulfillment` 自 tf-segment-lifecycle-closure/08 起对同一（租户+对象+尝试）的揽收会落第二个版本（`OffsitePickup.Correct` 回指前版）并重交意图（信封 ID 加版本段、载荷带 `pickupVersion`）；`parcel-shipment` 这一侧 `psinbox.OffsitePickupConsumer` 不读 `pickupVersion`，`AdoptOnOffsitePickupAdapter` 按键 `FindByKey` 读到链尾那一代，`AdoptNetworkIntakeHandler.Handle` 的采用键带版本所以不撞幂等，随后 `FindResponsibilityStart` 命中首登版本，更正版本经 `refuse` 落成 `SOURCE_NOT_ADOPTED`，依据 `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<首登版本>`。也就是说，「同来源同对象的更正」与「另一来源想开第二个责任起点」在编排里是同一格；`AT-PS-050` 无代码实现。

库面上，`migrations/parcel_shipment/0004_adoption_cancellation_final.sql` 的 `intake_adoption` 靠部分唯一索引 `intake_adoption_responsibility_start`（每租户+包裹至多一行 `adopted`）承担责任起点唯一——它同时也拦住了「第二版本也 adopted」。同一份迁移里 `final_outcome` 已经走过一次「判断版本化」：重派生翻旧插新、`prior_version` 回指、每包裹至多一行 `is_current`（`AT-PS-063`）。那条路要 UPDATE 旧行的 `is_current`，而本票的硬句是「原责任判断与承诺历史不删，只插不改」。

领域上，`FormalCommitment.Adjust(version, reason)` 已能形成带原因、回指前版的承诺新版本，但它把 `intake` 与 `effectiveAt` 原样带过——它写给的是「路由、ETA 或现实履约变化」那一格，来源更正改的恰是 `intake` 本身（发生时刻、地点、控制依据），承诺的生效时间必须随更正后的发生时刻走（UC-PS-003：生效业务时间来自被采用的实际收寄时间，处理时间不能替代）。

同族的另一张票 [first-tenant-runway/10](../../.scratch/first-tenant-runway/issues/10-judgment-ledger-has-no-submission-version-dimension.md)（判断账没有提交版本维，draft）写了两条形状不裁。本记录不实施它，但下面的取法与它的「甲」（判断账长出版本维、旧版本留作历史）同一方向，不与它写的任何一句矛盾。

## Decision

**一、「同来源更正」以来源自报的更正关系识别，不以键相同推断。** 合格物理来源引用（`IntakeSource`）增加一格可选的「被更正版本」（`Corrects`），由来源所有者的事实带出——揽收那一路是 `OffsitePickup.Corrects()`，消费方适配器只翻译不推断。判据三件缺一不成立：来源种类与当前责任起点相同；来源声明了它更正哪一版；那一版恰是该包裹**当前**采用的版本（链尾）。键相同而没有更正声明（同一对象的新一次尝试）不是更正，走 `AT-PS-049` 那一格。

**二、采用账在采用记录上长版本链，只插不改；「当前责任起点」按链尾派生，不存一列。** `intake_adoption` 增加 `supersedes_source_version`（被取代的同种类来源版本）；一条包裹的采用行由此成一条链：根（`supersedes_source_version IS NULL`）是先合法形成的责任起点，其后每一版更正回指前一版。两条部分唯一索引替代原来的一条：`每（租户+包裹）至多一行 adopted 且无回指`——第二个根撞墙，`AT-PS-049` 仍由库面承担；`每（租户+包裹+来源种类+被取代版本）至多一行 adopted`——同一版本至多被取代一次，链严格线性，并发第二个更正撞墙。被取代版本以自引用外键指回同（租户+包裹+来源种类）下登过的那一行：链不跨来源种类、不悬空，这两句由库面说而不靠编排记得。`FindResponsibilityStart` 改答链尾（没有任何行回指它的那一行 adopted），编排据以核对更正关系与写拒绝依据；旧行一字不动。不采另一种结果词：更正版本形成的仍是「正式承诺已形成」，链上的位置由记录自己说（`SupersedesVersion`、承诺的前版与原因），结果代数不为它加格。

**三、承诺调整版本与采用判断版本同笔形成，不另立票。** 更正版本被采用时，编排以更正后的有效网络收寄重述承诺：新承诺版本号、`intake` 换成更正后的那一份、生效时间随之等于更正后的发生时刻、`priorVersion` 指回被取代采用行上的承诺版本、原因 `SOURCE_CORRECTED/<来源种类>/<被更正的来源版本>`。领域为此新增一条构造门（形照 `Adjust`，但收新的 `EffectiveNetworkIntake`，并要求同包裹、同接受基线、同来源种类）；`Adjust` 原样保留给不改收寄的那类调整。UC-PS-003 步骤 7「同一提交保存有效网络收寄采用结果、责任起点、正式承诺」对更正版本逐字成立——采用行既带新收寄也带新承诺版本，任一半单独落地都不许。

**四、消费方按信封所指的版本读回，不按键读当前版。** `psinbox.OffsitePickupConsumer` 读 `pickupVersion`（缺席即毒丸——TF 自 tf/08 起总带它，缺的那封重投不会长出字段），`AdoptOnOffsitePickupAdapter` 按（键+版本）取回并核对读回的版本等于信封所指。每一封信代表一代；按链尾读会让先后两封读到同一代，更正链在 PS 这一侧就少了中间那一代。

**五、`AT-PS-049` 的拒绝一字不动。** 当前责任起点存在而来源不满足决定一的三件——种类不同、没有更正声明、或声明更正的不是链尾——照旧不采用，依据 `RESPONSIBILITY_ALREADY_STARTED/<种类>/<链尾版本>`；只是依据里的版本从首登版本换成链尾版本，因为责任起点当前所在的那一代就是链尾。两格例外单列：更正所指的前版在采用账上**尚无任何记录**（先后两封乱序，或前版那封还没消费到）时不拒也不采，交回`资格判断未决`，原因 `CORRECTION_PREDECESSOR_UNJUDGED`——重投会改变结果；更正所指的前版**有记录但不是链尾**（分叉：它已被另一版取代，或它本就是不采用行）时不采用，依据 `CORRECTION_TARGET_NOT_CURRENT/<种类>/<链尾版本>`——那不是等谁，是要人看。

## Consequences

- `AT-PS-050` 第一次有代码实现，且与 `AT-PS-049` 在编排里分成两格：判据是来源自报的更正关系是否接到链尾，不是键。
- 采用记录（`IntakeAdoptionRecord`）多一格 `SupersedesVersion`；承诺列多两格（`commitment_prior_version`、`commitment_adjustment_reason`），三格同在或同缺，库面 CHECK 钉住；不采用行照旧不带。`adoptionColumns` 原来的「采用记录只承载首版承诺」那道门改为「根承载首版、更正版承载带前版的承诺」。
- 网络收寄意图（`OutboxNetworkIntakeHandoff`）不改：信封 ID 已含来源结果版本，更正版本自然多一封；`network-routing` 的复核消费者按采用键读回，读到的就是那一代。
- 取消边界、资格与接受基线三道门对更正版本**重新走一遍**，用的是更正后的内容——那正是 tf/08 裁决说的「PS 必须再判一次」。更正版本在这三道门被拒时，根采用原样站着，拒绝行带依据；PS 不替人裁「更正与原判断谁对」。
- `FindResponsibilityStart` 的语义从「那一行 adopted」变成「链尾」。采用编排（`AdoptNetworkIntakeHandler.Handle`）按它核对更正关系、写拒绝依据；读面若日后要列「责任起点历史」，读的是整条链，不是一行。
- 代价：PS 迁移 `0017`（新列、两条索引换一条、CHECK、指回同种类前版的自引用外键）；`OffsitePickupFinder` 消费方接口从按键读改为按键与版本读，TF 的 `OffsitePickupRegistrations` 要多一个 `FindByKeyAndVersion`（与 `EffectiveDeliveries` 同名同形，TF 地盘，另行占号）；`cmd/parcel-dispatch/assemble.go` 装配不必改——仍传同一个登记库实例。
- 不在本记录内：节点收寄那一路（`node-operations`）今天不落更正版本，`Corrects` 对它恒缺席；更正改了发生时刻之后路由复核要不要换路由归 `network-routing`；判断账的提交版本维归 first-tenant-runway/10。

## Alternatives considered

- **另立结果词（如 `SUPERSEDING_ADOPTION`）。** 否决：UC-PS-003 结果语义契约的六格里没有它，而它的业务含义与「正式承诺已形成」相同——包裹进入网络服务责任范围，只是承诺换了版本。多一格结果词就要多一格消费译法与读面译法，而记录自己已经说清了链上的位置。
- **照 `final_outcome` 的形状：`is_current` 列，翻旧插新。** 否决：翻旧要 UPDATE 已落的责任判断行，与本票硬句「只插不改」相违；链尾按回指派生在 `transport-fulfillment` 的揽收登记（迁移 0015，`offsite_pickup` 表无 current 列）已走通一次，同形照搬即可。`final_outcome` 那条路不改。
- **只按键判同来源：同（租户+包裹+来源种类）的新版本一律视为更正。** 否决：同一对象的新一次尝试（退运再出、召回后再揽收）在 PS 看来也是同种类同包裹的新版本，按键判会把新一段服务的揽收当成对上一段责任起点的更正；更正关系只有来源所有者说得清，PS 只引用不推断。
- **承诺调整另立票，本票只做采用判断版本。** 否决：UC-PS-003 步骤 7 要求采用结果、责任起点与正式承诺同一提交；采用了更正后的收寄而承诺仍指着旧的发生时刻，就是半份客户责任结果。领域已有 `Adjust` 的形状，多一条收新收寄的构造门代价很小。
- **在消费者里按 `pickupVersion` 与当前版比对、不同即跳过。** 否决：跳过等于把那一代从 PS 的历史里抹掉——更正所指的前版永远没有记录，后一封只能停在 `CORRECTION_PREDECESSOR_UNJUDGED`。按版本读回让每一封各归各代。

## Links

- [UC-PS-003](../application/parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md)：`AT-PS-049`、`AT-PS-050`、一致性节与步骤 7
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)：责任起点、「源事实更正……形成新的派生判断版本，不能按消息到达顺序覆盖」、「正式承诺后……形成带原因的新版本」
- [ADR-0005](./0005-source-facts-effective-events-derived-state.md)：来源事实、有效事件与派生状态分离——链尾是派生，不是存的
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：意图由结果标识认领——更正版本自然多一封
- [票 label-channel-service-first-release/24](../../.scratch/label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md)：量到的事实与本记录的实施
- [票 tf-segment-lifecycle-closure/08](../../.scratch/tf-segment-lifecycle-closure/issues/08-offsite-pickup-correction-model.md)：TF 侧更正链与「PS 必须再判一次」
- [票 first-tenant-runway/10](../../.scratch/first-tenant-runway/issues/10-judgment-ledger-has-no-submission-version-dimension.md)：同族的判断版本化，本记录不实施
