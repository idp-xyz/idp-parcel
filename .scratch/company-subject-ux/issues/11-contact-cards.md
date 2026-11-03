# 11 联系人改成卡片；读出表首列内边距；两处漏改的字

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 看「联系人」签截图（`scratchpad/ux-le2/contacts-table-feedback.png`）问有没有更好的设计，并指出选中行文字贴着底色左边缘；协调方建议联系人用卡片，用户认可。

## 诊断

五列表里只有姓名和用途是要逐列比对的，部门 / 职务、联系方式、社交账号三列都是一格叠几行，每行一百五六十像素、四个人占满一屏；社交账号列是裸 ID 加平台名两行一组；「主要」用链接色但不可点；邮箱在域名中间折行；首列没有内边距，选中 / 悬停底色从表格最左缘起，字顶在边上；右上「第 6 版 · 生效自 2026-11-03 15:55:09 UTC」没按日期、没按显示时区；脚注「更新法人资料」是漏改的旧词。

## 要做的

1. **联系人卡**（`LegalEntityContactsTable.tsx` 改名或替换为 `LegalEntityContactCards.tsx`；`LegalEntityContactsSection.tsx`、`LegalEntityProfileContent.tsx` 的联系人卡内改用它）：一人一张卡，随容器宽度排 1–3 列（复用 `legal-entity-profile-readout.css` 的容器查询，加一档 ≥ 66rem 三列）。卡内从上到下：姓名（13px 半粗）；部门 · 职务一行弱色（没有就不显）；用途一行小胶囊，主要联系人在胶囊里加 ★ 或「主要」小字，不用链接色；电话、邮箱、社交账号各一行，前面 14px 弱色小图标（`Phone` / `Mail` / `MessageCircle`），邮箱 `break-all` 不在域名中间折；社交账号一行内「微信 SYN-1-…  ·  企业微信 SYN-2-…」，账号等宽。没有的项整行不显示，不画「—」。卡样式与资料读出卡同一种（细边框、无投影），不带浅底标题条（卡标题就是人名）。
2. **读出表首列内边距**（`LegalEntityProfileReadout.tsx` 或 `.css`、`LegalEntityBankAccountsTable` 等读出表共用的样式）：行的悬停 / 选中底色保留，首列与末列各加 0.75rem 内边距，字不再贴底色边缘；收款账户表、版本对比表同一处改。
3. **联系人签头部**（`LegalEntityContactsSection.tsx`）：「第 N 版 · 生效自 …」改成 `Instant precision="day"`（显示时区、按日期）；脚注「联系人资料可通过右上方「更新法人资料」维护。」→「要改联系人，用右上方的「更新公司资料」。」
4. 联系人卡的判读（哪些行显示、用途与主要的折算、社交账号串）进纯函数钉 node:test；三道门；浏览器 1440 与 2000 宽各截联系人签（详情栏与对象页）。

## 不做

- 不改表单里的联系人编辑（票 04 的形）；不动 `templates/*`、`components/*`。

## 完成判据

- 三道门退 0；联系人签是卡片、2000 宽三列 / 1440 宽两列；用途胶囊带「主要」标记、无链接色；首列不贴底色边；头部日期按显示时区到日；脚注用「更新公司资料」。

## 完成记录（2026-11-03，分支 `claude-csux-11`，末笔 `f55261fd1`）

**落点**：`LegalEntityContactsTable.tsx` → `LegalEntityContactCards.tsx`（git mv，内容重写）；新 `legal-entity-contact-cards.ts`（卡的判读纯函数 `contactCardOf` / `socialAccountsText`）与 `legal-entity-contact-cards.test.ts`；`legal-entity-profile-readout.css`（卡组自己是容器、一 / 二 / 三列，`.legal-entity-readout-table` 首末列内边距）；`LegalEntityProfileContent.tsx`（概要签的联系人卡改调卡、收款账户表挂 `.legal-entity-readout-table`）；`LegalEntityContactsSection.tsx`（改调卡、头部 `Instant precision="day"`、脚注）。落点之外未动一行；`templates/*`、`components/*` 未碰。四笔：`7192cccfd` 卡、`a2a40c75e` 读出表内边距、`08c9143d9` 头部日期与脚注、`f55261fd1` 社交账号按「平台 账号」整组换行（三列实测在账号连字符处断成两截，补的）。

**判据**（浏览器核验走 vite `:5212` 接 `:19080`，`VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`，脚本断言零写请求；截图在协调方 scratchpad `csux11/`：`object-contacts-{1440,2000}`、`object-summary-{1440,2000}`、`list-contacts-{1440,2000}`、`list-contacts-2000-wide`）：

- 三道门退 0 ✅（`f55261fd1` 上 tsc / node:test / vite build；`git merge origin/main` 为 Already up to date，main 仍在 `4e4a6d977`）。
- 联系人签是卡片 ✅；列数按容器宽 ◑：对象页 1440 与 2000 **都是两列**——`DetailPageTemplate` 把内容列钉在 960px，卡组容器两处都量到 888px，2000 宽也到不了 66rem；详情栏 1440 一列（559px）、2000 两列（839px）、把分隔条拖宽到 1259px 时三列（`list-contacts-2000-wide.png`）。「2000 宽三列」因此只在详情栏拖宽后成立。三列档留在票面写的 66rem：要让 2000 的默认详情栏三列得压到 52rem 以下，那会让 1440 的对象页也三列、且每列不到 17rem 装不下一行邮箱；要对象页三列得改 `templates/DetailPageTemplate.tsx` 的 960px 上限，不在本票，待协调方裁。
- 用途胶囊带「主要」、无链接色 ✅（卡内 `.text-idpxyz-accent` 零个；「主要」为 11px 半粗、同正文色，胶囊底 `bg-idpxyz-editor`）。
- 没有的项整行不显示、不画「—」✅（卡内无「—」；演示租户四人项齐全，缺项分支由 node:test 钉：部门 / 职务只剩一个、电话邮箱空白、账号空的社交条目剔掉、primaryFor 不在 purposes 里的补成胶囊）。
- 首列不贴底色边 ✅（收款账户表首列 `padding-left` 12px、末列 `padding-right` 12px，`object-summary-*.png` 悬停行可见；资料版本签的对比表走 ui-primitives `TableCell`、表单差异预览表本就每格 px-3，没改）。
- 头部日期按显示时区到日 ✅「第 6 版 · 生效自 2026-11-03」，完整时刻在 `title`。
- 脚注「要改联系人，用右上方的「更新公司资料」。」✅；`更新法人资料` 全仓零命中。

**未验**：暗色主题下胶囊底色与卡边；真有缺项的联系人只在 node:test 里走过，演示数据没有这种人；超长邮箱的 `break-all` 折法未实测；CI `admin-web` job 在分支上未看。
