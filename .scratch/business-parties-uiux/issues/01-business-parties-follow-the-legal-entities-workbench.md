# 01 业务参与方页照集团与法人页改成工作台形态

Category: enhancement
Status: in-progress——2026-10-28 tops-20（idp-parcel-91）立并认领（用户「参考集团与法人实现」），分支 claude-bp-ui
Blocked by: 无

## 现象

`BusinessPartiesPage.tsx` 仍是 `ListPageTemplate` 旧形态：三签（参与方身份 / 参与方关系 / 登记）、`<select>` 筛选与排序下拉、点行弹 `Drawer`、登记签里用胶囊按钮切三张表单。同一组的 `GroupLegalEntitiesPage.tsx` 已是 `WorkbenchPageTemplate`：指标可点即筛、状态胶囊带计数、表头点排序、单击行右栏开详情（可拖宽、次要列自动收起）、详情栏「更多」或对象地址开整页对象标签（`LegalEntityDetailPage`，`DetailPageTemplate`）、`/` 检索 ↑↓ 换行 Esc 收栏、登记从页头主动作进住在地址上的登记视图（`?register=1`，草稿未提交时离开先确认、刷新由 beforeunload 拦）。两页并排，参与方页显得粗糙。

## 做什么（只改 `apps/admin-web/**`）

照 `GroupLegalEntitiesPage` / `LegalEntityDetailPanel` / `LegalEntityDetailPage` / `group-legal-entities-address.ts` / `legal-entity-list.ts` 的形态与纪律，把业务参与方页改成同一套：

1. **两册各成工作台**：参与方身份册与参与方关系册仍分开（两册状态代数不同，见原文件头注），各有自己的指标（按册上已有的状态与「待补 / 需关注」判据，不发明状态）、状态胶囊带计数、关系册的角色筛选、表头排序（`WorkbenchSort`，改写 `business-party-list.ts` / `party-relationship-list.ts` 的排序为按键排，保留并更新其单测）、右端总数与当前显示数、`noMatchText` 与清筛。两册之间怎么切由你按模板能力定：优先不改 `templates/*`；确需给 `WorkbenchPageTemplate` 加插槽（例如头部下方一个册切换位）就加可选 prop、向后兼容。页头副标题照参照页带「集团 <租户>」（`useBoundTenant`）。
2. **右栏详情代替抽屉**：身份行详情栏沿用两签——「全景」（默认，`PartyOverviewSection`）与「身份与修订」（原各格 + `RevisionHistorySection`）；关系行详情栏列原各格，撤销 / 到期动作（`PartyRelationshipEndActions`）在栏内、按写授予收起。详情栏头区照 [group-legal-entities-uiux/06](../../group-legal-entities-uiux/issues/06-detail-panel-header-slimmed.md) 不重复事实。
3. **对象页**：参与方对象地址 `#/business-parties/<partyId>`（壳层按前两段开独立标签，同 `detailHash` 的做法），`DetailPageTemplate`，与详情栏同一套结构（[09](../../group-legal-entities-uiux/issues/09-object-page-matches-detail-panel.md)）：头区引用签、指标带、概要（全景）、时间线（修订历史）。关系对象页可不做，详情栏够用就写明。
4. **登记从页头主动作进登记视图**：三本册（参与方身份、参与方关系、身份停用）由页头主动作进入（一个主动作带册选择，或主动作 + 「更多」，照参照页 `LegalEntityMoreMenu` 的味道，不要三颗并排大按钮），登记视图住在地址上（扩展 `business-parties-address.ts`，例如 `?register=business-party|party-relationship|identity-deactivation`），刷新仍停在登记视图、后退回列表、草稿未提交时离开先确认、beforeunload 拦（[04](../../group-legal-entities-uiux/issues/04-registration-view-is-addressable.md)、[party-commercial-uiux/17](../../party-commercial-uiux/issues/17-unsaved-draft-warnings.md)）。**必须保住的既有契约**：法人页「到业务参与方页停用」深链（`deactivationPresetOf` / `withoutDeactivationPreset`、`?q=` 检索词）落到身份停用表单并预填；三张表单切走再回来草稿还在（[party-commercial-uiux/09](../../party-commercial-uiux/issues/09-drafts-survive-tab-switches.md)，`useVisited` 那一套）；写入口按授予收起（ADR-0315，`useMayWriteRegistry`），没有授予时地址里的登记视图落回列表。
5. **可访问性与对比度**照 [08](../../group-legal-entities-uiux/issues/08-page-accessibility.md)：次要文字用 breadcrumb 令牌不用 textMuted、不出现 <12px 小字、进度条有名字、主动作用 `primaryActionClass`。
6. 空态文案里的「在「登记」页签…」改成新入口的说法（`entryHint`）。

不做：不改后端、不改端点契约；筛选排序仍只在已取回的一页上做；不改法人页。

## 完成判据

- `tsc -b`、全部单测、`vite build` 通过；list 判读与地址判读有单测（排序键、筛选、计数、`register` 与 `deactivate` 深链并存的解析）。
- 真浏览器核一遍（没有 Playwright MCP 就按前两次彩排的办法用缓存 Chromium，或至少 curl 端点 + 写明「未经浏览器目测」）：两册工作台、点指标筛选、表头排序、右栏详情两签、对象页、三张登记表单可达且法人页停用深链仍预填、无授予账户（查阅员）看不到登记入口。
- 碰 `templates/*` 或 `components/*` 时按 `docs/agents/workflow.md`「前端切片」要一份非作者评审（协调方会派）。
