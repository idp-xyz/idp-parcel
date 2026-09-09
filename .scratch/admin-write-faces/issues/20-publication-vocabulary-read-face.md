# 20 商业发布词表读口：一口按 kind 答各册正文的封闭集，表单不内置枚举

Category: enhancement
Status: resolved——通道 4 于 2026-09-08 19:5x 认领、20:0x 完工，分支 `mcp4-awf20`（树 `D:/tops/idp-parcel-mcp4-awf20`，基 `5a209f70`），逐笔 SHA 见「完成记录」；进 main 记录待推送方重放后由其广播补入；形状由通道 1 代裁（用户 2026-09-08 18:3x「你自决」授权，见伞票 07 Comments 同刻那条）：**一个通用读口按 kind 答**，不是一册一口；只答码不答中文；集合的唯一权威在 PC 领域枚举。本票是第 2 波 12/13/15/17 的前置公共半边，与 16 之于第 1 波同一角色
Blocked by: 无（08 与 16 已进 main：五步路径四口与前端公共半边都在场）

## 缺什么

第 2 波四张表单票（[12](./12-acceptance-rule-package-form.md)、[13](./13-pre-acceptance-financial-control-policy-form.md)、
[15](./15-settlement-policy-form.md)、[17](./17-authorization-rule-form.md)）的票面都写着「封闭集由服务端词表读口供下拉，表单
不内置枚举」——理由是 ADR-0126 之后正文由服务端按册规范化，表单若自带一份枚举就是同一封闭集的第二份写法，漂了无人报。
可是这个读口今天不存在：`cmd/parcel-api/endpoints.go` 里商业发布只有五步路径的四口（预览 / 录入 / 批准 / 发布，
`/commercial-publication-*`），admin-web 里封闭集的中文词表各页自持（`presentation.ts` / `*-rows.ts` 那种「集外取值原样示出」的写法），
没有一处向服务端问「这一册正文的某格今天允许哪些词」。四张票同时开工就会四个人各造一口或各内置一份枚举。

## 裁决（2026-09-08，通道 1 代裁）

1. **一口，按 kind 答。** `GET /commercial-publication-vocabularies?kind=<CommercialObjectKind 的线上名>`，答该册正文里所有封闭集：
   每个集合一个名字（就是载荷里那格的键名，例如接单规则包的 `stage` / `intent`，结算政策的 `method`，授权规则的请求方那格）和
   按领域枚举顺序排列的**码**。理由：五步路径已经是一套载荷 `CommercialPublicationPayload` 带 `kind` 分册正文（ADR-0126），词表
   跟着同一条分派走是同一形；一册一口要在 `endpoints.go` 加四行、四张票四个人改同一文件，正是第 1 波刻意避开的撞点。
2. **只答码，不答中文。** 中文留在 admin-web 各页词表（既有约定：集外取值原样示出，不冒充既有一格）。服务端给中文会让呈现层的词
   进 PC 上下文，所有权错位；给码则表单拿到的正是规范化器接受的那些词，「表单内置枚举漂了」那一格从此不存在。
3. **集合的权威在领域。** 各册的封闭集今天已经是领域枚举（规范化器按它拒「集外取值」）；本票在 `internal/partycommercial/domain`
   给每册一个 `PublicationVocabulary(kind)` 式的只读口，从**同一个枚举**列出词——不另写一份常量表。某册没有封闭集（服务产品、
   客户合同的正文没有枚举格）就答空集合列表，不答 404：kind 合法、只是没词。
4. **未知 kind 答问题不答空。** kind 不在 `CommercialObjectKind` 内 → 与预览口同一种载荷问题格式（`PayloadProblemRecord`），不静默。
5. **前端一处取。** `party/publication-draft-api.ts` 加一个 `fetchPublicationVocabulary(kind)` 与类型；表单票各自只消费，不各写 fetch。
   下拉的选项 = 服务端码 × 本页中文词表；词表没收录的码原样示出（沿用 `channel-selection-decisions.ts` 那句判据）。

## 要做的

- 领域：`internal/partycommercial/domain` 新文件 `publication_vocabulary.go`——`PublicationVocabulary(kind CommercialObjectKind) ([]VocabularySet, error)`，
  逐册从既有枚举列词（接单规则包：`stage`、`intent`、判断类型 / 来源 / 终局种类等票 12 点名的那几格；接受前财务控制策略：
  票 13 点名的封闭集；结算政策：`method`；授权规则：请求方那格——各以票面「要做的」为准，**不多列票面没要的**）；测试钉「每个词
  都被该册规范化器接受、规范化器接受的每个词都在集合里」（双向，防漂）。无封闭集的册答空列表。
- http：`internal/partycommercial/adapters/http` 新文件 `publication_vocabulary.go`——`GET`，query `kind`，答 `{kind, sets:[{name, codes:[...]}]}`；
  未知 kind 答 `PayloadProblemRecord`；测试三格（有集 / 空集 / 坏 kind）。
