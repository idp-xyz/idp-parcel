# 14 `PRICE_RULE` 版本的运营主路径：逐字段表单（方向 × 方案绑定从价卡目录选 + 口径条件节）

Category: enhancement
Status: resolved——2026-09-08 20:2x 通道 2 交付（task-7afbd72a；分支 `mcp2-awf14` 基 main `5a209f70`，代码 tip `6bfd2f1b`、清点笔 `c7635837`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA、非作者评审结论待通道 1 重放后补「进 main 记录」）。完成判据三条全落：商业价格政策册旁一签「发布价格政策版本」五步走公共半边、发布落定切到 PRICE_POLICY 册重读且读面多一列口径；tsc 退 0 / run-tests 127 / 127；Go 侧本册接进 PCC-1 一格（覆盖正文与口径，连同应用层两个方向与载荷一格）。此前 in-progress——2026-09-08 19:4x 通道 2 认领（通道 1 派单 task-7afbd72a；分支 `mcp2-awf14`，基线 main `5a209f70`，隔离树 `D:/tops/idp-parcel-mcp2-awf14`）。此前 ready-for-agent——形状已裁清（逐字段表单 + 口径作条件节，显隐是呈现不是裁门；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿（08 已于 2026-09-08 resolved，边解除）；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08

## 册与载荷

显示在**政策页·商业价格政策册**（册名 `PRICE_POLICY`，发布类别 `PRICE_RULE`——两条分类轴，票 03 已在册名旁说明）。
`declarations.pricePolicyBody{direction, pricingPlan, planDirection, conversion, scope, effective…, caliber?{taxDisposition,
taxClassification?, volumetricFactor?, fx?{quoteType, asOfSemantics, asOfPolicyVersion}}}`（0010 正文 + 0022 口径）。
口径各格的在场规则钉在库上 CHECK：税务分类只在含税 / 未税时在场、体积口径只在销售方向在场、汇率三格同在同缺。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，口径作条件节。** 频次低、配置员操作、正文十来格无子表。`pricingPlan` 从价卡目录**选**（跨上下文
只传引用，与票 11 同一句）；`planDirection` 与 `conversion` 是发布当时保全的答复与声明（ADR-0057），表单如实收、
不从方案反推。口径节按 `taxDisposition` / `direction` 显隐条件字段——**显隐是呈现，不是裁门**：显了没填、隐了却
传了，都由服务端按 CHECK 同形的构造门答。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：口径随正文**同笔**登记是发布编排的纪律（`PricePolicyRow.HasCaliber` 注释），
表单提交的是一份载荷；不给「先发正文、回头补口径」的两步。

## 完成判据

商业价格政策册旁多一签「发布价格政策版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见
（含口径列）；tsc / run-tests 绿；Go 侧只加本册规范化一格（覆盖正文与口径）。

## 边界

不动 0010 / 0022；不读方案内容。

