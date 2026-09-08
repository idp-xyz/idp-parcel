# 16 `CREDIT_POLICY` 版本的运营主路径：逐字段表单（额度「金额 / 比例」二选一）——建议作公共半边的首例

Category: enhancement
Status: in-progress——2026-09-08 14:23 MCP-5 认领（通道 1 派单 task-2e9b1361，本票作第 0 波：信用政策表单 + 前端公共半边；09/10/11 等本票的落点广播）；分支 `mcp5-awf16`，基线 main `0ef63897`，隔离树 `%TEMP%\idp-parcel-mcp5-awf16`。此前 ready-for-agent——形状已裁清（逐字段表单 + 额度二选一控件，恰一由服务端裁；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿（08 已于 2026-09-08 resolved，边解除）；票 08 建议拿本册做规范化摘要的首例——若采纳则两票同批落、由 08 的认领人一并认领本票；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
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
