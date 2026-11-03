# 09 读出分组改成带图标标题的卡、签上带计数（照 WMS ASN 详情栏）

Category: enhancement
Status: in-progress
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
