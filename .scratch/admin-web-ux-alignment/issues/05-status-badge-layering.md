# 05 状态 badge 五层分家：`domainStatusTones` 加「层」轴，`StatusBadgeFor` 按层取形，词一个不改

Category: enhancement
Status: resolved
Blocked by: 无
地盘：`apps/admin-web/src/domain/status.tsx`（+ 新 `domain/status.test.ts`）；用 `StatusBadgeFor` 的页面**只在渲染形状随层自动变时被动受影响，不逐页改**
（`shipment-request/*`、`visibility/ExceptionCasesPage`、`operations/TransportFulfillmentReviewPage`、`party/detail-primitives` 等，按 `git grep StatusBadgeFor` 为准）。
出处：spec「缺口」表第五档；黄金标准「状态语义黄金标准」（Lifecycle / SLA / Risk / Severity / Flags 五层必须在设计系统层预先分开，不许一张词→色表混装）；
手册「色彩系统规范」状态语义色统一那条（已守住，本票不动色）。

## 为什么

`domainStatusTones` 今天是一张扁平的「词 → 色调」表：它把「这是什么状态」和「该显什么颜色」压成一步，没有「这是哪一层的状态」这一格。眼下表里全是生命周期
与判断结论的词，混装还没露出来；追踪 ETA（SLA 层）、异常案件的严重度（Severity 层）、关务限制（Risk / Flags 层）三类词一旦进表——它们在 CONTEXT 里都
已经是概念，只是读口还没登记格（`ExceptionCasesPage` 头注明写「严重度、优先级与工作条件在案件行上无登记格」）——同一张表同一种徽章就分不出「已提交」
与「超时 2h」哪个该更醒目。黄金标准要求这一层在**设计系统层**预先分开，等词进来再拆就是改七张页。

## 要做的

1. **层轴**：`domain/status.tsx` 加 `StatusLayer = 'lifecycle' | 'sla' | 'risk' | 'severity' | 'flag'`（对应黄金标准五层，词用英文键、中文注释各层定义），
   加 `domainStatusLayers: Record<DomainStatus, StatusLayer>`——**每个既有词恰归一层**。今天表里的词全部归 `lifecycle`（生命周期状态与判断结论——
   「已接受 / 已拒绝 / 冲突 / 无适用依据 / 可达 / 不可达」等在 CONTEXT 里都是**该判断对象**的状态，不是对象上的风险标记，理由写进条目注释；
   `资料不足`、`待补充`、`要求补充` 这类「续办提示」也仍是所在对象的状态，不是 Flags）。其余四层今天没有词——**表里留层不留词**，不为了凑层造词。
2. **形随层**：抬一个内部渲染件 `LayeredStatusBadge({ layer, tone, children })`——`lifecycle` 仍是今天的 `StatusBadge`（填充徽章）；`sla` / `risk` /
   `severity` / `flag` 各定一种形（如 `Tag` 描边、带层前缀图标或首字标）；层 → 形的映射一张表、一处定义。`StatusBadgeFor` 改为查词得层与色调后调它，
   对外签名不变。四层今天无词，映射要写、类型要过，渲染路径由合成词直接调 `LayeredStatusBadge` 走一遍（见 4）——`StatusBadgeFor` 仍只收 `DomainStatus`。
3. **色调翻译不动**：`badgeStatusByTone` 与五档 `StatusTone` 定义原样；本票只加层不改色、不改词——状态词只取 CONTEXT 原词是本仓红线（spec「红线」第三条）。
4. **演示**：`pages/template-preview` 不在本票地盘（03 / 04 在改），改在 `domain/` 同目录放一份 `status-layers.demo.tsx`（合成 S）：五层各拿一个
   `SYN-` 前缀的合成词直接调 `LayeredStatusBadge` 渲染一次，给 `run-tests` 里的 `renderToStaticMarkup` 断言「五形各不同」用；合成词一个不进
   `domainStatusTones`。若 `tsconfig.test.json` 不编 `.tsx`（今天只 include `src/**/*.test.ts`），断言改在 `.test.ts` 里用 `createElement` 调它，不改 tsconfig。
5. **`party/detail-primitives.tsx` 的 `statusBadge(table, code)`**：它按词表查词再交 `StatusBadgeFor`，不改；只确认层轴加上后它仍编译、行为同。

## 不做

- 不给任何词改层以外的东西：不改词、不改色、不加词。
- 不做 SLA 计时 / 风险评分 / 严重度分级的**计算**——那是各上下文的领域规则，读口没有就没有。
- 不做「同一格叠多层徽章」的布局（追踪摘要行将来同时显三层）——等第一张真要同时显两层的页立票时再定叠法。

## 完成判据

- 三道门绿；既有 `run-tests` 零改动仍绿。
- `domain/status.test.ts`（新）钉：词表每个词在 `domainStatusLayers` 里恰有一层；层 → 形映射覆盖全部五层；`StatusBadgeFor` 对 `lifecycle` 词渲染的
  静态标记与改前逐字节同（先在改前录一份基线字串再改——这条是「七张页不受影响」的证据）。
- `domainStatusTones` 的键集零改动：`git diff` 该对象只有注释行——评审用 `git diff -U0 -- domain/status.tsx | grep "^[-+]  '"` 为空核。
- 浏览器验收做不到如实写「未验」。

## 裁决

