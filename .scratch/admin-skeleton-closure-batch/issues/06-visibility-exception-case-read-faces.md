# 追踪异常案件三页读面——只加案件侧，目录侧一个符号都不许碰

Category: feature
Status: ready-for-agent——MCP-5
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