## 完成记录（2026-09-08，通道 2；分支 `mcp2-awf14`，基 main `5a209f70`；task-7afbd72a）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| 分支 SHA | 内容 |
|---|---|
| `c3695d2a` | 票面认领转 in-progress |
| `b0942646` | 领域：价格规则册接进 PCC-1 不换号。新文件 `publication_canonicalization_price_policy.go(+_test)`：正文输入面 `PricePolicyBody`（0010 正文连同发布期 `PlanDirection` / `Conversion`，ADR-0057）与嵌在其内、可缺的 `PricePolicyCaliberBody`（0022：税务、体积、可缺汇率）；文档节键名镜像批文 `pricePolicyBodyDocument` / `pricePolicyCaliberDocument` / `fxCaliberDocument`，口径节无方向键；`validate` 与发布时同一套门（逐格非零、`checkPlanBinding`、`NewPricePolicyCaliber` 判据、`ConsistentWithDirection`），只少拥有版本一判；导出反查 `PriceDirectionNamed` / `PlanBindingConversionNamed` / `TaxDispositionNamed`。共享文件 `publication_canonicalization.go` 纯加行 +20/−0 |
| `57341038` | 应用：`publicationContentOf` 一 case（对账门对本册开门，甲路）+ `declarationsOfContent` 一支（载体正文连同口径交回同一份 `PricePolicyBodyDeclaration`，既有发布用例紧跟正文写 0022——「口径随正文同笔登记」在结构上成立）。既有用例四例甲路下必然的一改：两例正向改配算出摘要（新 helper `pricePolicySpec`）；两例反向（未声明转换的跨向绑定、口径方向不一致）原由 `declarationWrites` 报 error，现由对账门的规范化先答`未受理`带同一成因；新用例 `TestPublishingAPriceRuleDraftRegistersBodyAndCaliberInTheSameStroke` |
| `3c451050` | 传输面：新文件 `publication_draft_payload_price_policy.go(+_test)`：`PricePolicyBodyPayload` / `PricePolicyCaliberPayload` / `FxCaliberPayload`，问题落 `pricePolicy.<键>`；封闭集三格按 `*Named` 反查、空转换不代填 NONE；口径两条件格的在场规则由 `NewTaxCaliber` / `NewVolumetricCaliber` 判并答在**条件格**上（`taxClassification` / `volumetricFactor`）；汇率节在场三格缺一即点名。共享 `publication_draft_payload.go` 纯加行一格 + 一段 |
| `c22218b4` | Web：新文件 `party/price-policy-form.ts(+.test.ts)`（草稿 → 载荷；口径节永远随正文同送；汇率三格全空即整节缺席、填任一格整节送；封闭集四格不预选、空串照送；条件格显隐是呈现——隐掉的格有值照送；认领路径与载荷键一一对应外加节级 `pricePolicy.caliber.fx`）、`party/PricePolicyPublicationForm.tsx`（挂 `PublicationDraftFlow kind="PRICE_RULE"`；方案从价卡目录选取 `planId@planVersion`、目录行显方向不按方向过滤、读面 403 退回手填；隐掉的条件格有值时显值 + 「随载荷照送」+ 清空键 + 服务端问题）。共享纯加行：`publication-draft-api.ts` 三型 + `pricePolicy?` 一格、`presentation.ts` 加 `taxDispositionLabels` / `planBindingConversionLabels`、`CommercialPoliciesPage.tsx` 一签 + 一 import |
| `6bfd2f1b` | **地盘外、单独成笔**：读面口径列。`party/api.ts` `PricePolicyRecord` 加 `caliberDeclared` / `caliber`（镜像 Go `query_commercial_policies.go` `pricePolicyBody` 既有键）；`policy-rows.ts` PRICE_POLICY 加一列「计价口径（税务;体积;汇率）」，`pricePolicyCaliberCell` 三态（未登记 / 已登记三段各显真话 / 布尔说已登记而节缺了点名为响应不合契约）；`policy-rows.test.ts` 一例。全部纯加行 |
| `c7635837` | 机制清点在 `6bfd2f1b` 干净检出重生成（PC 文件面 +2 生产 / +3 测试） |

**完成判据逐项**：

1. 商业价格政策册旁多一签「发布价格政策版本」：表单 → 预览摘要 → 存为待批准 → 批准 → 发布五步由公共半边 `PublicationDraftFlow` 走；载体到达发布那一步 `onPublished` → `notePublished('PRICE_POLICY')`，读面切到商业价格政策册并重读——`c22218b4`。今天四口挂 `UnconfiguredIntake{}`，页面如实显示 403（ADR-0085 两阶段），不假装可用。**含口径列**：读面此前没有口径列（后端 `pricePolicyBody` 早已透 `caliberDeclared` / `caliber`，前端 `PricePolicyRecord` 未镜像），`6bfd2f1b` 补上——派单地盘未列 `party/api.ts` / `policy-rows.ts`，单独成笔，推送方可整笔取舍。
2. tsc / run-tests 绿——见验证强度。
3. Go 侧本册规范化一格——`b0942646`；要真正「接进同一号」还需应用层两个方向（`57341038`）与载荷一格（`3c451050`），三笔合为本册的机制半边（同票 11 的形状）。

**硬句在场**：表单不算摘要、不收也不送批准人（`price-policy-form.test.ts` 钉载荷里无 tenant / submitter / approver / contentDigest / approval / canonicalization）；预览与录入同一份载荷、同一段解码、同一处算摘要（`TestPricePolicyPreviewAndSubmissionShareTheDigest`）；表单不裁任何门——连「必填」都不拦，不给 localProblems（全文本格）；**口径随正文同笔**：载荷永远带 `caliber` 节（CONTEXT「商业价格规则必须声明含税、未税或税务不适用」），不给「先发正文、回头补口径」的两步，汇率节按三格全空 / 任一非空整节缺席 / 在场（`declaresFx`）。

