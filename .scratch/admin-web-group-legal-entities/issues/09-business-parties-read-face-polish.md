# 09 业务参与方页读面打磨：徽章、时间悬停原串、筛选排序、行详情抽屉、复制、操作者文案（与票 01 同形）

Category: enhancement
Status: resolved
Blocked by: 无
地盘：`apps/admin-web/src/pages/party/BusinessPartiesPage.tsx`（**两张读签 + 抽屉；不动登记签那段，归票 10**）、
`apps/admin-web/src/pages/party/business-party-list.ts` 与 `party-relationship-list.ts`（新，纯逻辑 + node:test）、
`apps/admin-web/src/domain/status.tsx`（关系四词补色调）、`Instant` 组件从 `GroupLegalEntitiesPage.tsx` 抬到 `pages/party/` 共享文件。
出处：通道 1 评估「业务参与方」页（钉 main `dbe989cc`，2026-09-16 20:2x），spec「第二轮」表第二档。

## 为什么

票 01 把法人页打磨到运营配置员能用，同一模块的参与方页两签仍是骨架期形态：状态纯文字、时间无悬停原串、无筛选排序、行点不进去、
标识不可复制、描述是工程师口吻（「角色中立的参与方身份本体的最新登记修订，状态按装载时点导出」）。同一模块两页一精一粗，
操作者在两页间切换要重新学一遍。做法全部照 01——那张票的裁决 1–3 在本页同样成立，不另造。

## 要做的

1. **状态列用 `StatusBadgeFor`。** 身份签三词（`已登记` / `已生效` / `已停用`）01 已补齐；关系签五词里 `已生效` 已有，词表补
   `候选关系`（info：登记进来、等批准）/ `已到期`（neutral：合法终局）/ `已撤销`（warning：有依据的主动终止，看的人要知道边界）/
   `已替代`（neutral，格里带后继）。出处 party-commercial CONTEXT Lifecycles 下「参与方关系」。停用 / 终止的时点与依据照旧同格。
2. **时间悬停原串。** 两签所有时刻格用 01 的 `Instant`（`<time dateTime title={ISO}>`）；`formatRange` 的区间两端同样给原串。
   `Instant` 今天写在 `GroupLegalEntitiesPage.tsx` 里，抬成 `pages/party/` 下的共享件，两页引同一份。
3. **筛选与排序。** 身份签：「状态」下拉（从 `identityStatusLabels` 派生，不另抄）+ 「排序」（登记时间 新→旧【默认】/ 参与方标识 A→Z /
   生效时点 早→晚）；关系签：「角色」下拉（从 `partyRoleLabels` 派生）+ 「状态」下拉（从 `relationshipStatusLabels` 派生）+ 「排序」
   （生效起 新→旧【默认】/ 关系标识 A→Z）。都在已取回数据上做，纯函数 `filterBusinessParties` / `sortBusinessParties` /
   `filterPartyRelationships` / `sortPartyRelationships` 由 node:test 钉；同秒按解析值比（01 评审 Standards 1 的教训）。筛空文案与
   计数摘要按 01 裁决 1（筛出为空不是空态）。
4. **行详情抽屉。** 身份行：参与方标识、修订、名称、状态、依据、生效时点、停用时点、停用依据、登记时间、租户；「修订历史」区今天如实写
   「读口尚未建立」指向票 12，不造数。关系行：关系标识、修订、持有方 + 名称、相对方 + 名称、角色、适用范围、依据、有效区间、状态、
   终止时点、终止依据、后继、租户。抽屉按标识重找行（01 的做法），列表重取后以新答案为准。
5. **复制。** 身份：参与方标识、依据；关系：关系标识、持有方、相对方、依据。复用 01 的 `useCopyToClipboard`（一并抬到共享文件）。
6. **操作者文案。** 身份签描述：「业务参与方是与本网络发生商业往来的对象——货主、承运商、代理、转售商；一个参与方可以同时是法人，
   也可以不在任何关系里。每次登记形成新修订，历史不可覆盖」；关系签描述：「参与方关系记谁对谁持有什么角色、在什么范围、多久；
   撤销、到期与替代只影响边界之后的新决定」。空态不写 CLI 命令行，改「在「登记」签登记第一个」；未配置那句照旧（给排障的人看）。

## 不做

- 不加分页、不下推筛选排序（归票 04）。
- 不动登记签那段（票 10）、不改三签结构、不做修订 diff 视图。
- 不加导出。

## 完成判据

