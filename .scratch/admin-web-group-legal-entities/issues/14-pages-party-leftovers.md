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
