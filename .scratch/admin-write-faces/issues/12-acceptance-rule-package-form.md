# 12 `ACCEPTANCE_RULE_PACKAGE` 版本的运营主路径：分节表单——正文一节、每条声明通道各一节，空节即未声明

Category: enhancement
Status: resolved——通道 2 于 2026-09-08 21:3x 从 `mcp3-awf12@febfcd2e` 接手（通道 3 会话 crash，通道 1 派单 task-2b69f06d「接着做，不另起」）、21:5x 完工，分支 `mcp2-awf12`（树 `D:/tops/idp-parcel-mcp2-awf12`，基 origin/main `f722d9f0`；通道 3 的五笔原样 rebase 到其上，新旧 SHA 见「完成记录」），逐笔 SHA 见「完成记录」，进 main 记录待推送方广播后补入。此前 in-progress——2026-09-08 20:0x 通道 3 认领（通道 1 派单 task-8325f4ee；分支 `mcp3-awf12`，隔离树 `D:/tops/idp-parcel-mcp3-awf12`，基 main `c135049b`；20 的落点已广播，Go 先做、表单等通道 4 推出 `publication-draft-api.ts` 词表那一笔后借入）。此前 ready-for-agent——形状已裁清（分节逐字段表单，空节即未声明；伞票点名的「先答声明随发布怎么在表单里表达」在本票「选形与理由」答，无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面并改正 pc-gaps/09、10 两票落地后各加什么，见伞票 Comments）
Blocked by: 08（已进 main）；[20](./20-publication-vocabulary-read-face.md)（词表读口公共半边，2026-09-08 通道 1 代裁立票；落点广播前表单里的下拉先按票面写成占位、不内置枚举）

## 册与载荷

显示在**政策页·接单规则包册**。这是十类里声明通道最多的一类，`declarations` 里归它的有：`rulePackageBody{serviceProduct,
contract, legalEntity, scope, effective…, rules[{category, reference}]}`（0014 正文）、`asOfPolicies[{judgment, semantics,
policyVersion}]`（0005 时点锚）、`acceptanceContent{applicableGroups[], manualReview}`（接单规则正文声明）、
`pendingRoutingBasis`、`intakeQualification{sources[], qualifications[]}`（0013 收寄资格）、`finalRules[{outcome, finalKind}]`
（0013 终局规则）。pc-gaps/09 **已落地**（ADR-0119，迁移 0026）：终局规则节多一格 `finalRuleValidity{anchor, duration}`——`anchor`
是封闭集下拉（首发只有 `CHANNEL_RESULT_OBSERVED`，由服务端词表读口供）、`duration` 收 ISO-8601 子集 `P[nD][T[nH][nM][nS]]`
（年 / 月 / 周不收），整格留空 = 未声明有效期（不失效），表单不给默认时长；它仍随 `FinalRuleChannel` 同一通道、**不是**新通道，
且只填这一格不填 `finalRules` 行服务端整项拒。pc-gaps/10 **已落地**（ADR-0120，迁移 0027，新通道 `SOURCE_DATA_AMENDMENT`）：
多一节 `sourceDataAmendment{closed, rules[{dataGroup, stage, intent, allowance}]}`——`closed` 是必填的一格布尔（false = 缺格转复核、
true = 缺格即不允许，表单不给默认、不预选）；`rules` 是一张几行的表，`stage` 六格与 `intent` 三格由服务端词表读口供下拉（词是
`parcel-shipment` 原词，表单不内置）、`allowance` 只有 `ALLOWED` / `DISALLOWED` 两值（没有「未声明」这一项——那是缺格的读法，
不是一行能选的值）、`dataGroup` 是开放串；`closed=true` 时表可以留空（这一版什么都不许改），`closed=false` 时零行服务端整项拒；
整节留空 = 未声明本族。

## 选形与理由（ADR-0101 决定八）

**分节的逐字段表单，不走模板导入。** 频次低、配置员操作；载荷虽宽但每一节都是「几格 + 一张几行的表」，没有一节
是矩阵——模板导入是为上百格的价卡设计的（ADR-0101 Alternatives 第二条），拿它装几行声明是把工具用错对象。

