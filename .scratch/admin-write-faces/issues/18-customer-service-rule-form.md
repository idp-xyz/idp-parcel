# 18 `CUSTOMER_SERVICE_RULE` 版本的运营主路径：逐字段表单（适用对象恰一 + 期限表 + 材料表）——先等它的读面

Category: enhancement
Status: resolved——2026-09-09 17:2x 通道 4（接续 5，接续单 task-e62ba34f；分支 `mcp3-awf21` 基 main `74ef0da8`、紧接票 21 的 `4de2d30a`，tip 见完成记录）：管理台客户服务规则册旁多一签「发布客户服务规则版本」，Go 侧本册接进 PCC-1 一格（领域规范化 + 应用两向 + 载荷一格 + cmd 夹具换算出的串），tsc 0 / run-tests 188 pass；进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-09 17:0x 通道 4 接续 5（16:3x 会话中断；Go 半边三笔 `e8958fce` / `e33aa81e` / `fc51e407` 已在 origin，树上 4 份 admin-web 未提交按「接手别人在途产出」先自列判据再对照 diff，对得上接着写，见完成记录「接手对照结论」）。此前 in-progress——2026-09-09 16:2x 通道 4 接续（通道 3 会话 16:0x crash，接续单 task-b061d8a9；同树 `D:/tops/idp-parcel-mcp3-awf21` 同分支 `mcp3-awf21` 往上做，树上 16 份未提交按 parallel-sessions「接手别人在途产出」先自列判据再对照 diff）。此前 2026-09-09 14:2x 通道 3 认领（接续单 task-134c240c，同分支 `mcp3-awf21` 基 `74ef0da8`，紧接票 21 的 `4f956057`）；同笔经 ready-for-agent（票 21 判据 5：读面 resolved，本票 Blocked by 已无未落项）。此前 draft——形状已裁清（逐字段表单 + 两张子表可加行），但**等一件裁决**：管理台要不要先立并落客户服务规则册（读面），票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 完成记录末尾已交 MCP-1 定；那张读面票立了且 resolved、本票补上它的编号进 Blocked by 之后，才转 ready-for-agent。伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 复核：`apps/admin-web/src` 里 `CUSTOMER_SERVICE_RULE` 仍零命中）。伞票原写九类，`CommercialObjectKind` 在 `95182b9d` 上已是十类（pc-gaps/04 把客户服务规则版本纳入封闭集），本票补第十册
Blocked by: 无——08（已进 main）；[21](./21-customer-service-rule-register-read-face.md)（管理台客户服务规则册读面，2026-09-09 通道 1 代裁立票——用户授权自决；已 resolved 于 `mcp3-awf21@4f956057`，main 上的 SHA 待通道 1 重放）；[20](./20-publication-vocabulary-read-face.md)（词表读口，已进 main）

## 册与载荷

后端读面是第八册 `?kind=CUSTOMER_SERVICE_RULE`（0023，ADR-0104），**管理台今天没有这本册**。`declarations.
customerServiceRuleBody{serviceProduct | customerContract, responsible, scope, claimDeadlines[{kind, startEvent, days,
calendar}], minimumMaterials[{claimKind, materials[]}]}`：适用对象恰一（产品或合同）、两张子表至少一项有内容。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，两张子表可加行。** 频次低、配置员操作、正文是两个引用 + 两张几行的表，不是矩阵。适用对象用二选一
控件呈现、恰一由服务端裁；期限种类与起算事件的封闭集由服务端词表读口供。（落地时改口：起算事件在领域里是开放引用
`DeadlineStartEventReference`，解释权在 visibility-exception，词表读口对本册只答 `kind` 一集——`e8958fce` 的提交信与
`publication_vocabulary.go` 本册那一支写明了理由；表单把起算事件当引用串手填。见完成记录「接手对照结论」。）

**为什么多一条 Blocked by**：伞票落点判据是「写签跟着读签走」（票 03）。读面不在，写签摆不到任何一页——先立并落
管理台的客户服务规则册（与票 06 的做法同形：`CommercialPolicyKind` 加格、Record、列向、标签、来源提示句），再谈本票。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：VE 那侧已有的两本册（通知义务、索赔类型覆盖）与本册正文归谁，pc-gaps/05 记着要
走 ADR——本票只发布 PC 这一侧的正文，不替那个所有权裁决开口。

## 完成判据

管理台客户服务规则册旁多一签「发布客户服务规则版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻
可见；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0023；不碰 VE。

