# 07 商业发布各类的运营主路径：按 ADR-0101 决定八逐册裁形

Category: enhancement
Status: in-progress——伞票；已拆十一张子票 [08](./08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md)–[18](./18-customer-service-rule-form.md)（表见「子票」节）。**08 已 resolved（2026-09-08，MCP-1 接 MCP-5 续做完，分支 `mcp1-awf08`）：公共半边落地，子票 09–17 的 Blocked by 解除、进入前沿**；接表单时各票要用的东西见 08「完成记录」（载荷线格式 `CommercialPublicationPayload`、预览口与载体三口路径、`DRAFT_AWAITS_EFFECTIVE_START` 那一格）。18 留 draft，等什么写在它的 Status 行与下表。本票转 resolved 的判据照 issue-tracker「Complete a parent」：十一张全 resolved
Blocked by: 无（票 03 已落 JSON 镜像签）

## 缺什么

票 [03](./03-publication-write-face-blocked-by-two-misaligned-closed-sets.md) 给发布口落了
一签「受控发布（JSON 镜像）」。按 [ADR-0101](../../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md)
决定一，那是受控批量口的在线镜像，**不是运营配置员的主路径**；决定八要求每册在自己的
实施票里写明选了哪一形（逐字段表单 / 模板导入 → 草稿 → 批准 → 发布）与理由。立票时（2026-09-03，
随票 03 的裁决拆出）发布对象一册都没裁；本票立时写的是九类，`CommercialObjectKind` 到 `95182b9d` 已是十类
（pc-gaps/04 把客户服务规则版本纳入封闭集），第十册由子票 18 补。

## 各类，各自先答「登记频次 × 操作者角色 × 载荷结构」

下表是立票时的**初步印象**，逐册的裁决在各子票「选形与理由」节，以子票为准。

| 对象类别 | 显示在 | 初步印象（不是裁决） |
|---|---|---|
| `SERVICE_PRODUCT` | 服务产品页 | 低频、结构简单 → 逐字段表单候选 |
| `CUSTOMER_CONTRACT` | 客户与合同页 | 低频、正文（`0012`）有几组引用 → 逐字段表单候选 |
| `SUPPLIER_AGREEMENT` | 供应商协议页 | 同上（`0021`） |
| `ACCEPTANCE_RULE_PACKAGE` | 政策页·接单规则包册 | 正文是规则集合 + 多种声明通道（时点锚、收寄资格、终局规则）→ 要先答声明随发布怎么在表单里表达 |
| `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` | 政策页·接受前财务控制策略册（票 06 落的第九册） | 一格 + 一张几行的控制项表 → 逐字段表单候选 |
| `PRICE_RULE` | 政策页·商业价格政策册 | 正文是方向 × 方案绑定 + 口径（`0010`/`0022`）→ 逐字段表单候选，方案引用要能从价卡目录选 |
| `SETTLEMENT_POLICY` | 政策页·结算政策册 | 低频 → 逐字段表单候选 |
| `CREDIT_POLICY` | 政策页·信用政策册 | 额度两键恰一（`0020`）→ 表单要把「金额 / 比例」做成二选一 |
| `AUTHORIZATION_RULE` | 政策页·授权规则册 | 取消授权按请求方逐格 → 表单要能加行 |
| `CUSTOMER_SERVICE_RULE` | 管理台今天无册（后端第八册 `0023` 已落） | 两个引用 + 两张几行的表 → 逐字段表单候选；写签跟着读签走，先要读面 |

**印象不是裁决**：拆子票时逐册按决定八写理由；价卡首例走了模板导入（ADR-0101 决定二），
但那是矩阵型，这十类没有一类是矩阵。

## 子票（2026-09-07 拆于 `95182b9d`；接管会话同日于 `92579b0a` 逐张核过，见 Comments）

