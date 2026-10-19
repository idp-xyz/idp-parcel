# 96 客户追踪视图口还挂字面量未配置，首方客户渠道读不到已形成的客户视图

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg96（2026-10-19 作者子代理，见文末「完成记录」与「评审记录」；隔离评审 0 阻断，非阻断已按评审记录处置）。此前 needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点二）。
Blocked by: 无
地盘：首方客户族的查询 Intake（`internal/accessidentity` 或 VE `adapters/http`，照提交口那一口的分法）；`cmd/parcel-api/endpoints.go` 里 `/customer-tracking-view` 那一行；能力词表；种子 `scripts/demo-seeds/data/access/08` 一行授予与 README 授予说明。
出处：[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四（覆盖面点名查阅面的 `/customer-tracking-view`，逐端点换）、决定八（查阅口不判准入范围）；[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

演示包裹有效交付、PS 终局之后，VE 已按投影形成 `SYN-ACCOUNT-01` 名下这件包裹的客户视图（`visibility_exception.customer_view` 一行，带前版回指）。`GET /customer-tracking-view` 带配置员令牌、带合成货主系统的首方客户令牌，都答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。

`assembleBusinessEndpoints` 里这一口的 Intake 是字面量 `visibilityhttp.UnconfiguredIntake{}`。`visibilityhttp.QueryIntake` 的头注仍写认证方式「属 `PAR-INT-01` 待提供」，早于 ADR-0139。提交口与送站预报两口已换首方 Intake，这一口没有承接票。

## 三类

- 机制：首方客户族的查询 Intake——令牌核验、客户渠道册取（租户，客户账户），查询键只取册上那一行，请求里自报的账户不看；查阅口不判准入范围（ADR-0139 决定八）；答复格照决定四分格。
- 产品策略：客户面能力词表给这一口一格，粒度随 [first-party-api/01](../../first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md)。
- 租户取值：演示租户给合成货主系统登这一格授予（种子 `data/access/08`），证据记 `S`。

## 完成判据

- 首方客户令牌读自己账户名下包裹的视图答 `200`；别的账户的包裹与不存在的包裹同答（UC-VE-008 不可枚举）；操作者令牌答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`；没有这一格授予答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。
- 首方族三件不设时仍答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。
- `QueryIntake` 头注改指 ADR-0139。
- 真库装配用例；演示上重走一次，证据记 `S`。

## 规模

S–M

## 完成记录（2026-10-19，作者子代理，分支 `claude-dcg96`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。本票不属交接与派送方案（`handover-dispatch-plan.md`）也不属 TF 链方案，权威是 ADR-0139 决定二、四、八与 UC-VE-008；先例是 [first-party-api/00](../../first-party-api/issues/00-first-party-customer-intake.md) 的提交口与 [ps-external-mark-relations/09](../../ps-external-mark-relations/issues/09-pre-advice-submission-on-the-first-party-customer-channel.md) 的送站预报口。没有遇到 ADR-0139 没裁的难逆转取舍，没立 ADR（预留的 0221 没用）。

### 做了什么

1. **能力词**：`internal/accessidentity/customer_client.go` 的客户面能力封闭词表追加 `CUSTOMER_TRACKING_VIEW_QUERY`（`CustomerCapabilityTrackingViewQuery`），只追加常量与词表一项。迁移 `migrations/access_identity/0017_customer_tracking_view_query_capability.sql` 重建 `customer_client_grant_capability_decided`，全集以 0016 为底再加这一个词。它不进 `customerCapabilitiesAdmittedAtMint`：查阅口不判准入。
2. **查阅身份**：新文件 `internal/accessidentity/customer_client_query.go` 给出 `CustomerClientMinter.MintTrackingViewQuery`。它沿用 `admit`：校验令牌、查客户渠道册、核证书绑定、核这一口的授予，不判准入范围。交回的 `CustomerTrackingViewQueryEnvelope` 只带（租户，客户账户），字段不导出，零值不可用（`Minted`）。它不读请求。
3. **VE Intake**：
   - 新文件 `internal/visibilityexception/adapters/http/customer_tracking_view_intake.go` 定义 `CustomerTrackingViewIntake`。它先认证，查询键的租户与账户只取自认证结果；查询串只认 `parcel` 一格，其余任何一格都答坏请求。
   - 首方族在 VE 包里新增两格：`ErrCustomerClientCredentialRejected`、`ErrCustomerClientNotGranted`。依赖读不动沿用 `ErrIdentityDependencyUnavailable`。
   - `NewQueryCustomerTrackingViewEndpoint` 的失败映射收进 `writeCustomerQueryIntakeProblem`，按 ADR-0139 决定四分格。
   - `QueryIntake`、`ErrMalformedRequest`、`ErrAccessChannelNotConfigured` 的头注改指 ADR-0139。
4. **VE 认证适配**：新文件 `internal/visibilityexception/adapters/accessidentity/customer_tracking_view_authenticator.go`，经首方铸造器铸查阅身份，并逐格翻译共享接入身份能力的错误。
5. **装配**：
   - 新文件 `cmd/parcel-api/assemble_customer_tracking_view.go`，含 `buildCustomerTrackingViewIntake` 与 `customerTrackingViewQueryIntake`。后者在 Intake 缺席时交回字面量未配置。
   - `endpoints.go` 只改 `/customer-tracking-view` 那一行，并在参数表末尾追加一格。
   - `main.go` 追加装配段与实参，用的铸造器与提交口是同一只。`endpoints_test.go` 的辅助函数在末尾追加 `nil`。
6. **种子**：`data/access/08` 追加授予 `SYN-GRANT-CUSTOMER-TRACKING-VIEW-QUERY-01`。README 组成表 access 那一行补这一格，「首方客户渠道」一节末尾追加「客户追踪视图」一项。
7. **文档**：
   - UC-VE-008 补 `AT-VE-171`–`173`。UC-VE 原最大号是 `AT-VE-170`。
   - 演示动线在「送站交接单」之后追加「客户追踪视图」一节，带实走记录。
   - 模块指南 `/customer-tracking-view` 那一行改指首方客户渠道。
8. **立票**：
   - [101](./101-customer-tracking-view-locates-by-the-internal-parcel-id-only.md)：查阅口只认正式包裹号，客户经首方 API 拿不到这个号。
   - [102](./102-claims-line-has-no-first-party-customer-intake.md)：`/claims` 还挂字面量未配置。

### 代决

1. **能力词一口一词**：词是 `CUSTOMER_TRACKING_VIEW_QUERY`，照 first-party-api/00「代决」一节「客户面能力词表一口一词」。带 `CUSTOMER_` 前缀，是为了与运营侧的追踪投影查阅（`/tracking-projections`）在词面上分开。
2. **查阅身份另立一型，不经 `seal`**：查阅不形成来源请求，没有来源请求键可推。若借 `SourceEnvelope` 封一个，就得给请求键编一个值；另立一型，也让查阅身份在编译期递不进任何写口。这一型不带来源格（客户端主体），理由见越权风险点 3。
3. **查询串封闭**：只认 `parcel`，恰好一次、不空白；带别的格（含自报 `customerAccount`、`tenant`），或查询串解不开（分号分隔、坏百分号转义），一律答 `400 MALFORMED_REQUEST`，照提交口载荷封闭解码的做法。悄悄忽略，会让客户系统以为它指名的账户起了作用。查询串在认证之后才读。
4. **定位只认正式包裹号**：视图库的键就是它。UC-VE-008 另两种查询标识（授权客户参考号、版本化外部标识）要跨 PS 解析，归票 101。
5. **VE 包里的哨兵**：首方族另立两格（令牌不过、未授予），不复用操作者那两格，理由同 nodeopshttp 的同名注释。依赖读不动与操作者族共用 `ErrIdentityDependencyUnavailable`：恢复动作相同，码也相同。
6. **准入那几格认证方不译**：查阅口不判准入，那几格真出现，说明铸造侧走错了口。这时落到端点的 `INTAKE_FAILED 500`，不冒充任何一种恢复动作。
7. **别账户的包裹答 `200 VIEW_NOT_FOUND`，不是 404**：协调方要点里写的是「换别账户包裹答 404」。端点照 ADR-0022 只用状态码答「答案形成没有」，`VIEW_NOT_FOUND` 是既有封闭两格之一，并承担 ADR-0029 的探针同答。改成 404，既改了 ADR-0022 的口径，也让「端点不存在」与「对象不可见」不可分。所以端点不动，票面完成判据「同答」照此满足。
8. **演示里的别账户对照用 SQL 直插的视图行**：种子里只有一台首方客户端（绑 `SYN-ACCOUNT-01`），`SYN-ACCOUNT-02` 经首方口提交不了。直插一行 `SYN-ACCOUNT-02` 名下的视图，只用来证「别人名下确有的视图也查不到」，演示记录里写明了。

### 越权风险点（待 owner 复核）

1. 能力词的名字与「一口一词」的粒度进了库上 CHECK 与演示种子。first-party-api/01 若归并（例如 `tracking:read`），改的是 CHECK、枚举与已登的授予行。
2. 查询串封闭：ADR-0140 的契约日后若给这一口加可选参数（分页、语言、批量），要同笔放开，否则客户系统带上就答 400。
3. 查阅身份不带客户端主体，查询不留「谁、何时查了哪件」的痕。UC-VE-008 没要求查询留痕，ADR-0139 也没裁。owner 若要对外查询审计，要在查阅身份里补来源格并加读日志，不改答复。
4. 生产形态下，这一口对真实客户系统几乎不可用：它只认正式包裹号，而首方 API 上拿不到这个号，演示靠操作者渠道读出（票 101）。
5. 查阅口不判准入范围（ADR-0139 决定八字面）。阶段治理暂停一个租户的生产写时，客户查询照答。owner 若要「暂停期间对外查询也停」，要另裁。
6. 迁移号 0017 取的是 main 上最大号的下一号（协调方指定的取法）。若别的票或另一会话同时取了 0017，或者晚进 main 时又重建了同一条 CHECK，进 main 时要改号，全集按「取此前各份的并集」重写。
7. `internal/accessidentity/customer_client.go` 属另一会话 idp-parcel-9b 声明的「客户面能力」地盘。本票只在常量块与词表末尾各追加一项，协调方要点授权了加能力词；合并时若两边都追加，按并集保留。

### 用例与完成判据对照

- 首方客户令牌读自己账户名下包裹的视图答 `200`，别的账户的包裹与不存在的包裹同答：
  - `cmd/parcel-api/assemble_customer_tracking_view_test.go` 的 `TestACustomerClientReadsItsOwnTrackingViewAgainstARealDatabase`（真库，含查询串自报账户答 400、准入范围未设照答、撤销授予之后答未授予）；
  - `internal/visibilityexception/adapters/http/customer_tracking_view_intake_test.go` 的 `TestCustomerTrackingViewThroughTheFirstPartyIntakeCannotProbeOtherAccounts`。
- 操作者令牌答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`，没有这一格授予答 `403 CUSTOMER_CLIENT_NOT_GRANTED`：`TestCustomerTrackingViewLineRefusesOtherFamiliesAndClientsWithoutTheGrant`（同一个发行方替身签的操作者令牌、集成客户端令牌；不在册；只持提交与预报授予）。
- 首方族三件不设时仍答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`：`TestCustomerTrackingViewLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset`。Intake 缺席与三件未设装出的 Intake 逐字节相同，且都不读视图库。
- `QueryIntake` 头注改指 ADR-0139：见「做了什么」第 3 项。
- 铸造：`internal/accessidentity/customer_client_query_test.go`（身份取自绑定、授予按口、不问准入、各格与令牌一步不读册、词进词表但登不进准入对照）。
- 认证适配：`internal/visibilityexception/adapters/accessidentity/customer_tracking_view_authenticator_test.go`。
- AT：`AT-VE-150`、`151`、`153`（HTTP 与认证半边），`171`、`172`、`173`（新增）。

### 验证

- `go build ./...`、`go vet`（accessidentity、visibilityexception、cmd/parcel-api、cmd/parcel-access-register）、`gofmt -l` 干净。
- `go test -p 1 -count=1` 全过：`./internal/accessidentity/...`、`./internal/visibilityexception/...`、`./cmd/parcel-api/`、`./cmd/parcel-access-register/`、`./internal/architecture/`。全量与机制清点没跑，按协调方约定留到进 main。
- 演示实走（实测，只记 `S`）：
  - 环境：`79ad6122` 上编出 `parcel-api` 与 `parcel-dispatch`。一次性库 `dcg96_demo`，`seed.sh` 退 0，新授予答 `RECORDED`。私有 Dex 用同一份 `integration-dex.yaml`，宿主端口 5596，`iss` 不变，因为共享那一只仍不认合成货主系统。
  - 提交与视图：`submit-one-shipment.sh` 经首方口提交 `SHR-TCTWPC4IVFG5JV2HZUZ4S5XILY`。dispatch 几拍之后，VE 形成 `DPCL-HZGZRSETALCQU3RUXADEQGJNOM` 的视图 `CVW-Y6LB5VLNUWLJBOMIUIENS4XLIY`。
  - 本账户：`customer-token.sh` 取令牌查它，答 `200 CURRENT_VIEW`。
  - 别账户与不存在：直插的 `SYN-ACCOUNT-02` 视图与不存在的包裹都答 `200 {"outcome":"VIEW_NOT_FOUND"}`。
  - 拒绝面：自报 `customerAccount` 答 `400`。不带令牌、配置员令牌、合成报关服务商的集成客户端令牌都答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`。`customer-client-revoke` 撤掉授予之后答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。
  - 收尾：走完已停进程、删容器、删库，二进制与令牌已清。详见演示文档「客户追踪视图」一节。

### 偏离与待裁

- 协调方要点写的「换别账户包裹答 404」，实际答 `200 VIEW_NOT_FOUND`，理由见代决 7。
- 号段：
  - AT 用 `AT-VE-171`–`173`（UC-VE 原最大 170），新立票用 101、102，都在协调方给的范围内。
  - 迁移号 0017 照「main 最大号的下一号」自取，已写进越权风险点 6。
  - 没立 ADR，0221 没用。
- 跨票热点：
  - `endpoints.go` 只改那一行并追加参数；`main.go` 只新增行，不重排。
  - README 组成表 access 那一行是行内补一句，不是新增行，合并时注意。
  - 演示文档的新一节插在「对照组」标题之前。
- `docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md` 的入口表别的行早已过时（如 `/shipment-requests` 仍写「真实渠道未配置时拒绝」），本票只改了自己那一行。

## 评审记录（2026-10-19，隔离评审；修补者为作者子代理，分支 `claude-dcg96`）

阻断 0 项。非阻断 8 项：标「进 main 前修」的 5 项全修；其余 3 项里，UC-VE-008 前向指针一项顺手修了，合并热点与 owner 复核两项落文字。评审建议里写「如需」的 ADR-0139 补记没采纳，理由见下。

### 已修

1. **查询串解析吞掉错误**：`URL.Query()` 丢掉 `ParseQuery` 的错误，带分号或坏百分号转义的那一段被悄悄扔掉，余下照常作答，与代决 3、`AT-VE-171` 相悖。
   - `CustomerTrackingViewIntake.IntakeQuery` 改用 `url.ParseQuery(request.URL.RawQuery)`，出错答 `ErrMalformedRequest`，仍在认证之后读。
   - `TestCustomerTrackingViewQueryStringIsClosed` 补「分号段」「坏百分号转义」两格。在旧实现上这两格实测失败（`err = <nil>`），新实现上通过。
   - 代决 3 补上「查询串解不开」一类；`ErrMalformedRequest` 头注同补。
2. **`AT-VE-153` 两处说法不一**：
   - 原坏请求映射用例改名 `TestAMalformedQueryIsBadRequestWithoutOutcome`。Covers 只证「Intake 构造不出查询即 400、不带 outcome」，并补了不带 outcome 的断言。
   - 该用例注释里不再出现 `AT-VE-153` 字样。AT 盘点按扫描口径把注释里的 AT 号归到所在测试函数，字样留着就会被再归到这里。
   - `AT-VE-153` 点到 `cmd/parcel-api` 的 `TestCustomerTrackingViewLineRefusesOtherFamiliesAndClientsWithoutTheGrant`：不带令牌答 401，只持别口授予答 403。两格都带着包裹定位，答的都不是坏请求，也不读视图。HTTP Intake 取键那条（`TestCustomerTrackingViewIntakeKeysTheQueryByTheAuthenticatedAccount`）仍记「HTTP 半边」。
3. **AT 盘点没追记**：`.scratch/at-coverage-inventory.md` 文末加「2026-10-19 追记（票 demo-chain-gaps/96）」，`AT-VE-171` 至 `173` 入①，①、VE、合计各加 3。① 表补三行，`AT-VE-153` 改点，`AT-VE-150`、`151` 增引首方各面的用例。绝对数进 main 时随同波票按批重算。
4. **`internal/accessidentity/doc.go` 没跟上**：首方族那段补一句，说明同一只铸造器在客户查阅面另铸查阅身份（如 `CustomerTrackingViewQueryEnvelope`）。查阅身份不是 `SourceEnvelope`、不经 `seal`，理由见类型头注；它也不是绕过 `seal` 的路，因为它在编译期递不进任何收来源信封的写口。只在原句之后追加，不改原句，减少与另一会话在这一包的合并面。
5. **按序号指别的文件**：
   - `TestTrackingViewQueryWordJoinsTheVocabularyButNotTheAdmissionMapping` 的 Covers 改引 first-party-api/00「代决」一节「客户面能力词表一口一词」。
   - 票面代决 1 同改。
   - 「做了什么」第 6 项对 README 的「第 9 项」改成按条目名「客户追踪视图」指。
6. **UC-VE-008「首发约束与验收」段**：补一段前向指针（评审没标进 main 前修，改动小，顺手修了）。内容三句：
   - 查询接入归首方客户渠道（ADR-0139 决定四）；
   - 「隔离账户」所指的隔离形态照 ADR-0150 决定三逐口退场；
   - 租户取值未确认时，挡住查询的是客户渠道册上的查阅授予（`PAR-INT-13`），以及投影与视图派生所读的里程碑映射与披露策略，查询口本身不另设开关。

   原句不改。

### 没采纳

- **ADR-0139 文末补记**：没加。
  - 决定三的要旨是三族互不相认，信封类型让错铸在编译期走不通。查阅身份另立一型正是这条的落法：哪一族铸什么、哪一口归哪一族都没变，属于实现形状，不是难逆转取舍。
  - 包级权威说明（`doc.go`）与类型头注都已写明。在已接受 ADR 上再加补记，只是多一处同义复述，还多一个 `docs/adr` 合并热点。
  - owner 若要在 ADR 层留痕，文末补记即可，不涉正文。

### 落文字、不改代码

- **合并热点**：`endpoints.go` 参数表末尾、`main.go`、README 的 `data/access` 行与演示文档，进 main 时按并集合并：参数表只在末尾追加，README 行内补句保留。README 里第二条过时的 `data/access` 行（「此刻只有提交」）归票 106 去重，本票不碰。迁移 `0017` 进 main 前复核一次没有撞号（见越权风险点 6）。本轮修订没再动这几个文件。
- **owner 复核三件**：维持原登记，不另立票。
  - 查询不留「谁查了哪件」的痕：越权风险点 3。
  - 阶段治理暂停期间查阅照答：越权风险点 5。
  - 生产形态下客户系统拿不到正式包裹号：越权风险点 4，票 101 由 VE、PS owner 裁定位方式。

### 复跑

- `go build ./...`、`go vet`（`./internal/accessidentity/...`、`./internal/visibilityexception/...`、`./cmd/parcel-api/`）、`gofmt -l internal cmd` 都干净。
- 带 DSN、`-p 1 -count=1` 跑 `./internal/accessidentity/...`、`./internal/visibilityexception/...`、`./cmd/parcel-api/`、`./internal/architecture/`，全部 ok。`cmd/parcel-api` 的真库用例单跑 `-v` 确认实跑、没有跳过。
- 种子与迁移本轮没改，没重灌。全量与机制清点照约定留到进 main。
