# 09 `SERVICE_PRODUCT` 版本的运营主路径：逐字段表单（版本壳 + 引用）

Category: enhancement
Status: resolved——2026-09-08 15:0x MCP-3 完成（task-7fc5a960；分支 `mcp3-awf09`，隔离树基 main `0ef63897`、变基到通道 5 公共半边 `mcp5-awf16@283aa7d3` 之上；逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA 待重放后对照，非作者评审由通道 1 派）。完成判据三条全落：服务产品页多一签「发布版本」走五步、发布落定刷同页目录读面；tsc / run-tests 绿；Go 侧只加本册规范化一格（无正文册的落法见「本册规范化判断」）。此前 in-progress——14:2x 认领，08 已 resolved、阻塞边解除。此前 ready-for-agent——形状已裁清（逐字段表单，本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已 resolved）

## 册与载荷

显示在**服务产品页**。发布载荷只有版本壳：`kind / objectId / version / scope / effectiveStartsAt / effectiveEndsAt? /
references{}`，`declarations` 里没有为它开的正文通道（`translate.go` 的 `declarationsDocument` 十几格里没有服务产品
正文）。服务产品的属性与渠道映射走另一条登记路（`register_products`，票 admin-remainder-mechanism-batch/02），
这里发布的是**商业版本壳**——它给合同、接单规则包、价格政策一个可引用的版本身份。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低（一个产品一年几版）、操作者是运营配置员、载荷是几格标识与一个区间——没有矩阵、没有
子表。表单字段：对象标识、版本号、范围引用、有效起止、引用表（键值对可加行，键的词汇由服务端答、表单不代填）。
摘要与批准照 08 的机制来，表单只呈现服务端答的摘要。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：**表单不得替操作者拟引用键**——`references` 是开放词汇，键名从哪来、指向什么
由发布用例与领域答，表单给的是「加一行」不是「从这几个里挑」，除非服务端提供了词表读口。

## 本册规范化判断（2026-09-08，MCP-3；票内小裁决，不改 ADR-0126 任何一句）

**问题**：[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)
Decision 一说规范化文档「只盖正文不盖壳」，而本册**没有正文**——载荷就是壳。文档里放什么？两个候选：
(a) 文档 = `{canonicalization, kind}`，本册每一版摘要同一个串；(b) 把壳上的 `references` 读进文档当正文。

**选 (a)。** 理由：

1. `references` 是壳的一格，不是正文。它在 `PublicationDraftShell` 与 `CommercialVersionSpec` 上都与范围、区间并列，
   `sameReleasedContent`（发布登记册判重放 / 冲突）与 `SameSubmissionAs`（载体判重放 / 修订）都已经把它逐项比进去。
   (b) 让同一格既进壳的逐项比对又进摘要，「同引用换范围」与「同范围换引用」两种修订会一个折成壳不同、一个折成两串
   不同——ADR-0126 Alternatives 否决「摘要盖住版本壳」的那条理由在这里逐字成立。且 (b) 要让对账门从 `Spec.References`
   而不是 `Declarations` 读正文，那是把壳读作正文，对账门会对每一条服务产品批文开门。
2. (a) 下的串是诚实的：本册的正文就是「空」，摘要盖住的就是空（Decision 二原话「正文缺席……摘要盖住的是空」）。它不是
   退化——`PCC-1:<sha256 of {"canonicalization":"PCC-1","kind":"SERVICE_PRODUCT"}>` 与信用政策的串一样带版本、可重算、可比。
   同键重发（壳同）→ `DRAFT_REPLAYED`；换范围 / 区间 / 引用 → `DRAFT_REVISED`（待批准）或 `CONTENT_FIXED`（已批准）；
   落到 `commercial_version` 后由 `sameReleasedContent` 同一套判据分重放 / 冲突。判据没有一处为本册另开。