| 号 | 册（发布类别） | 选形 | 状态 | 阻塞边 |
|---|---|---|---|---|
| [08](./08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md) | 公共半边（不对应某一册）：服务端按册规范化算摘要、批准从 `OperatorEnvelope` 来、预览与发布同一路径 | 机制，不是表单 | **resolved**（2026-09-08，ADR-0126；PC 迁移 0028；四口进端点表） | 无；其余十张的 Blocked by 已解除 |
| [09](./09-service-product-version-form.md) | `SERVICE_PRODUCT` | 逐字段表单（版本壳 + 引用可加行；本册无正文，文档只盖 kind） | **resolved**（2026-09-08 MCP-3，分支 `mcp3-awf09`；进 main 见票内「进 main 记录」） | 08（已 resolved） |
| [10](./10-customer-contract-form.md) | `CUSTOMER_CONTRACT` | 逐字段表单（正文 + 绑定表可加行 + 合同级控制声明，两层分两节） | **resolved**（2026-09-08 MCP-4，分支 `mcp4-awf10`；进 main 见票内「进 main 记录」） | 08（已 resolved） |
| [11](./11-supplier-agreement-form.md) | `SUPPLIER_AGREEMENT` | 逐字段表单（采购方案从价卡目录选） | **resolved**（2026-09-08 MCP-6，分支 `mcp6-awf11@a2a44679`；进 main 见票内「进 main 记录」） | 08（已 resolved） |
| [12](./12-acceptance-rule-package-form.md) | `ACCEPTANCE_RULE_PACKAGE` | 分节逐字段表单，空节即未声明；pc-gaps/09 落地加一格、pc-gaps/10 落地加一节 | **resolved**（2026-09-09 进 main，通道 3 起 / 通道 2 接手 `mcp2-awf12`；非作者评审通道 3 两轴无阻断；进 main 见票内「进 main 记录」） | 08、20（均已进 main） |
| [13](./13-pre-acceptance-financial-control-policy-form.md) | `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` | 逐字段表单（共同通过条件 + 控制项可加行） | **resolved**（2026-09-09 进 main，通道 5 起 / 通道 6 接手 `mcp6-awf13` / 通道 5 代作者补笔；非作者评审通道 4 与通道 5 两轴无阻断（通道 5 补记的 1 阻断由补笔 `e58a085a` 修平）、封存笔 Go 主体由通道 3 补评无阻断；进 main 见票内「进 main 记录」） | 08、20（均已进 main） |
| [14](./14-price-rule-form.md) | `PRICE_RULE` | 逐字段表单（方案从价卡目录选 + 口径条件节） | **resolved**（2026-09-09 进 main，通道 2 `mcp2-awf14`；非作者评审通道 5 两轴无阻断（通道 4 重复评审同结论）；进 main 见票内「进 main 记录」） | 08（已进 main） |
| [15](./15-settlement-policy-form.md) | `SETTLEMENT_POLICY` | 逐字段表单（合同引用对象 + 版本一起选） | **resolved**（2026-09-09 进 main，通道 6 `mcp6-awf15` + 补笔；非作者评审通道 3 两轴无阻断（通道 2 重复评审同结论）；推送方含 DSN 全量补出一处夹具，见票内；进 main 见票内「进 main 记录」） | 08、20（均已进 main） |
| [16](./16-credit-policy-form.md) | `CREDIT_POLICY` | 逐字段表单（额度二选一）——08 建议的首例；**连带前端公共半边**（`party/publication-draft-api.ts` / `publication-draft-flow.ts` / `PublicationDraftFlow.tsx`） | **resolved**（2026-09-08 MCP-5，分支 `mcp5-awf16`；进 main 见票 Status） | 08（已 resolved） |
| [17](./17-authorization-rule-form.md) | `AUTHORIZATION_RULE` | 逐字段表单（取消授权按请求方可加行） | **resolved**（2026-09-08 21:4x 进 main，通道 4 `mcp4-awf17`；非作者评审通道 2 两轴无阻断；进 main 见票内「进 main 记录」） | 08、20（均已进 main） |
| [18](./18-customer-service-rule-form.md) | `CUSTOMER_SERVICE_RULE` | 逐字段表单（适用对象恰一 + 两张子表） | **resolved**（2026-09-09 17:5x 进 main `c2a965c9`，通道 3 起 Go 半边 / 通道 4 接手收 Go 三笔 + admin-web，分支 `mcp3-awf21`（与 21 同分支）；非作者评审通道 5 两轴无阻断（通道 6 首评 crash 未交）；进 main 见票内「进 main 记录」） | 08、20、21（均已进 main） |
| [20](./20-publication-vocabulary-read-face.md) | （公共半边，不是一册）| 词表读口：一口按 kind 答各册正文封闭集的码，表单不内置枚举——第 2 波 12/13/15/17 的前置 | **resolved**（2026-09-08 20:5x 进 main，通道 4 `mcp4-awf20`；进 main 见票内 Comments） | 08、16（均已进 main） |
| [21](./21-customer-service-rule-register-read-face.md) | `CUSTOMER_SERVICE_RULE` 的**读面**（18 的前置，与 06 同形） | 管理台第八册读面：`CommercialPolicyKind` 加格 + 列向 + 标签 | **resolved**（2026-09-09 15:57 进 main `56ed4111`，通道 3 `mcp3-awf21`；非作者评审通道 6 两轴无阻断；进 main 见票内「进 main 记录」） | 无 |
| [22](./22-publication-form-private-helpers-lift-to-party-shared-layer.md) | （收口票，不是一册）| 九张表单私有 `Field` / `useLoaded` / 词表下拉 / 整数解析抬到 party 共享层；顺带 17 非阻断 (1) 节根问题渲染 | **resolved**（2026-09-09 21:5x 进 main `4209520b`，通道 4 三任会话 `mcp4-awf22`；非作者评审通道 6 两轴无阻断；进 main 见票内「进 main 记录」）| 21、18（均已进 main） |
| [23](./23-settlement-policy-counterparty-register-and-contract-shell-reference.md) | `SETTLEMENT_POLICY` 的两条裁决落地 | 相对方从业务参与方册选、六维合同镜像成壳引用（裁决在 PC CONTEXT） | ready-for-agent（2026-09-09 立；21:5x 起阻塞边解除，进入前沿）| 22（已进 main） |
| [24](./24-closed-set-named-lookups-unify-on-closed-code-named.md) | （收口票，Go 领域）| 各册 `*Named` 反查合一到 `closedCodeNamed` | ready-for-agent（2026-09-09 立）| 无 |

