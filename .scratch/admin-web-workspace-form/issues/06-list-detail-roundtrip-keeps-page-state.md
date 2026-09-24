# 06 列表 → 详情往返保住检索词与多选集：多标签壳层下「进详情再回来即清零」的出路

Category: enhancement
Status: resolved——2026-09-24 通道 1 在 `main` 上直接做完（workflow.md「前端切片」）：本地 `69f6413b` 壳层段 / `ebf8b025` 模板与页面段 / `c1b7f74b` 注释改口 + 票面 `cd4d8193`，**已进 main `9a477af9`**（2026-09-24 push，`3a47da8d..9a477af9` 纯 ff，CI 全绿，见 Comments 末条）。完成记录见文末；推送方自审可接受（用户令，其他通道忙），见 Comments。此前 in-progress——通道 1 接（通道 3 提议的分工）。此前 ready-for-agent——2026-09-24 用户授权通道 3 自决：C 的检索词半 + 壳层记住每张标签最后停在的完整地址，多选集进详情即丢（见「判断项答复」）。此前 draft——2026-09-24 通道 1 按票 01 评审 ← 通道 2 的 Spec 非阻断 1 立，形态取舍归用户
Blocked by: 无
地盘：`apps/admin-web/src/shell/workspace-state.ts` 与其 test、`Layout.tsx`（记地址、三条回程）、`templates/`（检索词读写地址的共用件、给页面的「回到某标签」口）、
`pages/shipment-request/ShipmentRequestListPage.tsx`、`pages/visibility/ExceptionCasesPage.tsx`，外加票 04 票面补记多选集改口。全在 `apps/admin-web/**` 与票面，走
workflow.md「前端切片」。
出处：票 01 评审 ← 通道 2 Spec 非阻断 1；票 04 第 5 条「换模块（组件卸载）即丢」。

## 为什么

票 04 给委托查阅的多选集写的承诺是「翻页 / 改检索词都不清；进详情再回来仍在」——当时列表与详情共用一个导航位，页面组件不卸载。票 01 把
`#/shipment-request-inquiry` 与 `#/shipment-request-inquiry/<id>` 认成两张标签，非活动标签卸载（`preserveInactiveTabContent={false}`），内容按标签 id 键住
（`<Fragment key={tabId}>`）：钻取时列表实例卸载，详情的「返回列表」写回列表 hash 时再挂一个新实例——检索词与多选集清零、列表重新取数。票 04 的
「换模块即丢」在票 01 之后实际是「进详情即丢」。票 01 已把 `ShipmentRequestListPage` 里两句失真的注释改成现状，行为留给本票。

## 判断项（归用户）

- **A. 同模块钻取不开新标签**：对象地址落在它的模块标签里（标签 id 只取模块段），详情在标签内切换。回到票 01 之前的单页区语义；代价是「同时开着
  一张队列和几个对象在比对」这件多标签的本意对同一模块失效。
- **B. 列表标签保活**：非活动的列表标签不卸载（按标签种类保活，或整组打开 `preserveInactiveTabContent`）。检索词、多选、滚动位置都在；代价是留着的页
  继续持有请求与内存——票 01 关掉 `preserveInactiveTabContent` 的理由正是这个。
- **C. 把页内状态抬出实例**：检索词进 hash 查询串（与保存视图的 `?view=` 同层）、多选集进按标签的 `sessionStorage`。标签照旧卸载，回来时读回；代价是每张
  想要这条行为的列表页各自接。

推荐 C 的检索词半（hash 本就是位置权威，检索词进地址还能收藏转发），多选集改口为「进详情即丢」并写进票 04——多选是一次批量动作的暂存，跨标签保留它的
场景尚未见到。这是形态取舍，等用户答。

## 判断项答复（2026-09-24，用户授权通道 3 自决）

用户 2026-09-24 经 IDP 队列授权（「你能帮我自决吗，你是业务和系统专家」）。定：**C 的检索词半 + 壳层记住每张标签最后停在的完整地址；多选集进详情即丢。**

