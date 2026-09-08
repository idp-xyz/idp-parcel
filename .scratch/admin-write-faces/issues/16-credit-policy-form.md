# 16 `CREDIT_POLICY` 版本的运营主路径：逐字段表单（额度「金额 / 比例」二选一）——建议作公共半边的首例

Category: enhancement
Status: resolved——2026-09-08 15:2x 进 main（推送方 MCP-1 记）：分支 `mcp5-awf16` 三笔 cherry-pick 零冲突 `283aa7d3→dff3331a`、`5e2aa3d1→799e0ed9`、`7d204fd5→cf4bd709`，非作者评审（通道 2，两轴无阻断）记录 `edea6c7f→f1e37065`，评审非阻断 (1)(2) 文案追笔 `mcp5-awf16-followup@9dacdbad→abdc5ef9`；验证钉 `cf4bd709`：tsc 0、run-tests 91/91、architecture ok、清点零差（纯 admin-web）。此前 in-progress——2026-09-08 14:23 MCP-5 认领（通道 1 派单 task-2e9b1361，本票作第 0 波：信用政策表单 + 前端公共半边；09/10/11 等本票的落点广播）；分支 `mcp5-awf16`，基线 main `0ef63897`，隔离树 `%TEMP%\idp-parcel-mcp5-awf16`。此前 ready-for-agent——形状已裁清（逐字段表单 + 额度二选一控件，恰一由服务端裁；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿（08 已于 2026-09-08 resolved，边解除）；票 08 建议拿本册做规范化摘要的首例——若采纳则两票同批落、由 08 的认领人一并认领本票；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**政策页·信用政策册**（第七册，pc-gaps/03）。`declarations.creditPolicyBody{legalEntity, authorityLevel, chargeType,
limitMinor | limitRatioBasisPoints, effective…}`（0020 正文）：额度两键恰一（库上 CHECK；读面 `creditLimitCell` 已把
「两键都缺」点名为坏响应）。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，额度二选一控件。** 频次低、配置员操作、正文五格无子表——十册里最小的一类，正因如此票 08 建议先在它
身上把「服务端按册规范化 + 摘要 + 待批准载体 + 预览」整条路走通。二选一控件呈现「金额 / 比例」，**恰一由服务端裁**：
两格都填或都空提交上去，答的是构造门的拒绝。零金额是登记方说出的「授予零信用」，表单不得把 0 折成缺席（读面同一
判据）。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：`CreditBasis` 的消费缝今天不存在（wiring-baseline-remainder/03），本票只管发布，
不替 SA 接。

## 完成判据

信用政策册旁多一签「发布信用政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见（额度列
按金额或比例恰一上列）；tsc / run-tests 绿；Go 侧本册规范化一格（若为首例，则连同 08 的机制同批）。

## 边界

不动 0020；不动 `ResolveCreditPolicy`。

## 完成记录（2026-09-08，MCP-5；分支 `mcp5-awf16`，基 main `0ef63897`）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 |
|---|---|
| `283aa7d3` | 前端公共半边三件（`apps/admin-web/src/pages/party/`）：`publication-draft-api.ts` 四口客户端 + 载荷/答复类型（镜像 Go 线格式）+ 四张 outcome 中文表与载体状态表（原词从 Go 抄）；`publication-draft-flow.ts` + `.test.ts` 五步纯逻辑（载荷键、到达步、放行边、每步答复覆盖下游、逐格问题归组）；`PublicationDraftFlow.tsx` 五步组件（props：`kind` / `assemblePayload` / `fieldPaths` / `children` render prop / `onPublished` / `localProblems`）。票面转 in-progress。落点 14:4x 广播给 09/10/11 |
| `5e2aa3d1` | 信用政策表单本体：`credit-policy-form.ts` + `.test.ts`（额度两格都照发、零金额照发 0、本地只拦编不进 JSON 整数的文本、可缺键缺席）；`CreditPolicyPublicationForm.tsx`；`CommercialPoliciesPage.tsx` 加「发布信用政策版本」一签，发布落定读面切到信用政策册并重读 |
| 第三笔 | 评审修正（去掉两处跨文件计数、复用 `labelOf`、删未用的类型再导出）+ 本记录；SHA 见完工报 |