## 完成记录（2026-09-09，通道 3 一任 + 通道 4 两任会话；分支 `mcp3-awf21`，基 main `74ef0da8`，紧接票 21 的 `4de2d30a`）

**接手链**：通道 3 会话 14:2x 认领并写下 Go 半边（16 份未提交，mtime 15:18–15:54），16:0x crash；用户 16:2x 指定交通道 4，
通道 4 前一任（接续单 task-b061d8a9）按「接手别人在途产出」核过后把 Go 半边分三笔原样提交并推（`e8958fce` / `e33aa81e` /
`fc51e407`），随后写 admin-web 纯逻辑半边到 16:38（4 份未提交）后 16:3x 会话中断；17:0x 起由本会话（通道 4 新会话，无前任
上下文，接续单 task-e62ba34f）从 git 与树上的状态接着做——现场按通道 1 16:4x 钉的清单核：`status --untracked-files=all` /
`diff --numstat` / mtime 与派单逐一对上，未凭任何自报，未 checkout / stash / reset。

**逐笔（分支 SHA；main 上的 SHA 由推送方重放后补）**：

| 分支 SHA | 内容 | 会话 |
|---|---|---|
| `e8958fce` | 领域：`publication_canonicalization_customer_service_rule.go` + 测试（正文输入面、镜像批文的规范化文档、折回、`ClaimDeadlineKindNamed` 走 `closedCodeNamed`）；`publication_canonicalization.go` 一格 + `CommercialObjectKindNamed` 改委托 `closedCodeNamed`（行为不变，属票 24 判据 1 的一格，前任顺手做了，接手方不为「干净」撤回）；`publication_vocabulary.go` 本册只 `kind` 一集；`customer_service_rule.go` 把跨行的门抽成 `filedCustomerServiceRuleItems`、两处排序抽成 helper（错误值与判定顺序一字不变，既有测试未改） | 通道 3 写、通道 4 前任核后提 |
| `e33aa81e` | 应用两向：`publicationContentOf` 一 case（对账门对本册开门）+ `declarationsOfContent` 一支（载体正文折回 `CustomerServiceRuleBodyDeclaration`）；载荷 `publication_draft_payload_customer_service_rule.go` + `publication_draft_payload.go` 一格；新测试 `publication_customer_service_rule_test.go` / `publication_draft_payload_customer_service_rule_test.go`；既有用例的壳改配算出的摘要 | 同上 |
| `fc51e407` | `cmd/parcel-commercial/publish_batch_test.go` 的 `customerServiceRuleBatchBody` 壳上摘要由 `sha256:csr-1` 换成算出的 PCC-1 串，两个变体各一份（13 / 15 都在这里红过，照 13 的 `e58a085a`） | 通道 4 前任 |
| `c31f3fda` | Web 纯逻辑 `customer-service-rule-form.ts` + node:test（11 条）：草稿（壳五格 + 适用对象控件取值与标识 + 责任方 / 规则范围 + 两张表）→ 载荷、认领路径随控件取值与行数走并含行本身与 `applicability`、时长整数格空即缺席 / 非整数文本记本地问题、材料多行文本一行一项、`serviceRuleCodesOf` 按名取词表一集、两张表各自增删改；`publication-draft-api.ts` 加一格 + 三型；`presentation.ts` 加 `claimDeadlineKindLabels`；票面 Status 补接续句 | 通道 4 前任写（16:35–16:38）、本会话核后提 |
| `dbd16d10` | Web 组件 `CustomerServiceRulePublicationForm.tsx` 挂 `PublicationDraftFlow`；`CommercialPoliciesPage.tsx` 一 import + 一签（+8/−0），发布落定 `notePublished('CUSTOMER_SERVICE_RULE')` | 本会话 |
| 本笔 | 票面转 resolved + 完成记录；「选形与理由」里「起算事件的封闭集」半句加落地改口 | 本会话 |

