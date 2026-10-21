# 117 投影条目落库不带前身，运营追踪查阅的 `supersedes` 一格永远是空

Category: bug
Status: needs-triage——2026-10-19 票 [103](./103-a-rederived-final-outcome-enters-the-projection-without-its-predecessor.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。实现 103 时写真库用例撞上（钉 `4cf203fb`，只记 `S`）。
Blocked by: 无
地盘：VE `adapters/postgres/projection.go` 的 `projectionEntryRow` / `marshalProjectionEntries` / `rebuildProjection`；要裁的那一格可能还碰 `application/derive_projection.go` 的 `Handle`。
出处：VE CONTEXT「被替代的条目继续留在投影版本内并标明替代关系；标准追踪里程碑、各追踪维度和追踪摘要只由当前有效即未被替代的条目派生」；运营追踪查阅「面向投影的全部并行维度、冲突关系与被替代条目」；`adapters/http/query_tracking_projections.go` 的 `projectionEntryBody.Supersedes`（注释「supersedes 缺席即首登事实」）。

## 现象（真库实测）

- 投影条目序列化成 `projectionEntryRow` 落进投影版本行，这一行没有前身那一格；`rebuildProjection` 读回时也就造不出 `Supersedes`。事实库（`accepted_fact.supersedes_version`）里前身是在的，只是投影条目丢了。
- 于是运营追踪查阅 `projectionBodyOf` 读 `fact.Supersedes()` 永远读到「无前身」，`supersedes` 永远缺席——按它自己的注释，等于把每一份更正都说成首登。
- 103 的真库用例 `TestACorrectedFinalOutcomeLeavesOnlyOneGenerationCurrentInTheProjection` 起初在投影条目上断言前身，读回为空；改从事实库断言才过。

## 影响

- 运营查阅看不出当前那一条是更正后的版本、它取代了谁。不挡动线，查阅答的是「少说」，不是答错的里程碑。
- CONTEXT 那句的前半「被替代的条目继续留在投影版本内并标明替代关系」今天也没落：派生编排先经 `CurrentlyEffective` 滤掉被替代条目再建投影（`DeriveProjectionHandler.Handle` 注释写明「留在事实库是为可追溯，不进条目」），被替代的条目根本不进投影版本。这是代码与 CONTEXT 的一处分叉，不是 103 引入的。

## 三类

- 机制：投影条目落库带上前身（JSON 加一格，旧行缺这一格读成无前身，不需迁移）。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 只补落库那一格（条目照旧只收当前有效的，当前那一条能说出「我取代了谁」），还是连同 CONTEXT 前半一起落——被替代条目也进投影版本、标明已被替代、派生各维度时跳过。后者要改 `TrackingProjection` 的条目形状与 `CarriesCustomerServiceFinal` 一类读法的前提，归 VE owner。

## 规模

S（只补落库）；M（连同被替代条目进投影版本）。

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/ve.md](../../docs-consistency/audit/ve.md) VE-03（归 C）：被替代条目不在投影版本里，与 VE CONTEXT「被替代的条目继续留在投影版本内并标明替代关系」、ADR-0065 决定七「被替代条目留在版本内但不参与派生」相反。这正是本票「要裁的」后一半，审计条目落在本票，不另立。按 main `67438a0f` 复核，现象仍在（`DeriveProjectionHandler.Handle` 先经 `domain.CurrentlyEffective` 滤掉被替代事实；测试 `TestASupersededFactStaysArchivedButLeavesTheEntries` 断言前身不在条目里）。

审计补充的一点：管理台 `apps/admin-web/src/pages/visibility/TrackingProjectionPage.tsx` 的 `currentEffectiveEntries` 按 CONTEXT 的假设在前端再滤一遍被替代条目，在后端今天的形状下什么也不滤——前后端对同一读模型的理解相反。本票裁「连同 CONTEXT 前半一起落」时，前端那一步才有意义；裁「只补落库那一格」时，那一步应删。

分级建议（文档自洽 02）：R3——运营查阅「少说」，不对客答错（客户视图只读当前有效条目）。若 owner 认定「事实库留档已够」，改走 B（新 ADR 部分停用 ADR-0065 决定七那句），归修订票 docs-consistency/04。
