# 14 复制规则统一：号码类一键复制，文本类悬停复制图标，其余不复制

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 问「所有只读点击都自动复制科学吗」；协调方答不科学并给出三类规则，用户裁「需要」。

## 规则（公司主体先落；别的页照此另票）

| 类型 | 行为 |
|---|---|
| 号码类：编码、注册号、监管登记号、账号 / IBAN、SWIFT / BIC、来源单据、电话、邮箱、社交账号 | 等宽显示；整个值是按钮，点击即复制；悬停出 14px 复制图标与「点击复制」提示；复制后两秒行内「已复制」反馈；键盘可达（Tab 到、Enter / 空格复制）；读屏名「复制<什么>」 |
| 文本类：公司名、开票抬头（含其他语言）、地址（整段，按行拼接换行）、户名、开户行 | 正常可选文字；值末尾悬停出一个小复制图标，点图标才整段复制，同样有「已复制」反馈 |
| 标签、状态、日期、说明、计数 | 不复制 |

复制永远不悄悄发生：没有反馈的复制等于没复制。

## 要做的

1. **复用一处**：`detail-primitives.tsx` 已有 `RefText`（点击即复制）与 `useCopyToClipboard`——把「已复制」反馈、悬停图标、按钮语义做进 `RefText`（若已有一部分就补齐），再加一个 `CopyAside`（文本类值末尾的悬停复制图标）。两件都在 `pages/party/detail-primitives.tsx`，不进 `components/*`（别的页要用时再上提）。
2. **读出**（`LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx`、`LegalEntityContactCards.tsx`、`LegalEntityProfileSummary.tsx`）：号码类全部换 `RefText`——监管登记号的号、收款账户表的账号与 SWIFT、来源单据、联系人卡的电话 / 邮箱 / 社交账号；文本类加 `CopyAside`——开票抬头各语言、三个地址（复制时按行拼接）、户名、开户行、联系人姓名。
3. **头区**（`LegalEntityDetailPanel.tsx`、`LegalEntityDetailPage.tsx`）：编码、注册号已是 `RefText`，补齐悬停图标与反馈；公司名加 `CopyAside`（对象页标题处若模板不让插，放 meta 行）。
4. **列表**（`GroupLegalEntitiesPage.tsx`）：行内的编码与注册号不做点击复制——行点击是选中详情，两种点击叠在一格上会冲突；只在详情栏复制。票面写明这条理由。
5. 判读（文本类复制内容的拼接、反馈时序）钉 node:test；三道门；浏览器核：对象页与详情栏各一张悬停态截图（图标可见）、一张点击后「已复制」态；playwright 读 `navigator.clipboard`（需授予 `clipboard-read` 权限）证复制的内容；零写请求。

## 不做

- 不给标签、状态、日期加复制；不改 `components/*`、`templates/*`；不动别的页。

## 完成判据

- 三道门退 0；号码类值 Tab 可达、Enter 复制、悬停有图标、点击后「已复制」；文本类值末尾悬停有图标、点图标复制整段；地址复制出来是多行；列表行不复制。

## 完成记录（2026-11-03）

作者子代理做完四条代码后撞用量上限停下（`1cebf4f5d` 第 1 条复制件、`c6e642acb` 第 2 条读出换件、`d6575673f` 第 3 条头区、`177ae42fb` 第 4 条列表理由）；三道门与浏览器核验由协调方接着做。

- 三道门退 0：`tsc -b --noEmit`、`run-tests` 1182 过 0 败、`vite build`（合 origin/main 后）。
- 浏览器（vite :5215 接 :19080，1440 宽，`clipboard-read` 授权后读 `navigator.clipboard`，零写请求、零页面错误；截图 `scratchpad/csux14/v1–v4`）：
  - 号码类点击即复制 ✅：账号点击后剪贴板 `SYN-CNY-0000000101`；Tab 到 SWIFT 按 Enter 后 `SYNBCNSH`；详情栏头区编码点击后 `SYN-LE-CN-SH-01`。
  - 文本类悬停出图标、点图标整段复制 ✅：注册地址末尾悬停有复制图标，点后剪贴板是四行（三行地址 + 「中国（CN）」）。
  - 对象页共 39 个复制按钮，读屏名覆盖：编码、注册号、来源单据、三个地址、开票抬头、两个监管登记号、户名、开户行、账号、SWIFT、联系人姓名、电话、邮箱、微信 / 企业微信账号。
  - 列表行不复制 ✅：剪贴板先写哨兵，点列表行里的编码文字后仍是哨兵。
- 未验：「已复制」反馈的出现时长只在 node:test（`copyNoteTimer`）里钉；暗色主题。