- **只做 C 达不到本票自己的完成判据。** 回列表的三条路都只写地址前两段：点标签（`Layout` 的 `onTabClick` → `hashForTab`）、关掉详情落回邻居
  （`applyWorkspace` → `hashForTab`）、详情的「返回列表」（`ShipmentRequestListPage` 给 `onBack` 写死 `#/shipment-request-inquiry`）。`?q=` 在三条回程上
  都被剥掉，只有浏览器后退带得回来。所以壳层要补一件：hashchange 时按事件的 `oldURL` 记下离开那张标签的完整地址（含第三段与查询串），三条回程都落到
  记下的地址；详情的「返回列表」改经壳层给页面的口「回到列表标签」，不再写死 hash。hash 仍是位置的唯一权威——标签只记「上次停在哪」，页面照旧只从
  hash 读状态。
- **记下的地址只在会话内，不进 `localStorage`。** 票 01 评审的阻断正出在存储读回的标签 id 未校验；刷新后活动标签的地址本就在地址栏里，非活动标签回到
  模块首址，代价可接受。
- **检索词写地址用 `history.replaceState`**：每敲一个字不压一条后退记录，也不触发 hashchange 让壳层空转。键名 `q`，与 `?view=` 同层，也与目录读口契约的
  关键词同名（`admin-web-group-legal-entities/04` 裁决第 3 条）——读口支持之后原样下推。读写做成模板层的共用件，首用委托查阅与异常案件两页。
- **A 否**：同模块钻取不开新标签，等于撤掉票 01 的本意（一张队列与几个对象并开）。**B 否**：保活的非活动页仍挂着全局 hashchange 监听，会把别的标签
  的地址当成自己的——委托查阅页在后台就会被详情地址切成详情、再取一遍详情；要让每张页认得「自己是不是活动标签」，改动面比 C 大，内存与请求倒是次要。
- **多选集进详情即丢**，如实写进票 04：它是一次批量动作的暂存，今天唯一的批量动作是导出；跨标签留着，反而会把重取后已经变了的行留在选择集里。

判据补充：委托查阅「检索 → 双击进详情 → 点列表标签 / 关详情标签 / 按『返回列表』」三条回程之后，检索词都在、地址带 `?q=`；敲检索词不增加后退记录；
异常案件切到别的标签再点回来，检索词在。

## 不做

- 侧栏、工作台与命令面板导航到已开的模块时写的仍是模块首址，不走回程记忆——那是「去某模块」，不是「回某标签」；判断项答复只定了三条回程。
- 记下的地址不进 `localStorage`（判断项答复）；多选集不跟标签（票 04 改口）。
- 检索词不下推读口：目录读口还不收 `q`，支持之后原样下推（`admin-web-group-legal-entities/04` 裁决第 3 条）。

## 完成判据

- 按所选方案：委托查阅「检索 → 勾选 → 双击进详情 → 返回列表」后，方案承诺保住的状态仍在（`scripts/dom-probe.mjs` 实测，结论写票面）；没承诺保住的，
  页面注释与票 04 如实写。

## 完成记录（2026-09-24，通道 1，`main` 上直接做）

**落点**

| 笔 | 文件 | 做了什么 |
|---|---|---|
| `69f6413b` | `shell/workspace-state.ts`、`.test.ts`、`Layout.tsx`、新 `templates/tab-return-context.tsx`、`templates/index.ts`、票面 | 壳层段：`TabAddressBook` 与 `rememberTabAddress` / `addressForTab` / `hashOfUrl`（node:test 4 条）；hashchange 按 `oldURL` 记下离开的完整地址；点标签、关标签落邻居、`TabReturnProvider` 的 `returnTo` 三条回程都走 `addressForTab`；Status → in-progress |
| `ebf8b025` | 新 `templates/address-query.ts`、`.test.ts`、新 `templates/use-address-keyword.ts`、`templates/index.ts`、`ShipmentRequestListPage.tsx`、`ExceptionCasesPage.tsx` | 模板与页面段：`?q=` 读写纯函数（node:test 4 条）+ `useAddressKeyword`（写用 replaceState）；两页改用它；委托详情「返回列表」改经 `returnTo` |
| `c1b7f74b` | `templates/list-selection.ts`、`ListPageTemplate.tsx` | 选择集生命周期注释改口：离开这张标签即丢 |
| `cd4d8193` | 票面、票 04、spec | 完成记录；票 04 补记改口；spec Status 与子票表 |
| `00b7acdc` | `shell/workspace-state.ts` 等 | 票 01 评审附带一句的修复（`decodeHashSegment`）顺带删掉 `rememberTabAddress` 里成了死码的 try/catch——`tabForHash` 已是全函数 |

