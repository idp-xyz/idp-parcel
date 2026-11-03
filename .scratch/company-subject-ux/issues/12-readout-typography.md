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

## 完成记录（2026-11-03，分支 `claude-csux-12`，末笔代码 `bef46e068`）

**落点**：`LegalEntityProfileReadout.tsx`（`ProfileReadoutAddress` 逐行 + 国家末行弱色、新 `ProfileReadoutAddresses` 三栏件、新 `ReadoutText` 粘段件、`ProfileReadoutPairs` 加 `headNowrap` 且两列同 13px）；`legal-entity-profile-readout.css`（`.legal-entity-readout-addresses` 一列 / ≥48rem 按列流等宽，值格与表格 `word-break: keep-all; overflow-wrap: anywhere`）；`LegalEntityProfileContent.tsx`（代表人一行 + 地址行、开票抬头清单与「主抬头」小标、表列宽、各自由文字格走 `ReadoutText`、等宽与备注格同正文）；`legal-entity-profile.ts`（纯函数 `addressLinesOf`、`invoiceTitleRowsOf`、`glueBracketGroups`）与 `legal-entity-profile.test.ts`（三条各钉一例）。**落点之外**：`LegalEntityProfileSummary.tsx`、`LegalEntityProfileSection.tsx` 各一行——读出小标题里「来源单据」的等宽字升到 13px（第 3 条点了名的那个等宽字在这两处）。`templates/*`、`components/*`、`shell/*` 未碰。五笔：`ad6434dca` 地址、`33b533d0f` 抬头清单、`6094e45d3` 字号、`2bba8fcf9` 列宽 + 折行、`bef46e068` 币种列 15%。

**判据**（浏览器核验走 vite `:5213` 接 `:19080`，`VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`，脚本断言零写请求、零页面错误；截图在协调方 scratchpad `csux12/`：`object-summary-{1440,2000}`、`object-readout-{1440,2000}`、`list-panel-summary-{1440,2000}`、`panel-readout-{1440,2000}`、`list-panel-summary-1440-tall` / `panel-readout-1440-tall`（1440×2000 看窄栏全貌）；脚本 `shots.js` 顺手量了字号、地址栏位置与「以全角「（」开头的行」）：

- 三道门退 0 ✅（`bef46e068` 上 tsc / node:test 1171 / vite build；`git merge origin/main` 为 Already up to date，main 仍在 `fc7144f32`）。
- 地址逐行、三栏并排 ✅：对象页 1440 与 2000 容器都是 888px（模板钉 960），三栏 x = 414 / 709 / 1004；详情栏 1440（559px）纵向堆叠，2000（839px）三栏。国家经 `CountryLabel` 作末行、`text-idpxyz-breadcrumb`。法定代表人仍标签在左。
- 开票抬头是「语言 → 抬头」清单、主抬头在前 ✅：左列 `简体中文（zh-Hans） [主抬头]`、`英语（en）`，`whitespace-nowrap`；「其他语言的抬头」标签在读出里零命中（版本对比与快照里的同名字段照旧）。主抬头那行仍 `font-medium`，判据不变。
- 等宽字与正文同高 ✅：计算字号 `pairsMono / tableMono / basisMono` 都是 13px，`pairsHead / pairsValue / tableNote` 13px；卡内 `dt` 12px、`dd` 13px 不变——与 `DetailRow` 同一把尺。票面写的「标签 13 / 值 14」按本仓的实际尺度对应 12 / 13（用户截图是 Windows 1.5× 缩放），没有整体放大一号；要放大另立票。「主抬头」小标 11px 半粗，与联系人卡「主要」同款。
- 户名折行无悬挂感 ✅，**用的是第三种办法**，票面列的几种实测如下（Chromium 1140，微软雅黑与文泉驿两种字体，`scratchpad/csux12/trim-test.js`、`glue-test.js`）：`text-spacing-trim: trim-start` 计算值生效但行首「（」进距仍 13px 整字宽（两种字体都不带 `halt`）；`trim-all` 与 `hanging-punctuation` 直接不被接受；`word-break: keep-all` 仍在「司（」之间折；`[overflow-wrap:anywhere]` + `text-align: left` 不改折点。有效的是 `glueBracketGroups`：全角括号组连同前一个字包成 `white-space: nowrap` 一段（不插零宽字符，户名 / 账号复制出去仍是原文），再配 `keep-all + overflow-wrap: anywhere` 让中文按空格 / 闭括号折、长名在格边兜底。四张截图里「以全角「（」开头的行」对象页为零；户名折成「IDP物流科技（上海）」/「有限公司（演示）」，地址行「…A 座 12」/「层（模拟地址）」。
- 列宽 ◑：币种 15 / 户名 24 / 开户行 25 / 账号 22 / 备注 14，不是票面的 14 / 24 / 26 / 22 / 14——14% 在详情栏拖到 2000 宽（表约 840px）时装不下「人民币（CNY）」一行、在「（」前折开；那 1% 从最宽的开户行列出，账号列照票面留给 IBAN。
- 版本对比与差异预览的 `profileAddressText` 未动 ✅；资料版本签的快照（`LegalEntityProfileHistory` 的 `Snapshot`）共用 `ProfileReadoutAddress`，随之逐行，未截图。

**残留 / 未验**：详情栏窄于约 830px 时收款账户表已到 40rem 最小宽横向滚动，币种格仍折成「人民币」/「（CNY）」（`CurrencyLabel` 的字串没走粘段——粘了会在格边折成「人民币（CN」更糟）；旧 18% 时 645px 以下同样如此，不是本票新引入。备注格 14% 装不下 12 个字的「跨境美元结汇账户（演示）」，按兜底在格边折成「跨境美元结汇账」/「户（演示）」。暗色主题下的末行灰与小标底色未看；真 IBAN 长度（27 位）只按 `overflow-wrap: anywhere` 推断会在格内折；CI `admin-web` job 在分支上未看。