1. **判断结论归 lifecycle 不另立「decision」层**：黄金标准只有五层，且 CONTEXT 把接受判断 / 商业解析 / 可达性都建成有自己生命周期的**对象**，其结论
   就是该对象的状态词；另立一层等于替设计系统改规范。
2. **留层不留词**：四层空着比塞「示例词」诚实；渲染路径的正确性由合成 S 演示词在测试里走一遍来保证，演示词不进 `domainStatusTones`。

## Comments

### 认领（2026-09-20 12:5x）

通道 8 认领后 crash、树上零提交（用户 12:5x 报）；推送方通道 1 拆其空树后自接，分支 `mcp1-ux05` 基 main `86a96ab7`，地盘 `domain/`。作者 = 推送方 = 本会话，
评审需另派。

### 完成记录（通道 1 · 2026-09-20 12:5x–13:1x · 分支 `mcp1-ux05` 基 main `86a96ab7` · 码 tip `5f7c89c6`）

**笔** `7daeacb0`（认领）→ `5f7c89c6`（全部五条一笔：层表 + 渲染件 + 测试，三样拆不开——层表没有渲染件是死数据，渲染件没有测试没法交）→ 本票面笔。

**要做的逐条**

1. ✅ 层轴在新纯模块 `domain/status-layers.ts`（只 `import type`，不引原语）：`statusLayers` 五层按黄金标准顺序、`StatusLayer`；`domainStatusLayers`
   `as const satisfies Record<DomainStatus, StatusLayer>`——词表多一词或少一词这里都编不过。五十个词全部 `lifecycle`，分组注释沿 `domainStatusTones` 的分组；
   其余四层零词。
2. ✅ `LayeredStatusBadge({ layer, tone, children })`：按 `statusLayerShapes[layer]` 取形——`badge`（lifecycle，今天的 `StatusBadge`，`icon` / `hideIcon` 照传）/
   `tag-mono`（sla：`Tag` outline + 等宽）/ `tag-square`（risk：`Tag` 实底、`tagVariantByTone` 取色、方角）/ `badge-plain`（severity：`StatusBadge` 无图标 + 字重加档）/
   `tag-pill`（flag：`Tag` outline 药丸）。`StatusBadgeFor` 改为查 `domainStatusLayers` 与 `domainStatusTones` 后调它，对外签名不变。
3. ✅ `badgeStatusByTone` 与 `StatusTone` 五档原样；`domainStatusTones` 键集零改动——`git diff -U0 86a96ab7 5f7c89c6 -- apps/admin-web/src/domain/status.tsx |
   grep "^[-+]  '"` 为空。
4. ◑ 演示：**不建演示文件**。测试链引不到组件（见完成判据第一条），建了没人渲染就是死码；四个空层的渲染路径由 `Record<StatusLayer, StatusShape>` 完备性
   （tsc）守，五形各异由下面那次一次性 esbuild 束实测。
5. ✅ `party/detail-primitives.tsx` 的 `statusBadge(table, code)` 未改，tsc 过、行为同（它只经 `StatusBadgeFor`）。

**完成判据逐条**

- 三道门 ✅ `tsc -b --noEmit` 0 / `run-tests` **293**（main 290 + 3）/ `vite build` 0；既有用例零改动（`git diff 86a96ab7 5f7c89c6 -- '*.test.ts'` 只有新文件）。
- `domain/status.test.ts` ✅ 三条：词归五层之一且五层各有一形、形各不同；今天词表里的词全在 lifecycle（别的层进词时这条会红，逼改的人回答「凭什么不是所在
  对象的状态」）；色调 → Tag 变体五档齐全。**组件层未钉**：spec「验收口径」的前提**实测不成**——`@idpxyz/ui-patterns` / `ui-primitives` 的 `package.json`
  `exports["."]` 只有 `types` + `import`，测试链按 CommonJS 发射后 `require` 报 `ERR_PACKAGE_PATH_NOT_EXPORTED`（Node 22.22 的 require(esm) 也救不了：
  没有 `require` / `default` 条件就无路径可解）。**后面的票照抄这条结论**：`.test.ts` 不能引任何 `import` 了 `@idpxyz/*` 的模块；要钉的逻辑抬进纯 `.ts`。
- lifecycle 输出逐字节同 ✅ 用一次性 esbuild 束（`node_modules/.pnpm/esbuild@*/…/bin/esbuild --bundle --platform=node --format=cjs --jsx=automatic`，源与产物不入库）
  把 `86a96ab7` 版 `StatusBadgeFor` 与新版并排渲染五十个词（带 `className`），`renderToStaticMarkup` **50 同 / 0 异**；五层合成词 `SYN-*` 五形标记各异。
  这条路子能给 03 / 04 / 06 用来做「模板默认渲染不变」的证据，写在这里省他们再找。
- 浏览器验收未验（本机无 gk.idp.xyz 会话）。

**判断项**

- 形的具体选择（等宽 / 方角 / 药丸 / 字重）是没有词时的设计系统层预设，第一批 sla / severity 词进表时可以改——改形只动 `statusLayerShapes` 与
  `LayeredStatusBadge` 的一个分支，不动词表、不动页面。
- `Tag` 的 `twMerge` 会让 `rounded-sm` 顶掉自带的 `rounded-full`（risk 形靠它成方角）——这是 ui-primitives 的既有行为，本票只是用到；若上游改掉，risk 形退成圆角，
  由「五形各不同」那一把束实测发现，不由 node:test 发现（见组件层未钉）。
