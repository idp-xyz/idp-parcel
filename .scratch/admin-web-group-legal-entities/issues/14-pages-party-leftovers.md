# 14 `pages/party` 收口尾巴：票 13 评审判断项里的死分派表、四份同形修订建议、其它模块的 `chipClass` 副本

Category: chore
Status: resolved
Blocked by: 无（13 已进 main，远端 main `090e250e`）
地盘：`apps/admin-web/src/pages/party/`（`identity-deactivation-form.ts` + test、`IdentityDeactivationForm.tsx`、四份 `*-form.ts` + test、
`registration-form.ts` + test）；第 3 条若做，`apps/admin-web/src/pages/{channel-selection,customs,network,operations,visibility}/` 里
只改 `chipClass` 那一行导入。
出处：票 13 评审 ← 通道 6（Standards N2 / N3、Spec S1）+ 票 13 完成记录判断项（`suggested*Revision` 四份同形、其它模块十一处 `chipClass`）。
全部「无阻断、判断项」。

## 为什么

票 13 收掉了 09 / 10 / 12 点过的副本，但它自己的完成判据「既有用例零改动」让两样东西留了下来：一张只剩测试在调的分派表，和一族只差
取键字段的「最新修订加一」。它们不挡任何事，但下一个改「停用口种类」或「修订建议」的人会先撞见死代码再找到活的那份。`chipClass` 抬到
`components/` 之后其它模块的十一处副本已经能直接改导入，不改就是十二份里只收了三份。

## 要做的（四条，每条独立成笔）

1. `identity-deactivation-form.ts`：`deactivationTargetsOf` / `deactivationTargetsByKind` / `IdentityRegisters` 生产侧无调用方（票 13 第 2 条后组件走
   `kindRegisters[kind].targetsOf`）。删掉，测试改钉 `identityTargetOf`（投影一份）与 `kindRegisters` 的键集 = `identityKindLabels` 键集（13 N2、S1）。
2. `suggestedRevision` / `suggestedBusinessPartyRevision` / `suggestedDeactivationRevision` / `suggestedPartyRelationshipRevision` 折成
   `registration-form.ts` 的一份 `suggestedNextRevision(rows, id, keyOf)`；各册保住导出名作薄包装（既有用例零改动）。法人那份多一道 `trim`
   ——**保住**，放在包装里，不进共用体（理由见 13 第 3 条不做那半）。
3. 其它模块十一处 `chipClass`（钉 `090e250e`：`channel-selection` 1、`customs` 1、`network` 2、`operations` 3、`visibility` 4）改从 `components/registration`
   导入，私有副本删；字串逐字同才改，不同的那份留着并在票面记为什么不同。
4. `IdentityDeactivationForm.tsx` 的 `kindRegister<Body>()` 擦 Body 为 unknown（13 N3）：只判要不要改成按 kind 收窄的判别联合；判「不改」写理由即可。

## 不做

- 不动 `legal-entity-form.ts` 的载荷裁空白（票 02 验收过、`legal-entity-form.test.ts` 钉着）：要统一「不裁空白」判据先改 02 的判据与用例，
  那是行为票不是收口票——立票前先问 owner 要不要统一。
- 不改任何页面文案；不改服务端。

## 完成判据

- 每笔：`tsc -b --noEmit` 0 / `run-tests` 既有用例零改动仍绿（第 1 条例外：删死代码时同笔改指新件的那几条用例，票面列名）/ `vite build` 0。
- 完成后 `apps/admin-web/src` 下 `const chipClass` 零命中（只剩 `components/registration/chip.ts` 的 `export const`）；`pages/party` 下
  「最新修订加一」的实现只剩 `suggestedNextRevision` 一处——`git grep` 自己数，票面写钉的 SHA。

## Comments

### 认领（2026-09-20 11:5x）

第 1 / 2 / 4 条 ← 通道 5，分支 `mcp5-adminweb14a` 基 main `e16497de`，地盘 `pages/party`；第 3 条 ← 通道 4 另一支（其它模块页面），完工由推送方代落。票面只通道 5 写。

### 完成记录·第 1 / 2 / 4 条（通道 5 · 2026-09-20 11:5x–12:3x · 分支 `mcp5-adminweb14a` 基 main `e16497de` · tip `baabfb1b`）

**笔** `fd1a47ee`（认领）→ `d29b984b`（1）→ `3877f60f`（2）→ `baabfb1b`（4）→ 本票面笔。每笔 push origin；不推 main、不占 55432、不跑 Go。

**要做的逐条**

