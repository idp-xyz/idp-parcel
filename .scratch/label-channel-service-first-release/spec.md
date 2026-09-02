# 面单渠道服务首发机制半边

Category: feature
Status: in-progress——两份盘点均已交，17 张子票已据其建议立齐（`01` 由 MCP-2 立，`02`..`17` 由 MCP-4 立）

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

### 裁决票（无阻塞，可并行认领）

| 票 | 要裁什么 |
|---|---|
| [`01` 渠道候选择优的平局处置归谁](./issues/01-channel-candidate-tie-break-authority.md) | `PAR-NET-16`「并列且无法选出唯一一条时为冲突」与 `SelectRouteCandidate` 现行标识升序收尾，争点在那个条件句的两种读法；三种归属各自的代价已列，取甲须新 ADR |
| [`02` 出向集成缝在本仓没有先例](./issues/02-outbound-integration-seam-has-no-precedent.md) | 出向调用的失败/超时/重试/幂等怎么表达。全仓出向 HTTP 零命中，取面单与轨迹源会同时问出这一问，**各答一次就会分叉** |
| [`03` 外部轨迹由谁收编、时间由谁认领](./issues/03-external-tracking-fact-owner-and-time-minting.md) | 缝备忘把轨迹指给 TF 与 NO，而面单交易在 PS；外部源常只给一个时间戳而 `AcceptedSourceFact` 要三个，ADR-0023 又禁代铸 |
| [`04` 面单文件是生成还是取回后加工](./issues/04-label-document-generate-versus-post-process.md) | 末端渠道大多直接回成品面单，「生成」与「取回后加工」是两件事，客户条目里的「面单 PDF 修改」指哪一件未判定 |

### 实现票

| 票 | 内容 | Blocked by |
|---|---|---|
| [`05` `ServiceProductForm` 第二取值](./issues/05-service-product-form-second-value.md) | 六处同步扩展，两条守卫用例随之变红（那是设计） | 无 |
| [`06` 面单交易写侧执行器链](./issues/06-label-transaction-write-side-executors.md) | 聚合方法齐备而编排一环没有；同笔更正 `production_wiring_baseline.txt` | 无 |
| [`07` 取面单出向端口形状](./issues/07-outbound-label-fetch-port-shape.md) | 四处公开形态差异要进类型 | `02` |
| [`08` 取面单合成替身](./issues/08-label-fetch-synthetic-double.md) | 走通结果不确定、部分成功、批粒度三条 | `07` |
| [`09` 渠道返回载荷的落点](./issues/09-channel-label-payload-landing.md) | 领域、库、读面三处；`PDF`/`ZPL` 全仓零命中 | `04`, `07` |
| [`10` 面单继续尝试决定登记册](./issues/10-continued-attempt-decision-registry.md) | 读面那一格今天派生自空历史 | 无 |
| [`11` 包裹终局的跨交易判断](./issues/11-parcel-final-across-transactions.md) | 与既有终局是不是同一个，要正面答 | `06` |
| [`12` 候选装配](./issues/12-channel-candidate-assembly.md) | 产品—渠道映射到候选集合之间无适配器 | `01` |
| [`13` `BUY` 批量评价与成本分值桥](./issues/13-buy-evaluation-to-cost-score-bridge.md) | 逐候选各算各的计费重；不可计价候选出局不得以零金额顶替 | `01` |
| [`14` 落选留痕对象](./issues/14-rejected-candidate-trace-object.md) | 复用路由候选还是另立，要有理由不能靠形状像 | `01`, `12` |
| [`15` 轨迹源的拉取/接收端口](./issues/15-tracking-source-inbound-port.md) | 形态选择（拉取/回调）与聚合平台能否与直连共用一口 | `02`, `03` |
| [`16` 外部轨迹的收编执行器](./issues/16-external-tracking-fact-adoption-executor.md) | 译成所有者自己的事实；下游三段一行不用改 | `03`, `15` |
| [`17` TF 两类事实补在线登记口](./issues/17-tf-handover-and-offsite-pickup-need-online-faces.md) | **不一定要做**——`03` 裁定不需要就置 `resolved` 并写明依据 | `03` |

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
