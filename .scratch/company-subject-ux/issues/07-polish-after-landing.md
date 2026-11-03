# 07 收尾抛光：空的概要卡、「关联」签显名称、头部注册号胶囊截断

Category: enhancement
Status: in-progress
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