1. ✅ `identity-deactivation-form.ts` 删 `deactivationTargetsOf` / `deactivationTargetsByKind` / `IdentityRegisters`；`git grep` 三符号在 `apps/admin-web/src`
   零命中（钉 `d29b984b`）。用例改动（本条票面允许、这里列名）：`identity-deactivation-form.test.ts` 的「按种类投影对应册」→「按种类把对应册的一行投成
   「标识 + 修订」」，直接钉 `identityTargetOf` 三册取键；「分派表与种类词表同键，集外不认」保留，仍钉 `identityTargetOf` 键集 = `identityKindLabels` 键集。
   **`kindRegisters` 的键集那半是 tsc 钉、不是 node:test 钉**：它住在组件 `IdentityDeactivationForm.tsx`，声明为 `{ [K in IdentityKind]: KindRegister<K> }`
   （少一格编不过、多一格是多余属性）；组件模块不进 `tsconfig.test.json` 的编译、仓内也没有任何 `*.test.ts` 引 `.tsx`，为一个与注解重言的检查把 React 拖进
   测试编译不值——评审若要 node:test 钉，可议把表抬进纯模块，但 `optionsOf` 那几件今天在 `party-registration-fields.tsx`，得连它们一起搬，超本票。`d29b984b`。
2. ✅ `registration-form.ts` 加 `suggestedNextRevision(rows, id, keyOf)`（`Row extends { revision: number }`，键按 `keyOf` 取、与 `id` 逐字比、`null` / 空数组 /
   空标识皆 1）；`suggestedRevision` / `suggestedBusinessPartyRevision` / `suggestedDeactivationRevision` / `suggestedPartyRelationshipRevision` 导出名与签名不动，
   各只交本册取键字段；法人那份 `legalEntityId.trim()` 保住、在包装里裁完再交进共用体（13 第 3 条那半不做的理由照旧：裁不裁是各册载荷判据，要统一先改 02）。
   `registration-form.test.ts` 新增一条「建议修订号取该标识最新修订加一，键由调用方取」（用 `{ code, revision }` 形状的行，证明取键字段与册无关）。
   顺手：`IdentityDeactivationForm.tsx` 格下句「建议停用落点 r{n}」改显 `form.revisionField.suggestion`（同一份 rows、同一标识算出，`known` 在时两者恒等，
   文案逐字同），不再自己 `+ 1`。`3877f60f`。
3. ⏳ 通道 4 另一支；完工由推送方代落。
4. ✅ **判「改」，已做进本条。** 理由不止类型：`kindRegister<Body>()` 擦成 unknown 靠方法形参双变过编译（13 N3），而它藏着一个真缺陷——换种类那一帧，
   `patch({ kind })` 触发的渲染先于 effect 的 `setLoaded(null)`，`loaded` 还是上一册的答案，`kindRegisters[新种类].targetsOf(旧册 body)` 会在
   `body.accounts.map` 一类取键处抛（三册体形键名各异 `parties` / `entities` / `accounts`；`LEGAL_ENTITY` ↔ `CUSTOMER_ACCOUNT` 互切即触发，
   `targetsFor` 在 `suggestion` 回调里被调、参数先于 `id === ''` 短路求值）。按 React 时序推得；**本机无浏览器验收路径，未复现**（与 13 同口径）。
   改法：`RegisterBodyOf` 一处说种类 → 体形；`KindRegister<K>` 按种类收窄，`load()` / `withAnswer()` 由册自己结成 `RegisterAnswer<K>`；状态存
   `AnyRegisterAnswer`；`selectedRegisterFor` 只在 `loaded.register.kind` 对上时用它，否则当这一册没取到；`targetsOf<K>` / `RegisterPicker<K>` 收整对，配对由同一个
   K 钉。**负向验证**：临时写入 `kindRegisters.LEGAL_ENTITY.withAnswer(参与方答案)` 与 `{ register: kindRegisters.CUSTOMER_ACCOUNT, answer: 参与方答案 }`，
   tsc 各报 TS2345 / TS2322，随后撤掉。不改页面文案；effect 依赖与重读时机不变；不加 Go、不加运行时用例（组件不进 node:test，见第 1 条）。`baabfb1b`。

**完成判据（本半）**

- 三道门 ✅ 每笔提交信带当笔数字；tip `baabfb1b`：`tsc -b --noEmit` 0 / `run-tests` **290**（main `e16497de` 289 + 1：registration-form 1；第 1 条一替一）/
  `vite build` 0。既有用例零改动：`git diff e16497de..baabfb1b -- '*.test.ts'` 删除行只在 `identity-deactivation-form.test.ts`（第 1 条票面点名的那一条用例
  及其 import）与 `registration-form.test.ts` 的 import 一行。
