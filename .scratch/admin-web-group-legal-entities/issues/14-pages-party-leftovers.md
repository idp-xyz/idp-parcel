# 14 `pages/party` 收口尾巴：票 13 评审判断项里的死分派表、四份同形修订建议、其它模块的 `chipClass` 副本

Category: chore
Status: in-progress
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
