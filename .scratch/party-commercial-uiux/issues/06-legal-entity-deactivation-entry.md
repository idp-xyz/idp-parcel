# 06 — 法人停用入口与确认

**Parent:** [spec](../spec.md)

**What to build:** 法人详情栏「更多」菜单里有一项「到业务参与方页停用 SYN-PARTY-…」（`LegalEntityHeaderParts` 的 `LegalEntityMoreMenu`）。在法人页上点名的却是参与方标识，点了只落到参与方列表（按参与方标识检索过），不是停用表单；停用表单在参与方页「登记」签里，身份种类与标识都要自己再选。读的人很可能以为要去停用那个业务参与方。

停用表单本身（`IdentityDeactivationForm`）的「提交停用」与普通提交同款，没有确认；说明只写「自停用时点起该身份不再支持新的商业决定」，没说停用是终态、不能撤回（PC CONTEXT 里「已停用」是终态）。

改法：

- 菜单项改名「停用此责任法人…」，直达停用表单，预填身份种类「责任法人」与这个法人的标识。
- 「提交停用」用危险样式，提交前弹确认框，复述：身份种类、名称与标识、停用时点（本地时间与 UTC）、停用依据，以及「停用后不能撤回；要恢复须登记新的身份」。
- 确认框照仓里已有的用法（委托撤回、取消用的 `ConfirmDialog`）。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 法人页菜单项名称点的是法人，点了落在停用表单，种类与标识已预填。
- [x] 提交停用前有确认框，复述上面五项；取消不发请求。
- [x] 「提交停用」是危险样式。
- [x] 预填与确认文案的组装是纯函数，带 node:test 用例。
- [x] 三道门退 0。

## 完成记录（2026-10-26，tops-54，前端切片，worktree `idp-parcel-pcux`）

- **落点：** 本笔提交。
- **改了什么：**
  - 新纯函数 `business-parties-address.ts`：`deactivationLinkHash` 生成 `#/business-parties?deactivate=<种类>&id=<标识>`；`deactivationPresetOf` 读回（种类不在封闭集或标识为空当没带）；`withoutDeactivationPreset` 用过后去掉这两键、别的键（检索词）照留。只带种类与标识：修订号、依据、时点要停用的人自己核过再填。
  - 法人详情「更多」菜单：「到业务参与方页停用 SYN-PARTY-…」改为「停用此责任法人…」，深链预填种类「责任法人」与这个法人的标识；不再点名它所钉的业务参与方。
  - 业务参与方页：页签改受控；首次挂载与 `hashchange` 时读预填，切到「登记 › 身份停用」交给停用表单，然后用 `history.replaceState` 从地址里去掉，换标签回来不会再预填一次。只动 `BusinessPartiesPage` 与 `RegistrationTab` 两个函数，没碰关系抽屉（ps-external-mark-relations/17 在那里加撤销、到期）。
  - 停用表单：收 `preset`，换一份预填才再填（按「种类:标识」为键）；「提交停用」改用 `dangerActionClass`，点了先开 `ConfirmDialog`（danger 语气，照委托撤回的用法），确认才发请求；确认框文案由纯函数 `deactivationConfirmation` 组装，复述种类、身份（下拉选项文字「名称 · 标识 · 当前状态」，手填时只报标识）、停用时点（本地与 UTC）、依据、修订号，并说清「停用后不能撤回；要恢复，只能登记一个新的身份」；用的是表单真正要送的那一份（修订号未改时带建议值）。
  - 停用表单说明、候选提示、修订号提示改成白话，去掉端点；JSON 折叠区标题「（受控批量口 parcel-commercial 的在线镜像）」去掉（四张登记表单共用这一处）。
- **判据：**
  - ✅ 演示环境：法人详情「更多」→「停用此责任法人…」落在业务参与方页「登记 › 身份停用」，种类「责任法人」、身份「合成运营方一号（演示） · SYN-LE-01 · 已生效」已选，修订号建议 r2，地址回到 `#/business-parties`。从地址栏直接输入深链同样。
  - ✅ 填依据与时点后点「提交停用」：确认框复述「责任法人 | 合成运营方一号（演示） · SYN-LE-01 · 已生效 | 2026-11-01 09:30:00 UTC+8（UTC 2026-11-01T01:30:00Z） | SYN-DEREG-DEMO-01 | r2」；点「不停用」后对话框关闭，资源计时里没有任何停用请求。没有点「停用」：会写演示库。
  - ✅ 「提交停用」为危险样式（实底红）。
  - ✅ 纯函数用例：`business-parties-address.test.ts` 4 条，`identity-deactivation-form.test.ts` 新增 3 条（复述齐全、未填照实说、手填只报标识）。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 619/619，`vite build` 退 0。
- **未验：** 确认后真发停用的那一路没走（会写演示库）；确认只是在原 `form.send` 前加一道，发送与落地判据未改。
- **评审：** 改动全在 `pages/**`，自查判据。