3. 「只盖正文」对无正文的册读作「正文为空，文档只剩规范化版本与 kind 两格」，是那一句的边界情形不是例外；**不换号**
   （加册不换号，Decision 一）。本册也没有 `ErrPublicationContentAbsent` 那一格——它没有正文槽位，缺席与在场是同一件事；
   `PublicationContent`、`canonicalPublicationDocument`、`CommercialPublicationPayload` 因此**不加格**，`RehydratePublicationContent`
   对本册跳过「正文缺席」判断。
4. **受控批文那一半与应用层两处**：`IsRegisterCanonicalized(SERVICE_PRODUCT)` 答真，但批文 `declarations` 里没有本册的正文
   通道，`publicationContentOf` 对本册答「不在场」（走既有 `default`，不加分支）——按 Decision 二「正文缺席的版本没有可比
   对象，照今天登记声明的串」放行，对账门对本册**不开**。`declarationsOfContent` 对本册答空声明（同样走既有默认）：发布只登
   版本壳，`declarationWrites` 零通道，由 http 测试 `TestServiceProductWalksTheFiveStepsWithoutDeclarations` 钉住。两处答的
   是同一句话：本册没有正文，既无从对账也无从声明。表单路径的 `Spec.ContentDigest` 就是算出的串，门恒成立。
   因此 `scripts/demo-seeds/data/commercial/publish-batch.json` 里 `SERVICE_PRODUCT` 项的 `sha256:syn-SYN-PROD-*` 声明串一字不动
   （取证于 `0ef63897`：两项，`SYN-PROD-CN-SG-EXPRESS` / `SYN-PROD-CN-SG-ECON`）、`cmd/parcel-commercial` 不动。查过重放
   seed 的测试（同 SHA）：没有 Go 测试读 `publish-batch.json`（只有 pricing seed 被 `cmd/parcel-pricing-register/main_test.go`
   读）；但 `cmd/parcel-commercial/publish_batch_test.go` 与 `cmd/parcel-dispatch/syn_pc_seed_test.go` 都以 `sha256:…` 旧串发
   `SERVICE_PRODUCT`，门不开所以照绿——若日后开门，seed 那两项与这两处一并换串。这一格是**有意留的**：逼 CLI 对无正文册也
   声明 `PCC-1:<常量>`，属「何时开始拒收无版本旧串」那一问（Decision 五归伞票收口时裁），不在本票自裁。
   **后果（只记不裁，通道 1 2026-09-08 复核时点名）**：同一版从表单路发布得 `PCC-1:<sha256 of {canonicalization, kind}>`；
   从批文路重发同一版本号带 `sha256:syn-…`，登记册答 `CONTENT_CONFLICT`（ADR-0031 比的是串）而**不是** `NOT_ACCEPTED` 带两串——
   对账门对本册不开，操作者从批文路拿不到算出的那一个串，只知道「与册上那版不同」。反向亦然：先批文后表单，表单路答
   `PUBLICATION_NOT_LANDED` 嵌 `CONTENT_CONFLICT`。这与「旧式 `sha256:` 声明串何时开始拒收」是同一件事（ADR-0126 Decision 五、
   票 08 完成记录「未做」），伞票 07 收口时一起裁；本票只记录，不裁。

**给通道 1 复核的一点**：第 4 条把「已接的册」与「CLI 必须声明算出的串」拆开了——对有正文的册两者同时成立，对本册只有前者。
若通道 1 认为接进规范化就该同时对 CLI 开门（seed 两行随之换串），那是 `publicationContentOf` 加一支 + seed 两行，本票可接，
但要等 10 / 11 重放后再动 seed 文件以免三方撞同一份 JSON。

## 完成判据

服务产品页多一签「发布版本」（表单 → 预览摘要 → 存为待批准 → 批准 → 发布），结果在同页的目录读面立刻可见；
tsc / run-tests 绿；Go 侧只在 08 落的机制上加本册的规范化一格。

## 边界

不动 `register_products` 那条登记路；不动服务产品目录读面。

## 完成记录（2026-09-08，MCP-3；分支 `mcp3-awf09`）