- 「最新修订加一」只剩一处 ✅（钉 `baabfb1b`，`git grep -n -E "revision \+ 1" -- apps/admin-web/src`）：实现只有 `registration-form.ts` 的
  `suggestedNextRevision`；另一命中是 `registration-form.test.ts` 的 Covers 注释。`export function suggested*Revision` 五处 = 共用体一 + 四份薄包装。
- `const chipClass` ⏳ 钉 `baabfb1b` 十二命中：`components/registration/chip.ts` 的 `export const` 一 + 其它模块页面十一（`channel-selection` 1、`customs` 1、
  `network` 2、`operations` 3、`visibility` 4，与票面钉 `090e250e` 的数同）；这十一处是第 3 条，由通道 4 报。
- 浏览器验收未验（同 13 口径）。

### 完成记录·第 3 条（通道 4 · 2026-09-20 11:5x–12:0x · 分支 `mcp4-adminweb14b` 基 main `e16497de` · tip `3b26cdfe`；推送方按其 `report_task` 代落）

3. ✅ 一笔 `3b26cdfe`，11 件 +11/−82：`channel-selection/ChannelSelectionDecisionsPage`、`customs/ComplianceRulesPage`、`network/NetworkCatalogPage` +
   `RoutePlansPage`、`operations/EffectiveTimeJudgmentPage` + `NodeOperationsReviewPage` + `TransportFulfillmentReviewPage`、`visibility/ClaimPrerequisitesPage` +
   `DisclosurePoliciesPage` + `ExceptionTriagePage` + `TrackingJudgmentRulesPage`——十一处私有 `const chipClass` 删、改 `import { chipClass } from
   '../../components/registration'`（五页已有 registration 导入行的只补名，六页新增一行）。**留存：无**——钉 `e16497de` 脚本逐块比对，十一处与 `chip.ts` 导出体
   逐字同。钉 `3b26cdfe`：`git grep -n "const chipClass" -- apps/admin-web/src` 命中 1（仅 `chip.ts:6` 的 `export const`），`pages` 下零。三道门 `tsc` 0 /
   `run-tests` 289（基线 289，用例零改动）/ `vite build` 0。未碰 `pages/party`、`components/registration`、票面。推送方另核：`git diff e16497de 3b26cdfe`
   除导入行外只有十一段同字模板串的删除。

**完成判据（合两支，钉进 main 的 tip `3164b7d1`）**：`const chipClass` 只剩 `components/registration/chip.ts` 一处 ✅；「最新修订加一」实现只剩
`suggestedNextRevision` 一处 ✅；`tsc` 0 / `run-tests` 290 / `vite build` 0 ✅。

### 评审 ← 通道 6 · 两支一并 · 14a 钉 `8ad23acf`（码 `baabfb1b`）/ 14b 钉 `3b26cdfe` · 基线 `e16497de` · 12:1x–12:2x（推送方按其两条消息代落）

（`task-13121320`；隔离树 `%TEMP%\idp-review-14`，两轴串行；三道门未复跑，以推送方 idp-land14 实跑为准。）

- **Standards（阻断 0 / 非阻断 4）**：**N1** 变更说明式注释——`identity-deactivation-form.ts` `identityTargetOf` 文档注「纯模块此前还留过一份…票 14 第 1 条删去」与
  `identity-deactivation-form.test.ts`「此前…删了」叙的是一次删除，git log 已载，留「种类 → 取哪一册住在组件 kindRegisters」那半即可；`KindRegister` 头注的
  「此前…真的错过」是配对设计的理由，不算。**N2**（判断项）第 4 条设计量——核心（答案与册结对 + `selectedRegisterFor` 种类不对当未取到）由真缺陷撑住、也确实
  撤了 13 N3 的擦型；但 `withAnswer` + `load()` 改交配对、`kindRegister` 里 `self` 自引，比「状态存 `{ kind, answer }` + 一个泛型 `pairFor`」重一层；三册而言可
  接受，不必回改。**N3**（判断项）同文件自由函数 `targetsOf<K>(paired)` 与 `KindRegister.targetsOf(body)` 同名异型；改 `pairedTargetsOf` 或内联。**N4** 14b：
  `chip.ts` 住 `components/registration/` 却被 route-plans / node-operations 等非登记页导入，位置名不副实；票面指定从此导入，不算本票发现，归后续上提 `components/`。
  无发现：注释全中文、无行号、无跨文件活计数（`RegisterBodyOf`「三册」同文件枚举）；第 2 条四份包装与原实现逐式同（`id === '' || rows === null` 先判、find 取首、
  +1），法人 `trim` 在包装里、共用体逐字比（新测 `'A '` → 1 钉住）；第 4 条 effect 依赖 `[selectedKind, reloadKey]` 与 `setLoaded(null)` 先行不变，`key` 同值，
  文案未动；格下句改显 `form.revisionField.suggestion` 与 `known` 同一 `targetsFor(draft.kind)`、同一 `find`，`known` 在时恒为 `known.revision + 1`，恒等 ✓。
  14b 十一处删的字串逐字同，只动导入。
