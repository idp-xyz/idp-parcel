# 面单渠道服务首发机制半边

Category: feature
Status: in-progress——32 张子票（`18`–`21` 于 2026-09-03 随 `16` 收口拆出；`22` 于 2026-09-04 随 `19` 收口立；`23` 于同日随 `14` 收口立；`24` 于同日由 tf/08 的 PS 侧核查立；`25`–`28` 于 2026-09-10 由通道 3 按 `11` / `12` 收口留下的后继立；同日用户经队列授权推送方代裁后：`26` 经 ADR-0134 转 ready-for-agent，`27` 要裁的清零、Blocked by `30` + pc-gaps/13，`28` 裁乙、Blocked by `29`；`29` 由通道 3 立为 `28` 的翻译适配器票（ready-for-agent）；`30` 由通道 2 立为 `27` 的 PS 写面上游（要裁的已清零，Blocked by pc-gaps/13——13 同日转 ready-for-agent）；`31` 由通道 6 随 ADR-0135 立为 `25` 的 TF 侧上游（ready-for-agent）；`25` Blocked by `31`；`32` 由通道 2 立为「06 `Establish` 核继续尝试登记册的关闭边界」（ready-for-agent，Blocked by `30`）——PS CONTEXT 硬句「关闭生效后拒绝边界后新交易」此前代码无守卫）；`01`..`19`、`21`、`23`、`24` 已 resolved（`19`、`21` 自分支 `mcp5-lc19-21`、`14` 自分支 `mcp2-lc14`、`23` 自分支 `mcp2-lc23`，均于 2026-09-04 进 main；`23` 的六处接线由 MCP-1 在 `4c466b0a` 落；`24` 自分支 `mcp5-ps-lc24` 同日快进入 main `24d94c94`..`11ee57aa`——ADR-0117，PS 迁移 `0017`，同来源更正版本在采用口形成新采用判断版本并回指前版，AT-PS-049 不动）；`20`、`22` draft（等 `18` 之后的第一家真源，`22` 另阻塞于 `20`）。状态行由通道 2 于 2026-09-04 对票面重核后改写，`14`/`19`/`21`/`22`/`23`/`24` 六格由 MCP-1 于同日重放入 main 后对齐，`24` 于同日 23:5x 由 MCP-1 再对齐一次（此后**以各票文件 `Status:` 为准**）

## 这个 feature 是什么

[ADR-0088](../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) 把面单渠道服务从「首发范围裁剪掉的形态」改为**首发对客销售的独立服务形态**。范围落文已完成（PILOT-SCOPE、参数登记册 `PAR-COM-04`/`PAR-COM-12`/`PAR-INT-02`、验收矩阵 `CPS-06`/`CPS-07`、PC CONTEXT 首发范围硬句、`UC-PC-001`/`UC-PC-002` 验收行、[渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)的范围口径都已随之修订）。

**范围落文不等于机制就位。** ADR-0088 的 Consequences 列出了「机制半边要补的能力形状」，并明说本 ADR *不预判各项在代码中的现状*，盘点归本目录。本 feature 承载的就是那次盘点及其派生的工程票。

盘点产物两份，均为只读、不改产品代码。**两份均已交**：

- [末端面单渠道能力形状盘点](./capability-shape-inventory.md)——客户尾程流「下单 → 取末端渠道面单 → 面单 PDF 返回/修改 → 轨迹回传 → 结算」这条链在主线上今天有哪些形状、缺哪些。
- [轨迹源接收缝盘点](./tracking-source-seam-inventory.md)——外部轨迹源（17track 与承运商直连）「拉取/接收 → 事实 → 里程碑映射 → 内部投影/客户可见」这条链同样逐段量。

## 边界

**在本 feature 范围内**：机制半边。端口形状、聚合与状态、执行器（应用层用例）、装配接线、迁移与读面——凡是不需要真实租户参数就能立起来的骨架。

**不在本 feature 范围内**：