**基线与依赖**：隔离树基 main `0ef63897`；前端流程组件接通道 5 的公共半边，分支因此**变基到 `mcp5-awf16@283aa7d3` 之上**
（那一笔纯 `apps/admin-web`，是本票各笔的祖先、不属本票）。重放进 main 时先落 16，再 `git cherry-pick 283aa7d3..mcp3-awf09`。
16 的后两笔（`5e2aa3d1` / `7d204fd5`）只改公共半边的内部与注释，未改本票用到的 props 与类型，核过。main 此刻 `83aae6cb`，
自 `0ef63897` 起只多 `adapters/postgres/publication_draft.go` 一处修补（`c84230ce`）与 `.md`，与本票无重叠。

**逐笔（分支 SHA）**：

| SHA | 内容 |
|---|---|
| `69cc885e` | 票面认领转 in-progress，落「本册规范化判断」（裁决先于代码） |
| `9fad4185` | 领域：服务产品册接进 PCC-1——新文件 `publication_canonicalization_service_product.go`（两格文档 + `registerHasNoBody`）；共享文件 `publication_canonicalization.go` 三支；领域测试四条；真库测试 `publication_draft_service_product_test.go`（postgres） |
| `3f43525e` | 传输面：`publication_draft_payload.go` 一行注释；契约测试 `publication_draft_payload_service_product_test.go` 四条（生产代码零改动即走通） |
| `063b0c22` | admin-web：`service-product-form.ts`（纯函数 + 六条 node:test）、`ServiceProductPublicationForm.tsx`、`ServiceProductsPage.tsx` 加签「发布版本」+ `refreshKey` 刷读面 |
| `07d7b5da` | 双轴评审三条修复（测试注释不按「第 N 条」指票面、`any` → `hasRows`、去「封闭十词」计数） |
| 本笔 | 票面完成记录 + 转 resolved |

**完成判据逐项**：

1. 服务产品页多一签「发布版本」：表单（壳四格 + 有效起止 + 引用表可加行）→ 预览摘要 → 存为待批准 → 批准 → 发布，五步由
   `PublicationDraftFlow` 走；载体到达发布那一步时 `onPublished` 递增 `refreshKey`，同页目录读面重读，新版本立刻可见。
   **今天四口挂 `UnconfiguredIntake{}`**，浏览器里五步会答 403（诚实答案，ADR-0085 两阶段）；五步走通的证据在服务端：
   `TestServiceProductWalksTheFiveStepsWithoutDeclarations`（预览 PREVIEWED → 录入 201 DRAFT_SUBMITTED → 批准 DRAFT_APPROVED 同摘要 →
   发布 201 DRAFT_PUBLISHED 嵌 PUBLISHED_EFFECTIVE、零声明通道）与真库 `TestAServiceProductDraftRoundTripsWithoutABody`。
2. `tsc --noEmit` 退 0；`run-tests` 91 ok / 0 fail（含本票六条与 16 的状态机测试）。
3. Go 侧只在 08 的机制上加本册一格：`CanonicalizePublicationContent` 一支、`IsRegisterCanonicalized` 一册、`RehydratePublicationContent`
   无正文早返回一支；`PublicationContent` / `canonicalPublicationDocument` / `CommercialPublicationPayload` 不加格（本册没有正文槽位）。

**硬句逐条**：表单不算摘要、不裁任何门——本地只报「同一引用键两行」这一件编码层的事（载荷里放不下，静默留一行就是丢输入），空字段 /
时刻格式 / 集合外的键一律送上去让服务端逐格答；不收也不送批准人——载荷只有壳；不得替操作者拟引用键——键自填、不给候选、不内置词表；
预览与发布同一路径——同一份载荷过预览与录入同摘要（`TestServiceProductPreviewAndSubmissionShareTheDigest`）。`DRAFT_AWAITS_EFFECTIVE_START`
由公共半边的 `publishOutcomeLabels` 显给操作者，本册不另写。

