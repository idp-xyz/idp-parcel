# 182 管理台网络目录登记拒因缺九格中文标签

Category: enhancement
Status: resolved——2026-10-26 会话 `tops-20`（Claude Code）照 workflow「前端切片」在 main 上直接做（用户 2026-10-26「按你的建议开工吧」），见文末「完成记录」。此前 ready-for-agent——R3（不急）：缺标签的拒因页面按 `RegistrationAnswerNote` 的纪律示英文原名，不冒充别的答案、不丢答案，只是登记方读不到中文续办说明。2026-10-26 会话 `tops-20`（Claude Code）据票 [176](./176-a-connection-code-with-the-window-basis-separator-is-accepted-at-registration.md) 完成记录越权风险点 3 与其评审记录 Spec 非阻断 2 立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。
Blocked by: 无
地盘（建议）：`apps/admin-web/src/pages/network/api.ts` 的 `registrationRefusalReasonLabels`；diff 全在 `apps/admin-web/**`，照 [workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)一人在 main 上直接做。**code/29 也在改这个文件**：只往表里加条目，不重排、不改既有各行，缩小冲突面。
出处：票 176 完成记录「越权风险点」3、评审记录 Spec 非阻断 2。

## 现象（读码，钉票 176 分支 tip `e0d4e92a9`；评审逐格对过 Go `String()`）

`registrationRefusalReasonLabels` 对照 `internal/networkrouting/application/register_network_catalog.go` 的 `CatalogRefusalReason.String()`，缺下列各格（括号里是 Go 头注说的续办，供写标签取词）：

- `RANKING_FORM_UNKNOWN`（路由策略版本声明了族外的排序形态；没声明不在此列）
- `COVERAGE_MALFORMED`（覆盖不成形：国家码、前缀，或没登覆盖国家却带前缀——改覆盖）
- `NODE_ROLE_BLANK`（节点角色里有空白节点身份——补节点）
- `NODE_ROLE_DUPLICATED`（同一角色里一个节点登两次——删重）
- `NODE_ROLES_WITHOUT_COVERAGE`（没登覆盖却登了节点角色——先登覆盖）
- `CALENDAR_CONTENT_OUT_OF_RANGE`（截单不在本地当日分钟内，或处理时长、衔接缓冲、派送时长为负——改那个数）
- `COST_BASIS_MALFORMED`（线路逐段成本依据不成形：段号不在段链内、同段两条、种类不在封闭两类或引用空白——改依据那一行）
- `LEG_ZONE_MALFORMED`（段计价区域不成形：没给段、段号为负、同段两次、区域或依据空白——改那一行）
- `LEG_ZONE_WITHOUT_COST_BASIS`（要登区域的段没挂成本依据，或线路版本不在册——先挂依据）

## 修法（建议）

- 逐格补中文标签，措辞取各格 Go 头注的续办动作；呈现组件不改。
- 可选：加一条前端用例，把标签表的键与登记答案的拒因封闭集对齐（集合以 Go `String()` 为准，前端手抄一份时写明取自哪个 SHA）。

## 完成判据

- [x] 上列各格都有中文标签，键与 Go `String()` 逐字一致。
- [x] admin-web 三道门：`tsc -b --noEmit`、`node scripts/run-tests.mjs`、`pnpm exec vite build`。

## 完成记录（2026-10-26，会话 tops-20，前端切片）

- **落点**：`apps/admin-web/src/pages/network/api.ts` 的 `registrationRefusalReasonLabels` 在 `SOURCE_MISSING` 之后补九格（`RANKING_FORM_UNKNOWN`、`COVERAGE_MALFORMED`、`NODE_ROLE_BLANK`、`NODE_ROLE_DUPLICATED`、`NODE_ROLES_WITHOUT_COVERAGE`、`CALENDAR_CONTENT_OUT_OF_RANGE`、`COST_BASIS_MALFORMED`、`LEG_ZONE_MALFORMED`、`LEG_ZONE_WITHOUT_COST_BASIS`），措辞照「问题——续办」既有写法、取自各格 Go 头注；既有各行顺序不动，呈现组件不改。
- **判据**：
  - ✅ 键与 Go 一致：脚本把 `internal/networkrouting/application/register_network_catalog.go` 里 `CatalogRefusalReason.String()` 的返回值与标签表的键逐一比对，两边各 24 格、无缺无多（钉 main `c442019f6` 之后、本笔之前）。
  - ✅ 三道门（Node 22）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 676/676；`pnpm exec vite build` 退 0。
- **未做**：修法里可选的「前端用例对齐拒因封闭集」没加——要在前端手抄一份 Go 封闭集，抄本本身会漂；改由上面那次脚本比对作证。
- **评审**：只改业务页（`pages/**`），按 workflow「前端切片」第 5 步自查，未另派评审。