- **实例半边的任何取值。** 渠道产品与账号（`PAR-INT-02`）、逐份渠道报价表及其价表族/体积系数/进位分段（`PAR-SET-03`）、对客价卡（`PAR-COM-10`）、面单服务终局规则版本（`PAR-COM-05`/`PAR-COM-07`）——本 feature 一个都不填，也不写技术默认值顶替。ADR-0088 Decision 五：机制里不内置任何渠道字段名、体积系数或金额。
- **采购过程。** 不向渠道询价、竞价、授予。PILOT-SCOPE 两条既有约束原样适用于面单渠道服务。
- **对客户的价格承诺。** 渠道候选择优是运营企业内部成本决策，首发只按成本单维（`PAR-NET-16`）。
- **领域语言的新造与改写。** ADR-0088 Decision 二：面单渠道服务按 PC/PS CONTEXT 既有定义进入首发，首发范围的变化只发生在产品文档。要改领域语言得先回 `CONTEXT.md`，走 [AGENTS.md 的「改文档」](../../AGENTS.md#改文档)。
- **范围决定本身。** 已由用户裁定并落 ADR-0088，本 feature 不重开。
- **网络服务主链路的试点约束。** 一条线路、一个锚点客户、三阶段执行与 `PN-08` 治理不变；面单渠道服务按同一 `PN-08` 治理走，不另设阶段模型。

## 与既有票族的关系

- [渠道适配缝备忘](../../docs/design/channel-adapter-seams-design-note.md)（票 `product-story-and-demo/02` 的产物）已经把六类渠道数据各指到所有权上下文与既有缝位，并写明「本文是缝位登记，不是工作包」，触发条件是「能力范围判据点名真实商业模式时按本文缝位立票」。**ADR-0088 就是那次点名**，因此本 feature 的票按备忘的缝位落，不重做缝位判断。
- [`tenant-implementation-01/requirement-mapping.md`](../tenant-implementation-01/requirement-mapping.md) 尾程流表的「待建」项是本次盘点的起点；该表锚在 `2ab7f89`，两份盘点重取了证据，判定以盘点为准。

## 子票

清单 32 张。`11` 留下的 `JudgeLabelServiceFinalHandler` 三个调用方与 `12` 留下的择优链组合根与调用入口已于 2026-09-10 立为 `25`–`28`（通道 3），其上游与拆出的适配器同日立为 `29`–`31`。PC 半边 `DeclaredResponsibilityOutcome` 加面单渠道两行已由 pc-gaps/12 承接（2026-09-10 ready-for-agent）；`27` 的上游（关闭 / 重开决定写面）已立为 `30`（PS 半边）与 pc-gaps/13（PC 授权动作加两格）；`25` 的上游（TF 侧首次有效收寄判断）已于 2026-09-10 经 [ADR-0135](../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 裁形并立为 `31`。四张裁决票（`01`..`04`）不写实现，它们阻塞其后的实现票——不先裁，后面每张都会各自假设一个答案，而**重复或分叉的实现在 `go build` 与 `go test` 下全绿**，要到集成才看得见。

### 裁决票（四张均已 resolved，2026-09-02 由 MCP-4 经 owner 授权裁定）

| 票 | 裁决结果 |
|---|---|
| [`01` 渠道候选择优的平局处置归谁](./issues/01-channel-candidate-tie-break-authority.md) | **取乙**：渠道择优另立比较器落 `parcel-shipment`，不动 `SelectRouteCandidate`，**不需新 ADR**。「不得任选」读作**禁无业务依据的选择**，故标识升序在渠道侧不合规，平局交冲突。据此 `12` 落 PS 侧、`13` 的出局判断落择优层、`14` 不复用 `RouteCandidate` |
| [`02` 出向集成缝在本仓没有先例](./issues/02-outbound-integration-seam-has-no-precedent.md) | 落 [ADR-0090](../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)：失败代数按**恢复动作**三分（继承 ADR-0029），`答案未确定` 为默认格，**不镜像 ADR-0022**（对端不受它约束，UniUni 恒 200 是反例），未配置即拒，超时与重试无生产默认值。平台包随 `07` 落 |
| [`03` 外部轨迹由谁收编、时间由谁认领](./issues/03-external-tracking-fact-owner-and-time-minting.md) | 归 **TF 但新立一类**「外部承运轨迹事实」，不塞进自营作业事实。三时间分铸：`ReceivedAt` 本仓铸、`OccurredAt` 源不给即拒收、`EffectiveAt` 由 TF 作显式判断。**实现前必须先补 TF `CONTEXT.md` 与一篇三时间 ADR**（前置条件记在 `16`） |
| [`04` 面单文件是生成还是取回后加工](./issues/04-label-document-generate-versus-post-process.md) | **无需新裁决**——现行 SCENARIOS「不能以重新生成文件代替渠道业务操作」与 PS CONTEXT「重打／换单」两句已答完：取回渠道成品、不自行生成、不加工，故 `09` 只留一份载荷。盘点漏掉是因为它只盘代码不盘 `docs/domain/` |

### 实现票

四张裁决票结清后，下面六张已解除阻塞可直接认领；`Blocked by` 一栏只列**尚未** `resolved` 的票。

| 票 | 内容 | 状态 / Blocked by |
|---|---|---|
| [`05` `ServiceProductForm` 第二取值](./issues/05-service-product-form-second-value.md) | **已落地**：`LABEL_CHANNEL_SERVICE` 八处同步扩展（盘点漏列 CLI 与管理台两处）＋两条守卫用例；迁移另起 `0017`，不得就地改 `0008` | `resolved` |
| [`06` 面单交易写侧执行器链](./issues/06-label-transaction-write-side-executors.md) | **已落地**：五步编排共用一套按恢复动作分格的代数；棘轮基线两条剪掉并改正一处错了的计数；不发起任何渠道调用 | `resolved` |
| [`07` 取面单出向端口形状](./issues/07-outbound-label-fetch-port-shape.md) | **已落地**：出向缝落 `internal/platform/outbound`（两条链共用），端口落 PS `ports`；四处差异各有类型落点；`确证未受理`过举证门（举不出实据即降级）；「不得重发」与「只能查询收口」两条纪律做进谓词而非留给人记住；另加一道出向缝不得看见传输层的门禁 | `resolved` |
| [`10` 面单继续尝试决定登记册](./issues/10-continued-attempt-decision-registry.md) | **已落地**：登记册按（租户＋包裹）成册、决定只追加、判断由 Judge 现算不存列；读面那一格换真输入（登记册＋当前有效终局），并另交代决定历史在不在——「没有人作过决定」与「最近适用决定为重开」派生同一格，不加第三格 | `resolved` |
| [`12` 候选装配](./issues/12-channel-candidate-assembly.md) | **已落地**：装配三片＋择优编排＋成本取数适配器，装配器接上择优端口（`cb86027`）；整条链的组合根与调用入口不在本票，待另立票 | `resolved` |
| [`13` `BUY` 批量评价与成本分值桥](./issues/13-buy-evaluation-to-cost-score-bridge.md) | **已落地**：三道缝（批量评价口、成本分值桥、择优比较器）`54ae107`，收口评审补刀 `5e9d688`；比较器由 `12` 的装配器接上（`cb86027`） | `resolved` |
| [`15` 轨迹源的拉取/接收端口](./issues/15-tracking-source-inbound-port.md) | **已落地**：形态与端口形状裁于票面 Answer，端口与替身落 `7904003` | `resolved` |
| [`08` 取面单合成替身](./issues/08-label-fetch-synthetic-double.md) | **已落地**：三条各有测试证据，`07` 的端口形状装得下、无一条需回；替身落测试专属包，「不进生产装配」由编译器守 | `resolved` |
| [`09` 渠道返回载荷的落点](./issues/09-channel-label-payload-landing.md) | **已落地**：裁「只入引用」并落 [ADR-0092](../../docs/adr/0092-channel-label-payload-lands-as-a-reference-and-the-body-store-is-an-unconfigured-outbound-seam.md)——本体存放是未配置的出向缝；摘要必备、定位符可缺；载荷追加不可覆盖。**无新迁移**（引用是小值，走既有快照），真库实跑改证快照往返 | `resolved` |
| [`11` 包裹终局的跨交易判断](./issues/11-parcel-final-across-transactions.md) | **已落地**：与既有终局的关系正面作答，实现 `0e5a4ea`；三个调用方留作后继（见本节开头） | `resolved` |
| [`14` 落选留痕对象](./issues/14-rejected-candidate-trace-object.md) | **已落地**：渠道择优决定作为只追加的决定记录落 PS 侧——领域对象逐候选四格 + 重建门 + `ports.ChannelSelectionDecisionRegistry` + postgres 头行/子行两表（PS 迁移 `0015`）+ 择优编排落定后同事务写入（Deps 纯加法）+ 标识签发器 `CSDN`；只引用候选与 BUY 评价、不拷内容（`c0ddc4fb`..`f43c99ee`）。运营查阅面另立 `23` | `resolved` |
| [`16` 外部轨迹的收编执行器](./issues/16-external-tracking-fact-adoption-executor.md) | **已落地**：落文 `2bb9300`（ADR-0102 ＋ TF CONTEXT 两词），TF 侧认领/判断/登记/outbox（`a3e28ff`）、VE 侧消费与译装＋路由表（`65b369f`）；一条外部轨迹走到投影。刻意留下三格拆成 `18`–`21` | `resolved` |
| [`17` TF 两类事实补在线登记口](./issues/17-tf-handover-and-offsite-pickup-need-online-faces.md) | **已判定不做**——`03` 裁定走新立事实，这两个口无用例在守；缺口登记仍留在盘点里 | `resolved` |
| [`18` 外部承运凭证登记册](./issues/18-external-carrier-credential-registry.md) | **已落地**：登记册五件往返、作废/失效/替代各成新版本，解析口按适用状态×适用范围×标识对象类别作答（`024cb5f`，TF 迁移 `0012`） | `resolved` |
| [`19` 轨迹源有效时间规则目录](./issues/19-tracking-source-effective-time-rule-catalogue.md) | **已落地**：一源一链带版本的规则目录 + postgres 登记册兼解析口 + 登记编排 + 在线登记口 `/transport-fulfillment-effective-time-rule-registrations`（TF 迁移 `0014`）；无规则如实答`无`，不默认等于发生时间；`EffectiveTimeRules` 从机制清点「缺」名单消失 | `resolved` |
| [`20` 轨迹拉取节拍](./issues/20-tracking-pull-beat.md) | `TrackingSource.Pull` → `Adopt` 的生产入口；随第一家真源的适配器立，节奏属 `PAR-INT-02` | `draft`｜`18` |
| [`21` 有效时间显式判断的在线面](./issues/21-effective-time-judgment-online-face.md) | **已落地**：查阅读口 + `GET /transport-fulfillment-external-tracking-facts?source=&view=pending\|current` + `POST /transport-fulfillment-effective-time-judgments`（`UnconfiguredIntake{}` 起步）+ 管理台「外部轨迹有效时间判断」页 | `resolved` |
| [`22` 按已登记规则批量重判待判断事实](./issues/22-rejudge-pending-facts-by-registered-rule.md) | 规则登记之后的回填入口：显式触发、不随登记自动发生、复用规则判断路径；随第一家真源一起立实施 | `draft`｜`20` |
| [`23` 渠道择优决定的运营查阅面](./issues/23-channel-selection-decision-operations-read-face.md) | **已落地**：读端口 `ChannelSelectionDecisionRead` 另立（TIED 列表 + 按标识取逐候选，不拓宽登记册端口）+ postgres 读适配器 + `GET /channel-selection-decisions`（`view=tied` / `decisionId=`，隔离读放行按 ADR-0078 判入格）+ 管理台「委托受理」区新页；无金额无评价内容列、无隐式时间截断；并列冲突的人工裁决动作留待 `/domain-modeling` | `resolved` |
| [`24` 更正版本在 PS 采用口被当作第二责任起点](./issues/24-source-correction-version-refused-as-second-responsibility-start.md) | tf/08 核查所得：`AdoptNetworkIntakeHandler` 采用键按版本幂等，但随后 `FindResponsibilityStart` 命中首登版本，更正版落 `SOURCE_NOT_ADOPTED`；UC-PS-003「一致性」节与 AT-PS-050「来源更正形成新的采用判断版本」无代码——链到 PS 为止今天是断的 | `draft` |
| [`25` 实际承运商首次有效收寄到达 → 终局判断](./issues/25-external-carrier-first-pickup-triggers-label-final-judgment.md) | `11` 三个调用方之一：PS 侧 inbox 消费者 + 处理方适配器（照有效交付那一路）→ `JudgeLabelServiceFinalCommand` 含 `FirstEffectivePickup`。**量到 TF 今天没有「实际承运商首次有效收寄」事实与信封**——`external-carrier-tracking.judged` 按 TF CONTEXT 不构成收寄，lc/11 那句「PS 侧消费该信封」是误读；PS 半边形状已定，等 TF 侧那张票；「要裁的」1 由 ADR-0135 答，上游立为 `31`——**`31` 已 2026-09-10 进 main，本票 2026-09-11 14:1x 转 ready-for-agent**（「要裁的」2 失效版本重派生归 PS owner，不阻开工：先落显式未决）。**2026-09-11 21:1x 已进 main**（分支 `mcp4-lc25` 基 main tip `1cc61efe`，纯 ff 不换号：代码 tip `c03eea37`、清点 `cbd0ed23`、完成记录 `6e10cd8f`、评审回修 `a710a0e7`；PS inbox 消费者 `CarrierFirstEffectivePickupConsumer` + `adapters/transportfulfillment` 的 `JudgeOnCarrierFirstEffectivePickupAdapter`（按信封所指版本取回、五值逐格、失效版本停显式未决）+ `parcel-dispatch` 路由行与装配，接 lc/26 / 27 共用的 `labelFinalJudgmentCore`；作者通道 4 → 接手通道 3 → 收尾通道 5 三度失去会话，完成记录由推送方代写；非作者评审 ← 通道 6 Spec 0 阻断 / 5 非阻断、通道 2 Standards 1 阻断回修后关闭 / 4 非阻断；「要裁的」2 停在 `ErrVoidedCarrierPickupRederivationUndecided` 归 PS owner） | `resolved`（已进 main）｜无 |
| [`26` 面单交易定案那一拍 → 终局判断](./issues/26-label-transaction-settlement-beat-triggers-label-final-judgment.md) | `11` 三个调用方之二：`RecordChannelResult`（及可能的 `AppendFollowUpAction`）`Save` 之后判。两条路并列——同一次调用内 / 落库后交一封信后置一拍——代价写在票面「要裁的」；**2026-09-10 经 [ADR-0134](../../docs/adr/0134-label-service-final-judgment-triggers-are-deferred-one-beat-through-pointer-envelopes.md) 裁乙**（落库同事务入队指针式信封、后置一拍；两拍都触发；06 不加 Transactor，同事务由 `RequireExecutor` 既有约束守、事务由组合根壳开）。**已进 main**（分支 `mcp2-lc26`，代码 tip `d6d50e30`，main 重放 tip `83ab2032`；非作者评审 ← 通道 6 两轴 0 阻断） | `resolved`｜生产可达随 `28` |
| [`27` 受控关闭 / 重开决定生效 → 终局判断](./issues/27-controlled-close-reopen-decision-triggers-label-final-judgment.md) | `11` 三个调用方之三：决定写面 `Save` 之后判，两种决定都触发、分格归领域判断；「生效」= 追加即生效（`standingClosure` 不看时钟），无需定时重判。**决定口今天连写入方都没有**（端口头注与 lc/10「刻意没做」原句），写面票于 2026-09-10 裁立两半：`30`（PS 写面）与 pc-gaps/13（PC 授权动作加两格）；触发形状随 ADR-0134 乙；余工表「`RehydrateContinuedAttemptRegister` 仍在基线」已过期。**2026-09-11 11:5x 通道 4 落地**（分支 `mcp4-lc27` 基 `b68baddf`，代码 tip `868725ca`、清点 `70d77d91`；两张 Blocked by 已进 main，由 draft 直转 in-progress 再 resolved）：写侧入队那半由 `30` 落地只核对；本票落 PS inbox 消费者 `ContinuedAttemptDecisionJudgmentConsumer`（三维毒丸、kind 不进译码、与 26 分账）+ 接共用核的 `ContinuedAttemptDecisionJudgmentAdapter`（判据 2 四格、判据 3 重放同源版本 / 再关换版本、判据 4 共核）+ `parcel-dispatch` 路由行与装配（lc/26 块抽出 `labelFinalJudgmentCore`）；带 DSN inbox / parcel-dispatch / architecture 三包 ok。**2026-09-11 12:1x 已进 main**（非作者评审 ← 通道 5 两轴 0 阻断，非阻断 2 + 1 随票记；与 lc/32 同批，批 tip `6bf9861b`）。**余工（无票号）**：写 → 入队 → 派发 → 判 → 终局落行的单树一条龙验收，写侧（`cmd/parcel-api` 真库）与消费侧（`cmd/parcel-dispatch` 路由探针 + inbox 门）今天各证各的，同一事件类型串由消费者测试钉；归 PS owner 立票 | `resolved`｜无 |
| [`28` 渠道择优链的组合根与调用入口](./issues/28-channel-selection-composition-root-and-call-entry.md) | `12` 收口留下的「生产可达仍差两步」：`cmd/` 下择优、06、07 至今零装配，只接了两张读面；甲（运营端点）/ 乙（06 编排前置步）代价以 lc/12 两条 Comment 为权威。**2026-09-10 17:1x 通道 1 裁**：取乙，06 编排的新输入是一个「择优结果」对象（候选 + 七类依据引用 + 评价痕迹引用）、编排不问它从哪来，首发只接乙、甲不立票；候选标识 → 七类引用的翻译另立 `29`。**2026-09-10 23:0x 通道 4 落地**：06 建立一步改收整份「择优结果」、前置步编排、`cmd/parcel-api` 组合根（三取数口显式未配置、07 未配置壳、事务壳），真库验收路径两条。**已进 main**（分支 `mcp4-lc28`，代码 tip `05afb2d1`，main 重放 tip `784ad076`，与 sa-cc/03 同批 2026-09-10 21:3x 推出；非作者评审 ← 通道 3 两轴 0 阻断，「生产可达」改读为可装配、未触发，触发面归后继票待立） | `resolved`｜无 |
| [`29` 择优结果 → 面单交易七类依据引用的翻译适配器](./issues/29-channel-selection-result-to-label-transaction-basis-translation.md) | `28`「要裁的」2 另立：PS `adapters/partycommercial/` 里把候选标识（= PC 渠道产品引用）译成 `Establish` 的七格。逐格取证：账号 / 持有人取自 `ChannelAccountUseAuthorization`，合同取自 `SupplierAgreement.Version()`——但 PC 没有「渠道产品 → 供应商协议」绑定、也没有按渠道产品反查授权的读口；服务方 / 结算相对方 / 费率 / 责任依据快照四格答不出，四条「要裁的」**2026-09-10 17:4x 通道 1 全取默认**（实例半边源照同目录先例、两格同取 `Supplier()`、费率由择优步带出的 PP 价卡版本、责任依据快照 = 本交易七格 + 接受时商业解析回指一格）并补强：「择优结果」对象由本票定义并产出、`28` 只接线。**已进 main**（分支 `mcp3-lc29`，代码 tip `2d19bc32`，main 重放 tip `58c59daf`；非作者评审 ← 通道 6 两轴 0 阻断）——`28` 的 Blocked by 由此解除 | `resolved`｜无 |
| [`30` 受控关闭 / 重开决定的写面（PS 半边）](./issues/30-controlled-close-reopen-decision-write-face-ps-half.md) | `27` 裁决拆出的写面 PS 半边（2026-09-10 通道 2，task-138ab1c9）：命令编排（两条命令一个 handler，问授权 → 读当前有效终局 → 开册 / 读回 → `Append` → `Insert` / `Save` → 触发尾段按 ADR-0134 乙）+ 入口壳照 `transactionalWithdrawal` + 授权适配器照 `WithdrawalAuthorizationAdapter`；请求方 / 实际决定方 / 授权角色 / 授权依据快照四件从授权答复取。PC 半边是 pc-gaps/13（授权动作加关闭、重开两格）。要裁的已清零（2026-09-10 17:5x 通道 1 裁：`AuthoritativeCutoffBoundary` = 关闭决定标识；随 pc-gaps/13 三条——不比等级、PS 侧核同一货主账户 + 证据非空、例外支不开）。**pc-gaps/13 已 2026-09-10 20:3x 进 main**，本票转 ready。**2026-09-10 23:0x 落地**（通道 3 落三个口后 crash，通道 4 按用户指示在同一分支 `mcp3-lc30` 接续；代码 tip `892ffa3f`）：`FormContinuedAttemptDecisionHandler` 两条命令一个 handler（四件取自授权答复、`CutoffBoundary` = 决定标识、重开对货主指令关闭核同一账户 + 证据非空、不比等级）、PC 授权适配器（三件取自裁定、一口两问互不蕴含）、`/shipment-requests/continued-attempt-closures` 与 `/continued-attempt-reopenings` 两端点（`UnconfiguredIntake{}` 起步）、组合根事务壳、`Save` 后同事务入队指针式信封；真库两例；「同决定标识重放」一格因标识由本上下文铸而不可达，未做。**2026-09-11 11:1x 已进 main**（非作者评审 ← 通道 6：首评 Spec 1 阻断——重开时原关闭种类段认不出被放行，反 22:0x 裁决 ②——回作者通道 4 同分支修 `5681d97d`，重评关闭、两轴 0 阻断；main 重放 tip `6a5fb181` 含清点；Standards 5 / Spec 1 非阻断随票记，第三值 `EXTERNAL-RESTRICTION` 推送方拍接受） | `resolved`｜无 |
| [`31` 实际承运商首次有效收寄：TF 侧事实聚合、版本链、登记册、outbox 与显式判断入口](./issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md) | `25` 的 TF 侧上游。[ADR-0135](../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 裁定新一类独立控制事实：在合格来源之上一次显式判断形成、自有登记册与事件 `transport-fulfillment.carrier-first-effective-pickup.registered`、按（租户，载运对象）一条版本链、已形成即参与进入（`ParticipationEntryKind` 第三格）、待确认是版本但不提供、更正沿来源更正关系换替代 / 失效版本。本票做 TF 半边：聚合与版本链、第三格、登记册与迁移、outbox、显式判断入口与进段 / 重派生挂点、在线口；收寄判读规则目录与规则那一路的节拍随第一家真源另立。**已落地**（分支 `mcp6-lc31`，tip `1d9a24c0`，TF 迁移 `0020`，八笔；判断题八道交非作者评审） | `resolved` |
| [`32` 06 `Establish` 核继续尝试登记册](./issues/32-establish-label-transaction-checks-continued-attempt-register.md) | lc/30 立票时量到的缺门（通道 1 2026-09-10 17:5x 裁立）：`Establish` 前逐覆盖包裹读登记册 + 当前有效终局、`Judge(currentFinalPresent)` 为受控关闭则拒整笔建立、结果代数加一格并交回被拒包裹；后四步不核（硬句「不阻断既有交易的……定案」）；重放先于核册；并发窄格的事后「违反截断边界的业务判断」另票无票。**2026-09-11 11:4x 已落地**（通道 3，分支 `mcp3-lc32` 基 `2c7326ef`，代码 tip `5dd8db59`，三笔）：`LabelTransactionDeps` 两只读口（`ContinuedAttemptRegisterView` / `CurrentFinalView`，`ports/**` 未动）、构造器返 error 拒 nil、`Establish` 先 `FindByID` 重放再逐包裹 `Judge`、`PARCEL_CONTINUED_ATTEMPT_CLOSED` + `ClosedParcels()`、`cmd/parcel-api` 组合根真装两口；带 DSN cmd 两包 + PS postgres ok。**2026-09-11 12:1x 已进 main**（非作者评审 ← 通道 4 两轴 0 阻断，非阻断 3 + 3 随票记；与 lc/27 同批，批 tip `6bf9861b`） | `resolved`｜无 |
| [`33` 首次有效收寄判断的两格语义修正](./issues/33-carrier-pickup-judgment-first-predicate-on-pending-and-derived-corrects.md) | `31` 非作者评审（通道 2）Spec 非阻断两格的后继（通道 1 2026-09-10 派立，通道 6 自立自做）：① 待确认 → 已形成那一步（`reconsiderPending` / `formOrHold` 的 `found` 分支 / `rederive`）补问首次判据，在控答 `NOT_FIRST` 不形成、待确认链尾留原样；② 轨迹来源自动派生的 `corrects` 在链上无版本时视作首次判断照正路走，显式指名的仍 `BASIS_NOT_CURRENT`。只动 TF application 那一对文件。**已进 main**（分支 `mcp6-lc33`，代码 tip `2ed45954`，main 重放 tip `1d1d405c`；非作者评审 ← 通道 4 两轴 0 阻断；作者 crash，完成记录推送方代写） | `resolved`｜无 |
| [`34` lc/28 组合根的触发面：`main` 启动时装配、构造期错误 fail-fast](./issues/34-label-channel-chain-startup-assembly-fail-fast.md) | `28` 非作者评审（通道 3）Spec 非阻断 ② / 判断题 (b) 的后继（通道 4 2026-09-10 立）：`buildLabelChannelOrchestration` 在 `9ddbafcf` 上无生产调用方，`parcel-api` 从不构造这条链、构造期错误不在启动时暴露。本票只做 `main` `run` 启动时装配一步（零行为变化、动共享 `main.go` 须占号）+ 三处「尚无组合根 / 没有触发面」头注改口；谁发起一笔面单交易（运营端点 / 进程内触发）是产品题，不在本票。**2026-09-11 11:3x 通道 4 落地**（分支 `mcp4-lc34` 基 `2c7326ef`，代码 tip `38f2322c`）：`run` 在两张读面之后 `buildLabelChannelOrchestration(db)` 构造即丢、错误即退出；四处头注改口（含 `assemble_label_channel.go` 文件头）；带 DSN `cmd/parcel-api` + `internal/architecture` 两包 ok、LabelChannel 真库两例 PASS；端点表 / 探针 / 放行表零 diff。**2026-09-11 11:5x 已进 main**（非作者评审 ← 通道 5 两轴 0 阻断，Standards 2 非阻断随票记；与 sa-cc/14 同批，批 tip `72a7ef49`，清点零差） | `resolved`｜无 |
| [`35` 择优决定册与面单交易的对账 + `Select` 把并列 / 无人参选改成结果格](./issues/35-establish-replay-decision-register-reconciliation-and-select-result-shape.md) | `28` 非作者评审 Spec 非阻断 ③ / 判断题 (c)(d) 与 Standards ① 的后继（通道 4 2026-09-10 立）：`Flow.Establish` 重放会在决定册多写一条 SELECTED，而决定（scope + mapping）、择优结果、交易之间无任何引用、无法对账；`Select` 把并列 / 无人参选当错误交回，同一分类在 `Select` / 前置步编排 / `transactionalChannelSelection` 三处各写一遍。做法二（结果格）先做；重放对账二选一（先查交易已存在则跳择优 / 决定记录带交易引用）归 PS owner。顺带把 lc/26 评审 Standards 四条与 lc/29 评审非阻断五条在 `9ddbafcf` 上逐条核过：五条归本票、一条已由 lc/28 修、一条归 PC owner 无票号、两条无需修 | `draft`｜无（要裁一条） |
| [`36` 四份评审的同族非阻断项一笔收口](./issues/36-ps-label-final-review-follow-ups-header-comments-and-constructor-guards.md) | lc/34 S①（`main.go`「六个实例半边缝」跨文件计数）、lc/27 S①（`labelFinalJudgmentCore`「三路 / 六口」+ `judge_parcel.go` 包头注「三路」）、lc/32 S①③（`NewLabelTransactionHandler` 全口构造期拒 nil、`closedParcels` error 前缀中英混杂）、lc/30 S①②③⑤（`NewContinuedAttemptDecisionAuthorizationAdapter` 拒 nil `adjudicate`、ports 查询头注 Requester 去向讲错、`AuthorityRole` 暂行未标、合成 / 解析收成一对）、lc/30 头注「导出给 27」改口（27 自写串、只测试对照）。不做 lc/32 S②（抽 helper）、lc/30 S④（种类抬成领域类型）。零判断语义改动。2026-09-11 14:2x 通道 1 立，ready；**16:2x 通道 2 完工；17:0x 进 main**（`mcp2-lc36` 基 `807de571`，代码 tip `b5a7fb81`、清点 `614cda04`；main 上代码 `efc0c4b5`、批清点 `dfd3c3f5`（与 ve-disc/02、sa-cc/09 同批）；非作者评审 ← 通道 5 两轴 0 阻断，Standards 1 非阻断随票记；九条对应八笔见票面 Comments） | `resolved`（已进 main）｜无 |
| [`37` lc/25 两轴评审非阻断项一笔收口](./issues/37-lc25-review-follow-ups-superseding-version-case-header-counts-and-exported-event-type.md) | lc/25 通道 6 Spec (1)（信封指替代代 v2 的直接用例）、(4)（UC-PS-004 依据表括注「（外部承运轨迹事实）」→「（实际承运商首次有效收寄事实，ADR-0135）」，越权风险点 5）；通道 2 Standards 1（测试文件头「三路 / 四个读口」换点名）、2（TF `carrierFirstEffectivePickupEventType` 导出为 `CarrierFirstEffectivePickupRegisteredEventType`，对照断言落 `cmd/parcel-dispatch/assemble_test.go`，PS 测试头注写实）、3（`assemble.go`「ADR-0049 第三条」→「决定三」）。不做 `CarrierTrackingFactReference` 改名（PS owner）、「要裁的」2 失效版本重派生（等裁决）、与交付适配器同形抽 helper（只记不抽）。除新增用例外零行为。2026-09-11 21:2x 通道 2 自立自做（`mcp2-lc37` 基 `02e1dfc4`）；**21:4x 完工；22:5x 进 main**（通道 1 推送方重放，与 sa-cc/16、sa-cc/01 同批：main 上 `7a0b4827` / `3e9ed3a6` / `73d729c7` / `86736403` / `0c0c10ae` / `8c56b6b1`，批清点 `fe975e1b`（本票零差）；非作者评审 ← 通道 1 两轴 0 阻断、Standards 2 非阻断归 PS owner；`fe975e1b` 带 DSN 全量 110 ok）：五条对应五笔 + 立票一笔，完成记录随条 5 那一笔同提交；作者带 DSN 五包 ok、探针 PASS 21 / SKIP 0、清点零差；四条判断项与裁法见票面 Comments | `resolved`（已进 main）｜无 |
| [`38` lc/37 评审 Standards 非阻断 + lc/25 裁决末条一笔收口](./issues/38-lc37-review-tails-adr0049-citation-five-values-count-and-fact-reference-rename.md) | lc/37 通道 1 Standards ①（`ADR-0049 第三条` 全部统一为带引文「没有订阅者的事件类型显式失败并入账」的「决定三」——`cdf17834` 上重量实得十四处四文件，含 `assemble.go` / `assemble_test.go` / PS `advance_acceptance_chain_test.go` / 平台 `direct_publisher_test.go`，非派单写的四处）、②（收寄路 `judge_on_carrier_first_effective_pickup{,_test}.go` 三处「五值」计数换点名）；lc/25 裁决末条「类型名是否随之改归实施时判」→ 本票判改：`CarrierTrackingFactReference` / `CarrierTrackingFactVersion` → `CarrierFirstEffectivePickupFactReference` / `CarrierFirstEffectivePickupFactVersion`，纯改名。不做 lc/25「要裁的」2、psr/09「必登」、同形抽 helper、任何 ADR 正文。2026-09-14 08:5x 通道 4 按通道 1 派单 task-65ce5275 自立自做（`mcp4-tails` 基 `cdf17834`，与 sa-cc/17 同分支） | `in-progress`｜无 |

### 盘点判为「无缺口」因而不立票的三段

轨迹源那条链的下游三段——里程碑映射、内部投影、客户可见——形状齐备，[轨迹源盘点](./tracking-source-seam-inventory.md)逐段查明它们**在等一个还没有生产者的输入**。为它们立票只会立出三张「等上游」的空票。同盘点还查明 ADR-0088「不直插投影」那半句**今天已经由类型守住**（`domain.SourceContext` 封闭五值 + `MilestoneClassification` 只能由 `AcceptedSourceFact` 构造），那是已成立的机制不是缺口，因此也不立票。

本 spec 只有在每一张子票都 `resolved` 之后才转 `resolved`。

## Comments

- 2026-09-02 MCP-6：建目录与本 spec。本目录此前不存在而 ADR-0088 已引它作为盘点归处，这次补上那处悬空引用。取证基线 `origin/main` = `8c32d78`。未立任何子票，未改 `docs/**`，未改任何既有票面。
- 2026-09-02 MCP-1：MCP-6 在写完 spec 后随第三次并行崩溃停摆，两份盘点未开始；原 Comment 自称「并出两份只读盘点」与事实不符，已改。本 spec 原样取进主线是为了保住它，不是为了宣布盘点已办——盘点重派 MCP-6。
- 2026-09-02 MCP-2：能力形状盘点其后已交并进主线，故本文两处已成假的事实一并更正——状态行原写「子票待 MCP-1 据两份盘点立」、正文原写「两份都尚未写出」。轨迹源盘点仍未开始，那处悬空引用照旧保留为悬空，不假装它在。同时立子票 `01`。取证基线 `398a148`。
- 2026-09-02 MCP-4：**轨迹源盘点已交，两份盘点的建议一并落成 `02`..`17` 共十六张子票，清单至此完整。** 立票原归 MCP-1，2026-09-02 由 MCP-3 改派本会话（MCP-1 不在本批）。取证基线 `9e6d53a`。

  **与 MCP-2 的 `01` 撞过一次号，处置记在这里。** 本会话写盘点期间 MCP-2 立了 `01`，而我按「编号从 01」也起了一张 `01`（出向集成缝）。两张内容不同、文件名不同，故谁都没被覆盖，但同号违反约定。**让位给 MCP-2 的 `01`**，我那张改为 `02`，并回改 `07`/`15` 的阻塞边与正文引用。

  让位不只是因为它先到：我原本还有一张「渠道候选择优归属与并列即冲突」，与 MCP-2 的 `01` 同题而**读法更粗**——我写的是「两条口径互相否定」，而 MCP-2 正确地指出 `PAR-NET-16` 的条件是「并列**且无法选出唯一一条**」，标识升序恰恰是一种能选出唯一一条的办法，真正的争点是「不得任选」禁的是不确定性还是无业务依据。**它那张比我那张准，所以我删掉自己那张，不并存两个说法。** 我那张里唯一不重合的一问（不可计价候选出局不得以零金额顶替，判断放在哪一层）已并入 `13` 的「必须守住的一格」。

  盘点第三、四、五段判为无缺口，故不立票，理由写在上面「不立票的三段」一节——**不为凑齐而立空票**，判据同票 `admin-write-faces/02` 的「无用例可接就如实跳过」。

- 2026-09-02 MCP-4：**四张裁决票一次裁清，`17` 随 `03` 判定不做，共五张转 `resolved`；六张实现票据此解除阻塞。** 经 owner 明确授权裁决（原话「你作为业务和系统专家，你的明确的建议呢，不要管其他的 agent 了，你自己完全独立工作」）。各票裁决正文与能力边界写在各自票面，本处不复制第二套口径，只记三件跨票的事。

  **一、`04` 本来就不该是一张裁决票。** 它问的三件事，现行 [SCENARIOS](../../docs/domain/SCENARIOS.md)「面单渠道」一节与 [PS CONTEXT](../../docs/domain/parcel-shipment/CONTEXT.md) 已经答完——前者「不能以重新生成文件代替渠道业务操作」直接排除「自行生成」，后者把`重打`与`替换`定义成两个不同业务动作，客户条目里的「面单 PDF 修改」落的正是这两个词。**两份盘点都没查到，是因为它们只盘 `internal/` 不盘 `docs/domain/`**：在代码里搜 `PDF`/`ZPL`/`render`/`template` 得零命中，据此判为「一张白纸」，而这一段的口径本来就不写在代码里。这条教训值得记在 spec 上——**零命中只证明代码里没有，不证明没人裁过**。

  **二、`02` 的裁决更正了它自己票面的一句话。** 该票原写出向结果代数「判据同 ADR-0022 在入向侧立的那一条，方向相反但分界线是同一条」。[ADR-0090](../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md) 判定**不能镜像**：ADR-0022 是本仓作为服务端的自律，对端不受它约束，而能力形状盘点第二段已取证 UniUni 的 HTTP 恒回 200。照原句实现会把每一次失败读成成功。真正可继承的先例是 [ADR-0029](../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)「按恢复动作分格，不按失败原因分格」——同一条规则从跨上下文边界搬到跨进程边界。

  **三、`03` 的裁决要动领域语言，因此它 `resolved` 不等于 `16` 可以开工。** 新立一类 TF 外部承运轨迹事实、以及「外部事实三时间按归属分铸」这条对 ADR-0023 的适用解释，都必须先落 TF `CONTEXT.md` 与一篇新 ADR（[AGENTS 改文档](../../AGENTS.md#改文档)）。前置清单写在 `16` 票面。**落文若在评审中被否，`03` 与 `16` 一并重开**——裁决先记下来是为了让 `15`/`16`/`17` 现在有一个可指的答案，不是为了绕过落文。

  裁决人的能力边界逐票写在各票面，共同的一条是：**未打开 `docs/wooolink/` 与 `docs/reference/xls/` 下的客户文件**，因此裁的一律是结构，不含对任何客户的承诺。
