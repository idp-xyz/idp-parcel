# 05 管理台「导入价卡」签

Category: enhancement
Status: resolved——2026-10-10 通道 1 收口进 main（通道 2 现场 `f64e61a6` 原样入库，通道 1 补存草稿开关与本记录）；完成记录见文末。此前：in-progress——2026-10-10 通道 2 在 main 上做（前端切片）。此前：ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 无（03 已进 main `f8177a9d`）
地盘：`apps/admin-web/src/pages/pricing/`（价卡页、纯逻辑、`api.ts`），以及 01 定下的模板文件存放处（若在管理台内）。
出处：[spec](../spec.md)；ADR-0101 Consequences 第二条与末条。

## 要做的

1. 价卡页的「登记价卡」签改为「导入价卡」。流程：下载模板 → 上传 → 校验结果与摘要预览 → 存为草稿。
   - 预览走 02 的口，存草稿走 03 的口，两次上传同一份字节。
   - 逐格问题按表、行、列排给人看，能对回模板里的那一格。
2. JSON 快照签退为高级口，不出现在运营配置员的主路径上。
3. `api.ts` 里「请求体形状此刻没有契约」那一段按 ADR-0101 改写。
4. 四态如实：端点今天答 403，页面照实呈现未配置，不拿演示数据顶替。

## 验收

- 三道门：`tsc -b --noEmit`、`run-tests`、`vite build`。
- 浏览器验证：真后端的 403 一态；预览与存草稿两态用真适配器序列化的响应注入，注入方式照 [operator-workspace-gaps/06](../../operator-workspace-gaps/issues/06-estimate-admin-page.md) 完成记录。

## 形态

diff 全在 `apps/admin-web/**` 与票面，按 workflow「前端切片」在 `main` 上做。

## 完成记录（2026-10-10，通道 1；前端切片，共享树 `main`；证据只记 `S`）

**落点**

- `f64e61a6`：通道 2 在共享树 `main` 上留下的未提交现场，一字未改入库（六份改动、三份新文件，mtime 都停在 12:01:42；通道 2 自 16:09 起无 MCP 活动，用户令通道 1 独立完成后续）。导入签 `PriceCardImportPanel`、`api.ts` 的导入两口与类型、`catalogue-api.ts` 新增的上传写面 `postMasterDataForm`、逐格问题排法 `sortPriceCardProblems`、JSON 快照签改名。
- 本笔：存草稿开关改按 `draftSubmittable`——只在这一份文件的预览形成答案之后才开。见下「验证中发现并修掉」。

**判据逐条**

- ✅ 要做的 1：价卡页三签为「价卡目录 / 导入价卡 / 高级：JSON 快照」，「登记价卡」签已无。导入签：下载模板 → 选文件 → 校验并预览 → 存为草稿；预览打 `/pricing-price-card-previews`，存草稿打 `/pricing-price-card-drafts`。两次送同一份字节：浏览器里截下两次请求，表单都只有 `file` 一格、同为 72272 字节、SHA-256 同为 `39a3a37d…`，请求头只有 `Accept`，multipart 边界由浏览器带上。逐格问题按表、行、列排：乱序注入四条，页面排成 `card/2/planVersion` → `tables/3/currency` → `tables/3/weightTo`（同格两条保持到达顺序）；单测钉在 `price-card-import.test.ts`。
- ✅ 要做的 2：JSON 快照签名「高级：JSON 快照」，提示首句「这不是运营配置员的主路径」，指回「导入价卡」。
- ✅ 要做的 3：`api.ts` 原「请求体形状此刻没有契约」那段已按 ADR-0101 决定一改写：价卡这一册的运营面形状由产品定，主路径是导入模板，JSON 快照退为高级口；参考序列两段里转述旧口径的句子随之改掉。
- ✅ 要做的 4：真后端两口今天都答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，页面只呈现未配置说明，不画表、不拿演示数据顶替。
- ✅ 验收·三道门（共享树钉本笔，Node 22.23.3）：`tsc -b --noEmit` 退 0；`run-tests` 484/484；`vite build` 退 0。
- ✅ 验收·浏览器验证，见下节。

**浏览器验证（Cursor 内置浏览器，证据 `S`）**

- **真后端**：parcel-api 取 `f64e61a6` 构建（共享树当时无未提交 `.go`），连 55432 上一只临时库（迁移助手施加全部 205 步，验完即删），监听 `127.0.0.1:19080`；管理台走 `vite` 开发服务器的 `/api` 代理。上传真模板 `price-card-import-template-PPT-1.xlsx` 点「校验并预览」，真端点答 403，页面呈现未配置；`curl` 直打与经代理两口同答。
- **预览与存草稿四态**：操作者渠道今天没有真路径能答出这些格，所以注入。注入体不是手写：一次性测试（跑完即删、未入库）借 `adapters/http` 测试夹具 `validatedDraft` / `problemDraft`，让真端点 `NewPreviewPriceCardImportEndpoint`、`NewSubmitPriceCardDraftEndpoint` 序列化出 `VALIDATED`、`HAS_PROBLEMS`（四条乱序问题）、`DRAFT_SUBMITTED`（201，带册上那一行）、`CONTENT_FIXED`（200，不带行）。注入点在页面内的 `fetch`，不是 [operator-workspace-gaps/06](../../operator-workspace-gaps/issues/06-estimate-admin-page.md) 那样的本机代理；替换的只有这两口的答复，请求照常由页面组装。四态逐格呈现：方案概要各行、`DRAFT_SUBMITTED` 那一行的状态与录入者、`CONTENT_FIXED` 的「这次没有写下册上的行」。换一份文件后旧预览与存草稿结果清空、存草稿复禁。
- **验证中发现并修掉**：预览答 403 之后「存为草稿」就亮了——开关原先只判「预览不为空」，而未配置、传输失败、未形成答复都没让人看到读法，与页面自己写的「存草稿要先看到这一份文件的预览」相反。改为 `draftSubmittable`（预览形成答案才开），先写用例见红再改；真后端 403 之后复验，存草稿保持禁用。
- **没在浏览器里走的**：调用方问题、未形成答复、传输失败三格只由穷举 `switch` 的类型检查与 `draftSubmittable` 用例兜住；预览 `NOT_ACCEPTED` 与存草稿 `DRAFT_REPLAYED` / `DRAFT_REVISED` / `NOT_ACCEPTED` 只核了词表原名，未注入；填好的真工作簿经真解码出预览——端点今天答 403，这一格要等操作者渠道发行方登记后才走得到。
- **披露**：为越过 OIDC 登录门，只在浏览器会话里写了一条 `sessionStorage` 会话桩，未入库；这一版管理台不把令牌发给 `/api`，桩不影响后端答复。截图留在本机临时目录，未入库。本机 `/usr/bin/node` 是 18.19.1，没有全局 `File`，`price-card-import.test.ts` 的上传表单用例在它上面红一条；三道门用 `~/.local/node` 的 22.23.3 跑，与 CI `admin-web` job 同大版本。

**评审**：diff 只碰业务页 `pages/pricing/**` 与 `pages/catalogue-api.ts`（只新增 `postMasterDataForm`，已有函数未动），没碰 `templates/`、`shell/`、`components/`，按 workflow「前端切片」做业务页自查。没有其他会话可做非作者评审，本记录不算非作者评审。