- `cmd/parcel-api/endpoints.go` **加一行**（PC 组，紧跟四口之后；改前广播占号；不动别的行）；端点表测试跟一行。
- admin-web：`party/publication-draft-api.ts` 加 `fetchPublicationVocabulary(kind)` + `VocabularySetRecord` 类型；一个小纯函数
  `vocabularyOptions(codes, labels)`（码 × 中文词表 → 下拉选项，集外原样）+ node:test。不改任何表单。
- 广播落点（函数名与类型名）给 12/13/15/17 的认领人；票 07 子票表加本票一行。

## 边界

不动五步路径四口与其载荷；不动各册规范化器（只读它接受的词）；不加迁移；不动 `ports.go`；不碰 `CommercialPoliciesPage.tsx`
与任何表单文件（那是 12–17 的地盘）；不给中文；不给「默认选中」——票面硬句「表单不给默认、不预选」照旧归表单票。

## 完成判据

`GET /commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE` 答票 12 点名的那几个集合、码序同领域枚举；`kind=SERVICE_PRODUCT`
答空列表；坏 kind 答问题格；领域测试双向钉住集合与规范化器同词；admin-web tsc / run-tests 绿；`endpoints.go` 只多一行；
含 DSN 全仓由推送方重放后跑一次（本票不带 DSN 也应全绿：无 postgres 改动）。

## 完成记录（通道 4，2026-09-08，分支 `mcp4-awf20` 基 `5a209f70`）

逐笔（分支上的 SHA；进 main 后由推送方广播新旧对照）：

- `509b67ce` 票面 Status 转 in-progress。
- `0fb66cd8` 领域：`internal/partycommercial/domain/publication_vocabulary.go` + `_test.go`——`PublicationVocabulary(kind) ([]VocabularySet, error)`、
  `VocabularySet{Name, Codes}`。集合从既有枚举的接受判据（`valid` / `declarable` / `Declared`）与 `String()` 逐值列出（`closedCodes` 扫整个
  uint8 值域），不另写常量表；其余合法 kind 答空非 nil 列表，坏 kind 答 `ErrInvalidCommercialVersion`。测试以导出构造门为预言机双向钉住
  （每个词被构造门接受 / 构造门接受的每个词都在集合里 / 码序同枚举），另钉票 12 / 15 / 17 点名的字面；变异自查：把 `allowance` 的判据换成
  恒真，测试当场红（多出 NOT_DECLARED）。
- `d894751a` http：`internal/partycommercial/adapters/http/publication_vocabulary.go` + `_test.go`——`NewQueryPublicationVocabularyEndpoint(intake
  PublicationVocabularyIntake)`；答 `{outcome: "PUBLICATION_VOCABULARY_LISTED", kind, sets:[{name, codes}]}`。**地盘外同包一处**（占号广播已点名、
  通道 1 已准）：`unconfigured_intake.go` 加 `_ PublicationVocabularyIntake = UnconfiguredIntake{}` 一行与 `IntakePublicationVocabularyQuery` 一法。
- `4eb2492f` 装配：`cmd/parcel-api/endpoints.go` 加一行 `/commercial-publication-vocabularies`（PC 组紧跟四口之后，挂字面量 `UnconfiguredIntake{}`）
  + 行上注释写准入理由；`endpoints_test.go` 探针表跟一行。同笔改写 `PublicationVocabularyIntake` 注释（通道 1 指正：理由不是预览口那条）。
- `17a962a5` admin-web：`party/publication-draft-api.ts` 文件末尾追加 `VocabularySetRecord` / `PublicationVocabularyResponseBody` /
  `publicationVocabularyEndpoint` / `fetchPublicationVocabulary(kind)` / `VocabularyOption` / `vocabularyOptions(codes, labels)`，import 行多引
  `exchangeMasterData`；新文件 `party/publication-vocabulary.test.ts`。不改任何表单。
- `b0667d3b` 机制清点在 `17a962a5` 的干净检出上重生成（PC 端点 +1、生产 / 测试文件各 +2），只作本笔取证，推送方在 tip 上重生成兑底。

各册集合（= 载荷键名，码按枚举顺序）：

- `ACCEPTANCE_RULE_PACKAGE`：`category`、`judgment`、`applicableGroups`、`manualReview`（NOT_REQUIRED / REQUIRED）、`sources`、`outcome`、`anchor`
  （CHANNEL_RESULT_OBSERVED）、`stage`、`intent`、`allowance`（ALLOWED / DISALLOWED）。票 12 载荷里的 `semantics` / `policyVersion` / `qualifications` /
  `finalKind` / `dataGroup` / `pendingRoutingBasis` 是开放引用，不成集合；票 12 措辞里的「终局种类」在领域里是开放引用 `FinalKind`，封闭的是
  `outcome`（责任结果），按领域答。
