# 04 审批职责规则、批准与发布

Category: enhancement
Status: resolved——2026-10-11 00:18 通道 1 重放进 main：`7749db93`…`cf7af06d`，清点 `b07939a7`；修复笔推送方自审，进 main 记录见文末。此前 阻断已修，待重放——2026-10-11 00:1x 通道 4（单 task-d1ca5f8d）：评审 ← 通道 2（`task-0a040d5e`，钉 `09795163`）Standards 阻断一条（S1）修于 `38e366f6`，S2 注释笔 `303d79cd`，票面是其后一笔；评审照录与逐条处置见文末 Comments。此前：in-progress——**完工，待评审与重放**（2026-10-10 通道 4；分支 `mcp4-pci04`，代码 tip `f3e79282`，基 `e266a876`；其后是登记册、清点与本票面三笔）；完成记录见文末。此前：in-progress——2026-10-10 通道 4 认领（单 task-ea81d298-cf3d-4f0e-adc3-2e4add1af98f，重派 task-d4d0063c），分支 `mcp4-pci04` 基 `e266a876`，工作树 `/home/tops/workspace/idp-parcel-mcp4-pci04`。此前：ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 03（已解：03 resolved，main `f8177a9d`）
地盘：
- `migrations/parcel_pricing/`；
- `internal/parcelpricing/`：审批职责规则、操作者主体消费面、批准与发布编排及 HTTP；
- `cmd/parcel-api/`；
- `docs/product/PILOT-PARAMETER-REGISTER.md` 一行；
- `docs/product/MECHANISM-INVENTORY.md` 重生成。
出处：[spec](../spec.md) 自决第 2 格；ADR-0101 决定五、六；ADR-0126 决定三。

## 要做的

1. **审批职责规则**（PP 自有，按租户）：录入者与批准者须否为不同主体、批准者须持哪一格授予。
   - 立读口与表；写口只给测试用。
   - 两格都不要求也是一条合法的租户声明。
2. **操作者主体消费面**：主体引用加授予集，由 Intake 把信封译成它。形状同 PC 的 `OperatorSubject`，各上下文各立一个。
3. **批准**：只接 `已校验`。
   - 先读审批职责规则：未登记即答`未配置`、不放行；主体相同或授予不足各答一格。
   - 通过后记批准者与批准时刻，转 `已批准`。
4. **发布**：只接 `已批准`。
   - 用草稿上的方案快照、源文件身份、方向授权引用，加上作为 `publicationApprover` 的批准者主体，立登记，交既有 `RegisterPriceCard`。
   - 不重解析、不重算（ADR-0101 决定四）。
   - 答案代数一格不改；落定后草稿转 `已发布`。
5. **端点**：`POST /pricing-price-card-draft-approvals` 与 `POST /pricing-price-card-draft-publications`，挂 `UnconfiguredIntake{}`。载荷只收草稿引用。（2026-10-10 通道 4：照 spec 自决第 4 格 2026-09-25 更正改挂操作者渠道的登记册 Intake，依据见判断项 1；原句留痕。）
6. **参数登记册**增一行「价卡发布审批职责规则」：实例半边，租户取值留空，答`未配置`。

## 验收

- 批准门每一格有测试：未配置、主体相同、授予不足、通过、状态不对。
- 发布路径有测试：交给登记用例的摘要与草稿上的逐字节相等；登记用例的每格答复原样交回。
- Postgres 适配器带 DSN 测试；端点表测试含新行；机制清点重生成。

## 形态

碰 Go、SQL 与端点表，走并行会话那条路。

## 完成记录（2026-10-10，通道 4；分支 `mcp4-pci04` 基 `e266a876`，代码 tip `f3e79282`；待非作者评审与重放）

派单 `task-ea81d298`；证据层级 `S`（合成替身与 `SYN-` 夹具，演示租户 `SYN-TENANT-01`）。进 main 的 SHA 由重放那一笔换，这里记分支上的：