**完成判据逐项**：

1. 信用政策册旁一签「发布信用政策版本」，五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由 `PublicationDraftFlow` 走；发布落定（载体到达发布那一步，含批准口答`载体已发布`）`onPublished` → 读面切到 `CREDIT_POLICY` 册并重读，额度列由既有 `creditLimitCell` 按金额或比例恰一上列，读面一行未改。
2. 额度二选一控件：两格并排都可填，两格都填 / 都空照样送预览，恰一由服务端裁、答在 `creditPolicy.limit` 挂到「额度」组；零金额照发 `0`（测试钉着）。表单不算摘要、不收也不送批准人、不裁任何门——本地只拦整数格填了编不进 JSON 整数的文本，空字段 / 区间先后一律送服务端逐格点名。
3. `tsc --noEmit` 退 0；`node scripts/run-tests.mjs` 91 tests / 91 pass / 0 fail。
4. Go 侧本册规范化一格：08 已落（`PCC-1` 首例信用政策），本票 **Go 未动**（`.go` / `.sql` 零变动）。

**验证强度**：隔离树 `mcp5-awf16`（基 `0ef63897`，`node_modules` 以目录联接指向共享树那份，未跑 `pnpm install`）：`tsc` 退 0；`run-tests` 91/91；`go test ./internal/architecture/ -run 'TestEveryAdminWebPath|TestTheAdminWebPathLiteral' -count=1` ok（四条新路径字面量都在 parcel-api 端点表上）。**没有端到端**：四口今天挂 `UnconfiguredIntake{}`，浏览器里走五步答的是 403，组件如实显示；「五步走通」只由状态机测试 + 类型检查证明。证据层级 **S**。

**评审**：`/code-review` 两轴隔离子代理未能起（harness 鉴权错），改由作者按 skill 正文串行自查——Standards 轴修了三处（`封闭十格` / `三格` 两处跨文件计数改成不带数的写法；组件里自写的 `label()` 换成 presentation 既有的 `labelOf`；删掉 api 文件里未用的 `ApiResult` 再导出），保留两处判断题：`payloadKey` / `normalizeMoment` 与 pricing/series-form.ts 同形不共用（跨页导入会让 party 页依赖 pricing 页的所有权，先例是 pricing/api.ts 对 party 类型「另立窄类型不去改那份」）；`FlowAnswerNote` 与 SeriesRegistrationForm 私有的 `AnswerNote` 同形（那份未导出，且各页 unconfigured 提示句不同）。Spec 轴无缺项；做了 spec 没点名的一件：「正文区间抄壳区间」按钮（省一次手填，不是默认，按钮旁写明）。非作者评审按 parallel-sessions「合入前独立评审」由通道 1 另派。

**未做（各归其票）**：其余九册的表单与正文格（09–17 各接 `CommercialPublicationPayload` 一格 + 自己的 `<册>-form.ts` / `<册>PublicationForm.tsx`）；壳上 `references` 的输入格（本册壳不要引用，服务端点名时落在「未认领」列）；`CreditBasis` 消费缝（wiring-baseline-remainder/03）；录入口若 400 带 `problems`（同一段解码在预览已拦，主路径走不到）`postMasterData` 只透错误码不透逐格——真渠道点亮后若要透，归 catalogue-api 一格另裁。

## Comments

### 评审 ← 通道 2 · 钉 `7d204fd5` · 14:53

非作者合入前评审。基 `0ef63897`，隔离 detached 检出 `%TEMP%\idp-review-awf16`，只读，两轴各一遍；三笔 `283aa7d3` / `5e2aa3d1` / `7d204fd5`
纯 `apps/admin-web/src/pages/party/` + 本票 .md，Go 零变动。outcome 原词与 JSON 键逐字对过 Go 侧 `publication_draft.go` / `preview_commercial_publication.go`
/ `register_commercial_publication.go` 的 answer 结构体与 `application/publication_draft.go` 四个 Outcome 的 `String()`：全对，无自造。

**Standards 轴**

