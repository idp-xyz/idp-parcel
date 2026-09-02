# 面单渠道服务首发机制半边

Category: feature
Status: in-progress——17 张子票立齐；`01`..`08` 与 `17` 已 resolved（四张裁决 ＋ 四张实现票），其余实现票待做

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

**清单已完整**，17 张。四张裁决票（`01`..`04`）不写实现，它们阻塞其后的实现票——不先裁，后面每张都会各自假设一个答案，而**重复或分叉的实现在 `go build` 与 `go test` 下全绿**，要到集成才看得见。

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
| [`10` 面单继续尝试决定登记册](./issues/10-continued-attempt-decision-registry.md) | 读面那一格今天派生自空历史 | `ready-for-agent` |
| [`12` 候选装配](./issues/12-channel-candidate-assembly.md) | 产品—渠道映射到候选集合之间无适配器；落 PS 侧 | `ready-for-agent` |
| [`13` `BUY` 批量评价与成本分值桥](./issues/13-buy-evaluation-to-cost-score-bridge.md) | 逐候选各算各的计费重；出局判断落择优层，不得以零金额顶替 | `ready-for-agent` |
| [`15` 轨迹源的拉取/接收端口](./issues/15-tracking-source-inbound-port.md) | 形态选择（拉取/回调）；`OccurredAt` 缺失要如实交出不得代补 | `ready-for-agent` |
| [`08` 取面单合成替身](./issues/08-label-fetch-synthetic-double.md) | **已落地**：三条各有测试证据，`07` 的端口形状装得下、无一条需回；替身落测试专属包，「不进生产装配」由编译器守 | `resolved` |
| [`09` 渠道返回载荷的落点](./issues/09-channel-label-payload-landing.md) | 领域、库、读面三处；只留一份载荷（`04` 已裁）；本体入库还是只入引用要正面答 | `draft`（`04`/`07` 均已 resolved，阻塞解除） |
| [`11` 包裹终局的跨交易判断](./issues/11-parcel-final-across-transactions.md) | 与既有终局是不是同一个，要正面答 | `draft`（`06` 已 resolved，阻塞解除） |
| [`14` 落选留痕对象](./issues/14-rejected-candidate-trace-object.md) | 不复用 `RouteCandidate`（`01` 已裁），落 PS 侧 | `draft`｜`12` |
| [`16` 外部轨迹的收编执行器](./issues/16-external-tracking-fact-adoption-executor.md) | 译成 TF 新立的一类事实；**开工前须先补 TF `CONTEXT.md` 与三时间 ADR** | `draft`｜`15` ＋落文前置 |
| [`17` TF 两类事实补在线登记口](./issues/17-tf-handover-and-offsite-pickup-need-online-faces.md) | **已判定不做**——`03` 裁定走新立事实，这两个口无用例在守；缺口登记仍留在盘点里 | `resolved` |

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
