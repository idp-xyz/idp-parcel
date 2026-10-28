# 04 管理台的操作者引用长串与显示时区

Category: enhancement
Status: resolved——2026-10-28 进 main（前端切片碰共享面 `components/*`，非作者评审两轮：首轮钉 `e542dbae8` 阻断两条——动线上委托详情「来源」、关务嵌入式「标签@发行方#主体」仍显长串，委托列表/详情、追踪等时间不走 `formatInstant`；作者修订 `59d8e1f46` 后复核阻断清零。推送方在重放 tip 上复跑 `tsc -b`、单测、`vite build` 通过）。非阻断余项（不在动线：接受前复核来源、价卡导入录入者、参考序列登记人、改路与初始路由决定人、关案决定人、`DeliveryHandoverNotesSection` 的 `formedAt`、模板预览）照冻结清单不另立票。此前 作者完工待评审——分支 claude-demo-ui（用户「都要修」；tops-20 立）
Blocked by: 无

## 现象

彩排（`talk-track.md` 所据彩排，钉 `d086bc3c8`）：
1. 「来源」「提交人」「授权依据」「处理人」等列原样显示 `http://127.0.0.1:5558/dex#Ch…` 这类「发行方#主体」长串，把表格挤窄；委托查阅与提交版本签最明显。
2. 页面时间按浏览器所在机器的时区显示；演示机若不在上海时区，给国内客户看的时间是错位的。

## 做什么（只改 `apps/admin-web`）

1. 一个共享的引用显示：形如「发行方#主体」的操作者 / 客户端引用，单元格显示短名（主体部分；主体是不透明长串时截短并加省略），`title` 悬停给全文；可复制全文。找出所有显示这类引用的列统一用它（先 `grep` 现有页面里怎么渲染这些字段）。不改后端、不改数据。
2. 显示时区可配置：所有时间格式化走一个共享函数，时区取构建 / 运行配置（例如 `VITE_DISPLAY_TIME_ZONE`，IANA 名），未配置时按浏览器时区（今天的行为）；显示时带时区标注（例如「UTC+8」）避免误读。产品不写死任何租户的时区。`scripts/demo-seeds/demo-run/README.md` 与 `talk-track.md` 开机清单改成「起管理台时设 `VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`」。

## 完成判据

- 单测覆盖引用短名（几种形状含无 `#` 的原样）与时区格式化（配置 / 未配置）；`tsc -b`、全部单测、`vite build` 通过。
- 碰共享面（`components/*`、`shell/*` 等）按 `docs/agents/workflow.md`「前端切片」要一份非作者评审。

## 完成记录（作者）

- 引用短名：`components/principal-ref.ts`（`shortPrincipalRef`，主体 >16 字符截短加「…」，无 `#` 或 `#` 后为空原样）与 `components/PrincipalRef.tsx`（`title` 给全文）。接入：待发布草稿「录入者」、委托查阅「来源」、阶段准入页执行人 / 决定人、价卡草稿面板录入者 / 批准者、价卡目录批准责任方、参考序列登记责任方、收寄后处置请求详情的请求方 / 授权 / 处理人（该处是纯文本，无 title）。未动：登记依据（自由文本）、验收复核与关务页内嵌文本串。
- 时区：`moment.ts` 新增 `resolveDisplayTimeZone`（`VITE_DISPLAY_TIME_ZONE` 合法则用，空取浏览器区，非法退回并 `console.warn`）；`main.tsx` 改走它；新增 `src/vite-env.d.ts` 声明该变量。原有 `formatInstant` 已是共享函数并带 `UTC+8` 标注。演示说明（demo-run README、talk-track 开机清单）改为起管理台时设该变量。
- 判据：单测（引用 6 种形状、时区配置 / 未配置 / 非法）、`tsc -b`、全部单测 829 通过、`vite build` 通过。碰共享面 `components/*`，待非作者评审。

## 评审修订（作者）

- 引用：新增 `shortenEmbeddedPrincipalRefs`（文本里的 `https?://…#主体` 片段换短名，`标签@` 等保留）与 `PrincipalText`（title 给原文）；接入委托详情 / AuthorizedDisposition 的来源、关务提交授权的授权依据 / 就绪判断依据 / 提交授权依据 / 撤销人 / 放行授权、可见性三页批准人、订舱册提交人与取消人。收寄后处置的请求方 / 授权 / 处理人加 title。关务与可见性、订舱册是表格文本格，只显短名无 title（全文不在页上）。未接入：待发布草稿抽屉里录入者 / 批准者（抽屉就是全文处，且其测试断言原串）。
- 时间：委托详情全部时间、委托列表提交时间（列与详情）、追踪投影 derivedAt、价卡目录与参考序列 registeredAt / lastReviewedAt、系列登记对照的 startsAt（含 `T` 才转，纯日期不动）、取消页 decidedAt、价卡导入面板 submittedAt 改走 `formatInstant`。`formedAt` 本就走 formatInstant，未改。价卡导入面板 `content.startsAt` 是业务日期，未动。
- 自验：tsc -b、全部单测 832 通过、vite build 通过。