**声明随发布怎么表达**（伞票要先答的那一格）：一版规则包的发布是**一次**提交，正文与全部声明在同一份载荷里、同一
个摘要下——这是 ADR-0042/0058 归属纪律与「声明只能随发布登记」的落法，表单不改它。所以表单是一份、分节：正文
一节 + 每条声明通道一节；**某一节整节留空 = 该通道未声明**（消费方照旧译 `NotDeclared` / `未配置`），表单不给
任何一节默认值、不把「留空」写成「无」。每一节内的封闭集（判断类型、来源、终局种类等）由服务端词表读口供
下拉，表单不内置。pc-gaps/10 的新通道落地时是**加一节**、pc-gaps/09 落地时是终局规则节**加一格**，都不改本票的形。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：`manualReview` 那一格答的是「要不要人工复核」（接单规则正文），与
`ManualReviewRequirementFor`「谁有权」那一问是两件（wiring-baseline-remainder/04），表单只收前者。

## 完成判据

政策页接单规则包册旁多一签「发布规则包版本」（分节表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻
可见、各节声明在册上各自的列里可见；tsc / run-tests 绿；Go 侧只加本册规范化一格（覆盖正文与全部声明通道）。

## 边界

不动任何声明表；不改「声明只能随发布」；pc-gaps/09 的新格与 pc-gaps/10 的新节在那两票落地后由它们自己加。

## 完成记录（通道 2 接续通道 3，2026-09-08，分支 `mcp2-awf12` 基 origin/main `f722d9f0`）

接手方式：通道 3 会话 crash 时分支 `mcp3-awf12` 停在 `febfcd2e`（树干净、零现场），通道 1 派单「从 `febfcd2e` 接着做，不另起」。
为借入 awf/20 的 `fetchPublicationVocabulary`，通道 3 的五笔原样 rebase 到 origin/main `f722d9f0` 之上（与 main 新进的笔零文件重叠、
无冲突；只改本分支的 SHA，`mcp3-awf12` 指针原样留作封存出处）。

逐笔（分支上的 SHA；进 main 后由推送方广播新旧对照）：

- 通道 3 的五笔，rebase 前 → 后：`6de32983`→`3c339029` 票面 in-progress；`b1043265`→`94df574c` 领域（新文件
  `publication_canonicalization_acceptance_rule_package.go` +_test：0014 正文 + 六节声明折进同一个 PCC-1、键名镜像批文，十个导出
  `*Named` 反查与 ISO-8601 时长子集解析）；`3f80aea9`→`d8ba1669` 应用（对账门对带正文的项开门、载体正文折回各声明通道）；
  `00c640ca`→`3f07466c` http（新文件 `publication_draft_payload_acceptance_rule_package.go` +_test：一格分节、逐格问题带下标）；
  `febfcd2e`→`b439a3a0` **地盘外** seed（SYN-RULEPKG-01 `contentDigest` 改为算出的 PCC-1）。
- `9a473cf0` admin-web（通道 2）：新文件 `party/acceptance-rule-package-form.ts`（+.test.ts，十例）与
  `party/AcceptanceRulePackagePublicationForm.tsx`；`publication-draft-api.ts` 只加 `AcceptanceRulePackageBodyPayload` 一族型 + 载荷上
  `acceptanceRulePackage?` 一格（键名以 http 那笔的 Go json 标签为准）；`CommercialPoliciesPage.tsx` 只加一签「发布规则包版本」+ 一个 import。
  - **空节即未声明**落成 `sectionDeclared`：某一节里没有任何一格有字、任何一行，载荷就不带那一节；节里有任何东西整节原样送
    （空行、没选的码都不代判、不补默认）。空草稿六节全留空、正文带一行空规则等人填；每节标题旁写着此刻「留空 → 未声明」还是
    「将随发布登记」（从草稿算出的事实），可「清空本节」回到未声明。
  - 封闭集十集（`category` / `judgment` / `applicableGroups` / `manualReview` / `sources` / `outcome` / `anchor` / `stage` / `intent` /
    `allowance`）一口从 `fetchPublicationVocabulary('ACCEPTANCE_RULE_PACKAGE')` 取；下拉首项「未选」、多选无一格默认勾上；读口 403
    显占位、不内置码。码 → 本页中文的表放在 form.ts（收寄来源、责任结果沿用 `presentation.ts` 既有两表；其余八集今天只有本表单在用，
    读面要用时再抬进 `presentation.ts`——本票没占那份文件）。
  - `sourceDataAmendment.closed` 三态按钮，没选就不送这一格（服务端答「须在场」）；`finalRuleValidity` 作终局规则节内一格，整格留空 =
    未声明有效期，不给默认时长；`manualReview` 下拉标签「要求 / 不要求人工复核」，说明句点明不是「谁有权」。
  - 认领的每条 JSON 路径组件都渲染了问题（awf/17 评审点名的「认领节根不渲染」在这里避掉）；草稿更新一律函数式。