- 三道门绿：`tsc -b --noEmit`、`run-tests`、`vite build`。
- `business-party-list.test.ts` 与 `party-relationship-list.test.ts`：筛选 / 排序 / 计数摘要 / 筛空提示各至少一条正向一条边界。
- 演示形态（`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`）：`SYN-PARTY-RETIRED-01` 显「已停用」徽章 + 自 时点 + 依据；点行开抽屉；
  关系签筛「候选关系」得筛空文案而不是「0 段」。浏览器验收做不到就照 01 / 03 如实写「未验」。

## 裁决

1. 排序默认登记时间新→旧，判据同 01 裁决 2（刚登进去的那条）。
2. 「已撤销」用 warning 而不是 neutral：到期是时间到了、替代有后继接着，两者都不需要人注意；撤销是有依据的主动终止，在边界之后
   仍拿这段关系做决定就是错，色调要把这一格点出来。

## 完成记录（2026-09-16，分支 `mcp6-adminweb09` 基 main `cadd2d46`，通道 6）

作者：`55d2d81d`（纯逻辑层 + node:test）与 `d5396299`（页面接线 + 共享件 + 词表）均为通道 6 所作；票面本笔另提。

对完成判据：

- 三道门：`tsc -b --noEmit` 0、`run-tests` 252/252（本票新增 12 例）、`vite build` 0（均在隔离树 `d5396299` 实测）。不动 Go、不占 55432。
- `business-party-list.test.ts`：搜索三格包含匹配 / 状态精确匹配（含筛空交回空数组）/ 三种排序不改原数组 / 同秒不定小数位按解析值排 /
  计数摘要与筛空提示 / 选项表从 `identityStatusLabels` 派生。`party-relationship-list.test.ts`：搜索七格 / 角色 × 状态叠加筛（含筛
  「候选关系」无候选行时交回空）/ 两种排序 / 同秒排序 / 计数摘要（单位「段」）与筛空提示 / 两张选项表从 `partyRoleLabels`、
  `relationshipStatusLabels` 派生（含 CONTEXT 原词「候选关系」）。
- 演示形态浏览器验收：**未验**。隔离树起 vite（`:5209` → `127.0.0.1:8090`）后无头 Edge 取到的是 `AuthGate` 登录页（要 gk.idp.xyz 会话），
  无会话进不到页面；对 `:8090` 两读口直接实探记数据事实——`SYN-PARTY-RETIRED-01` 为 `DEACTIVATED` r2、`deactivatedAt`
  `2026-02-01T00:00:00Z`、`deactivationBasis` `SYN-DEREG-BASIS-PARTY-RETIRED-01`（徽章 + 自 + 依据三件的数据都在）；关系册 2 段里
  `SYN-REL-AGENT-01` 是 `CANDIDATE`，所以今天筛「候选关系」得 1 段，筛「已到期」才走筛空文案那一格（纯函数已钉）。

六条逐条：① 身份签三词与关系签五词均经 `StatusBadgeFor`；词表补 `候选关系`（info），`已撤销` neutral → warning（裁决 2；
`transport-fulfillment` 总单适用状态同词同判据，注释写明）。② 两签全部时刻格用 `Instant`，有效区间用 `InstantRange` 两端各给原串；
`Instant` / `InstantRange` / `filterSelectClass` / `useCopyToClipboard` / `DetailRow` 抬到 `pages/party/detail-primitives.tsx`，法人页只改为
引用（`DetailRow` 与 `filterSelectClass` 一并抬走：抽屉与过滤条要同一形状，只抬两件会让另两件在参与方页多一份副本）。③ 身份签「状态」
「排序」下拉，关系签「角色」「状态」「排序」下拉；比较器抽到 `list-order.ts` 共用（`legal-entity-list.ts` 的私有副本不动，那是票 01 的
地盘）。④ `BusinessPartyDrawer` 十格 + 「修订历史」如实写读口尚未建立指向票 12；`PartyRelationshipDrawer` 十五格（比票面多「登记时间」
一格——行上有这个字段，抽屉列全字段就不该漏它）。⑤ 身份：参与方标识 / 依据；关系：关系标识 / 持有方 / 相对方 / 依据可复制。⑥ 两签描述
按票面原句；空态改「在「登记」签登记第一个」，不写 CLI；未配置那句由 `catalogueViewState` 照旧出。另：身份签补「登记时间」列——
默认排序按它排（裁决 1），排的键看不见时操作者判不出「为什么这一条在最上面」。登记签那段一行未动。

清点预报：`apps/admin-web` 新增 `.ts` 5 件（`list-order.ts`、两份列表逻辑、两份 `.test.ts`）+ `.tsx` 1 件（`detail-primitives.tsx`），
MECHANISM-INVENTORY 的 admin-web 文件数会随之变，重生成归推送方。

## Comments
