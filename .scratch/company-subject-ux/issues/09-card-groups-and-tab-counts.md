# 09 读出分组改成带图标标题的卡、签上带计数（照 WMS ASN 详情栏）

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 拿出 WMS「ASN 管理」页（同源的 idp-prism 工作台形态）问是否可参考；协调方评估后建议搬两样，用户裁「做吧」。参照截图 `scratchpad/ux-le2/wms-asn-reference.png`。

## 要做的

1. **读出分组改成卡**（`LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx` + `.css`）：现在是「节标题 + 间距」，改成四张带小图标标题的卡，与 WMS 详情栏「供应商与来源」「物流与预约」「备注」同形——细边框、浅底标题条（图标 + 13px 半粗标题）、卡内两列标签 / 值（标签列固定宽、≥44rem 才两列）。四张卡：**代表人与地址**（代表人、注册 / 办公 / 通信地址）、**税务与开票**（开票抬头、主抬头语言、其他语言的抬头、监管登记号小表）、**收款账户**（表格照旧，空时一行「未登记收款账户」）、**联系人**（对象页概要签里不带联系人时不画这张卡；联系人签里照旧是表）。图标用 lucide 里贴切的（UserRound / Receipt / Landmark / Contact 之类），与 `LegalEntityHeaderParts` 的图标风格一致。卡之间 12px 间距，不再用横线分节。详情栏与对象页同一份。
2. **签上带计数**（`LegalEntityDetailPanel.tsx`、`LegalEntityDetailPage.tsx`）：「资料版本」签显资料版本数（`listLegalEntityProfileRevisions` 答到的行数；页面若已为事实行读过就共用，没有就读一次，读不到不显数）；「关联」签显被引用数（`useLegalEntityReferences` 的 `referencedCount`，读不到不显）；「登记记录」签显登记记录数（修订历史行数，同样复用）。计数件用既有 `TabCountBadge`；对象页签的计数照模板 `count` 字段。
3. 资料维护提示那一块（「资料维护提示」列表）放在卡组之后，形式不变。
4. 三道门；浏览器在 1440 与 2000 两种宽度各截详情栏（SYN-LE-01 概要签）与对象页（SYN-LE-CN-SH-01），对照参照截图核「卡的边界感」与「签计数」。

## 不做

- 不加数量卡、不加全宽流程条（主数据页不需要）；不动 `templates/*`、`components/*`、`shell/*`。
- 不改列表页。

## 完成判据

- 三道门退 0；截图里四张卡边界清楚、标题带图标；「资料版本 6」「登记记录 1」「关联 6」三个签带数；对象页与详情栏同形。

## 完成记录（2026-11-03，分支 `claude-csux-09`，代码两笔 `a2f636f8e`、`538e5ab7d`）

落点：

- `LegalEntityProfileReadout.tsx`：新 `ProfileReadoutCard`——细边框、浅底标题条（`bg-idpxyz-editor`，工作区那层冷灰；不用 hover 色，浅色主题里它是选中蓝）、lucide 图标 3.5 单位弱显 + 13px 半粗标题、可选小字计数。自己用 Tailwind 排，不用 ui-primitives 的 `Card`：它带悬停投影与过渡，是给可点的卡的。
- `LegalEntityProfileContent.tsx`：四张卡——代表人与地址（`UserRound`）、税务与开票（`Receipt`）、收款账户（`Landmark`，空时一行「未登记收款账户」）、联系人（`Contact`，`includeContacts=false` 的概要签不画）；卡间 `gap-3`（12px），不再画横线分节。主抬头语言仍跟在开票抬头后弱显、不另起一格（票 08 的取舍，本票列的是卡内内容，没有推翻它）。资料维护提示留在卡组之后，形式不变。
- `legal-entity-profile-readout.css`：只改注释（容器仍是卡组外框，网格在卡内两侧各少 1rem；两列阈值 44rem 不变）。
- 新 `legal-entity-tab-counts.ts`：`revisionCountOf`（答到了且回显的是所问公司才是条数，读不到或慢答案后到都没数，0 照显）与 `useLegalEntityTabCounts`（各读一次 `listLegalEntityProfileRevisions` / `listLegalEntityRevisions`，钥匙与当前资料读口同一把）；`legal-entity-tab-counts.test.ts` 钉 `revisionCountOf` 五种情形。
- `LegalEntityDetailPanel.tsx`：资料版本 / 登记记录 / 关联三签带 `TabCountBadge`；`LegalEntityDetailPage.tsx`：三签给模板 `count`（非联系人签模板显小字，不是徽章；`templates/*` 不动）。

计数来源：资料版本 = 资料修订链的行数；登记记录 = 身份修订链的行数；关联 = `referencedCount(references.states)`，与首屏「被引用」同一份答案。页面此前没为事实行读过这两条链（事实行的「资料版本 第 N 版」来自按时点解析），所以各读一次；签里的区去到时照旧自己读。

判据：

- ✅ 三道门退 0：`tsc -b --noEmit`、`run-tests.mjs`（1168 pass，含新 5 例）、`vite build`。
- ✅ 卡边界清楚、标题带图标：截图 `scratchpad/csux09/after-list-panel-{1440,2000}.png`、`after-page-SYN-LE-CN-SH-01-{1440,2000}.png`；DOM 探针读到标题条 `代表人与地址 / 税务与开票 / 收款账户 N 个`，图标 class `lucide-user-round / lucide-receipt / lucide-landmark`；2000 宽卡内两列、1440 宽一列。
- ✅ 签带数：详情栏 SYN-LE-01 探针读到 `["概要","联系人 4","资料版本 6","登记记录 1","关联 6"]`；对象页 SYN-LE-CN-SH-01 读到 `["概要","联系人 4","资料版本 3","登记记录 1","关联 0"]`。
- ✅ 对象页与详情栏同形：同一份 `LegalEntityProfileContent`。
- ✅ 脚本断言零写请求、零 pageerror。

未验：

- 联系人卡只在「资料版本」签「查看某个日期的资料」的读出里出现（`includeContacts` 默认 true），没截图；与概要签同一份代码。
- 暗色主题下标题条底色没截。
- 资料更新落册后签上计数重读（钥匙与当前资料读口同一把）没在浏览器里走——核验脚本只读。

## 进 main 记录（2026-11-03，协调方）

- main `95724ac1f` 上 `git merge --no-ff origin/claude-csux-09`（tip `700c10772`），无冲突。三道门在 main 上退 0（tsc、run-tests 1168、vite build）。协调方看过 2000 宽的详情栏截图：四张卡、三签带数。没动共享面。