**选形落地**：`pricingPlan` 从价卡目录选取 `planId@planVersion`（复用票 11 的 `planReferenceOf`），目录行显方向 / 用途 / 范围只给人看、不按方向过滤、不预选；`planDirection` 与 `conversion` 如实收——操作者照目录所见填方案方向，表单不从选中方案反推、`conversion` 空串不代填 NONE（服务端按集合外点名；ADR-0057）。口径节按 `taxDisposition` / `direction` 显隐条件格（`showsTaxClassification` / `showsVolumetricFactor`），**显隐是呈现不是裁门**：隐掉的格草稿里有值照送，组件显值 + 清空键 + 服务端答在那一格的问题；服务端把两条件格的在场规则答在条件格上（`pricePolicy.caliber.taxClassification` / `.volumetricFactor`），跨格的绑定矩阵（SELL 绑 BUY 写 NONE）以预览成因一句回来。

**领域取舍一条（写明供评审核）**：`PricePolicyBody.validate` 把 `checkPlanBinding` 与口径方向一致也算进规范化前的门（不只逐格非零），理由是客户合同那一节的先例「折成文档前过与发布时相同的门，预览要在录入之前就把恰一答给操作者」；后果是发布用例上两条既有反向用例从 error 变成`未受理`带同一成因（`57341038` 注释写明），`NewCommercialPricePolicy` / `ConsistentWithDirection` 两道门仍守装载面（ADR-0057 Decision 二）与绕开规范化的调用方。

**共享文件各改了哪几处（全部纯加行）**：`domain/publication_canonicalization.go`（+20/−0：版本号注释一行、`PublicationContent` 一格、kind 不符一句 + switch 一支、`IsRegisterCanonicalized` 一册、`canonicalPublicationDocument` 一格、`Rehydrate` 一支）；`application/publish_commercial_authority.go`（+22/−0）；`application/publication_draft.go`（+19/−0）；`adapters/http/publication_draft_payload.go`（+6/−0）；`party/publication-draft-api.ts`（+36/−0）；`party/presentation.ts`（+14/−0）；`party/CommercialPoliciesPage.tsx`（+9/−0）；`party/api.ts`（+15/−0）；`party/policy-rows.ts`（+19/−0）。测试文件 `application/publish_commercial_authority_test.go`（+49/−15）是甲路下必然的一改；`policy-rows.test.ts`（+69/−1，那 1 行是 import 加一个名字）。**未动**：`endpoints.go` / `ports.go` / 0010 / 0022 / seed（`publish-batch.json` 的 `SYN-PRICE-RULE-CN-SG` 只有壳无 `declarations`，对账门对无正文的壳不开）/ `party/api.ts` 其余 / 公共半边三件。

**验证强度（钉 `6bfd2f1b`，隔离 detached 检出 `%TEMP%\idp-verify-awf14`，验后已拆；`c7635837` 只动清点 .md）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 全仓退 0；`go test -count=1` 动过的三包 + `go list` 反查的反向依赖（`cmd/parcel-api` / `parcel-commercial` / `parcel-dispatch`、四个 `*/adapters/partycommercial`、`parcelshipment/adapters/postgres`、`partycommercial/adapters/postgres`）+ `./internal/architecture/...` 共 15 包全 ok——**未设 DSN，PG 用例跳过**（本票 `.sql` 零变动、不动 `adapters/postgres`，按 parallel-sessions「无 postgres 改动不带 DSN」）；admin-web `tsc --noEmit` 退 0、`run-tests` 127 / 127（含本票 12 条）；机制清点在 `6bfd2f1b` 干净检出重生成为 `c7635837`，在 `c7635837` 上再生成零差。证据层级 **S**（隔离合成）。**没有端到端**：四口挂 `UnconfiguredIntake{}`，五步走通只由状态机测试 + 类型检查证。main 自 `5a209f70` 至 `0f9eaa1e`（20:2x）只前进 `.scratch` 三笔，与本分支无代码文件重叠。

**未做（各归其票）**：`bindingConversion` 读面列仍显原词（`planBindingConversionLabels` 已有，读面套词表归读面票）；`ReferencePicker` 与 `normalizeMoment` 抬到共享层（票 11 / 16 评审已记的判断题，本票各自就地复用：`planReferenceOf` 从票 11 文件导入、`normalizeMoment` 从票 16 文件导入、价卡选单自写一份 `PriceCardPicker`）；四口点亮归 ADR-0100 操作者接入渠道（机制半边待接线）；旧式 `sha256:` 声明串何时开始拒收（伞票收口时裁）。

