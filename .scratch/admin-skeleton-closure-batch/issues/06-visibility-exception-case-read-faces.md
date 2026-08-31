# 追踪异常案件三页读面——只加案件侧，目录侧一个符号都不许碰

Category: feature
Status: resolved——MCP-5 交付、MCP-1 审查代结（阶段一合入 `1f5d7ae` 装配，阶段二落主线 `255db1e`；见 Comments 末条与票 07）
Blocked by: 无

## 现状（取证于 `65b6cf2`）

`internal/visibilityexception` 的 `adapters/http` 包已有 `query_*`=3，服务着三张**已接线**的页
（`tracking-projection`、以及 `visibility-catalogues` 分派下的 `tracking-judgment-rules`、
`disclosure-policies`、`claim-prerequisites`）。本票要加的是**案件侧**读面。

该上下文 30 表 109 处 `tenant_id`，ADR-0077 形状逐字成立，不需要新 ADR。分野很清楚——目录侧
已有行、案件侧全零：

| 侧 | 表与行数 |
|---|---|
| 目录（**已接线，不许碰**） | `milestone_mapping_entry` 4、`triage_rule_entry` 4、`disclosure_policy_entry` 2、`claim_authorization_catalogue` 2、`claim_covered_kind` 2、`claim_authorized_applicant` 2、`claim_contract_scope` 1 |
| 案件（本票要开读面） | `signal_episode` 0、`triage_conclusion` 0、`disposition_request` 0、`exception_case` 0、`visibility_gap` 0、`claim_item` 0、`claim_material_receipt` 0、`claim_supplement_deadline` 0、`recovery_matter` 0、`recovery_action` 0 |

## 三页与表的对应（起点，需复核后定稿）

- `exception-triage` 异常分诊与处置协调 → `signal_episode`、`triage_conclusion`、`disposition_request`
- `exception-cases` 异常案件 → `exception_case`、`visibility_gap`
- `claims-recovery` 索赔与追偿 → `claim_item`、`claim_material_receipt`、`claim_supplement_deadline`、`recovery_matter`、`recovery_action`

**一条跨上下文边界要守**：索赔与追偿的**金额结算归 `settlement-accounting`**
（`customer_claim_amount`、`claim_amount_adjustment`、`recovery_receivable` 等在那边），
VE 这边只有索赔项与追偿事项本体。三页现有骨架已按此把金额列留零并注明——**接真时保留该分界**，
不要跨过去读 SA 的表。那半属票 04 地盘。

## 硬约束：目录侧一个符号都不许碰

目录侧的读端口、读适配器、`query_*` 处理器与三张页**全部已接线并在服务**。碰它们就是拿别人的
live 页冒险，而回归失败在页面上未必看得出来。本票只新增案件侧符号，既有目录侧符号一律不动、
不重命名、不「顺手重构」。

## 表会一直是空的，这是预期不是缺陷

案件侧的写入方是信号 → 分诊 → 建案的编排，而信号来自运行时业务事实，事实在接入渠道墙后面——
见 [spec 事实基线](../spec.md)。三页接完**仍是空册**。

**完成判据只能写成「空态文案说的是『读取入口已配置、登记册为空』，而不是『尚未接线』」，
不得写成「页面有数据」。** 不要造异常案件或索赔项种子——案件是业务事实不是规则目录，这正是
导航把三张目录页与三张案件页分区放的理由（票 `admin-web-page-wiring-frontier/02` 的导航裁决：
「案件页空着是三堵墙拦的，混进目录会让人以为墙降了」）。读适配器的真库测试照常写。

## 两阶段与次序

- **阶段一**：三组读端口 + 真库读适配器（含真库测试）+ 在既有 `adapters/http` 包里加 `query_*`
  与隔离读准入入格。自验绿后向频道交**已验 SHA** 与端点行；**不自改** `cmd/parcel-api` 装配
  四件（占号在票 07）。
- **阶段二**：收到 MCP-1「已装配」广播后，三页接真，`liveIds` 加三行——**只加自己那三行，
  不动邻行**。注意 `ExceptionTriagePage` 在 `apps/admin-web/src/pages/governance/` 下，另两页在
  `pages/visibility/`。

## 完成判据

三页转 live；空态文案说「读取入口已配置、登记册为空」而非「尚未接线」；索赔金额分界保留；
含真库全仓绿（注明）；四张已接线的目录/追踪页回归无变化。

## Comments

**[MCP-5] 阶段一完成，页-表对应定稿（与「起点」的三处出入均有判据）：**

