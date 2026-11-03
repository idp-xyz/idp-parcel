# 12 读出的可读性：地址按行、抬头成清单、字号统一、账户表列宽

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 看概要签读出截图（`scratchpad/ux-le2/readout-feedback.png`）问「文字、大小尺寸、布局、可阅读性好吗」；协调方诊断见下，用户认可改法。

## 诊断

1. 地址被「·」串成一段折三行，眼睛要在点和括号里找断句；两列网格又把它挤进窄栏。地址本来按行录入，该按行显示。
2. 开票抬头一行混了抬头正文（14px）、语言标签（13px 灰、在 `zh-` 处折行）；右半格把「其他语言的抬头 + 语言 + 抬头」挤一格。它与「监管登记号」是同一类内容却两种排法。
3. 字号不统一：正文 14px、标签 13px、等宽号码 12px，同一行三种尺寸；收款账户表「户名」折行后第二行以全角括号开头像悬挂缩进。

## 要做的（`LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx` + `.css`、`LegalEntityBankAccountsTable`（若独立文件）、`legal-entity-profile.ts` 的 `addressText` 只给 title / 对比用，不再给读出）

1. **地址按行**：读出里地址用存着的 `lines` 逐行显示（`white-space` 正常、每行一个 `<span class="block">`），国家经 `CountryLabel` 作最后一行弱色；三个地址（注册 / 办公 / 通信）放在卡内自己的一行，≥48rem 时三栏并排、标签在上（13px 弱色）、值在下，窄时纵向堆叠；没有的地址不画栏。「法定代表人」仍是标签在左的一行。
2. **开票抬头成清单**：「开票抬头」一个标签，右侧小清单与「监管登记号」同一种件（`ProfileReadoutPairs`）：每行「语言（固定宽、`whitespace-nowrap`）→ 抬头」，主抬头排第一、语言格后带一枚小字「主抬头」；没标语言的主抬头语言格显「—」。`其他语言的抬头` 标签撤掉。
3. **字号统一**：读出里等宽字（账号、登记号、来源单据）13px，与正文同高；语言 / 类型名等辅助文字统一 `text-idpxyz-breadcrumb` 13px；卡内标签 13px、值 14px 不变。
4. **收款账户表**：列宽重分（币种 / 用途 14%、户名 24%、开户行 26%、账号 · SWIFT 22%、备注 14%）；全角括号开头的折行用 `text-indent`/`hanging-punctuation` 或在户名格上 `[overflow-wrap:anywhere]` + `text-align: left` 消掉悬挂感——实测哪种有效用哪种，票面写明。
5. 版本对比与差异预览里的地址（`profileAddressText`）照旧一行文本——那是比对用，不改。
6. 纯函数（地址行折算、抬头清单排序：主抬头在前）钉 node:test；三道门；浏览器 1440 与 2000 宽各截对象页概要签（SYN-LE-CN-SH-01，双语抬头、两个币种账户）与详情栏概要签。

## 不做

- 不动卡的分组、标题条、收款账户表的结构；不动 `templates/*`、`components/*`。

## 完成判据

- 三道门退 0；截图里地址逐行、三栏并排；开票抬头是「语言 → 抬头」清单且主抬头在前；等宽字与正文同高；户名折行无悬挂感。
