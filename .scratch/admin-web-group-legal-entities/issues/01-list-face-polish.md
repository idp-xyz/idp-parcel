# 01 集团与法人读面打磨：徽章、时间本地化、撤同值列、筛选排序、行详情抽屉、复制、操作者文案

Category: enhancement
Status: resolved
Blocked by: 无
地盘：`apps/admin-web/src/pages/party/GroupLegalEntitiesPage.tsx`、`apps/admin-web/src/pages/party/legal-entity-list.ts`（新，纯逻辑）、
`apps/admin-web/src/pages/catalogue-view.ts`（`formatInstant`）、`apps/admin-web/src/domain/status.tsx`（加两词）

## 要做的

1. **身份状态列用 `StatusBadgeFor`。** 词表 `domainStatusTones` 补 `已登记`（info：登记已落册、生效时点未到，
   CONTEXT 原句「生效时点未到的登记不支持新的商业决定」——推进中、无需行动）与 `已停用`（neutral：合法终局，
   同 `已退役`），出处 party-commercial CONTEXT Lifecycles 下「参与方身份（业务参与方、责任法人、货主客户账户）」。`已生效` 词表里已有。
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
- 新增 `legal-entity-list.test.ts` 与 `moment.test.ts`（时间格式随 `formatInstant` 搬到 `pages/moment.ts`，测试随之；原写 `catalogue-view.test.ts`）：筛选 / 排序 / 时间格式各至少一条正向一条边界。
- 页面在本机演示形态（`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`）下：`SYN-LE-01` 显「已生效」徽章、时间显本地、
  点行开抽屉、筛「已停用」得空态文案（不是「0 个」——那条计数守卫不许在筛出空时报零？见裁决 1）。

## 裁决

1. **筛出为空不是空态。** 四态里的空态说的是「登记册为空」；筛选筛没了是「当前条件下无匹配」，两者续办不同（前者去登记，
   后者改条件）。筛空时表格区显一行「当前筛选条件下没有匹配的法人」，计数摘要照显「共 N 个责任法人，当前显示 0 个」。
2. **排序默认登记时间新→旧。** 运营配置员最常看的是「刚登进去的那条」。
3. **本地时区而不是 UTC。** 跨境小包运营分处 CN/SG 两地，同一事件本地时刻不同，但带显式偏移之后无歧义；悬停给 ISO。
   存储与线格式仍是 UTC，这条只改呈现。

## 完成记录（2026-09-16，分支 `mcp6-admin-web-legal-entities`）

作者：`bad2cea8`（逻辑层）与 `8314b7d8`（裁决 1 两件，通道 6 crash 后由通道 1 原样收入）为通道 6 所作；`e8d31b46`（页面接线）为通道 4 所作；`4306a34c`（评审后三处修正）为通道 1 所作。

对完成判据：

- 三道门：`tsc -b --noEmit` 0、`run-tests` 237/237、`vite build` 绿（通道 1 于 `4306a34c` 实测）。
- `legal-entity-list.test.ts`：搜索四格包含匹配 / 状态精确匹配 / 三种排序 / 同秒不定小数位按解析值排 / 计数摘要与筛空提示 / 选项表；`moment.test.ts`：UTC 默认、Asia/Shanghai→UTC+8、半点区 UTC+5:30、负偏移 UTC-3、毫秒剥净、不可解析原样交回、`formatRange` 开区间、墙钟→RFC 3339 含非法日期。
- 演示形态浏览器验收（`SYN-LE-01` 徽章 / 本地时间 / 点行开抽屉 / 筛「已停用」得筛空文案）：**未验**——落地会话未起前端栈；三道门与纯函数测试覆盖了除渲染外的全部判据。

七条逐条：① `StatusBadgeFor` + 词表补 `已登记`（info）/ `已停用`（neutral）；② `Instant` 组件 `<time dateTime title={ISO}>` 显本地带偏移；③ 撤「对象类型」列，种类进抽屉；④ 「身份状态」「排序」两下拉 + `filterLegalEntities` / `sortLegalEntities`；⑤ `LegalEntityDrawer` 十一格，「修订历史」区如实写读口未建指向票 03；⑥ 法人标识 / 参与方身份 / 登记依据三格复制，toast 反馈；⑦ 页描述与空态改业务语言。裁决 1–3 均落实。

## Comments

**评审 ← 通道 3（非作者）· 钉 `71c7d5a3` · 基线 `a608536d` · 17:46 / 17:47。** Standards 阻断 0 / 非阻断 4；Spec 阻断 0 / 非阻断 3。全文在任务 `task-825d2d6b` 的 report_task。

- 已在 `4306a34c` 修：Standards 1（`sortLegalEntities` 字符串比在 RFC3339Nano 剪尾零的同一秒内语义错——改按解析值比，退回字符串比兜不可解析）；Standards 2（`moment.test.ts` 头注的调用点计数——去数字）；Standards 4（`legalEntityStatusFilterOptions` 注释说不另抄却抄了一份——改从 `identityStatusLabels` 派生）；Standards 附（`status.tsx` 出处节名「身份生命周期」在 CONTEXT 里不存在——改为实际标题；票面与 spec 同错同改）。Spec 2（完成判据点名的 `catalogue-view.test.ts` 不存在、覆盖在 `moment.test.ts` 且等价更宽——票面改名）。
- 留票面记、未动：Standards 3——`moment.ts` 的 `FormatInstantOptions.timeZone` 与导出的 `zoneOffsetMinutes` 只有测试消费，注释说的「悬停并排显 UTC 与本地」没有生产调用点（`Instant` 的 title 放原 ISO 串）。投机性泛化，判断题；下次动 `moment.ts` 时删或用。Spec 3——地盘外三处改动（`ListPageTemplate.emptyRowsNote`、`main.tsx` 配时区、`components/registration/index.tsx` 导出）均为票面所需的纯加行，合入时无撞点。