**自审（`/code-review` 两轴，基线 `5a209f70`；子代理未起，作者串行自查）**：无阻断。Standards 轴核过：领域包新文件只导入 `fmt` / `time`；注释中文、跨文件引用无行号无计数；不给默认（封闭集不预选、`conversion` 不代填、fx 不代填）；`PriceDirectionNamed` 三个反查名单仍在 `String()` 一处。Spec 轴核过：完成判据三条各有落点；边界两条守住（0010 / 0022 未动、方案内容未读——只传引用串）。非作者评审由通道 1 派，结论写「进 main 记录」。

## Comments

- **评审 ← 通道 5 · 钉 `44f2b79d` · 21:52**（非作者；基线 `5a209f70`，只评 `b0942646..6bfd2f1b` 五笔代码；隔离 detached 检出验后已拆，junction 先 rmdir。）
  - **Standards · 阻断：无。非阻断 2**：(1) `party/policy-rows.test.ts` `pricePolicies` 夹具头注「照后端 query_commercial_catalogue_test.go 里价格政策册**那三行**的形状」
    是跨文件计数（AGENTS「写代码注释」：不用行号也不用计数），Go 侧那份夹具增减一行此句无声变旧；去掉数字即可。(2) 判断题：`PricePolicyPublicationForm.tsx`
    `PriceCardPicker` 与 `SupplierAgreementPublicationForm` 的 `ReferencePicker` 同形（作者头注已写明不跨文件借私有件）；`price-policy-form.ts` 从
    `credit-policy-form.ts` 借 `normalizeMoment`、组件从 `supplier-agreement-form.ts` 借 `planReferenceOf`——完成记录已列「抬到共享层」归后续（票 11/16 评审同记），不挡。
    **无发现**：领域新文件只导入 fmt/time；共享六件 + 地盘外三件（`api.ts` / `policy-rows.ts` / `presentation.ts`）逐 hunk 纯加行、无既有行被改；
    `publish_commercial_authority_test.go` 两条反向用例由 `errors.Is(err)` 改为 `Outcome()==NOT_ACCEPTED && errors.Is(RefusalCause())` 同一成因，「registry 一行不写」
    断言保留未削弱；注释中文、无行号；封闭集不预选、`PlanBindingConversionNamed` 空串答 false 不代填 NONE、fx 不代填。
  - **Spec · 阻断：无。非阻断 1**：`price-policy-form.ts` `withShellCopiedIntoPolicy` + 组件「范围与区间从版本壳带入」按钮——票面未要（scope creep 一格）；但它是显式
    动作抄文本、非默认，服务端仍逐格判，与票 11 `withShellCopiedIntoAgreement` 同形，记录即可。**无发现**：完成判据三条各有落点——`CommercialPoliciesPage.tsx` 一签 →
    `PublicationDraftFlow kind="PRICE_RULE"` → `notePublished('PRICE_POLICY')` 切册重读；口径列 `pricePolicyCaliberCell` 三态；Go 三层 `canonicalizePricePolicy` /
    `publicationContentOf` case / `PricePolicyBodyPayload.body`。硬句：`price-policy-form.test.ts`「载荷里没有身份也没有摘要」钉 tenant/submitter/approver/contentDigest/
    approval/canonicalization 全缺席，无 localProblems。①显隐是呈现：`pricePolicyPayloadOf` 不读 `shows*`，隐格有值照送；`ConditionalField` 隐且有值显值 +「清空」+ 服务端问题；
    服务端 `PricePolicyCaliberPayload.body` 把在场规则答在条件格 `taxClassification` / `volumetricFactor`。②`canonicalPricePolicyBody` 结构体钉字段序、UTC RFC3339；
    `PricePolicyBody.validate` 过 `checkPlanBinding` + 口径方向一致，预览答 `NOT_ACCEPTED` 带成因；`body()` 折回再 validate。③方案来源 `listPriceCards()`（pricing 目录读口），
    只传 `planReferenceOf` 引用串，不按方向过滤、不预选，403 退手填。⑤自验数字逐件对上 `diff --stat`；口径同笔：`caliber` 永随正文，fx 按 `declaresFx` 全空缺席 / 任一非空整节送。
    边界：无迁移文件，0010 / 0022 未动。评审树自跑 `go test -count=1` PC 四包 + `cmd/parcel-api` + architecture 全 ok（无 DSN）；tsc 0；run-tests 127/127。
  - **汇总**：两轴无阻断；Standards 非阻断 2、Spec 非阻断 1。可重放。