- `1ca18b67` 认领；
- `d5d79429` 领域、端口与编排：`OperatorGrant` / `OperatorSubject`、`PriceCardApprovalDutyRule`、`PriceCardDraft.Approve` / `Registration` / `MarkPublished`；端口 `PriceCardDraftProgress`、`PriceCardApprovalDutyRuleView` / `Registry`；编排 `ApprovePriceCardDraftHandler`、`PublishPriceCardDraftHandler`；
- `087b4185` 迁移 `parcel_pricing/0012_price_card_approval_duty_rule.sql` 与 Postgres 适配器：`PriceCardApprovalDutyRules`，`PriceCardDrafts` 加 `LoadPriceCardDraft` / `AdvancePriceCardDraft`（0012 落在既有 `parcel_pricing` 目录，整目录嵌入，共享接线文件不动）；
- `f3e79282` HTTP 两口与装配：`NewApprovePriceCardDraftEndpoint` / `NewPublishPriceCardDraftEndpoint`、`OperatorRegistryIntake` 两口译法与 `UnconfiguredIntake` 两口未配置译法、`OperatorIdentity.Grants` 与身份翻译层的授予集、`cmd/parcel-api` 端点表两行与 `buildPriceCardDraftApproval` / `buildPriceCardDraftPublication` 两只事务包装；编排结果改为导出字段（照本包 `SubmitPriceCardDraftResult`）；
- `3291b79c` 参数登记册 `PAR-SET-12`「价卡发布审批职责规则」；
- `d8e8bb68` 机制清点在 `3291b79c` 的干净检出上重生成（parcelpricing 生产 119→124、测试 108→112；parcel_pricing 迁移 11→12；接入面端点 137→139；端口声明 457→460，基线口径多缺的一格是 `PriceCardApprovalDutyRuleView`，清点自标「虚低：精确口径已实现」）；
- 本票面随其后一笔。

验收逐条：