**接手对照结论（admin-web 半边，本会话）**：先按票面完成判据 + 伞票 07 硬句 + Go 侧已提交三笔自列该钉什么，再读 4 份 diff。
自列的八条——载荷键逐字对 `publication_draft_payload_customer_service_rule.go`（`customerServiceRule{serviceProduct | customerContract,
responsible, scope, claimDeadlines[{kind, startEvent, days, calendar}], minimumMaterials[{claimKind, materials[]}]}`）；不给默认不预选；
表单不裁门不算摘要（恰一 / 至少一行 / 至多一行全交服务端）；认领路径覆盖自己能产出的每条键；`kind` 只从词表取、缺集回 null 不回退；
`startEvent` 作开放引用手填；本地只拦编不进 JSON 整数的时长文本；两张表增删改纯函数——diff 逐条对上，另有三处前任比自列更细：
(1) 适用对象选定的标识同一个值进壳上 `references.SERVICE_PRODUCT / CUSTOMER_CONTRACT`，壳与正文的适用一致由服务端
`ConsistentCustomerServiceRuleApplicability` 核；(2) 空草稿两张表**零行**而不是照 13 预开一行——理由写在 `emptyServiceRuleDraft` 注释
（两张表各自可以为空，预开一行替操作者预设「这张表有内容」），与「不预选」同一条红线；(3) 材料清单只认领到项 `materials[j]` 不认领
键本身（服务端对整张清单的话记在行上）。**对判据不对断言条数**：前任 11 条与自列八条切法不同、判据一致；一处派单与 Go 侧的分歧
（派单摘要写「起算事件从词表取」）以 Go 侧为准。原样接用，未重写、未改一字；组件与页面一签由本会话新写。

**完成判据逐项**：

1. 管理台客户服务规则册旁多一签「发布客户服务规则版本」：表单 → 预览摘要 → 存为待批准 → 批准 → 发布五步由公共半边
   `PublicationDraftFlow` 走；载体到达发布那一步 `onPublished` → 页面既有 `PublishedNotice` 切到客户服务规则册（票 21 落的第八册）
   并重读——`dbd16d10`。今天四口与词表口都挂 `UnconfiguredIntake{}`，页面如实显示 403，不假装可用。
2. tsc / run-tests 绿——见验证强度。
3. Go 侧只加本册规范化一格——`e8958fce` 的领域半边；要真正「接进同一号」还需应用层两向与载荷一格（`e33aa81e`）与 cmd 夹具换串
   （`fc51e407`），合为本册的机制半边，读法同 13 / 15 的进 main 记录。**地盘外的两改**（都在 `e8958fce`，完成记录披露）：
   `customer_service_rule.go` 抽 `filedCustomerServiceRuleItems`（同一条 CONTEXT 规则要在发布时与预览时两个时刻答，一处定义）；
   `publication_canonicalization.go` 的 `CommercialObjectKindNamed` 改委托 `closedCodeNamed`（票 24 判据 1 的一格，通道 6 做 24 时
   这一格已就位——推送方转告即可）。

**硬句在场**：表单不算摘要、不收也不送批准人（`customer-service-rule-form.test.ts` 钉载荷里无 tenant / submitter / approver /
contentDigest / approval / canonicalization，正文只有本册一格）；预览与录入同一份载荷、同一段解码、同一处算摘要
（`publication_draft_payload_customer_service_rule_test.go`）；表单不裁任何门——连「必填」都不在本地拦，适用对象恰一、种类集外、
引用在不在册、至少一行、至多一行一律送上去；`localProblems` 只有各行时长编不进 JSON 整数的文本。**本册特有**：只发布 PC 这一侧的
正文，起算事件 / 日历 / 索赔类型 / 材料按引用原词手填、不替 VE 造词（组件各格占位句写明解释归可见性与异常）；VE 那侧两本册与
本册正文归谁未裁（pc-gaps/05），本票未开口。

**选形落地**：适用对象二选一控件（`<select>` 三项：未选 / 随某个服务产品 / 随某个客户合同；取值即载荷键名原词，是表单自己的形状不经
词表）+ 标识一格，未选时标识格禁用并提示先选；期限种类下拉只吃 `fetchPublicationVocabulary('CUSTOMER_SERVICE_RULE')` 的 `kind`
一集 × `claimDeadlineKindLabels`；两张表可加行（`withClaimDeadlineRow*` / `withMinimumMaterialsRow*`），起始零行并各带一句
「这张表可以是空的——只要另一张不也是空的」；材料清单是多行文本一行一项，服务端对 `materials[j]` 逐项的话汇到文本框下带项号；
壳上引用 `references.<KEY>` 被点名时显在适用对象标识格旁并标「壳上引用」。时长 `days?: number` 对 Go `int` 不逐字同形，理由与 13 的
`order` 同（零与缺席在 Go 侧同义，表单不替操作者填 0）。