**完成判据**

- ✅ 三道门（钉 `ebf8b025`，WSL，Node 22.20.0，取法同票 01「评审后修复」的门）：`tsc -b --noEmit` 退 0 / `run-tests` 414 → 422 pass 0 fail（本票 +8）/
  `vite build` 成功。`go test ./internal/architecture/` 本机未跑，diff 全在 `apps/admin-web/**`，由 CI 兜。
- ✅ 探针（`scripts/dom-probe.mjs`，源 `/tmp/idp-probes/wsform06-probe.tsx` 不入库）**21 ok / 0 fail**；同一份对接票前的 `688a5ea9`（临时工作树，已拆）**11 FAIL**，
  正是下面几条。探针先证了 happy-dom 改 hash 时自己派 hashchange 且带 `oldURL`——壳层记地址读的就是它，探针全程没有手动补派。
  - 委托查阅敲检索词：地址成 `#/shipment-request-inquiry?q=%E7%94%B2+%E4%B9%99`，`history.length` 不变（判据补充「敲检索词不增加后退记录」）。
  - 进详情后三条回程——点列表标签、关掉详情标签落回左邻、按「返回列表」——地址都回到带 `?q=` 的那一个，检索框里仍是「甲 乙」。
  - 异常案件敲检索词进地址；切到委托查阅标签再点回来，地址与检索词都在；清空检索词即删键。
  - 工作区存储里没有记下的地址（不进 `localStorage`）。
- ◑ 「勾选」那半：按裁定多选集进详情即丢，不承诺保住；页面注释（`ebf8b025`、`c1b7f74b`）与票 04 Comments 如实写了。探针没有单测勾选——探针里页面读口挂起，
  表里没有行可勾；「双击进详情」同理以直接写详情地址代替（双击走既有的 `onRowOpen` → 写同一个地址）。
- ◑ 浏览器未验。

**判断项**

1. **记地址按 `oldURL`，页面写检索词时不通知壳层。** 页面写检索词用 replaceState，不触发 hashchange；壳层只在离开一张标签的那一次 hashchange 里读 `oldURL`，
   那一刻地址栏里就是它最后的样子。页面不必知道壳层在记，壳层也不必认识页面有哪些查询键（`?view=`、`?q=` 与将来的一视同仁）。
2. **回程只有三条。** 侧栏、工作台与命令面板导航到已开的模块写的是模块首址（见「不做」）。
3. **`rememberTabAddress` 用 `tabForHash` 认标签。** 与 hash 开标签同一个构造器，记下的地址认回来一定是那张标签，`applyWorkspace` 两次答成同一张的前提（票 01）
   不因回程改写而破；解不开的地址不记。
4. **关掉的标签，地址不删。** 重开（Ctrl+Shift+T）或详情「返回列表」回到一张已关的列表标签时，落回它上次停的地址；会话一结束就没了。
5. **`useAddressKeyword` 也听 hashchange。** 同一实例下地址被浏览器前进后退改了，检索框跟着地址走，页面不另存一份。