- `9c7e6da6` 机制清点在 `9a473cf0` 干净检出上重生成——PC 生产 +2 / 测试 +3 / adapters/http 生产文件 +1（数的是通道 3 那三笔 Go，
  端点表本票一行未加）。只作取证，推送方在 tip 上重生成兑底。

自验（`9a473cf0` 的 detached 检出 `%TEMP%\idp-verify-awf12`，未设 DSN——本票无 postgres 改动）：`gofmt -l .` 列出 0；`go build ./...`
退 0；`go vet ./...` 退 0；`go test -count=1` PC 领域的 14 个反向依赖包（`go list` 反查，含三个 `cmd/`）+ `./internal/architecture/...`
全部 `ok`；admin-web `tsc --noEmit` 退 0、`run-tests` 133/133（本票 10 例在内）。

对照票面：分节表单 → 预览摘要 → 待批准 → 批准 → 发布由公共半边走；「结果在同册立刻可见、各节声明在册上各自的列里可见」靠
`onPublished` 刷读面（`policy-rows.ts` 未动，列是既有的）；「Go 侧只加本册规范化一格（覆盖正文与全部声明通道）」是通道 3 那笔领域
改动；边界「不动任何声明表；不改『声明只能随发布』」——声明表与迁移一行未动，表单是一次提交。

## Comments

- 2026-09-08 21:3x · 通道 1 → 通道 2：通道 3 会话 crash，派「从 `mcp3-awf12@febfcd2e` 接着做，不另起」（task-2b69f06d，与 awf/17 评审同一单）。
- 2026-09-08 21:5x · 通道 2：完工报已发通道 1，评审待通道 1 另派；本票 `publication-draft-api.ts` / `CommercialPoliciesPage.tsx` 两处
  共享文件改动与第 2 波各票各自加的一格落在同一段落，撞了由推送方按「纯加行」解。**留给评审 / 后续的判断项**：八集中文表放在
  form.ts 而非 `presentation.ts`（避免碰未占号的共享文件）；`Field` / `Problems` / `RowFrame` 与兄弟表单同形复制，第 2 波落齐后可
  单开一票抽到 `PublicationDraftFlow` 旁。
- **评审 ← 通道 3 · 钉 `76c35c4a` · 2026-09-09 11:03**（非作者；基线 `f722d9f0` = merge-base main；隔离树 `%TEMP%\idp-review-awf12` 只读；
  评 `94df574c` / `d8ba1669` / `3f07466c` / `9a473cf0` + 地盘外 `b439a3a0`，三笔纯 .md 未评。两轴隔离子代理时限内未返回，报告为直读；三份 `_test.go` 与
  TSX / HTTP 适配器全文未逐行。）
  - **Standards · 阻断：无。** 直读 `publication_canonicalization_acceptance_rule_package.go`（`declare` / `canonicalAcceptanceRulePackageBodyOf`）、
    `publish_commercial_authority.go`（`publicationContentOf` 新支 / `acceptanceRulePackageBodyOf`）、`publication_draft.go`（`declarationsOfContent`）、
    `publication_draft_payload*.go`、form.ts 全文、TSX 关键节、api / page diff：注释全中文、只写取舍；跨文件引用用符号名 / 引文，未见行号或计数；
    无默认值（`closed *bool` 缺格拒「须在场」、`CodeSelect` 首项「未选」、多选无预勾、`emptyAcceptanceRulePackageDraft` 六节全空）；封闭集码全部来自
    `fetchPublicationVocabulary`（Go `PublicationVocabulary` 十集同名已在 main），form.ts 只有码 → 中文表；规范化文档排序确定（`closedCodesOf` 序 +
    引用 / 组排序）；构造门复用 `canonicalizationOwner` 占位、不抄规则；对账门与客户合同支同判据（正文缺席放行 = ADR-0126 既有语义，非本票引入）。
    判断项：Duplicated Code——`acceptanceRulePackageBodyOf` 与 `declarationsOfContent` 互为镜像（作者注释已认「同笔改」）；TSX `Field` / `Problems` /
    `RowFrame` 与兄弟表单同形。
  - **Spec · 阻断：无。非阻断 3**：(1) 语义疑——form.ts `declarationSections` / `sectionDeclared('finalRuleValidity')` 与 TSX `FinalRuleFields` /
    `DeclarationSectionFrame` 的 `clear`：票面「多一格不是多一节」「仍随 FinalRuleChannel 同一通道」，纯逻辑却把有效期立为独立 DeclarationSection；
    终局规则节「清空本节」只清 `finalRules` 不清有效期格 → 节头显「留空 → 未声明」而载荷仍带 `finalRuleValidity` → 服务端 `declare` 整项拒。不变式由
    服务端守住故非阻断；可改：清空同清有效期、节头 declared 含有效期格。(2) 票面错——「册与载荷」把 `pendingRoutingBasis` 列进归本册声明，与领域
    `DeclarePendingRoutingPermission`「规则包声明不了它」相反；代码（`acceptanceRulePackageBodyOf` 注释、`publication-draft-api.ts`）正确排除。改票面
    文字，非代码问题。(3) 未验——`b439a3a0` seed SYN-RULEPKG-01 的 `contentDigest` 串未由评审跑 Go 核与 `CanonicalizePublicationContent` 算出一致，
    由推送方全仓测试兑底。
  - **作者判断项结论**：八集中文表放 form.ts——放行（票面已定「码 → 本页中文的表放在 form.ts」，`presentation.ts` 未占号）；`Field` / `Problems` /
    `RowFrame` 同形复制——放行、另立票（第 2 波落齐后抽到 `PublicationDraftFlow` 旁）。
  - **汇总**：两轴无阻断；Spec 非阻断 3。可重放。