- **Spec（阻断 0 / 非阻断 2）**：**S1** 第 4 条修的那次渲染期抛没有自动化钉——守门 `selectedRegisterFor`（`loaded?.register.kind === kind`）住 .tsx，作者已如实记
  「不加运行时用例」；真要钉得把这一比较抬进纯模块，超本票，只记。**S2** 第 1 条旧用例「对应册没取到 → null，不拿别的册冒充」那半随删除消失，这正是第 4 条出事
  的规则；与 S1 同源，完成记录未点出这层关联。逐条 ✓：1 三符号 grep 零、用例改钉 `identityTargetOf` 三册取键、票面点名的那一条例外；2 `suggestedNextRevision`
  一份、四包装导出名与签名不动、法人 trim 在包装、新测一条；3（14b）十一件 +11/−82、删的六行逐字同、无其它改动；4 判「改」理由成立——**真缺陷成立**：13 版
  `suggestion: (draft) => suggestedDeactivationRevision(targetsFor(draft.kind), draft.id)` 实参先求值，种类下拉 `patch({ kind, id: '' })` 触发的渲染在 effect
  `setLoaded(null)` 之前，`loaded` 仍为旧册 outcome，`kindRegisters[新种类].targetsOf(旧 body)` 在 `body.accounts.map` / `body.entities.map` 抛 TypeError
  （`LEGAL_ENTITY` ↔ `CUSTOMER_ACCOUNT` 互切且上一册已答 outcome；`BUSINESS_PARTY` 方向 `loaded` 为 null 不触）；13 之前三册各有槽位不会错配，**是 13
  （`92ca5332`）引入的回归**；修法只改类型与取答案守门，文案 / effect 依赖 / key 不变 ✓。完成判据：`*.test.ts` 删除行仅那一条 + 两处 import ✓；`revision + 1`
  实现只 `registration-form.ts` 一处 ✓；`const chipClass` @`3b26cdfe` 只 chip.ts、@`baabfb1b` 12 与记录同 ✓。判断项「kindRegisters 键集是 tsc 钉」成立——
  `tsconfig.test.json` include 只 `src/**/*.test.ts`，仓内无 `.test.ts` 引 `.tsx`，而 `tsc -b` 编组件，少键报错、多键多余属性 ✓。无票外行为；「不改页面文案」
  成立；两支地盘无交。
- **一行**：Standards 0 / 4 · Spec 0 / 2。无阻断。

**处置（推送方）**：N1 + N3 在 idp-land14 tip 上改成一笔 `3164b7d1`（两处注释收成现状句、`targetsOf<K>(paired)` → `pairedTargetsOf`，调用一处随改；tsc 0 /
290 / vite 0，自审）；N2 / S1 / S2 只记——S1 / S2 说的同一件事：「种类对不上当没取到」这条规则今天只有 tsc 与 `selectedRegisterFor` 的一行守着，要 node:test
钉得把配对比较抬进纯模块，留作下一张收口票的候选（不立票，等它再被撞到）；N4 `chip.ts` 上提 `components/` 同为候选。

### 进 main 记录（推送方 = 通道 1 · 2026-09-20 12:24）

隔离树 `%TEMP%\idp-land14` detached 于 `c0b29402`（= 当时远端 main），先 14b 再 14a，六笔 cherry-pick 零冲突 `3b26cdfe→f1d6da55`；`fd1a47ee→b83fcf9c` /
`d29b984b→22d45537` / `3877f60f→4aeca5bb` / `baabfb1b→b043565f` / `8ad23acf→b485379a`（`patch-id --stable` 逐笔同；两支改过的文件对各自作者 tip 零 diff），
其上推送方 `3164b7d1`（评审 N1 / N3）。tip 上清点重生成 porcelain 空、`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；admin-web 全新 `pnpm install
--frozen-lockfile` 后 `tsc -b --noEmit` 0 / `run-tests` **290** / `vite build` 0；`const chipClass` 一处、`revision + 1` 实现一处；12:22 占号 → 带 DSN
`go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（12:22:17→12:24:39；本票不动 Go）→ 评审无阻断（上）→ `ls-remote` 核
`c0b29402` 未动 → 12:24 `push 3164b7d1:main` 成，**远端 main = `3164b7d1`**；共享树 ff 同 SHA。Status → resolved（本簿记笔）。
