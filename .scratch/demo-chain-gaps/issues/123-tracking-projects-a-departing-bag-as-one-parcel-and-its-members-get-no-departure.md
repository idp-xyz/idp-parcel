# 123 全程追踪把出发的一袋当一件包裹投，袋里的成员拿不到出发里程碑

Category: enhancement
Status: needs-triage——2026-10-20 票 [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0205](../../../docs/adr/0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 20（原指票 42，票 42 只修了门禁读数与阻断信号）与 [ADR-0225](../../../docs/adr/0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md)「不在本记录」第二条、越权风险点 9。
Blocked by: 无
地盘：VE 移动事实投影（`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact.go` 的 `objectsOf`，以及补派生 `backfill_movement_facts.go`）。

## 现象（读码钉票 42 的分支，只记 `S`）

- 全程追踪消费移动事实已形成信封时按键回读，对象取声明的段里事实发生那一刻在控的参与（ADR-0205 文末补记「展开口径」），逐对象派生投影。段里在控的是一袋时，投影按袋的引用立一件「追踪包裹」，袋里的成员拿不到这一条出发——客户与运营在成员的追踪上看不到 `DEPARTED`。
- 票 42 之后出发读数里已有袋的成员各一格（ADR-0225），阻断信号也落在成员上；但移动与到达不带读数，投影不能借读数展开，得像出发读数那样问节点作业那一刻的成员。
- 补派生（ADR-0224）沿同一口径，同病。

## 三类

机制（投影按成员展开：袋按事实发生那一刻的直接成员逐件派生，嵌套到包裹；袋本身投不投由 VE owner 定）。规模 M。

## 要裁的

- 袋本身还要不要作为一件追踪对象被投（今天它被当成包裹投，追踪页上多一件不是包裹的条目）。
- 成员取事实发生那一刻（与出发读数同口径），还是消费那一刻。
- 成员身份不成立、读不动时，投影入账不派生还是记未决重投（后者要先分确定性与暂时，见票 114）。

## 既有票

- [42](./42-departure-gate-reading-does-not-expand-consolidation-unit-members.md)、[53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（补派生）、[114](./114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)

## 文档自洽审计复核（2026-10-21）

审计条目：[audit/ve.md](../../docs-consistency/audit/ve.md) VE-10（归 C，审计已点名本票，不另立）：段里在控的袋或集运单元被当成一件包裹投影，与 VE CONTEXT「集运、装载或同批关系不合并包裹身份和客户轨迹」相反；出处 ADR-0205 文末补记越权风险点 20、ADR-0225 越权风险点 9。按 main `67438a0f` 复核，现象仍在；票 146、94 只改门禁读数，没有改投影展开。

分级建议（文档自洽 02）：R1——整袋出发是第一天的常态，袋里每件成员的 `DEPARTED` 永久缺失、追踪页多一件不是包裹的条目，属静默丢失，与已按 A 组处理的票 [53](./53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（`DEPARTED` 永久丢失）同一后果类。由协调方定稿。文档侧（UC-VE-002 `AT-VE-046` 附近注一句当前缺口）由修订票 docs-consistency/04 做。