- 2026-09-09 · 推送方核评审非阻断 (2)：票面「册与载荷」那句「`pendingRoutingBasis`」列错了归属，代码正确；票面文字留给伞票 07 收口时一并改，本票不再动
  正文（正文是通道 3 / 通道 2 写的口径，推送方只加簿记）。推送方替 (3) 兑底：重放 tip 上 `cmd/parcel-commercial` 带 DSN `ok`（seed 的 SYN-RULEPKG-01
  在真库发布用例里过对账门）。

## 进 main 记录（推送方通道 1，2026-09-09；与 awf/15 + awf/14 同一条链、同一次全量验证）

- 重放：隔离 detached 树 `%TEMP%\idp-replay-awf1514`，链上先是 15 八笔 + 14 七笔（基 main `6b1e0d63`，见那两票「进 main 记录」）与清点笔 `ef6cb914`，
  本票 `f722d9f0..76c35c4a` 跳清点笔 `9c7e6da6` 后七笔续在其后：`3c339029→2c05a8a4`、`94df574c→2cb23a35`、`d8ba1669→768ca5cc`、`3f07466c→233ac1b1`、
  `b439a3a0→36cd0b18`、`9a473cf0→85d8efdd`、`76c35c4a→2c823378`。六份共享文件（`publication_canonicalization.go`、`publication_draft.go`、
  `publish_commercial_authority.go`、`publication_draft_payload.go`、`publication-draft-api.ts`、`CommercialPoliciesPage.tsx`）各撞一次 17 / 15 / 14 的
  相邻加行，**逐 hunk 手工并**（各册的 `if` / `case` / `<TabsContent>` 各自闭合；`publication-draft-api.ts` 上本票的接口块落在 14 的 `FxCaliberPayload`
  之后，纯加块换位），每份并完核过「零删行、加行多重集与分支上那笔逐行相等」（`Compare-Object` 空）；重放完时链 tip 对分支 tip 的代码差恰为
  17 / 15 / 14 / 20 的文件集加 `publish_commercial_authority_test.go`（那几票改配的「没接的册」样例）与 `publish-batch.json`（15 / 17 的 seed 摘要），无多无少。
- 清点在链 tip 重生成 `6edabc5c`（PC 生产 113→115 / 测试 120→123、http 适配器 23→24）；分支清点笔 `9c7e6da6` 量的是 `f722d9f0` 上的数，不重放。
- 验证：见 tasks.md 2026-09-09 本节（三票共一次含 DSN 全量，钉最终代码 tip）。
- 本笔之后 `--ff-only` 进共享 main。分支 `mcp2-awf12` 内容已全在 main，指针改名 `merged/`，树 `D:/tops/idp-parcel-mcp2-awf12` 归通道 2 自拆；
  `mcp3-awf12` 是被接手的旧来源（与 `salvage/mcp4-awf12` 同 SHA `febfcd2e`），改名 `salvage/`。