**评审**：碰共享面（`templates/*`、`shell/*`、`Layout.tsx`），按 workflow 第 5 步要一份 Spec 轴。复核单派给通道 3 后，用户令推送方自审（其他通道忙），单已撤回——
自审见下一节，**不算非作者评审**。派单时的初步自审所得：三条回程都走 `addressForTab`，`Layout` 里不再有写首址的
回程（`hashForTab` 只剩注释提及）；`TabReturnProvider` 的默认实现写首址，壳层外照旧回得去；`templates/index.ts` 只追加；两页以 `useAddressKeyword` 替掉
`useState('')`，检索框与筛选的用法不变。

**推送**：未推，见票 01「评审后修复」的推送一节。

## Comments

### 自审 ← 通道 1 · 钉 `00b7acdc`（基 `688a5ea9`，推送方自审，**不算非作者评审**）· 2026-09-24 14:3x

复核单 `task-1d27bdfe` 派给通道 3 后，用户令推送方自审（其他通道忙），单已撤回。门同票 01 的「自审」（钉 `00b7acdc`：`tsc -b --noEmit` 退 0 / `run-tests` 424 pass
0 fail / `vite build` 成功）；探针 `wsform06-probe` 在 `00b7acdc` 上重跑仍 21 ok。

**Spec** — 阻断：无。

1. **三条回程都落回记下的地址，没有第四条写首址的回程。** 全 `src` 写 `location.hash` 的地方逐个过：`applyWorkspace`（关 ×、右键关其它 / 关右侧 / 全关、Ctrl+W、
   Ctrl+Shift+T 重开都经它）与 `TabReturnProvider` 的 `returnTo` 写 `addressForTab`；`onTabClick` 经 `navigate` 写 `addressForTab`。其余都是「去某处」而不是「回某标签」：
   侧栏与工作台（`setActive` 写模块首址）、命令面板（`moduleHash` / `recentObjectHash`）、最近对象与保存视图页、委托查阅开详情（`detailHash`）、检查器快速动作、
   默认口 `firstAddressController`——与「不做」第一条一致。
2. **记下的地址不会记错标签。** 键由离开的那个地址自己经 `tabForHash` 认出；hash 是位置权威，离开时活动的就是这张。`tabForHash` 对解不开的地址答 null
   （`00b7acdc` 起是全函数），记下的地址认回来一定是这张标签，`applyWorkspace` 两次答成同一张的前提成立。
3. **`useAddressKeyword` 与别的 hashchange 不冲突。** 写地址用 replaceState，不派 hashchange——壳层不因敲字记地址，委托查阅的 `selectedId` 监听也不被触发；它自己听
   hashchange 只为跟读 `q`，与 `selectedId` 是两份独立的读。离开列表时旧实例在卸载前多读一次（`q` 为空），无害。`#/` 守卫挡住了「没有 hash 时 replaceState 改到
   search 上」那条路。
4. **完成记录属实。** 判据逐条有探针对应；判断项 1–5 与代码对得上；「不做」三条属实（命令面板写 `moduleHash` 已核）。与通道 3 的「判断项答复」无偏离：键名 `q`、
   replaceState、只在会话内、三条回程、多选集进详情即丢都照做。

非阻断（自审新记）：
1. 在已活动的列表标签上点侧栏里同一模块，写的是模块首址，检索词随之清空。这是「去某模块」的语义（「不做」第一条），不是缺陷；若操作者反映「点侧栏把检索冲掉了」，
   改法是 `setActive` 对已开的标签也走 `addressForTab`。
2. `hashWithQueryValue` 经 `URLSearchParams` 重新序列化查询串，别的键的编码可能被规整（`%20` → `+`）；读的一方（`savedViewIdFromHash`、`hashQueryValue`）都用
   `URLSearchParams` 解，语义不变。

结论：**可接受**，无阻断。

- 2026-09-24 · 进 main：`origin/main` = `9a477af9`（`3a47da8d..9a477af9` 纯 ff，本票各笔 SHA 不换；用户完成 gh 设备码授权后由推送方推）。CI run `35967167893` 七个 job
  全绿；第四道门 `go test ./internal/architecture/` 另在 `c7ae9f51` 本机实跑 ok（go1.26.8）。上文各处「未推」在这次推送之后失效。