- **批准门每一格有测试。** 未配置：`TestApprovalAnswersNotConfiguredWhileTheTenantHasNoRule`（草稿一字不动）。主体相同、授予不足、通过：领域 `TestApprovalIsGatedByTheApprovalDutyRule`（另含「两格都不要求时录入者自批放行」）与编排 `TestApprovalGateAnswersEachCellOfTheRule`。状态不对：领域 `TestOnlyAValidatedDraftCanBeApproved`、编排 `TestApprovalAnswersFromTheCellTheDraftIsIn`（`草稿`答 `DRAFT_NOT_VALIDATED`，`已批准` / `已发布`各答其格，三格都不读规则）。另有并发被替换答 `DRAFT_CHANGED`、依赖读不动上抛（`TestApprovalThatLosesTheRaceOrItsDependenciesIsNotAnApproval`），HTTP 每格取 200（`TestTheApprovalEndpointAnswersEveryCell`）。
- **发布路径有测试。** 摘要逐字节相等：`TestPublishingHandsTheApprovedDraftItselfToTheRegistration` 拿交给登记用例的那一份比录入时算出的内容摘要与规范化版本，源文件身份、方向授权引用照抄，`publicationApprover` 是批准者而不是录入者；领域 `TestTheRegistrationIsTheApprovedDraftItself` 同证。每格答复原样交回：`TestEveryRegistrationAnswerIsHandedBackAsIs` 逐格演 `RECORDED`、`ALREADY_REGISTERED`（落定、草稿转`已发布`）、`CONTENT_CONFLICT`、`CANONICALIZATION_DIFFERS`（未落定、草稿留在`已批准`）与 `UNDECIDED`（原因随错误上抛），HTTP 侧 `TestThePublicationEndpointHandsTheRegistrationAnswerThrough` 原名照交。`NOT_ACCEPTED` 经草稿到不了：已批准草稿交出的登记必非零值，未单测。
- **Postgres 适配器带 DSN；端点表含新行；机制清点重生成。** `TestApprovalAndPublicationAdvanceTheDraftRow`、`TestAnAdvanceFromAStaleReadIsSuperseded`、`TestTheProgressWritesNeedATransaction`、`TestThePriceCardApprovalDutyRuleIsRegisteredOncePerTenant`；`businessEndpointProbes` 与换口名单各加两行，`TestEveryAssembledEndpointAnswersUnconfigured`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel` 按表断言；装配点 `TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase` 在真库上走「规则没登答未配置 → 登规则后换人批准 → 发布入册 RECORDED → 再发布答已发布」。清点见 `d8e8bb68`。

各层用例先于实现写下；没有单独记录红跑——实现落地前这些用例引用的符号都不存在，编译即红。没有做变异复跑。

判断项（标「越权风险点 · 待 PP owner 复核」的是照先例自裁、票面没写定的格）：

1. **两口挂 `operatorRegistries.pricing`，不挂票面第 5 条写的 `UnconfiguredIntake{}`。** 依据：spec 自决第 4 格 2026-09-25 更正（「本批各口因此直接挂这个 Intake……票 02 起照此办」）；03 已照此办且通道 1 2026-10-09 同意；operator-channel/04 归类表写明价卡导入这一批的口归 price-card-import、并说 spec 第 4 格与它一致，同票第 2 件判断项 2 也写明批准者取自信封走的是本票这条主路径。批准门要的主体与授予集只有操作者渠道交得出，挂未配置 Intake 则批准门永远走不到。两头只做了这一头。
2. **授予集的词汇与来源。越权风险点 · 待 PP owner 复核。** PP 的授予是不透明的名字（`OperatorGrant`），只比相等；身份翻译层把信封里持有的能力面逐格译成授予名（`grantableFaces`：登记册配置写、主数据与运营查阅读；运营决定一格按决定种类另答、不进 `Holds`，不在其中）。规则要求的授予若不是能力面名，经操作者渠道恒答`批准者不合格`——要表达 spec 举的「定价主管那一级」这类租户自定等级，得等接入身份能力的授权模型扩到等级，归 ADR-0100 那一族与 ADR-0085 决定四另裁，不在本票。PC 那一条用的是商业权限等级，且其 Intake 译法尚未接；两边词汇不同在 spec 自决第 2 格「形状同、不共用」之内。代价的另一半（评审 ← 通道 2 Spec P1 补记）：两口经 `AuthenticateRegistryWrite` 以登记册配置写（`REGISTRY_CONFIGURATION_WRITE`）铸信封，到得了批准门的人必持这一格——规则要求它与不要求等价，空转放行。能力面的原义是「能在哪一族端点上做什么」，不是审批资格；`grantableFaces` 的 grantable 也不是接入身份能力 `checkGrantable` 的「可授」。两处注释已在 `303d79cd` 写明（评审 S2 后半）；登记册 `PAR-SET-12`「所需证据」原先把本项写成了定论，已改为指向本项（`38e366f6`，评审 S2 前半）。
3. **「状态不对」分三格答，且不读规则。越权风险点 · 待 PP owner 复核。** `草稿` → `DRAFT_NOT_VALIDATED`，`已批准` → `DRAFT_ALREADY_APPROVED`，`已发布` → `DRAFT_ALREADY_PUBLISHED`。照 PC `ApprovePublicationDraftHandler` 的分格；PP 多`草稿`一格是因为 PP 生命周期有四态，三种情形恢复动作不同。代价（评审 ← 通道 2 Spec P2 补记）：这几个答案名已随批准口上线，06 要逐格接；owner 若要并格，就是改线上契约。
4. **发布的答案代数。越权风险点 · 待 PP owner 复核。** `DRAFT_PUBLISHED`（登记答 `RECORDED` 或 `ALREADY_REGISTERED` 即算落定）/ `PUBLICATION_NOT_LANDED`（其余各格，登记那一格随结果原名交回）/ `DRAFT_NOT_APPROVED` / `DRAFT_ALREADY_PUBLISHED`（不重交登记）/ `DRAFT_NOT_FOUND`。登记 `UNDECIDED` 时编排连原因上抛、整笔回滚，HTTP 答 `NO_ANSWER_FORMED`，同登记口。登记答案代数一格未改（ADR-0101 决定五）。照 PC `PublishPublicationDraftHandler` 的形；PP 没有生效边界那一格。代价（评审 ← 通道 2 Spec P2 补记）：这些答案名已随发布口上线，06 要逐格接；owner 若要并格，就是改线上契约。
5. **并发不加锁。** 推进口按「前一格 + 录入者 + 录入时刻 + 内容摘要」做条件 UPDATE，推进成发布时另核批准者与批准时刻；判不上答`已被替换`、行一字不动。批准据此答 `DRAFT_CHANGED`；发布在登记落定后草稿跟不上即上抛，登记写入与草稿推进同一笔事务，整笔回滚。
6. **批准、发布各一笔事务，时刻取系统时钟。** 判据同 `transactionalPriceCardDraftSubmission`：落点是业务答案就提交，返回错误整笔回滚。
7. **状态码。** 批准每格 200（改的是已有那一行，不新落行）；发布登记新落一版取 201，其余 200；载荷封闭只收 `planId`、`planVersion`，夹带身份格按未知键拒。
8. **审批职责规则只立读口、表与只给测试用的写口**，同 ADR-0126 决定五的形；登记面（CLI 或管理台）归治理写面那一族另裁。
9. **无新 ADR**，未改任何已接受 ADR 的决定，没有占 0177。

没做的、留给后续：

- 审批职责规则的登记面（同判断项 8）；租户上线时登 `PAR-SET-12` 那一行是实施的事。
- parcel-pricing `CONTEXT.md` 没加「价卡发布审批职责规则」词条：地盘不含 CONTEXT，概念权威在 ADR-0101 决定六与 spec 自决第 2 格；PC 那一条在 PC CONTEXT 有词条，要不要对齐由 PP owner 定。评审 ← 通道 2 Spec P3 同指这一格（连同操作者主体词条），由通道 1 另立票接。
- 06（管理台草稿签）等本票进 main；管理台调这两口时载荷只带 `planId`、`planVersion`。

验证（钉 `3291b79c` 的干净 detached 检出，含 PG）：

- `gofmt -l internal/parcelpricing cmd/parcel-api` 零行；`go build ./...` 与 `go vet ./...` 全仓退 0。
- `go list -test` 反查依赖本票动过的 parcelpricing 六个包、`migrations` 或 `cmd/parcel-api` 的全部包，共 62 个（其中 `cmd/*` 14 个），加 `./internal/architecture/...`，带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable` 跑 `go test -count=1 -p 1`：退 0，59 ok / 4 无测试 / 0 FAIL。
- `-v` 单跑 `cmd/parcel-api` 的 `TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase` 与 `adapters/postgres` 的 `TestApprovalAndPublicationAdvanceTheDraftRow`：均 `--- PASS`，非 SKIP。
- 清点在同一检出上重生成，见 `d8e8bb68`。
- 未跑全仓 `go test ./...`，留推送方重放后那一跑。

## Comments

### 评审 ← 通道 2 · 钉 `09795163` · 23:5x

（`task-0a040d5e`，非作者评审；通道 1 派修复单 `task-d1ca5f8d` 时整段转来，作者照录。原文的二级标题降为四级，文字不动。）

非作者评审 price-card-import/04（基 e266a876 → 09795163，代码 tip f3e79282）。结论：代码逻辑无阻断；1 条阻断在参数登记册（改一个词即可）；Standards 非阻断 2、Spec 非阻断 3。未修任何东西。

#### Standards

【阻断】
S1 docs/product/PILOT-PARAMETER-REGISTER.md PAR-SET-12「当前登记」以「未配置（租户取值留空）」开头，违反同文件「登记规则」：当前登记“必须以以下一种状态开头：待提供、待核验、待决策、已确认、本期不适用、已失效”（「参数就绪门槛」再申只能用这六种）。兄弟行 PAR-COM-18 与 ADR-0126 决定五登的都是「待提供」。改成「待提供：未配置……」即合规，后文不动。
【非阻断】
S2 adapters/accessidentity/operator_registry_authenticator.go `grantableFaces`：机制没越权（只问 envelope.Holds、用 accessidentity 原名、不另设闸），但把 CapabilityFace（accessidentity 注释：操作者能在哪一族端点上做什么）读成审批资格；PAR-SET-12「所需证据」写死「授予名即操作者信封里持有的能力面」，把标着待 PP owner 复核的判断项 2 先落成登记册口径——票面说待定、登记册说已定（单一权威）。domain `OperatorGrant` 注释「本上下文不枚举」与本上下文这份枚举相抵；`grantableFaces` 与 accessidentity `checkGrantable` 的「可授」同词异义（判断题）。
S3（判断题·Duplicated Code）cmd/parcel-api/assemble_pricing_import.go 两只新事务包装与既有 transactionalPriceCardDraftSubmission 同形，沿 cmd 先例，可不动。
【无发现】注释全中文，新增注释无行号与跨文件计数；PP 自立 OperatorSubject / PriceCardApprovalDutyRule，不引 partycommercial（architecture 测试过）；0012 不种行，scripts/ 未动，未配置不给默认。

#### Spec

【阻断】无。
【非阻断】
P1 判断项 2 的代价只写了一半：批准口经 AuthenticateRegistryWrite 以 REGISTRY_CONFIGURATION_WRITE 铸信封，到得了批准门的人必持这一格——规则要求它与不要求等价（空转放行）；票面只写了「非能力面名恒答批准者不合格」那一半。
P2 判断项 3、4 写了理由（照 PC 分格；PP 多一态、无生效边界），没写代价：这些答案名已上线，06 要逐格接，owner 若要并格就是改线上契约。
P3 parcel-pricing CONTEXT.md 未收「价卡发布审批职责规则」与操作者主体词条；作者已列为留给 owner，地盘不含 CONTEXT，需另票接（AGENTS「改文档」）。
【无发现】验收三条逐条有测：批准门五格领域与编排双层都有；发布摘要逐字节相等、登记各格原样回（UNDECIDED 上抛→500 NO_ANSWER_FORMED，与 base 上 NewRegisterPriceCardEndpoint 同）；PG 带 DSN、端点表两行、清点重生成。判断项 1 依据确在 spec 自决第 4 格 2026-09-25 更正（本批各口直接挂这个 Intake，票 02 起照此办），第 5 条原句留痕写清。判断项 5：READ COMMITTED 下被挡的 UPDATE 按已提交新行重判 WHERE，0 行即 SUPERSEDED、行一字不动；发布失配上抛，整笔回滚连登记一起撤。NOT_ACCEPTED 未单测理由成立：PriceCardDraft.Registration 经 NewPriceCardRegistration，零租户先以 error 返回，到不了登记用例。无票外改动（OperatorIdentity.Grants 是第 2 条「由 Intake 把信封译成它」所需）。

#### 读 diff 前先列的清单（逐条对 diff 均符合，除上列）

批准门：不在→DRAFT_NOT_FOUND；`草稿`→未校验；`已批准`/`已发布`各答其格且不读规则；只有`已校验`才读规则；未登记→未配置、不放行；要求不同主体且同人→需换人；要求授予而不持→不合格；两格都不要求→放行（含自批）；通过→记批准者与批准时刻（不早于录入）转`已批准`；读写之间被替换→DRAFT_CHANGED、行不动；他租户规则/零值主体/时刻倒挂→error。
发布交给登记：只接`已批准`；交具体 *RegisterPriceCardHandler 的是 草稿租户 + 方案快照原值 + 源文件身份 + 方向授权引用 + publicationApprover=草稿上记的批准者；RECORDED/ALREADY_REGISTERED 落定转`已发布`，其余草稿留`已批准`、登记格原样带回；无身份格；登记与推进同一事务。
不能变：登记答案代数六格不增不改名；不调模板读口、不重算摘要；规则未登记答未配置且不放行、不默认单人或双人；草稿四态与 ResubmissionOf 不改；受控 CLI 与 seed 不变。

#### 变异（go test -overlay，副本与 JSON 在 /tmp/mcp2-mut，检出未改）

M1 批准门跳过「主体相同」：杀死——domain TestApprovalIsGatedByTheApprovalDutyRule、application TestApprovalGateAnswersEachCellOfTheRule/主体相同。
M2 发布把录入者当 publicationApprover：杀死——domain TestTheRegistrationIsTheApprovedDraftItself、application TestPublishingHandsTheApprovedDraftItselfToTheRegistration（cmd 装配测不查 approver，未杀，属预期）。
M3（加做）条件 UPDATE 去掉录入者/录入时刻/摘要三格：杀死——postgres TestAnAdvanceFromAStaleReadIsSuperseded。

#### 验证（检出 09795163）

go build ./... 与 go vet ./... 退 0；带 DSN go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/architecture/... 全 ok；./cmd/parcel-api/... ok；-v 单跑 TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase 与 TestApprovalAndPublicationAdvanceTheDraftRow 均 --- PASS，非 SKIP。git worktree remove /tmp/idp-review-pci04-mcp2 已做（无 --force）。

### 阻断修复记录（2026-10-11 00:1x，通道 4，`task-d1ca5f8d`；同一分支 `mcp4-pci04`，基 `e266a876` 不动、未 rebase）

| 笔 | 做了什么 |
|---|---|
| `38e366f6` | 登记册 `PAR-SET-12`：「当前登记」改以「待提供」开头（「待提供：未配置（租户取值留空），……」，后文内容不动）；「所需证据」里「授予名即操作者信封里持有的能力面」改为指向本票判断项「授予集的词汇与来源」 |
| `303d79cd` | 只改注释：领域 `OperatorGrant` 改说领域不枚举、哪些名字会出现由授权模型与消费侧译法（`grantableFaces`）一起决定；`grantableFaces` 写明此 grantable 非 `checkGrantable` 的「可授」，并写下拿能力面当授予格这一暂定译法的两头代价。代码一字未动 |
| 本笔 | 票面：评审照录、本修复记录、判断项 2 / 3 / 4 补代价、「没做的」补 P3 去向、Status |

逐条处置：

- **S1（阻断）**：已修，`38e366f6`。写法照同文件「登记规则」与兄弟行 `PAR-COM-18`。
- **S2（非阻断）**：前半已修，`38e366f6`——登记册不再替待复核的判断项下定论。后半（`OperatorGrant` 注释与 `grantableFaces` 的枚举相抵、「可授」同词异义）派单给的两条路里取「改注释」，`303d79cd`：那句「本上下文不枚举」留在代码里会误导下一个读的人，只记在票面上它照样在那里。判断项 2 也一并补记。
- **S3（判断题）**：不动。两只事务包装沿 `cmd/parcel-api` 既有 `transactionalPriceCardDraftSubmission` 的先例，评审自注可不动。
- **P1**：补进判断项 2（本笔）。
- **P2**：补进判断项 3、4（本笔）。
- **P3**：只记，见「没做的、留给后续」CONTEXT 那条；由通道 1 另立票。

验证（`mcp4-pci04` 工作树，提交前的工作副本内容即 `303d79cd`，WSL，go1.26.8）：改动的两份 `.go` `gofmt -l` 无输出、无 CR、无 BOM；`git diff -U0` 下增删行全是注释行；`go build ./...`、`go vet ./...` 退 0。只改了 `.md` 与注释，按派单不重跑测试，由通道 1 自审。

### 修复自审（通道 1 · 2026-10-11 00:1x）

改动只是 `.md` 与注释（parallel-sessions「不评什么」），推送方自审：`38e366f6` 的「当前登记」以「待提供」开头，合乎同文件「登记规则」；「所需证据」改为用引文指向判断项，不再替待复核的格下定论。`303d79cd` 的 `.go` 增删行去掉注释行为空（`git diff -U0 09795163 303d79cd -- '*.go'` 实测）；两段新注释只写取舍与代价，无行号与计数。无发现。

## 进 main 记录（通道 1 · 2026-10-11 00:18）

- 重放：上一个通道 1 会话 2026-10-10 23:18 在 `idp-parcel-replay-c1b` 上叠在 `70f32c2a` 的那一版不含修复，不再用；本次在隔离检出 `/tmp/idp-land-pci04` 上从 `9024a892` 另起，cherry-pick 分支各笔，零冲突；分支上的清点笔 `d8e8bb68` 不搬，清点在重放 tip 上重生成。本票改过的每份非清点文件与作者 tip `aecd5857` 逐文件 `git diff` 为空。

  | 分支 `mcp4-pci04` | main |
  |---|---|
  | `1ca18b67` 认领 | `7749db93` |
  | `d5d79429` 领域、端口与编排 | `537179a7` |
  | `087b4185` 迁移 0012 与 Postgres 适配器 | `c31da436` |
  | `f3e79282` HTTP 两口与装配 | `fa7f19fa` |
  | `3291b79c` 参数登记册 `PAR-SET-12` | `fb91122b` |
  | `d8e8bb68` 清点 | 不搬，由 `b07939a7` 重生成 |
  | `09795163` 完成记录 | `fa5f5e5f` |
  | `38e366f6` 阻断修复（登记册） | `d692904e` |
  | `303d79cd` 注释 | `b0a5ff9f` |
  | `aecd5857` 评审照录与修复记录 | `cf7af06d` |

- 清点 `b07939a7`（在 `cf7af06d` 的检出上重生成）：parcelpricing 生产 119→124、测试 108→112；parcel_pricing 迁移 11→12；接入面端点 137→139；端口声明 457→460，基线口径多出的缺口是 `PriceCardApprovalDutyRuleView`（清点自标「虚低：精确口径已实现」）。
- 门（同一检出 @ `b07939a7`）：gofmt 两改动目录无输出；`go build ./...`、`go vet ./...` 退 0；真库探针 `TestFreezeScopesAreInvisibleToEachOther` 为 PASS 非 SKIP；带 DSN `go test -count=1 -p 1 ./...` 00:15:30→00:18:06，138 ok / 0 FAIL / 14 无测试 / 0 cached；`-v` 单跑 `TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase` 与 `TestApprovalAndPublicationAdvanceTheDraftRow` 均为 PASS。
- 推：00:18:41 `ls-remote` 核 `9024a892` 未动 → 00:18:45 `push b07939a7:main` 成，远端 main = `b07939a7`（本票各笔与清点，其下无他人提交）；共享树 ff 同 SHA。本笔簿记在其上，纯 .md，推送方自审。
- 评审：非作者 ← 通道 2（Standards 阻断一，已修；见上 Comments）；修复笔推送方自审（见上）。
- P3 立票 [07](07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md)（needs-triage）。
