# 01 集团与法人读面打磨：徽章、时间本地化、撤同值列、筛选排序、行详情抽屉、复制、操作者文案

Category: enhancement
Status: in-progress
Blocked by: 无
地盘：`apps/admin-web/src/pages/party/GroupLegalEntitiesPage.tsx`、`apps/admin-web/src/pages/party/legal-entity-list.ts`（新，纯逻辑）、
`apps/admin-web/src/pages/catalogue-view.ts`（`formatInstant`）、`apps/admin-web/src/domain/status.tsx`（加两词）

## 要做的

1. **身份状态列用 `StatusBadgeFor`。** 词表 `domainStatusTones` 补 `已登记`（info：登记已落册、生效时点未到，
   CONTEXT 原句「生效时点未到的登记不支持新的商业决定」——推进中、无需行动）与 `已停用`（neutral：合法终局，
   同 `已退役`），出处 party-commercial CONTEXT「身份生命周期」。`已生效` 词表里已有。
2. **时间本地化。** `formatInstant` 改显操作者本地时区、到秒、不带毫秒，并带显式偏移（`2026-09-16 12:27:55 UTC+8`）；
   原 ISO 串放进 `<time dateTime title>` 供悬停比对。它是共享函数，改动波及全部册页——这是要的：全站同一种时间写法。
   纯函数带 `timeZone` 参数以便测试不依赖机器时区。
3. **撤「对象类型」列。** 本册今天只有 `RESPONSIBLE_LEGAL_ENTITY` 一格（`legalEntityKindLabels` 只登这一词），整列同值
   无信息，判据同撤「运营集团租户」列那条注释。种类进详情抽屉。
4. **状态筛选与排序。** 过滤条加「身份状态」下拉（全部 / 已登记 / 已生效 / 已停用）与「排序」下拉（登记时间新→旧【默认】/
   法人标识 A→Z / 生效自 早→晚）。都在已取回数据上做，纯函数 `filterLegalEntities` / `sortLegalEntities` 由 node:test 钉。
5. **行详情抽屉。** 点行开右侧 `Drawer`，列全部字段：法人标识、修订、种类、参与方身份 + 名称、身份状态、登记依据、
   生效自、停用时点、停用依据、登记时间、租户。抽屉里「修订历史」区今天如实写「读口尚未建立」并指向票 03，不造数。
6. **复制。** 法人标识、参与方身份、登记依据三格可一键复制（`navigator.clipboard`），成功以 toast 或行内短提示反馈。
7. **操作者文案。** 页描述改为业务语言（「责任法人是对外签约、开票与结算的经营主体；每次登记形成新修订，历史不可覆盖」），
   空态改为「当前租户尚无责任法人登记。在「登记法人」签登记第一个，或用受控 CLI 灌入」。主责上下文与出处仍留在
   `moduleInfoById`，未配置态那句照旧显（那句是给排障的人看的，位置对）。

## 不做

- 不加分页控件：端点无分页参数，前端假分页会掩盖票 04 的问题。
- 不加导出。
- 不动两签结构。

## 完成判据

- `node node_modules/typescript/bin/tsc -b --noEmit`、`node scripts/run-tests.mjs`、`pnpm exec vite build` 三道绿（本机 README 口径）。
- 新增 `legal-entity-list.test.ts` 与 `catalogue-view.test.ts`：筛选 / 排序 / 时间格式各至少一条正向一条边界。
- 页面在本机演示形态（`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`）下：`SYN-LE-01` 显「已生效」徽章、时间显本地、
  点行开抽屉、筛「已停用」得空态文案（不是「0 个」——那条计数守卫不许在筛出空时报零？见裁决 1）。

## 裁决

1. **筛出为空不是空态。** 四态里的空态说的是「登记册为空」；筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，
   后者改条件）。筛空时表格区显一行「当前筛选条件下没有匹配的法人」，计数摘要照显「共 N 个责任法人，当前显示 0 个」。
2. **排序默认登记时间新→旧。** 运营配置员最常看的是「刚登进去的那条」。
3. **本地时区而不是 UTC。** 跨境小包运营分处 CN/SG 两地，同一事件本地时刻不同，但带显式偏移之后无歧义；悬停给 ISO。
   存储与线格式仍是 UTC，这条只改呈现。

## Comments