**本票转 resolved 的判据不变**：08–18 十一张全 resolved（21 是 18 的前置，随 18 算）。22 / 23 / 24 是评审判断题收成的**收口票**，列在这里
只为让人在同一张表看见它们的去向，它们 resolved 与否不挡本票收口。

**Status 口径**：ready-for-agent 只给「形状裁清且票面无待裁问题」的子票，Blocked by 边另记、不混进
Status——一张 ready 的票在其阻塞边 resolved 之前不在前沿，这是 issue-tracker 的既有语义，不另造
`blocked` 一格（triage-labels 里没有它）。没有一张子票走模板导入：十类没有一类是矩阵，
ADR-0101 Alternatives 第二条否决逐字段表单只针对上百格的价卡。

## 硬句（从 ADR-0101 与票 pricing/08 带过来，逐册都适用）

- 表单不算摘要、不裁任何门：`contentDigest` 与受理都由服务端答，表单只呈现。
- 若某册要「提交前预览」，预览与发布共用一份规范化路径，同一份载荷过预览与过发布得到逐字节
  相同的摘要（决定四）。
- 操作者身份从 ADR-0100 的 `OperatorEnvelope` 来，表单不收也不送批准人。
- 每册子票开工前占 `cmd/parcel-api/endpoints.go`（若要加预览端点）与该页文件。

## 边界

本伞票不写代码。JSON 镜像签保留为高级口（票 03 已落），不因主路径落地而删。

## Comments

- 2026-09-07 · MCP-6（接管会话，task-aafca372；分支 `mcp6-awf07`，基线 `92579b0a`）：**收伞票。**
  十一张子票 08–18 是前一会话（通道 6，task-faee318d）13:13–13:16 写下、未提交的现场，原会话
  13:16 后无响应，本会话 15:43 点名自报无记忆，按 parallel-sessions「未提交现场」先原样封存再核
  （封存笔不进 main，重切时其证据句并入本批首笔提交信）。**逐张核对的结果**：
  - 三问（频次 × 角色 × 载荷）与「选哪一形与理由」：09–18 每张在「选形与理由」节都答了，无缺。
  - 硬句在场：每张原写「伞票四条逐字适用」——数了别处的东西（AGENTS「不用计数」那条），改为
    指本票「硬句」一节的名字；**没有把四句复制进各票**，理由是 AGENTS「单一权威」：复述的那半会在
    本节改动时无声变旧。要逐票复制的话是一次机械改动，MCP-1 若要，说一声。
  - 08 是其余十张的 Blocked by：十张全写了，无缺。
  - 取证复核（于 `92579b0a`）：08 引的 `translate.go` 三格、`NewCommercialContentDigest` 只查非空、
    `/commercial-publications` 挂 `UnconfiguredIntake{}` 均属实；08 原写 `publish_commercial_authority.go`
    「没有一处提到摘要」不准——有一句注释提到，改为「没有一处算或比对摘要」。12 原写 pc-gaps/09、10
    「各多一节」不准——按 ps-port-remainder/01 的裁决，09 的有效期声明走 `FinalRuleChannel` 同一通道
    （多一格不是多一节），10 才是新通道，已改。18 引的「管理台 `CUSTOMER_SERVICE_RULE` 零命中」
    在 `92579b0a` 仍成立。12、16 引的 `wiring-baseline-remainder/03`、`/04` 取证时（`92579b0a`）只在分支
    `mcp6-pp-ratchet` 上，16:00 随 `0fcbed6e` 进 main（MCP-1 广播），两票引用照旧、不留指路句。
  - Status：原各写 `blocked`，triage-labels 无此格；按「形状裁清且不需要新裁决才转 ready」改口——
    09–17 转 ready-for-agent，08（三问都是新裁决）与 18（等读面票要不要立）留 draft 并写清等什么。
  - **未做**：不替 08 裁三问，不取 ADR 号，不立管理台客户服务规则册读面票（归 MCP-1）。
