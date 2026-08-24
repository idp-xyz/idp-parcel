# 三个路由证据视图卡在 PAR-NET-14，机制半边需要单独切出来

Category: enhancement
Status: needs-info

发现于 `a5095ae`（NR/TF 端口换真）。那一笔带走了七口，`NetworkEvidenceView`、
`InitialRouteEvidenceView`、`AutoRerouteFactsView` 三口没带走。本票记的是**为什么没带走**，
以及**其中哪一部分今天就能做**。

三口的端口声明与领域评估函数都已就位，缺的只是取数侧。本票不替领域定任何规则取值。

## 为什么现在写实现会撞红线

三层，缺租户数据只是最外面那层，且不是主要理由。

### 一、规则本身未定，且明禁默认

[`PN02-W04` 证据工作单](../../../docs/design/pn-02-w04-pre-acceptance-logical-reachability-evidence-request.md)
在开头的「不规定」清单里点名**路由算法**，并在「给开发的交接」一节写死：

> 不得写入默认国家、区域、线路、节点、时区、日历、截单、口岸、资格或新鲜度。

而这三个视图要产出的正是逐候选事实。产出它们所需的规则——候选生成、过滤、排序，
服务区域与业务时区的采用方式，服务日历与截单的采用方式，冻结边界，自动/人工改路
条件与权限——在[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md)里全部
归 `PAR-NET-14` 一行，状态「待提供」。写实现就是把这些规则定死成生产默认。

领域侧对同一件事已经有明文。`ServiceAreaResolutionSpec` 的注释说「真实区域表与地址
语义属实例半边（`PAR-NET-*`），机制只规定事实的形状与判定规则」；`RouteRequirementSpec`
说「哪些候选满足该要求**由事实提供方解析**」；`CandidateTimeProjection` 说「**取数侧**按
版本化服务日历、截单、节点处理时间与衔接缓冲为一个候选算出的预计完成窗口」。三处都在
指同一个尚不存在的取数侧。

### 二、`NetworkEvidenceView` 还缺一条真缝

它要按目的地解析服务区域，而 [network-routing CONTEXT](../../../docs/domain/network-routing/CONTEXT.md)
写明「客户地址继续由 `parcel-shipment` 保存。`network-routing` 只保存服务区域、节点覆盖
版本和当次解析依据」。`internal/networkrouting/ports/ports.go` 里没有任何读 PS 地址的端口，
[CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) 的 `parcel-shipment → network-routing`
一条里客户地址确实由 PS 提供，但提供路径未定。补这条是跨上下文边界决策，不是端口实现。

### 三、实例半边确实也空着

`PAR-NET-01` 的登记册原话：

> 待提供：当前没有真实线路；`LANE-PILOT-01` 仅为未来受控映射的预留标识，不能据此生成
> 生产路由

`PAR-NET-02` 至 `PAR-NET-06`（始发区域、目的区域、节点、自营节点、场站及作业方式）同为
待提供。这一层是常态，不构成不做实现的理由，单独列在下面的阻断项里。

## 「先搭个读快照表的机制壳」这条路已经否掉了

评估过，形状上就错，别再走一遍。

设想是：建一张按判断键存放已发布证据快照的表，视图只读它，未配置即报错。问题出在键上
——`LoadNetworkEvidence(ctx, key)` 的 `ReachabilityJudgmentKey` 含 `JudgmentAsOf`，而 `asOf`
由发起方按接单规则包的时点策略在**请求期**解析并传入。任何生产者都无法预先为一个尚未
发生的 `asOf` 写好快照行。那张表因此不是「暂时空着、等租户登记就会满」，而是**结构上
填不满**——与
[`ve-milestone-mapping-key/issues/01`](../../ve-milestone-mapping-key/issues/01-mapping-catalog-keyed-on-fact-reference-is-unfillable.md)
记的是同一类错误，只是那边是键取了实例引用，这边是键取了请求期才存在的值。

真正对的机制是「**按 `asOf` 查版本化网络目录**」：目录行带自己的有效区间，查询按判断时点
选版。它解得开这个问题，但它的规则正文就是 `PAR-NET-14`。

## 机制半边：今天就能做的部分

以下不依赖任何 `PAR-NET-*` 取值，只依赖「事实要长什么形状、怎么按时点选版、读不到时
怎么答」这三件已经由 CONTEXT 与端口注释确定的事。

1. **版本化网络目录 schema**。节点、有向连接、线路、服务区域、服务日历与截单、临时网络
   可用性调整、路由策略，各自一张表，各自带版本与有效区间。CONTEXT 的硬句已经把形状
   定死了：「稳定网络定义和临时网络可用性调整必须分离」「节点、连接、线路、服务区域、
   服务日历或路由策略发生永久变化时形成新版本，不覆盖原版本」「新版本自明确生效时间起
   参与适用范围内的新判断」。表的列与 CHECK 由这些句子推得出来，不需要任何取值。

2. **按 `asOf` 选版**。给定判断时点，在每张目录里选出该时点生效的那一版；两版同时适用
   交回错误而不是挑一个（先例：VE 的映射目录已按这条办）。业务时区按各自节点/线路解释、
   跨节点衔接按绝对时刻比较，这两条同样是 CONTEXT 明文，不是取值。

3. **显式未配置报错**。三个端口的注释已经把答复定死：`NetworkEvidenceView` 那条写「调不通、
   超时、**配置读不到**都要作为错误返回，由应用层形成`未形成判断`」；`AutoRerouteFactsView`
   的第二个返回值为 false 即「事实目录未配置」，且注释写明「不猜：只失效不改路，连改路
   建议都形不成」。两种答复语义不同，实现时别压成一格。

