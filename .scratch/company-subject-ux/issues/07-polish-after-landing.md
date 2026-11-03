# 07 收尾抛光：空的概要卡、「关联」签显名称、头部注册号胶囊截断

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无（01–04 已进 main `c61937b3f`）
出处：协调方在 main 上复核四票合并后的结果（截图 `scratchpad/ux-le2/`）。

## 要做的

1. **对象页空的「概要」卡**（`LegalEntityDetailPage.tsx`）：撤掉「对应的往来单位」「停用的来源单据」后，概要卡只剩一行「技术详情」。把「技术详情」移到当前资料读出区块的底部（与详情栏概要签同位），已停用时「停用来源单据」一行放在停用横幅下；`basicFields` 传空数组或不传，看模板哪种不画空卡（不改模板）。
2. **「关联」签显名称**（`LegalEntityReferencesSection.tsx`、`legal-entity-references.ts`）：行上的供应商、客户、合同等今天只显标识（`SYN-PARTY-CARRIER-LM-01`）。供应商与客户经往来单位册（`listBusinessParties`，页面若已取过就共用）译成名称，标识进括注或 `title`；译不到照显标识，不报错。版本标识 `SYN-SUPPLIER-LASTMILE-SG-01@v1` 照旧为主键显示（它是对象名），但把 `@v1` 拆成「第 1 版」放状态旁。判读纯函数钉 node:test。
3. **头部注册号胶囊截断**（`LegalEntityDetailPanel.tsx`）：类型名长时（演示目录「模拟注册号（仅演示，非真实证照格式）」）胶囊截断成「…SYN-CN-REG…」，号反而看不见。胶囊里号在前、类型名在后并允许换行，或类型名进 `title`、胶囊只显号——二选一，与对象页头部那一行同一种。
4. 用语照 spec 表；三道门；浏览器只读核验。

## 不做

- 不动 `templates/*`、`components/*`；不动票 06 的落点（`LegalEntityProfile*`、`legal-entity-profile*.ts`）。

## 完成判据

- 三道门退 0；浏览器核：对象页没有只剩技术详情的空卡；「关联」签供应商行显「合成末端承运方（演示）」之类名称；详情栏头部注册号完整可见。

## 完成记录（2026-11-02，分支 `claude-csux-07`，起点 main `c01b08a8c`）

**落点**

- 第 1 条 `a6241313b`：`LegalEntityDetailPage.tsx`——撤掉 `buildBasicFields`，`basicFields` 传空数组（分签形态下模板按有没有字段判要不要画卡，空的不画）；技术详情跟在 `LegalEntityProfileSummary` 之后（与详情栏概要签同位）；已停用时「停用来源单据」一行（`DetailRow`）放在「需要处理」卡里停用横幅之下。
- 第 2 条 `ca4a71a55`：`legal-entity-references.ts` 加 `partyNameOfAnswer`（往来单位册答到且名非空才译，否则 `undefined`）与 `versionOrdinalText`（`v1` → 「第 1 版」，取词走 `updateOrdinalLabel`；不是 `v` 加数字的照显原串），两者钉在 `legal-entity-references.test.ts`；`LegalEntityReferencesSection.tsx` 的 `useLegalEntityReferences` 第五本读 `listBusinessParties`（不占段状态、不给重试、不进「被引用」计数），`LegalEntityReferences` 多一格 `partyNameOf`；供应商与客户行走 `PartyRef`（名称为主、标识括注；译不到只显标识）；`VersionHead` 主键只留对象标识，「第 N 版」放状态旁，版本标识原串进 `title`。
- 第 3 条 `ebc3df576`：二选一取「类型名进 `title`、胶囊只显号」。`LegalEntityDetailPanel.tsx` 注册号一号一枚 `RefChip`，`value` 是号、`title` 是类型名（复制到手的也只是号）；`LegalEntityDetailPage.tsx` 的 `buildMeta` 同一种（号上 `title` 类型名，多号中文分号）。落点之外动了 `legal-entity-identity.ts`：`IdentityLayerCells` 加 `numberCells`（逐号拆开，与 `numbers` 同一条译名规则，抽成 `lifetimeNumberCells`），`legal-entity-identity.test.ts` 跟上；其余读 `identityLayerCellsOf` 的地方只用 `country` / `numbers`，不受影响。
- 用语照 spec 表：「停用来源单据」「第 N 版」「供应商」「客户」；没有新词。

**判据**

- ✅ 三道门退 0（`tsc -b --noEmit`、`run-tests.mjs` 1159 过 0 败、`vite build`），每笔各跑一次，收尾 `git merge origin/main` 为 Already up to date。
- ✅ 对象页 `#/group-legal-entities/SYN-LE-01` 概要签没有「概要」卡（DOM 里 card-title 为空集）；`技术详情` 的 `<summary>` 在 `section[aria-label="当前有效公司资料"]` 之后。截图 `scratchpad/csux07/object-summary.png`。
- ✅ 「关联」签供应商行显「合成末端承运方（演示）（SYN-PARTY-CARRIER-LM-01）」、客户行显「合成货主一号（演示）（SYN-PARTY-SHIPPER-01）」；行首「SYN-SUPPLIER-TRUNK-CN-SG-01  第 3 版  已生效」，正文里不再有 `@v1`。截图 `object-related.png`。
- ✅ 列表第一行 SYN-LE-CN-SZ-01 的详情栏头部胶囊文本「注册号 SYN-CN-REG-0102」、`title`「模拟注册号（仅演示，非真实证照格式）：SYN-CN-REG-0102（点击复制）」，`scrollWidth` 不超 `clientWidth`（未截断）；对象页头部同一行 `<span title="模拟注册号（仅演示，非真实证照格式）">SYN-CN-REG-0102</span>`。截图 `panel-only.png`、`object-header-SYN-LE-CN-SZ-01.png`。
- ✅ 核验脚本断言零写请求（`WRITE REQUESTS: 0`）；vite 自起 5207、验完已停。

**未验**

- 已停用公司主体的「停用来源单据」一行放在横幅下（演示数据里 SYN-LE-01 与列表首行都未停用），只过 `tsc` 与 build，未在浏览器里看。
- 多个注册号的公司主体（一号一枚胶囊并排）演示数据里没有，只有单号实例；多号拆分钉在 `legal-entity-identity.test.ts`。
- 票 06 稍后改 `LegalEntityProfile*`，本票未碰；两边无共同文件。

## 进 main 记录（2026-11-03，协调方）

- main `c01b08a8c` 上 `git merge --no-ff origin/claude-csux-07`（tip `02c61e526`），无冲突；合并提交 `d946e2ea7`。三道门在 main 上退 0（tsc、run-tests 1159、vite build）。没动共享面，不需评审。
