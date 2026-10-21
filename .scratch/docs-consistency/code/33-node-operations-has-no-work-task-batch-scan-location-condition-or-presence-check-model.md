# 33 NO 没有作业任务、作业批次、扫描、作业位置、物理状况观察与在场核对的模型

Category: enhancement
Status: needs-triage——建议 R3（不急）：没有生产路径，不答错；UC-NO-003 以现行口吻写着这几块，今天只有集运单元与方向依据记录落地。它们是机制，不等租户参数；租户取值（作业清单、设备与校准、位置层级的具体划分）仍按 `PAR-NET-04..06` 登记，未登记答未配置。2026-10-21 文档自洽 05 隔离评审立（用户「你作为业务和系统专家自决」授权），分支 claude-dc05。分级与排期由协调方定稿。
Blocked by: 无（实际测量与当前有效实测另有票：pp-pricing-input-seams/04、product-strategy-boundary/08，不在本票）
出处：[audit/nr-no.md](../audit/nr-no.md) L3 的同类（以现行口吻写尚无代码的能力）；UC-NO-003「给开发的交接」未实现表末行。

## 现象

UC-NO-003 的目标、作业阶段、结果表与验收行写着作业任务（对象、动作、范围、依据，完成由有效事实派生）、作业批次（只组织不取得成员）、扫描与位置来源事实、当前位置派生、物理状况观察、在场核对（匹配、缺失、多出、位置差异）。`internal/nodeoperations/` 里这几块都没有模型：`domain/` 里有的是收寄、身份与身份冲突处置、送站预报与预报时窗、送站交接单与交付范围、控制转移、关务协作承接、集运单元与方向依据等，没有这几类，`application/` 没有对应编排，迁移里没有对应表。

## 证据

- 文档：node-operations CONTEXT Language「作业任务」「作业批次」「作业位置」「物理状况观察」「在场核对」；「作业授权与执行」「测量、位置、状况与核对」两节；UC-NO-003 `AT-NO-034`、`AT-NO-043` 等。
- 代码：`internal/nodeoperations/domain`、`internal/nodeoperations/application` 的文件清单里没有这几类对象（取证于分支 claude-dc05，基 main `205eb254`）。

## 三类

机制：任务与事实分层、任务完成由有效事实逐对象派生、批次不取得成员与控制、扫描与位置先保全来源再派生当前位置、状况观察只表达可见事实、在场核对逐对象形成差异。产品策略：任务完成的派生判法、当前位置的派生判法若内置，属产品策略。租户取值：作业清单、位置层级、设备与校准，未登记答未配置，不用行业默认。

## 规模

L：可按「来源事实保全 → 当前位置派生 → 在场核对」与「作业任务 / 批次」两条线拆票。

## 最小的诚实修法

分诊时拆成子票；先做有消费方的那一条（在场核对与当前位置要被 VE 的差异案件与 NR 的偏离判断消费）。在此之前 UC-NO-003 照现在的未实现表注明。

顺手（docs-consistency/05 第 2 轮评审登记）：三处测试的 Covers 注释仍写「`AT-NO-062` 末句」，UC-NO-002 已把那一格拆成 `AT-NO-131`，随本票第一次动 `internal/nodeoperations` 时改指 `AT-NO-131`：`application/delivery_scope_judgment_test.go::TestAnAmbiguityOnlyConflictWithoutAScopeIsReleasedIntoAnUnverifiedPendingUnit`、`adapters/http/reception_scope_basis_test.go::TestAManualRecheckThatReleasesAnAmbiguityIntoAnUnverifiedUnitAnswersCreated`、`cmd/parcel-dispatch/node_identity_ambiguity_release_test.go::TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit`。

## 要裁的

拆分与先后，分诊时定。
