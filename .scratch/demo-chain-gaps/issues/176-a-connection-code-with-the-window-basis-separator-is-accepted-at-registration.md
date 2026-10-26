# 176 连接代码含窗口依据分隔符 `+` 时登记口照收，TF 准备那一路每封初始路由信封都响亮失败

Category: bug
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领，分支 `claude-dcg176`（worktree `idp-parcel-dcg176`）。此前 ready-for-agent——R3（不急）：要租户把连接代码登成带 `+` 才撞，撞了是响亮失败（准备那一路每封信都失败、可见），不静默答错；补法清楚。2026-10-26 会话 `tops-20`（Claude Code）据票 [142](./142-network-routing-guards-the-connection-window-basis-spelling.md) 完成记录越权风险点 1 立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。
Blocked by: 无
地盘（建议）：NR 连接登记口（`RegisterConnectionVersion` 所在的 `internal/networkrouting` 应用层与领域构造门）及其 HTTP、CLI 拒因标签；admin-web 若有该拒因的中文标签一并补。
出处：票 142 完成记录「越权风险点」1；[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二与越权风险点 25；代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) R4。

## 现象（读码，钉票 142 分支 tip `6d1363429`）

- NR 目录证据视图 `CatalogInitialRouteEvidence` 折候选路径时，`plannedLeg` 给节点间段拼窗口依据 `"CONNECTION/" + versionReference(代码, 版本)`。
- TF 侧 `internal/transportfulfillment/adapters/networkrouting` 的 `connectionOf` 按 `+` 切格、认 `CONNECTION/` 前缀、要求恰一格、在最后一个 `@` 处切版本。
- 连接登记口 `RegisterConnectionVersion` 与库上 CHECK 只要求代码非空白。代码含 `+` 时，NR 拼出 `CONNECTION/A+B@1`，TF 切成两格、读不出带版本的连接，准备那一路在每封初始路由信封上响亮失败（ADR-0201 越权风险点 25 那一种）。
- 票 142 的用例守的是「恰一格 `CONNECTION/<代码>@<版本>`」这条契约在合法代码上成立，不守代码本身带分隔符。

## 修法（建议）

- 连接登记口加一个确定性拒因：代码含窗口依据分隔符 `+` 时答`未受理`并带原因；库上 CHECK 同步收紧（新迁移，取号时核在途分支）。
- 拒因是登记答案集的改动：HTTP 载荷、CLI 与 admin-web 的拒因标签都要认这一格。
- 存量：迁移前核演示种子与各在途分支的连接代码没有 `+`（今天的 `SYN-` 种子没有）；若真有，迁移不能静默失败，要先列出。

## 完成判据

- [ ] 领域/应用用例：带 `+` 的连接代码登记答未受理，原因指名分隔符；不带的照旧。
- [ ] 真库：CHECK 拒带 `+` 的代码；迁移在演示种子上施加不失败。
- [ ] HTTP、CLI、admin-web 拒因标签认这一格。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`；走 Go 那条路。