- 2026-09-07 18:09 · MCP-6（17:4x 新绑会话，无上一会话记忆，按 MCP-1 18:05 广播补记）：**封存出处。** 分支 `mcp6-awf07`
  （基 `92579b0a`）五笔由 MCP-1 在隔离树重放进 main，远端 main = `e374b1ea`（MCP-1 `git ls-remote` 18:04:16）。分支 → main
  对照：拆子票 `42224e35`→`e52c71b6`、收伞票 `e9452c15`→`9f60058b`、立 pc-gaps/08–10 `8c1e6ed5`→`52001f02`、
  `ports.go` 头注改口 `2417b7bb`→`d1a03e00`、ADR-0116 `c9ec35d7`→`e374b1ea`。拆树前核 `git diff c9ec35d7 e374b1ea -- <本批 19 件>`：
  只 `docs/adr/README.md` 多四行（0113 / 0118 / 0122 / 0123，他人条目自动合并进来的），0116 那一行两侧一致，本批内容原样在 main。
  树 `idp-parcel-mcp6-awf06` 已拆（status 两问零行；`worktree remove` 退 255 卡在 `node_modules` 长路径，三样核对后按
  parallel-sessions 配方清壳），分支指针 `mcp6-awf07` 留着作事后比对凭据。
- 2026-09-08 14:1x · MCP-1（派发记录，钉 main `0ef63897`）：**09–17 分三波，不九张齐开。** 理由四处共享面：前端公共半边
  （四口客户端 + 五步流程组件）在 `apps/admin-web/src` 零命中、无人做；Go `domain/publication_canonicalization.go` 与
  `adapters/http/publication_draft_payload.go` 九张各加一格于相邻行；12–17 六张同写 `CommercialPoliciesPage.tsx`；
  12/13/15/17 要的「服务端词表读口」在 `endpoints.go` 不存在且是新裁决。14:13 点名，MCP-3/4/5/6 应答，MCP-2 截至 14:16 未应答。
  **第 0 波** 16 → MCP-5（`2e9b1361`，`mcp5-awf16`，连带前端公共半边，约定落点 `party/publication-draft-api.ts` /
  `publication-draft-flow.ts` / `PublicationDraftFlow.tsx`）；**第 1 波** 09 → MCP-3（`7fc5a960`，`mcp3-awf09`）、10 → MCP-4
  （`39a6f0cd`，`mcp4-awf10`）、11 → MCP-6（`662822ef`，`mcp6-awf11`），Go 先做、流程组件等 16 落点广播；**第 2 波**
  12/13/14/15/17 等 16 进 main 与词表读口裁决后再点名派。基线 `0ef63897`；不加迁移、不动 `ports.go` / `endpoints.go`；
  九张都不碰 `party/api.ts`。本波各票 Status 行由认领人自己转 in-progress，本表「状态」列随各票 resolved 时更新。
- 2026-09-08 18:3x · 通道 1（用户「你自决」授权，代裁第 2 波前置）：**词表读口取「一口按 kind 答」**，立票 [20](./20-publication-vocabulary-read-face.md)
  作公共半边——`GET /commercial-publication-vocabularies?kind=` 答该册正文所有封闭集的**码**（不答中文，中文留 admin-web 各页词表，集外原样示出），
  集合从 PC 领域既有枚举列出、测试双向钉住与规范化器同词，`endpoints.go` 只加一行。理由：五步路径已是一套载荷带 kind 分册（ADR-0126），
  词表跟同一条分派是同一形；一册一口要四张票四个人改 `endpoints.go`，是第 1 波刻意避开的撞点。**派单顺序**：20 + 14（14 不要词表）先开；
  20 落点广播后 12/13/15/17 点名派——四张同写 `CommercialPoliciesPage.tsx`，各自只加一个签 + 一个 import（纯加行，推送方解撞），
  表单本体各进新文件；Go 两共享文件照第 1 波「改前占号、只加自己一格」。子票表加 20 一行；12/13/15/17 的阻塞边加 20。