- `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`：`jointPassCondition`、`control`、`onFailure`（键名与读面 `query_commercial_policies.go` 的控制项同名）。
- `SETTLEMENT_POLICY`：`method`。`AUTHORIZATION_RULE`：`party`。其余 kind：`sets: []`。

自验（`17a962a5` 的 detached 检出 `%TEMP%\idp-verify-awf20`，未设 DSN——本票无 postgres 改动）：`gofmt -l .` 列出 0；`go build ./...` 退 0；
`go vet ./...` 退 0；`go test -count=1` 动过的三包及其反向依赖（`go list` 反查得 14 个，含三个 `cmd/`）+ `./internal/architecture/...` 全部 `ok`；
admin-web `tsc --noEmit` 退 0、`run-tests` 123/123（本票 7 例在内）。工作树上 `go test` 领域 7 例、http 4 例各 PASS。

与票面的两处出入，写明便于评审：

- 「`endpoints.go` 只多一行」：端点表**条目**只多一行；行上另有几行注释写准入理由，是通道 1 20:0x 消息要求的（「那行的注释要写真理由」）。
- 坏 kind 的答复取 400 + `MALFORMED_REQUEST` + `error.problems[{field: "kind", problem}]`（与命令口的逐格问题同形、与 `/commercial-policies`
  坏 kind 同码），不是预览口那种 200 + NOT_ACCEPTED：一个 GET 带集合外的 kind 是调用方式问题，重发同样内容不会好；`PayloadProblemRecord`
  那一格的形状（`{field, problem}`）与预览口相同。答复多带一格 `outcome`（`PUBLICATION_VOCABULARY_LISTED`），沿本仓读口「业务判别走 outcome」
  的通例，`{kind, sets}` 两格照票面。

**判断点（归 owner 复核）：词表读口在隔离读态不放行。** 它与四口同挂字面量 `UnconfiguredIntake{}`，隔离读开关（ADR-0078）换不了它。理由：
它唯一的消费者是四口喂的表单，四口开不了时它单独开只让一张提交不了的表单多几行下拉；且它不读任何存储读面，不满足 ADR-0078 入格判据
「消费本上下文自己的存储读面」。**解锁条件**：要在隔离读态放它，得给 `TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines` 的二分
（放行 → 500 / 不放 → 403）加第三桶（放行且不经读口 → 200）并改 ADR-0078 判据措辞，另立票。
**owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁）：不放行。两条理由都成立——隔离读态放行的判据是「消费本上下文自己的存储读面」，
词表口读的是编译进二进制的封闭集，放它进去要改判据；而它单独开没有任何操作者能从中得到一个能提交的表单。四口点亮那天它随四口一起点亮，
不另立票。

顺带一格（不在本票范围，记下免得丢）：客户合同正文的 `preAcceptanceControl.requirement` 在领域里也是封闭集（`PreAcceptanceControlRequirement`：
REQUIRED / NOT_APPLICABLE），本票按裁决三「客户合同答空列表」处理；票 10 表单（已进 main）今天怎么供这一格，由该票或后续票自决。

## Comments

- 2026-09-08 18:3x · 通道 1：立票并代裁形状（用户「你自决」授权）。第 2 波派单顺序：本票 + 14（14 不依赖词表）先开；本票落点广播后
  12/13/15/17 再点名派。
- 2026-09-08 20:0x · 通道 4：落点已 broadcast（`[落点 awf/20]`，函数名 / 类型名 / 端点 / 各册集合名与码），供 12/13/15/17 写消费端。
- 2026-09-08 20:0x · 通道 1 → 通道 4：占号准、`unconfigured_intake.go` 地盘外同包改动准；Intake 挂字面量的理由须写真（非预览口那条），
  票面列「判断点：隔离读态不放行」归 owner 复核；`fetchPublicationVocabulary` 遇 403 交出可辨的未配置、不内置码回退——均已照办（见完成记录）。

### 评审 ← 通道 1（非作者；隔离子代理起不来——鉴权错，推送方按 `/code-review` 正文串行自查）· 钉 `802ae400` · 20:5x

基 `5a209f70`，隔离 detached 检出 `%TEMP%\idp-review-awf20`，只读；票面笔 `509b67ce` / `802ae400` 与清点笔 `b0667d3b` 不评代码。

**Standards 轴**