**验证强度（钉 `07d7b5da`，隔离 detached 检出 `%TEMP%\idp-verify-awf09`）**：`gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0；
无 DSN `go test -count=1 ./internal/partycommercial/... ./cmd/parcel-api/ ./cmd/parcel-commercial/ ./internal/architecture/...` 全 ok；
含 DSN `./internal/partycommercial/...` 全 ok（postgres 包 65s），`TestAServiceProductDraftRoundTripsWithoutABody` **PASS**（0.34s），探针
`TestTheWiredPublicationDraftPathLandsAgainstARealDatabase` 无 DSN **SKIP** / 含 DSN **PASS**；admin-web `tsc --noEmit` 退 0、`run-tests`
91 ok / 0 fail（验证树的 `node_modules` 是指向共享树的临时联接，验完即拆）。证据层级 **S**（隔离合成）。`.go` 有变动（上表）、`.sql`
无、迁移无。

**共享接线文件各改了哪几处**：`publication_canonicalization.go`——`publicationCanonicalizationVersion` 注释尾加一句；`RehydratePublicationContent`
在「正文缺席」判断前加 `registerHasNoBody` 早返回一支；`CanonicalizePublicationContent` 的 switch 加 `case ServiceProductObject`；
`IsRegisterCanonicalized` 的单行 `return` 改成每册一 `case` 的 switch——这一处**必然改邻行**（单行表达式加不了册），改成 switch 是让 10 / 11
接进来时成为纯加行。`publication_draft_payload.go`——`CommercialPublicationPayload` 尾加一行注释。`ServiceProductsPage.tsx`——表格收
`refreshKey`、页面加签、空态文案与文件头注释。**未动**：`ports.go`、`endpoints.go`、`publish_commercial_authority.go`、`application/publication_draft.go`
（本册走两处既有 `default`，见「本册规范化判断」）、`party/api.ts`、seed、迁移、`register_products` 登记路、服务产品目录读面（只多一个
`refreshKey` 入参）。

**未做（各归其处）**：引用键的词表读口（票面硬句留的口：有读口才可「从这几个里挑」，今天没有，是新裁决）；CLI 对账门对本册开门 +
seed 两项换串（归伞票「何时开始拒收无版本旧串」）；既有测试 `TestCanonicalizeAnswersThreeDistinctRefusals` 的失败信息「first release covers
credit policy only」已过时（邻行未动，伞票收口时统一改）；时刻在本地不补零点不换时区（pricing 的 `normalizeMoment` 是另一种便利，09–17
表单要不要统一归伞票）；浏览器端到端在接入渠道未配置前答 403，走通的证据在服务端测试。

## Comments

### 评审 ← 通道 2 · 钉 `07d7b5da` · 15:06

非作者合入前评审。基 `283aa7d3`（16 公共半边，不属本票），隔离 detached 检出 `%TEMP%\idp-review-awf09`（@ `f4be0bc9`），只读，两轴各一遍；
代码 tip `07d7b5da`，`6d5dea48` / `f4be0bc9` 簿记不评。

**关键判断——「本册规范化判断」选 (a) 与 ADR-0126 是否自洽：非阻断，不需改 ADR 一句。** 依据：(1) 文档形状 = `{canonicalization, kind}` 落在
Decision 一「文档只盖正文不盖壳」与 ADR「裁决方的能力边界」明写下放给子票的「各册自己的规范化文档形状……同号加册」之内；`references` 与范围、
区间同为壳的一格，已由 `sameReleasedContent` / `SameSubmissionAs` 逐项比，盖进摘要正撞 Alternatives 否决「摘要盖住版本壳」的那条理由。(2) 对账门对
本册不开：Decision 二首句「已接的册声明摘要必须与算出的相等」字面上会开门（本册算得出、是一个常量串）；但对无正文册，「开门」与
Decision 五明写留给伞票收口的「何时开始拒收没版本的串」是**同一个动作**——本册没有正文可对，门一开拦下的只会是旧串本身。作者把它归伞票
收口而不在票内自裁，与 Decision 五一致，且未动 `publish_commercial_authority.go`。**「接了」确被用成两个意思**：`IsRegisterCanonicalized` 现在
答的是「表单路算得出」，对账门实际用的是它 **且** `publicationContentOf` 在场——两个词在有正文册重合、在无正文册分开。这不是本票新造的
机制（08 的门就是这两段），是本票让它第一次可见；应在伞票 07 收口时把「无正文册的已接 = 表单路可算，对账门另按有无声明通道」写进
那一句的裁决，届时若开门再改 ADR/seed。**后果**（作者已记）跨路重发答 `CONTENT_CONFLICT` 不是 `NOT_ACCEPTED` 带两串——两串确实不同，
答案不假，只是少了「算出的那一个」的可见性，属诊断性缺口不属不变式。

**Standards 轴**

- **阻断**：无。
- **非阻断**：
  1. `domain/publication_canonicalization.go` `IsRegisterCanonicalized` 头注「对账门用它分辨『声明的串与算出的不等』与『这一册无从对账』」——
     对本册已不成立（本册它答真而门不开）。改注释一句：门 = 本函数 ∧ 批文有该册正文通道；否则下一个读 08 门的人会以为服务产品
     批文已被对账。
  2. 同文件 `RehydratePublicationContent` 的 `registerHasNoBody` 早返回只对 `ServiceProductObject` 开，且放在信用政策一节解码之后——
     夹带别册正文的快照仍走到再规范化的 kind 不符（测试 `TestServiceProductSnapshotRehydratesWithoutABody` 伪造节钉住）。语义对；
     只记一句：10/11 若也是无正文册要加进 `registerHasNoBody`，那是它们的一格。
- **无发现**：`CanonicalizePublicationContent` 加一支、`IsRegisterCanonicalized` 单行改 switch 语义不变（信用政策仍真、默认仍假），两 `case`
  分行是为 10/11 纯加行；领域包 import 未增；注释中文、符号引用无行号、`07d7b5da` 已去计数与「第 N 条」。前端：只用
  `PublicationDraftFlow` 既有 props、公共三件未动；`serviceProductLocalProblems` 只报「同键两行」——`references` 线格式是 JSON 对象，
  同名键放不下、静默留一行就是丢输入，属编码层不属领域校验（空键、集合外键、空值都照送，测试与注释都钉住）；引用键自填无候选，
  不内置词表（票 09 硬句）；载荷无身份/摘要键；`serviceProductFieldPaths(draft)` 按行动态生成再交给 `fieldPaths`，是对 16 评审
  非阻断 (3)「静态路径装不下可加行」的现成解法，10 可照抄。

**Spec 轴**

- **阻断**：无。
- **非阻断**：无。
- **无发现**：完成判据三条——服务产品页「发布版本」签在册签之后登记签之前，五步由公共流程走，`onPublished` → `refreshKey` 递增 →
  `ServiceProductVersionsTable` 重读；tsc / run-tests 作者已跑；Go 侧只加本册一格（`PublicationContent` / `canonicalPublicationDocument` /
  `CommercialPublicationPayload` 都未加格，与「无正文槽位」自洽）。边界：`register_products` 与目录读面未动（读面只多 `refreshKey` 入参与
  空态文案）。「未做」四项反查：词表读口无（自填）；对账门未开、seed 与 `cmd/parcel-commercial` 未动、`publish_commercial_authority.go`
  未动；旧测试信息「credit policy only」邻行未动；时刻不补零点（与 16 的 `normalizeMoment` 不一致，作者已记归伞票统一）。服务端五步走通
  由 `TestServiceProductWalksTheFiveStepsWithoutDeclarations`（零声明通道、嵌 `PUBLISHED_EFFECTIVE`）与真库
  `TestAServiceProductDraftRoundTripsWithoutABody` 证，S 级诚实。

**结论**：Standards 2 条非阻断（最重：`IsRegisterCanonicalized` 头注对无正文册失真，一句注释）；Spec 0 条。关键判断 (a) **非阻断、不改 ADR**，
但伞票 07 收口时须把「无正文册的已接与对账门开门拆开」连同「何时拒收旧串」一起裁。**无阻断，可重放。**