| 页 | 端点 | registry | 表 |
|---|---|---|---|
| exception-triage | GET `/exception-triage-records` | `signal-episode` | `signal_episode` LEFT JOIN `triage_conclusion`（同键两表，0002 同笔提交，结论三件成对透出） |
| | | `disposition-request` | `disposition_request`（judgment/cancellation/supersededBy 缺席即答案，替代不是删除） |
| exception-cases | GET `/exception-case-records` | （单册无参） | `exception_case`（mergedInto 在场即受控归并，原编号照列） |
| claims-recovery | GET `/claims-recovery-records` | `customer-notification` | `customer_notification`（milestones 整列 jsonb 照登记转写为数组，不折成「已通知」） |
| | | `claim-item` | `claim_item` + `claim_supplement_deadline`（列面只计期限版本数，逐版历史属详情读口） |
| | | `recovery-matter` | `recovery_matter` + `recovery_action`（两类动作各取最近节点，LATERAL；预先通知与正式主张不折并） |

与起点清单的出入：

1. **`visibility_gap` 不开读法**——exception-cases 页没有缺口栏；缺口是「预期观察届满未得」
   的证明，无栏可供就不设读法，造一个没人消费的册子只会引人把缺口读成案件。
2. **`claim_material_receipt`（含撤销表）不开读法**——claims-recovery 三页签的栏目里没有收讫
   列；收讫减撤销的在手口径属资格审核的证据视图，不是列面。
3. **`customer_notification` 补入**——起点清单漏了它，但页签一「客户异常通知」整签由它供数
   （0004 建表，行数 0，与其余案件表同为空册预期）。

页面栏目在存储上**没有登记格**的键（行体结构上不存在，不代填，接真时前端留空即如实）：
发作期详情「事实依据」（`accepted_fact` 行上不指回发作期）；案件「严重度/优先级/当前工作
条件/响应周期」（0008 刻意只落精简主生命周期）；索赔「首次索赔期限」（合同侧口径）；追偿
「对方响应/外部责任结论」（0004 两表均无响应/结论列）。**金额零键**：赔付/追回金额归
settlement-accounting（跨上下文分界照守，未读 SA 任何表）。

准入复用既有 `OperationsTrackingIntake`（未配置 403 + `ACCESS_CHANNEL_NOT_CONFIGURED`，
与既有查阅端点同签名；`UnconfiguredIntake`/`IsolatedOperationsReadIntake` 零改动）。目录侧
`query_visibility_catalogues.go` 等既有符号零触碰（`git diff` 可证：http 包只新增 6 文件）。

自验：`go build ./...`、`go vet ./...`、`gofmt -l` 全干净；全仓测试含 VE 真库
（PostgreSQL 16，`adapters/postgres` 37s 实跑）全绿。装配四件占号票 07，未动 `cmd/parcel-api`。
阶段二（三页接真 + `liveIds` 三行）等 MCP-1 装配广播。

### 阶段二审查与代结（MCP-1，2026-08-31）

MCP-5 会话 crash 时阶段二改动已在工作树未提交；MCP-1 按批务接手，逐项审查后于 worktree 提交
`e587776`、序移落主线 `255db1e`。交付形：

- `pages/visibility/case-api.ts`：三端点镜像，返回类型按 registry 收窄；`case-presentation.ts`：
  封闭词表——案件三相（0008）、处置判断/取消各四值（0005）、资格三值（0004+0013）、责任结论
  四值（0004）、追偿节点七值（0004），审查逐词对过迁移 CHECK；通知里程碑四词对过
  `domain.NotificationMilestone` 封闭六值（`FAILED` 是通知侧词，非追偿侧 `DELIVERY_FAILED`）。
- 分诊页双册 chip：发作期 ReviewFlow 连分诊结论（未分诊如实示「尚未分诊」），处置请求 List
  （替代不是删除，原行照列）；四项分诊决定如实禁用并注明命令端点未建。案件页单册：无登记格
  四列（严重度/优先级/工作条件/响应周期）不代填。索赔追偿三签：通知里程碑分别记录不折并、
  三判分步转写、两类追偿动作各取最近节点不折并成「已追偿」。
- 审查核过：三份 TS 记录形状与 Go 查询处理器 JSON 键逐字段互镜；目录侧零触碰（diff 只含案件侧
  六文件）；金额零键分界照守（未读 SA 任何表）；`liveIds` 只加自己三行未动邻行；空态句三页均
  「读取入口已配置，登记册为空」。

提交态验证（构建与含真库全仓绿、门禁单跑 PASS）见票 07 收口注记。