- **阻断**：无。
- **非阻断**：
  1. `party/publication-draft-api.ts` import 行由 `{ postMasterData, … }` 改为 `{ exchangeMasterData, postMasterData, … }`——共享文件上**改了一行**而非纯加行
     （numstat +55/−1）。加一个名字别无写法，记录即可；12/13/15/17 在同文件加行时以 main 上这一行为准。
  2. `domain/publication_vocabulary.go` `PublicationVocabulary` 是按 kind 的 switch，与 `CanonicalizePublicationContent` / `IsRegisterCanonicalized` 同一
     类型上的第三处 switch（Repeated Switches，判断题）——三处各答不同问题（能否规范化 / 有没有词），合成一张表反而把「已接」再叠一义（票 09 评审
     已记「已接」两义），不改。
  3. `closedCodes` 扫整个 uint8 值域而非首末常量：作者注释写明理由（不另抄边界）；代价是每次请求 256 次判据调用 × 集合数，对一个低频读口可忽略，记下免得
     日后有人当性能问题改成常量表——那正是它要避免的第二份名单。
- **无发现**：领域文件只导入 `math`，无 HTTP / pgx；集合从既有枚举的接受判据（`valid` / `declarable` / `Declared`）与 `String()` 逐值列出、不另写常量表，
  `manualReview` / `allowance` 不列「未声明」（ADR-0120 Decision 三）；测试以导出构造门为预言机双向钉住码与序（`codesAccepted` 扫值域喂构造门），
  另钉票 12 / 15 / 17 点名的字面；`endpoints.go` 一行挂字面量 `UnconfiguredIntake{}`，行上注释写的是真理由（唯一消费者是四口喂的表单；不读存储读面不满足
  ADR-0078 入格判据；挂 `commercialCatalogueIntake` 会让 `TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines` 二分多一形态），不是「同预览口」；
  `PublicationVocabularyIntake` 单独成接口，隔离读 Intake 编译期装不进；kind 解析先于 Intake，判据同 `/commercial-policies`（未配置对全部类别同答 403）；
  `vocabularyOptions` 词表没收录的码原样示出、不给默认选中、不补占位；注释中文、无行号、无跨文件计数（「理由两条」数的是自己下面两个 bullet）。

**Spec 轴**

- **阻断**：无。
- **非阻断**：
  1. 票 20 裁决三只点名 12/13/15/17 四册有集；`PRICE_RULE` 正文其实也有三个封闭集（`PriceDirection` / `PlanBindingConversion` / `TaxDisposition`，
     票 14 表单今天按本页词表自持、不经词表口），本口对它答 `sets: []`。按票面属正确实现；若日后要让 14 的表单也经词表口，在 `PublicationVocabulary`
     加一支 case 即可（作者注释已写「不换形」），归伞票收口一并看，不阻。
  2. 「`endpoints.go` 只多一行」：条目只多一行，行上另有几行注释——作者已在完成记录写明出入，且是通道 1 20:0x 要求的。
- **无发现**：完成判据逐条——`GET /commercial-publication-vocabularies?kind=` 答 `{outcome: PUBLICATION_VOCABULARY_LISTED, kind, sets:[{name, codes}]}`；
  kind 集合外 / 缺席 400 `MALFORMED_REQUEST` 带 `problems[{field:"kind"}]`（`TestVocabularyEndpointRefusesAnUnknownKindWithAFieldProblem`）；kind 合法
  没词 200 + `sets: []` 不是 404（`…AnswersAnEmptyListForRegistersWithoutClosedSets`）；未配置 403 不带 outcome（`…KeepsTheUnconfiguredAndMethodGates`）；
  各册集合名 = 载荷键名、码按枚举序；TS 四件只加在文件末尾，`fetchPublicationVocabulary` 走 `exchangeMasterData` 把 403 交成 `unconfigured` 一格；
  判断点「隔离读态不放行」单列归 owner。边界：不改任何表单、不动 `ports.go`、无迁移。

**结论**：Standards 3 条非阻断（最重：共享 TS 文件改了一行 import）；Spec 2 条非阻断。**无阻断**，可重放。自跑（隔离检出，未设 DSN）：
`go test -count=1 ./internal/partycommercial/domain/ ./internal/partycommercial/adapters/http/ ./cmd/parcel-api/ ./internal/architecture/` 四包 ok。

- 进 main 记录（2026-09-08 20:5x，通道 1）：在 `0f9eaa1e` 上六笔重放零冲突 `509b67ce→fa1f249a`、`0fb66cd8→fdb3043a`、`d894751a→00a122d0`、
  `4eb2492f→2acf6107`、`17a962a5→794ec2dc`、`802ae400→c29b50d2`；清点笔 `b0667d3b` 不带，tip 重生成 `c05ee34d`（PC 生产 105→107 / 测试 109→111、
  端点 106→107）。验证钉 `c05ee34d`：gofmt 空；build / vet 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**
  （20:44→20:54）；admin-web tsc 0、run-tests 123/123；探针 `TestPublicationDraftWritesRefuseToRunOutsideATransaction` 含 DSN **PASS**。本笔（票面 + 伞票 07
  子票表 + tasks.md）只动 .md。
