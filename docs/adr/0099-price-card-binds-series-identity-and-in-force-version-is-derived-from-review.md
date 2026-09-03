# ADR-0099: 价卡绑定序列标识而不是序列版本；在用序列版本按评价形成时刻从复核记录派生，解析结果冻结进评价的版本清单

Status: Accepted（2026-09-03，用户经 IDP 队列通道 3 授权本会话「作为业务和系统专家自决」，据此对计价参考序列的运营形态作出裁决。裁决能力边界见文末）
Date: 2026-09-03

## Context

[ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 把计价参考序列的「登记、版本化、发布治理和按计价基准时点的解析结果」交给 `parcel-pricing`。[syn-wall-door-audit/08](../../.scratch/syn-wall-door-audit/issues/08-pricing-reference-series-register-missing.md) 建了登记册、`ResolveAt`（租户 + **序列版本引用** + 计价基准时点）与登记口，并在票面评论里写明「评价用例接解析口（消费面）不在本票」。发布治理没有任何机制件。

动笔前核出的事实（实测于 `0d492b8`）：

- `ReferenceSeriesBinding` 持一个 `VersionReference`，而 `NewVersionReference` 要求 id、version、digest 三格皆非空。`NewPricingPlanVersion` 把每条绑定的版本引用并入方案的版本清单，`canonicalReferenceSeriesValue` 把它折进方案内容摘要。`resolveSeries` 要求输入快照里的取值与方案绑定的版本引用**逐格相等**，否则评价落 `REFERENCE_SERIES_VERSION_MISMATCH`（冲突）。
- 合起来的含义：**序列每出一版，引用它的每一张价卡都必须再登一版**，否则新取值进不了评价。`ReferenceSeriesBinding` 的类型注释写着「绑定指名的是序列，绝不是取值」，代码绑住的却是版本——意图与形状分岔。
- 行业里汇率按日公布是常态：SAP 的 `TCURR` 按「文档日期当日或之前最近一条生效行」取值；CargoWise 把取值日做成发票日 / 过账日 / 当日 / 孰早等可配规则。按日出版本乘以每卡重登，在结构上不可运营；而 SAP 式的隐式取值又是用**当前表**回看，历史评价会随补登与更正而变，正撞上 CONTEXT「重放使用原序列取值，不读取当前值」。
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-SET-11` 把「登记责任方」与「复核责任方」分列为两种责任；登记册今天只有登记。摘要自校守的是登记之后被改，守不了登记之时抄错——人工转录错误会带着合法摘要入册。价卡有「已校验 → 已批准 → 已发布」生命周期，序列版本在登记之后没有任何状态。

于是缺的不是一个页面，是三件互相咬合的决定：价卡到底绑什么；一版取值凭什么可以被新评价采用；「采用哪一版」这个答案记在哪里。

## Decision

**一、价卡绑定序列标识，不绑序列版本。**

`ReferenceSeriesBinding` 改为（种类 + 序列标识）。方案的版本清单与内容摘要不再含任何序列版本引用；规范化形状按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 纪律递增为 `PPC-4`，不就地改写 `PPC-3`。类型注释那句「绑定指名的是序列，绝不是取值」从此与形状一致。

**二、序列版本有登记之后的生命周期，推进它的是追加式的复核记录。**

已登记 → 在用（复核通过）；已登记 → 已退回（复核退回）；在用 → 已替代（同序列更新的版本进入在用）。

复核是一条独立事实——复核责任方、复核时刻、结论、依据——**追加**在版本之旁，不改登记行一个字节。复核责任方不得与该版本的登记责任方相同（`PAR-SET-11` 分列两种责任的机制落点）。复核不改变证据等级：`VERIFIABLE` / `ASSERTED` 是取值凭证的属性，复核确认的是转录——一版 `ASSERTED` 可以复核通过，通过之后照样只能支撑隔离 `S`。退回不删除、不进在用；同一版本可再次复核，仍是追加。

**三、在用版本是派生结论，按（租户、序列标识、评价形成时刻）从复核记录算出，不是一个可维护的指针。**

在用版本 = 该时刻之前复核通过的最新版本。「最新」按复核时刻，同刻按登记时刻，再同则按版本号字典序——判定必须确定，不留任选。

这一条把两个时点分开了，而分开它们正是本记录的核心：**计价基准时点决定期次，评价形成时刻决定版本。** 一份 2026-09-01 计价基准时点的评价，若在 09-03 形成，用的是 09-03 那一刻在用的版本里覆盖 09-01 的那一期；09-05 补登一版更正并复核通过，不改变它——重放读清单里冻结的版本，不重新算在用。

没有在用版本时评价落待判断，沿用 `REFERENCE_SERIES_UNRESOLVED` / `EXCHANGE_RATE_UNRESOLVED`，原因文字区分「无已登记版本」「有版本但未复核」「在用版本无覆盖该时点的期次」。三者恢复动作都是登记侧的一次动作（登记 / 复核 / 补期次），属 [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 的「等运营登记」格，**不新开结果格**。

**四、解析在评价用例的编排层完成；评价的版本清单 = 方案清单 + 本次解析到的序列版本引用。**

`EvaluatePricingHandler` 在形成评价之前，对方案的每条绑定：解析在用版本 → 按计价基准时点在该版本内解析期次 → 取值连同版本引用进入输入快照。`EvaluatePricing` 仍是纯函数，不读端口；它把解析到的版本引用并入评价自己的清单。重放携带原输入快照（内含原取值与版本引用），不再解析在用；清单不等即 `VERSION_MANIFEST_MISMATCH`——既有规则，此处只是让它守住正确的对象。

**五、延展与更正都是新版本，都经复核进入在用。**

延展（新增期次、闭合前一版开放的末期）不带更正关系；更正带 `PriorVersion` + `CorrectionBasis`（既有规则）。每一版都是**整版重述**——含该序列自起点以来的全部期次，解析只在一版之内完成，与 [ADR-0067](./0067-cost-correction-restates-the-whole-evaluation-result.md) 的「整组重述」同一形状。既有评价不动。

**六、来源连接器是登记责任方的转录代理，不是第二种登记。**

自动喂价落在 `adapters` 层，产出登记快照走同一登记用例；抓取到的公布记录原文（内容哈希 + 抓取时刻 + 地址）就是那一期的取值凭证，所以连接器登记的期次天生 `VERIFIABLE`。连接器登记的版本同样经复核进入在用。「某来源的版本免人工复核」是租户对该来源的信任声明，属实例半边：机制提供这一格，**无默认**——未声明即需人工复核；声明免复核时，复核记录由系统代写并把依据标为该声明及其版本。连接器抓取失败不补数、不沿用旧值，只留缺口与告警——缺口让评价挂起，正是 ADR-0013 要的行为。

**七、不在本记录。** 海关计税汇率是否作为第三种序列种类、由哪个上下文消费；加点规则（[party-commercial-context-gaps/02](../../.scratch/party-commercial-context-gaps/issues/02-pricing-caliber-is-a-required-reference-to-a-hollow-referent.md) 的裁决）；汇兑损益与重估（[ADR-0007](./0007-separate-operational-settlement-from-statutory-finance.md)）。

## Consequences

- 领域：`ReferenceSeriesBinding` 改形，`PricingPlanVersion` 清单与摘要去掉序列版本，`PPC-4`；新增序列复核值对象与「在用版本选择」纯函数；`EvaluatePricing` 组合清单。`PPC-3` 快照按既有纪律在重建门被拒——本仓无租户、无生产价卡，代价只有重跑 `scripts/demo-seeds/seedgen`。
- 持久化：`parcel_pricing.reference_series_review` 追加表（迁移 `0004`）；`reference_series_version` 不改一列。
- 端口：不拓宽 `ReferenceSeriesRegister`（拓宽会拆全部写侧替身，`ports/catalogue_read.go` 记过这条）；另立复核写口与在用解析读口。
- 应用：新增复核用例；`EvaluatePricingHandler` 增在用解析依赖。输入快照已自带取值的请求（重放、以及今天的全部调用方）不触发解析，行为不变。
- 接入与管理台：复核端点以未配置格进端点表（[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)）；受控 CLI 增复核种类供种子；管理台增复核动作、逐字段登记表单、更正动作、覆盖地平线看板——各为后续票。
- 种子：SYN 序列需一条复核记录种子才进在用，否则 demo 评价按第三条如实挂起。这是对的：demo 从此也分得开「登了没复核」与「没登」。
- `PAR-SET-11` 一字不改（它登的是实例）；「复核责任方」那一格从此有机制承接。

## Alternatives considered

- **保持价卡绑版本，序列每出一版就重登价卡。** 否决：把序列的公布节奏耦合进价卡治理；按日汇率乘以每卡重登在结构上不可运营；且价卡版本清单会因一个与卡无关的数变动而变，摘要失去「卡内容变了」的含义。
- **SAP 式隐式取值：不分版本，按基准日取最近生效行。** 否决：用当前表回看历史，补登与更正会改变既有评价可读到的值，直接违反重放不变量。它省掉的那个版本维恰是可重放的全部依据。
- **显式发布指针表，运营手动指定在用版本。** 否决：每次喂价都多一个人动的步骤——对连接器场景等于取消了连接器的价值；指针是可变状态，与登记册只增不改的纪律相悖。从追加式复核记录派生同样确定、同样可审，且无物可维护。
- **复核做成版本行上的状态列。** 否决：行只增不改；复核有自己的责任方与时刻，是另一件事实而不是原事实的属性。写成列会让「谁在何时凭什么通过」在结构上无处落。
- **自动喂价一律免复核。** 否决：`PAR-SET-11` 指名复核责任方；是否信任某个来源到免复核的程度是租户对来源的判断，产品替它默认等于替它承担转录责任。
- **把在用解析放进 `EvaluatePricing`。** 否决：领域纯函数不读端口；解析是编排，放进领域会让每次重放都要一个端口替身。
- **只做管理台表单，不动绑定形状。** 否决：表单登进去的每一版都进不了任何一张已发布价卡的评价——那是把一条不可运营的路径做得更好用。

## 裁决方的能力边界

本记录由 MCP-3 受用户明确授权自决。读过：ADR-0013 / 0014 / 0085 / 0094 / 0098 全文；`parcel-pricing` CONTEXT 的术语、不变量、生命周期三节；`reference_series.go`、`reference_series_register.go`、`plan_structures.go` 中 `ReferenceSeriesBinding` 与 `PricingPlanStructures` 的校验、`plan.go` 清单装配、`evaluation.go` 中序列解析与清单相关段、`fingerprint.go` 的规范化版本注释、`ports/reference_series_register.go`、`adapters/postgres/reference_series_register.go`、`application/evaluate_pricing.go`、`0003_reference_series_register.sql`、`scripts/demo-seeds/seedgen/main.go` 的绑定调用；`PAR-SET-11` 全行；头部企业与国内货代系统汇率管理的公开资料（SAP `TCURR`/`OB08`/`TBD4`、SAP TM 汇率日期 KBA、CargoWise 计费汇率规则与 CFX、DHL 服务条款与 CFX 指数、Freightos SOP、海关计税汇率规则）。**没有读**：`settlement-accounting` 消费评价清单的具体比对方式（决定四改变清单组成，消费侧若逐格比对方案清单会受影响——实施票须先核）；`network-routing` 择优对候选评价清单的处理；任何真实租户的汇率来源与复核组织方式（不存在）。

因此本记录裁的是**价卡与序列之间引用的粒度、序列版本进入在用的门、以及在用版本的派生规则与解析位置**。它没有裁复核端点的表单形状、连接器的抓取协议、覆盖告警的阈值——前两者属实施票，后者属实例半边。

## Links

- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：所有权划分——本记录在其「发布治理」一格上补形
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：`PPC-4` 依它递增
- [ADR-0067](./0067-cost-correction-restates-the-whole-evaluation-result.md)：整组重述——决定五同一形状
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：复核端点沿它进端点表
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：「等运营登记」格——决定三不新开结果格的依据
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：术语「计价参考序列」「序列版本复核」「在用序列版本」与生命周期「计价参考序列版本」随本记录更新
- [pricing-reference-series-operations](../../.scratch/pricing-reference-series-operations/spec.md)：实施票