- **阻断**：无。
- **非阻断**：
  1. `PublicationDraftFlow.tsx` `unconfiguredNote`：写「登记接入渠道认证参数（PAR-INT-01，实例半边）后……即放行」。登记册 `PAR-INT-01` 是**客户生产委托
     接入渠道**；四口的 Intake 是 ADR-0100 操作者身份那一族（ADR-0126 Decision 五「Intake 到位那天把信封译成它」），同分支 `publication-draft-api.ts`
     头注自己也写「它不是客户渠道载荷，那一半照旧等 PAR-INT-01」——同一笔里两处口径相反。文案抄自 visibility 页，那里指客户渠道是对的，这里错指。
     改指 ADR-0100 操作者接入渠道（登记册无条目则写「机制半边待接线」）。
  2. `publication-draft-api.ts` `submitOutcomeLabels.DRAFT_SUBMITTED`：「等一位不是录入者的批准者」把「须不同主体」写成定论；ADR-0126 Decision 三
     「不写死双人也不写死单人」，能否自批由租户 `PAR-COM-18` 说。文案改成「等批准（能否自批由租户审批职责规则说）」；`NEEDS_ANOTHER_APPROVER` 那句
     不动——那一格本身就是规则要求不同主体时的答复。
  3. `PublicationDraftFlow.tsx` `fieldPaths: readonly string[]` 是静态精确匹配：子票 10 的可加行子表路径带下标（`…items.2.x`），行数事先不知，无法
     预先认领，会同时出现在格旁与「未认领」列。装 09（只壳 + `references.<KIND>`，可枚举）与 12（多节静态）没问题。建议放宽为前缀或 `claims(path)`
     谓词；不阻 16 合入，10 接时再改 props 属追加不属改写。
  4. 判断题（作者已自报）：`payloadKey` / `normalizeMoment` 与 `pricing/series-form.ts` 同形不共用、`FlowAnswerNote` 与 `AnswerNote` 同形——
     Duplicated Code；作者的所有权理由成立，记录即可。
- **无发现**：硬句四条——`payloadKey` 不是摘要（注释与测试都钉住）、`creditPolicyLocalProblems` 只拦编不进 JSON 整数的文本（测试「空字段不在本地拦」
  「两格都填照发、都空照发」「零金额照发 0」钉住），预览与录入送同一个 `payload` 常量、批准与发布只送 `draftReferenceOf` 身份三元，载荷类型无
  tenant / submitter / contentDigest / approval 键；`unconfigured` 一格如实显 403（ADR-0085）；注释中文，跨文件计数已去；`normalizeMoment` 补零点 UTC
  有 pricing 先例且占位符写明，是输入格式归一不是领域默认。

**Spec 轴**

- **阻断**：无。
- **非阻断**：无。
- **无发现**：完成判据逐条——一签「发布信用政策版本」在册签之后镜像签之前；五步由 `reachedStep` 按 outcome 原词分派，放行边（`DRAFT_REPLAYED` /
  `DRAFT_REVISED` 放行、`CONTENT_FIXED` 停、批准口 `DRAFT_ALREADY_PUBLISHED` 越到发布、`DRAFT_AWAITS_EFFECTIVE_START` 停在批准并显票 08 点名的那一格）
  与 Go 语义一致且测试逐格钉；发布落定 `onPublished` 按键去重 → 页面 `PublishedNotice` 切册并重读，读面一行未改、额度列沿用 `creditLimitCell`。
  「五步走通只由状态机测试 + tsc 证」：四口挂 `UnconfiguredIntake{}` 是机制半边既定事实，前端拿不到真答复，S 级证据到此为止是诚实的；「同一份
  载荷过预览与录入」由 `run` 里同一个 `payload` 常量在结构上保证，读码可核。「正文区间抄壳区间」：显式点击才复制操作者自己填的壳区间、按钮旁写明
  「不是默认」、锁定时禁用——不是代填也不是默认值，红线不触。「未做」四项反查：`references` 无输入格（类型留着、表单不设）、`CreditBasis` 未碰、
  其余九册未动、录入口 400 逐格未透（`postMasterData` 只透码）——都真没做进去。

**结论**：Standards 4 条非阻断（最重：`unconfiguredNote` 错指 `PAR-INT-01`，与同笔 api 头注相反，一行文案）；Spec 0 条。**无阻断**，可重放；
非阻断 1/2 一行文案量级，可由作者追一笔或随 09/10/11 接时带走。