4. **视图修订标识的产生与原子性**。`NetworkViewRevision` 是消费方日后比对「关键依据已经
   失效、被替代或修订标识变化」的锚，而端口注释要求「事实与修订**一次取回**而不分多次
   调用：两次取回之间视图一变，事实与它标的修订就不再来自同一版」。修订从哪里来、怎么
   保证与事实同版，是机制问题。`create_initial_route.go` 的提交前重校（读两次证据比对
   `ViewRevision`）依赖它成立。

## 阻断项：不在本票范围，指向登记册

以下全部属实例半边，本票不定、也不建议用任何默认值代替：

- `PAR-NET-01` 成熟主力线路、`PAR-NET-02/03` 始发与目的区域、`PAR-NET-04` 节点、
  `PAR-NET-05` 自营节点、`PAR-NET-06` 场站及作业方式——真实拓扑与覆盖。
- `PAR-NET-14` 初始路由及收寄/实测后复核策略——候选生成/过滤/排序规则、日历与截单的
  采用方式、冻结边界、改善阈值、自动/人工改路条件与权限。
- 真实网络地图、地址样本、日历、截单与限制内容按 `PN02-W04` 的要求留在受控证据库，
  仓库只登脱敏标识与证据索引。

状态一律以[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md)为唯一权威；
机制半边做完不改变其中任何一行的状态。

## 还需要一个决定

机制半边做完之后，`NetworkEvidenceView` 与 `InitialRouteEvidenceView` 仍差「谁把目录折成
逐候选事实」。那一步是取数侧，规则归 `PAR-NET-14`。所以本票的机制半边落地后，三口仍不能
交付，只是阻断原因从「三层」收敛为「一层：`PAR-NET-14` 的规则正文」。

要不要在规则未确认前先建目录 schema，是本票要 triage 的那个问题：建了它，`PAR-NET-14`
到位时只剩折叠规则要写；不建，到位时目录与折叠一起写。两条路都不违规，差别在于先做的
那部分会不会因为规则形态而返工。

## 当下状态

- 三个端口声明与领域评估函数（`EvaluateServiceAreas`、`EvaluateRouteRequirements`、
  `EvaluatePathExecutability`、`EvaluateHardConstraints`、`EvaluateTimeFeasibility`、
  `SelectRouteCandidate`、`EvaluateAutoRerouteConditions`）都已就位并有领域用例覆盖，
  缺的只是取数侧。
- 编排侧的三条出口都已按端口注释接好：证据读不回形成`未形成判断`/`路由判断未决`，
  答复缺修订标识响亮上抛，事实本身不成立响亮上抛。本票解决前，这三口在生产装配里
  没有实现可注入。

## Comments

- 2026-08-20 MCP-1（用户批复「按建议办」）：**「还需要一个决定」那问的答案是建。** 机制四件
  （目录 schema、按 `asOf` 选版、显式未配置报错、修订标识原子性）现在开工，派 MCP-4，票名
  NR-CATALOG-MECH。依据即 AGENTS.md 红线原话「机制现在就做，实例留空并拒绝默认值」——本票
  已论证四件只依赖 CONTEXT 硬句。护栏三条：**schema 只用 CONTEXT 点名的名词建表，不预埋折叠
  语义列**（折叠规则归 `PAR-NET-14`，形态未定不入库）；**迁移只建结构，不种任何默认行**；
  **三口取数侧不接**，继续报「配置读不到 → 未形成判断」。做完后阻断收敛为一层（`PAR-NET-14`
  规则正文），参数登记册任何一行状态不变。`NetworkEvidenceView` 缺的 PS 地址提供路径属跨上下文
  边界决策，划出为本目录 `02` 号票，不混入本票。另：`syn-wall-door-audit` 审计票 05（自动改道
  事实目录未实装）与本票同域，采纳时已互链。
- 2026-08-20 MCP-4（NR-CATALOG-MECH 完工）：机制四件已落——迁移
  `migrations/network_routing/0008_network_catalog.sql`（七表+目录修订锚，只建结构不种行）、
  适配器 `internal/networkrouting/adapters/postgres/network_catalog.go`（七类登记口 + 单语句
  快照读口，三格答复；两版同时适用交回 `ErrAmbiguousNetworkCatalog`；调整族按历史链选当前
  陈述；每笔写入同事务推进修订）。决定记录 [ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md)
  （部分停用 ADR-0053「现在不建表」半句）。三口取数侧未接（0007 登记册照旧作答），登记册
  `PAR-NET-*` 行状态未动；PS 地址缝未碰（归 02 号票）。本票剩余问题照「还需要一个决定」节
  预告收敛为一层：`PAR-NET-14` 规则正文（解析层 + 内容列扩展）。提交 SHA 见 MCP-1 集成记录。
- 2026-08-24 MCP-6（状态簿记，无代码改动）：三口之一已关——`AutoRerouteFactsView` 的存储、
  装载口、写入方与登记口随 syn-wall-door-audit 票 05 落 main（`ed77025`，`assemble.go` 已换
  真适配器，空册行为与 nil 等价）。余两口（`NetworkEvidenceView`、`InitialRouteEvidenceView`）
  照本票预告只剩一层阻断：`PAR-NET-14` 规则正文（折叠规则，实例半边待提供）。仓内已无本票
  可推进的机制工，in-progress 改 needs-info；重启条件 = 参数登记册 `PAR-NET-14` 行状态变化。
