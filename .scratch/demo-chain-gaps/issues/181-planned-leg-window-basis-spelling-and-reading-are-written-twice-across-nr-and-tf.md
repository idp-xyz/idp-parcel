# 181 计划段窗口依据的拼写与读法分在 NR、TF 两处各写一份，两侧同步没有用例守

Category: enhancement
Status: ready-for-agent——R3（不急）：两侧今天一致，票 142、176 的用例在 NR 侧按 TF 读法另写一份读过；只有一侧改了拼写或切法、另一侧没跟时才撞，撞了多半是准备那一路每封初始路由信封都响亮失败，少数形状（某格前半截自带版本）静默读成另一条连接。2026-10-26 会话 `tops-20`（Claude Code）据票 [176](./176-a-connection-code-with-the-window-basis-separator-is-accepted-at-registration.md) 完成记录越权风险点 1 与其评审记录 Spec 非阻断 1 立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。
Blocked by: 无
地盘（建议）：`cmd/parcel-dispatch` 的测试（它同时导入 NR application 与 TF `adapters/networkrouting`）；NR、TF 生产代码不动。
出处：票 176 完成记录「越权风险点」1、评审记录 Spec 非阻断 1；票 [142](./142-network-routing-guards-the-connection-window-basis-spelling.md) 完成记录「越权风险点」2；[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二与越权风险点 25；代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) R4。

## 现象（读码，钉票 176 分支 tip `e0d4e92a9`）

- 拼写在 NR：`plannedLeg` 给节点间段拼 `CONNECTION/<代码>@<版本>`，候选投影以 `+` 连各格（`joinSortedUnique`）；登记受理门按 `windowBasisSeparator` 拒带 `+` 的连接代码，迁移 `network_routing/0028` 在库上同格拒。
- 读法在 TF：`internal/transportfulfillment/adapters/networkrouting` 的 `connectionBasisPrefix`、`windowBasisSeparator`、`connectionVersionDivider` 三个常量与 `connectionOf`（按 `+` 切格、认前缀、要求恰一格、在最后一个 `@` 处切版本）。
- NR 侧的契约用例（`internal/networkrouting/application/connection_window_basis_contract_test.go`）不导入 TF，按 TF 读法另写一份（`connectionCells`、`tfRead`）：守的是 NR 不改拼写、受理门拒的恰是这份读法读不回原代码的。TF 改了切法（换分隔符、改按第一个 `@` 切版本），NR 侧不红，受理门与 0028 也不跟着变。
- TF 侧用例拿手写的窗口依据串喂 `connectionOf`，不走 NR 的产出。

## 修法（建议）

- 在 `cmd/parcel-dispatch` 加一条用例，经两侧生产代码绑住拼写与读法：NR 登记受理门收下的那几种连接代码（含带 `@`、`/`、以 `CONNECTION/` 开头、结尾带 `@` 的）经 NR 真目录证据视图折成计划，再交 TF `PrepareOnInitialRoute`（`connectionOf` 不导出，走 `HandleFormedInitialRoute` 加一个记下命令的准备替身），断言每段准备命令的方向连接等于登记的代码；受理门拒的那几种（带 `+`），绕过受理门折成计划后交 TF，读不回原代码（报错或读成另一条）。
- 不在 NR 的测试里导入 TF：上下文方向不倒转（票 142 代决 2）。

## 完成判据

- [ ] `cmd/parcel-dispatch` 有一条用例经两侧生产代码绑住拼写与读法；变异 TF 的切法（如改按第一个 `@` 切）或 NR 的拼写，这条红。
- [ ] Go 门禁；带 PG 跑 `cmd/parcel-dispatch`；全量用 `scripts/test-full.sh`。