**共享文件各改了哪几处（数字为 `git diff --numstat 4de2d30a dbd16d10`）**：`apps/admin-web/src/pages/party/publication-draft-api.ts`
（+35/−0，一格 + 三型，排在 13 那一册之后）；`presentation.ts`（+9/−0，`claimDeadlineKindLabels`，排在 `jointPassConditionLabels` 之后）；
`CommercialPoliciesPage.tsx`（+8/−0，一 import 一签，排在接受前财务控制策略签之后、JSON 镜像签之前）；`domain/publication_canonicalization.go`
（+25/−8，一格 + `CommercialObjectKindNamed` 改委托）；`domain/publication_vocabulary.go`（+7/−0）；`domain/customer_service_rule.go`
（+48/−19，helper 抽取）；`application/publish_commercial_authority.go`（+19/−3）；`application/publication_draft.go`（+14/−2）；
`adapters/http/publication_draft_payload.go`（+6/−0）。测试改口：`publish_commercial_authority_test.go`（+26/−26，壳改配算出的摘要）、
`publication_canonicalization_test.go`（+27/−9）、`publication_vocabulary_test.go`（+22/−1）、domain / application 的 `publication_draft_test.go`
（此前拿本册当「没接的册」样本，改钉封闭集全接）、`cmd/parcel-commercial/publish_batch_test.go`（+13/−1）。`policy-rows.ts` / `party/api.ts`
本票未动（21 动过，18 不需要）。**seed 未动**：`scripts/demo-seeds` 没有本册的发布样本。

**验证强度**：admin-web 在 `c31f3fda` 与 `dbd16d10` 各跑一次：`node node_modules/typescript/bin/tsc --noEmit` 退 0；
`node scripts/run-tests.mjs` 188 / 188（awf/21 时 177，本册 +11）。Go 按各笔提交信：`e8958fce` 无 DSN gofmt 空 / build / vet 退 0 /
PC 四包 ok；`e33aa81e` 无 DSN PC 四包 + `cmd/parcel-api` + `internal/architecture` ok；`fc51e407` **含 DSN** `go test -count=1 -v
./cmd/parcel-commercial/` PASS 129 / FAIL 0 / SKIP 0，红一次（临时换回旧串 exit 2）绿一次。本会话在 tip `dbd16d10` 不带 DSN 复跑：
`gofmt -l` 两包空；`go build ./...` / `go vet` PC + `cmd/parcel-commercial` 退 0；`go test -count=1 ./internal/partycommercial/...
./internal/architecture/...` 全 ok（ports 无测试）——本会话只动 .tsx / .md，没再占 55432（通道 5 要量数）。证据层级 **S**（隔离合成）。
`.sql` 无变动、不加迁移、不动 `endpoints.go` / `ports.go` / 0023。

**判断题（留给评审 / 伞票收口，不在本票动）**：(1) 票面「起算事件的封闭集」半句写宽了，已在「选形与理由」加落地改口，未改原句——
若要把原句直接改掉是一行的事；(2) `Field` / `useLoaded` / `VocabularySelect` / `vocabularyPlaceholder` 在本组件又是一份私有副本，
与 13 评审判断题同源，抬共享层另立票；(3) 材料清单选了多行文本而不是逐项加行——正文里材料是「清单」不是「行」，逐项加行会让一张
表里再套一张表；服务端逐项的话仍能对回项号，代价是不能挂在某一项旁边；(4) 空表零行与 13 预开一行两册选形不同，各自理由写在
`emptyServiceRuleDraft` / `emptyControlPolicyDraft`，是否统一归伞票；(5) 词表读一次不重取，与 13 / 15 同一取舍。

**未做（各归其票）**：五步状态机归公共半边（票 16）；词表读口归票 20；四口与词表口点亮归操作者接入渠道（ADR-0100 那一族）；
VE 侧两本册与本册正文的归属走 ADR（pc-gaps/05）；伞票 07 子票表本行的状态由推送方在进 main 时改；非作者评审由通道 1 派。

## 进 main 记录（2026-09-09 17:5x，通道 1 推送）

分支 `mcp3-awf21` 的 `4de2d30a..ed842637` 六笔在隔离树重放到 `506bbfd2` 之后，零冲突、内容与分支逐文件零差：`e8958fce→918ef3e0` /
`e33aa81e→76710c17` / `fc51e407→6f3c76df` / `c31f3fda→e7a7d784` / `dbd16d10→81285be0` / `ed842637→83c6d81d`（`4de2d30a` 及之前三笔是票 21，早已进 main）。
清点在链 tip 重生成 `c2a965c9`（partycommercial 生产 117→119 / 测试 127→130、http 适配器 25→26；合计 877 / 834）。推送方在 `c2a965c9` 干净检出含 DSN
`go test -p 1 -count=1 ./...` 一次：**102 ok / 0 FAIL / 15 无测试，117 s**（`cmd/parcel-commercial` 含 DSN ok 3.6 s——13 / 15 都在这里红过的那格本票绿）；
admin-web 在 main 树上 `tsc --noEmit` 0、`run-tests` 188 / 188。**远端 `main = c2a965c9`**。伞票 07 子票表本行同笔转 resolved。分支指针改名 `merged/mcp3-awf21`。
评审两轴五条非阻断随票记（下），不另立票：Spec ① 判据口径归伞票 07 收口；Spec ② 原句改口与 Standards ① 父行门抽取、Spec ③ 文案，作者可随伞票收口
或 awf/22（全页重构）顺手改。