- **评审（第二份，重复派单）← 通道 4 · 钉 `44f2b79d` · 2026-09-09 11:06**：09-09 接手的推送方先读分支票面、没读回放树里未提交的上一条，误判本票无评审
  而再派；撤回令到时通道 4 已完工，报告有效、与通道 5 同 tip 同结论。**Standards · 阻断无，非阻断 2**：(1) Duplicated Code——`PricePolicyBody.validate`
  逐条重写 `NewCommercialPricePolicy` 的逐格 valid() 与 `checkPlanBinding`，`PricePolicyCaliberBody.validate` 同样重写 `NewPricePolicyCaliber` +
  `ConsistentWithDirection` 的判据；缺拥有版本因而不能直接调构造门，理由成立，但两份名单今后各改各的会静默分叉，可抽一个无版本的
  `validatePricePolicyBody` 供两处共用，另立票；(2) `publication_draft_payload_price_policy.go`、`publication-draft-api.ts`、`price-policy-form.ts` 三处
  写「0010 正文七格」，数的是 0010 的列（AGENTS「不用计数」），改「0010 正文各格」即可。**Spec · 阻断无**；票面点名的领域取舍——与 ADR-0057 Decision 二
  一致、非绕过：Decision 二禁的是「绕过绑定校验的重建门」，`validate` 跑的正是同一个 `checkPlanBinding`，快照折回 `canonicalPricePolicyBody.body()` 末尾
  再过一遍 `validate`（`corruptedBinding` / `corruptedCaliber` 钉住），装载面 `SavePricePolicy` / `NewCommercialPricePolicy` 一行未动；两条反向用例
  error → `未受理` 是既有对账门契约作用到新册，仍断言零写入、成因不变。非阻断 1：外层结果词 `未受理` vs 词表 `适用冲突`，与信用册同形，归伞票收口。

## 进 main 记录（推送方通道 1；重放 2026-09-08 22:0x，验证与推送 2026-09-09——前一任会话断在提交簿记之前；与 awf/15、awf/12 同一条链、同一次全量验证）

- 重放：隔离 detached 树 `%TEMP%\idp-replay-awf1514` 基 main `6b1e0d63`（awf/17 已在），先重放 awf/15 八笔再重放本票 `5a209f70..44f2b79d` 跳清点笔 `c7635837`
  后七笔：`c3695d2a→7eb53cd3`、`b0942646→b8b257be`、`57341038→2c6acacc`、`3c451050→9b753d84`、`c22218b4→e41b2185`、`6bfd2f1b→6af8fad1`、`44f2b79d→44008e42`。
  五份共享文件（`publication_canonicalization.go`、`publish_commercial_authority.go`、`publication_draft.go`、`publication_draft_payload.go`、`publication-draft-api.ts`）
  与 `CommercialPoliciesPage.tsx` 各撞一次 17 + 15 的相邻加行，**逐 hunk 手工并**（各册的 `if` / `case` 各自闭合，不用 union 拼接——那样会把后一册的 `if` 套进前一册的块里，
  能编过但语义错），每份并完核过「= HEAD + 本票自己的加行、零删行、加行多重集与分支上那笔逐行相等」；重放 tip 对分支 tip 的代码差恰为 17 + 15 + 20 三票的文件集，无多无少。
- 清点在链 tip 重生成 `ef6cb914`（PC 生产 109→113 / 测试 114→120、声明面 21→23，两票合计）；分支清点笔 `c7635837` 量的是 `5a209f70` 上的数，不重放。
  09-09 接手会话用 `git range-diff 5a209f70..44f2b79d 6de76d44..44008e42` 复核前任的重放：非共享文件各笔 `=`，共享文件各笔 `!` 且差异全在上下文行，加行逐字相等。
- 09-09 复验钉 `ef6cb914`：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；admin-web `tsc --noEmit` 退 0、`run-tests` 151/151；含 DSN `go test -p 1 -count=1 ./...`
  **99 ok / 1 FAIL / 16 无测试 / 0 cached**（555s）——红的一条属 awf/15（`cmd/parcel-commercial` 夹具旧式摘要，见票 15「进 main 记录」），本票无关。
  之后 awf/12 七笔续在同一条链上（见票 12「进 main 记录」）；三票共一次含 DSN 全量钉最终代码 tip，结果见 tasks.md 2026-09-09 本节。
- 本笔（票面 + 伞票 07 子票表 + tasks.md）之后 `--ff-only` 进共享 main。分支 `mcp2-awf14` 内容已全在 main，指针改名 `merged/`，树 `D:/tops/idp-parcel-mcp2-awf14` 归通道 2 自拆。