## Comments

**评审 ← 通道 5 · 钉 `ed842637` · 17:5x**（基 `4de2d30a`，隔离树 `%TEMP%\idp-review-awf18`；改派自通道 6（其会话 17:40 crash，分析已做报告未发）；原文在通道 1 台账 `task-b0b1b481`；先发了一条半成品）

- **Standards**：阻断 0。非阻断 ① `domain/customer_service_rule.go` `NewCustomerServiceRuleVersion` 与 `domain/publication_canonicalization_customer_service_rule.go`
  `CustomerServiceRuleBody.filed`：父行三格门（applicability / responsible / scope.valid → `ErrInvalidCustomerServiceRuleVersion`）各写一份——跨行门抽成
  `filedCustomerServiceRuleItems` 的理由对父行门同样成立；样板 13 的 `filedPreAcceptanceControlItems` 把 JointPass 一起收了（Duplicated Code，判断题）。
  ② 三处把「今天只有信用政策一格」改口「首例是信用政策一格」（`publication_canonicalization.go` / `publication_draft.go` / `publish_commercial_authority.go`）：
  改的是过时陈述，无害，但属地盘外且完成记录未列。无发现：注释全中文；无行号式引用；domain 新文件只 import fmt；tsx 无 defaultValue / selected，每个 select
  首项 value="" 未选，`emptyServiceRuleDraft` 两键空两表零行；`serviceRuleLocalProblems` 只拦编不进 JSON 整数的时长文本；载荷无 contentDigest / approval；
  共享文件只加不改邻册且排 13 之后；`ClaimDeadlineKindNamed` / `CommercialObjectKindNamed` 都走 `closedCodeNamed`；`IsRegisterCanonicalized` 十格全真与
  `ErrRegisterNotCanonicalized` 新注释自洽。验：gofmt 空；build 0；vet PC + cmd/parcel-commercial 0；`go test -count=1` PC 四包 + architecture 无 DSN 全 ok；
  补跑 `cmd/parcel-commercial` 含 DSN：PASS 123 / FAIL 0 / SKIP 0（通道 6 报 129 是计数口径不同，零红零跳一致）。
- **Spec**：阻断 0。非阻断 ① 判据「Go 侧只加本册规范化一格」字面写窄：实际含应用两向 + 载荷一格 + cmd 换串与地盘外两改，完成记录逐条披露且读法同 13 / 15——
  不是越界，是判据口径该改「接进 PCC-1 同号所需的机制半边」，归伞票。② 「选形与理由」原句「期限种类与起算事件的封闭集由词表供」仍在，只加括注；本册词表只答 kind，
  起算事件是 `DeadlineStartEventReference` 开放引用——不矛盾但原句半错，作者判断题 (1) 已自报，收口时直接改。③ `ClaimDeadlineRulePayload.rule`：表单时长留空不送
  days，Go 收 0，文案「收到 0」——操作者看到的不是「没填」；不用指针是与 13 `order` 同一取舍，只是文案。无发现（重点七条）：排序稳（两表各以键排、重键已在 filed 拒）、
  空表 `[]` 非 null、与 13 六件同形、跨行门抽取前后错误值与顺序一字不变；词表只 kind、`serviceRuleCodesOf` 缺集回 null 无回退；`reconcileDeclaredDigest` 对旧
  `sha256:` 串答 NotAccepted 带 declared / computed 两串、壳无正文不对账仍合法；Go / TS 三键（startEvent / claimKind / customerServiceRule）两侧逐字同；判断题三条
  各一句判（空表零行对、材料多行文本对、词表不重取可接受）；九份共享文件 numstat 与作者一致、减行全是旧循环与过时注释；`closedCodeNamed` 与旧循环在连续集上语义相等，
  main 上 `closed_set_named_test.go` 十成员往返直接盖到。边界：无 migrations/、无 visibilityexception/。
- **结论**：两轴无阻断，可进 main。推送方处置：五条非阻断随票记；Spec ① 归伞票 07 收口时改判据口径；其余作者随伞票收口或 awf/22 顺手改。
