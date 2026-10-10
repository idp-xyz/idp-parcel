# 13 各命令口的载荷规范化形状：ADR-0055 决定五第一项逐口解

Category: enhancement
Status: 阻断已修，待复评与重放——2026-10-10 15:31 通道 3（分支 `mcp3-oc13`，代码 tip `5a33ad14`，基 `1121ba61`，工作树 `/home/tops/workspace/idp-parcel-mcp3-oc13`；评审阻断与通道 2 预评阻断都已修，见下「阻断修复记录」）。此前：阻断已修，待复评与重放——15:21（代码 tip `adf2e0ba`），通道 2 预评钉 `04ff5484` 一条阻断；完工，待评审与重放——2026-10-10 11:43 通道 3（代码 tip `1176721c`），通道 1 非作者评审钉 `44f9c3c9` 一条阻断；in-progress——2026-10-10 11:21 通道 3 认领；ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）；按上下文拆笔
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：各上下文 `domain` 里命令载荷的规范化形状与摘要（NO、TF、CC、SA 各一笔），`adapters/http` 的译装不改答复。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第一条；先例 `PSC-1`（parcel-shipment）、`PCC-1`（party-commercial）。

## 做什么

1. 逐口列出作业事实与外部结果命令口，各自定一版规范化形状、带形状版本前缀，摘要进信封。
2. 同身份同摘要答重放、同身份异摘要答内容冲突；已有形状的口（如提交口）只核对不重做。

## 完成判据

- 每个口有形状版本与往返用例；未定形状的口在清单上标明，真渠道不对它开。

## 完成记录（通道 3，分支 `mcp3-oc13`；待非作者评审与重放）

形状都在各上下文 `domain`。文档是 JSON，版本写在文档里，摘要是 `版本:sha256`。字段集沿用各口原先的内容判据；集合排序，使提交顺序不构成另一份内容。已保存的旧摘要不回写、不迁移：比对按已存摘要的形状选算法——带本上下文形状前缀的按该版比，不带前缀的按各口改动前的算法（从基 `1121ba61` 原样恢复，即「无版本」那一版）重算本次命令再比，认不出的前缀按该口读失败的既有答复作答（ADR-0014；见「阻断修复记录」）。`adapters/http` 未改，答复未改。续办引用（`CONT-`）的拼法与回执短指纹未改。CC 处置执行核对与税费付款核对的续办引用原先取 `key.Digest[:8]`，在 CCC-1 键上截到的是形状前缀；预评阻断后改为截前缀之后的 8 位，无版本旧键上与改动前同一串（见「阻断修复记录」）。`PSC-1`、`PCC-1` 只核对、未重做。

| 笔 | 提交 | 形状 |
|---|---|---|
| 认领 | `5fbf3340` | — |
| NO | `0a1c8a08` | `NOC-1` |
| TF | `426ec2ed` | `TFC-1` |
| CC | `9465ff5b` | `CCC-1` |
| SA | `1176721c` | `SAC-1` |

### 已定形

- **NOC-1**（九个命令口，各有往返用例）：承接 `ACCEPT_COLLABORATION`、执行 `RECORD_EXECUTION`、集运 `OPEN_UNIT` / `ADD_MEMBER` / `REMOVE_MEMBER` / `SEAL_UNIT` / `UNSEAL_UNIT` / `CLOSE_UNIT`、收寄 `RECEIVE_DELIVERED_UNIT`。
- **TFC-1**：交接首登与更正、有效交付首登、场外揽收首登与更正（同一内容判据）、场外揽收执行、委托、订舱、订舱应答、替代旅程、监管承接、建立班期、建立运力池。
- **CCC-1**：外部结果（放行三件可空）、申报提交与原案内更正（同一内容判据）、税费付款核对、处置执行核对的事实集、登记监管凭证（`REGISTER_CREDENTIAL`，阻断修复第 4 条补，见下）。
- **SAC-1**：供应商预计成本、发布对账单、纳入迟到费用、纳入调整（与迟到费用分面）、开立争议、分摊与重分摊、派生与重派生、审核账单行、供应商贷项、索赔金额、应收、认可应收、调整索赔、接收供应商账单、评估垫付、形成追偿、调整追偿、采用资金事实、更正资金事实、映射资金、核销。核销的分配行保留提交顺序，与原先的内容判据一致。

### 未定形状

本票没有为下列命令发明字段，也没有新开 HTTP 路由。端点表里已有的行是前票装配，本票不改译装，所以没有把它们从真渠道上拆下来。

- **TF**：`CancelCommission`、`ReserveCapacity`、`ReleaseCapacity`、`ConsumeCapacity`、`CorrectDeliveryProof`、`RegisterEffectiveTimeRule`、`RegisterExternalCarrierCredential`、`ChangeCredentialApplicability`、`FormActualCarrierJudgment`、`RegisterMasterDocument`、`ReviseMasterDocument`、`RegisterFailedAttemptCharge`、`FormLoadAssignment`、`TriggerDeliveryDispatch`、`JudgeCarrierFirstEffectivePickup`、`RecordMovementFact`、`JudgeEffectiveTime`、`RecordDeliveryAttempt`、`OpenDispatchTask`、`CloseFulfillmentSegment`、`EndFulfillmentParticipation`。
- **CC**：`RegisterCandidatePort`、`RegisterDeclarationPath`、`ReceiveManifest`、`ReviseManifest`、`RegisterCaseRequirementRule`、`JudgeCredentialApplicability`、`EstablishCase`、`RegisterReadiness`、`RevokeReadiness`、`GrantSubmissionAuthority`、`RevokeSubmissionAuthority`、`RegisterInterpretationRule`、`RegisterObligationCatalog`、`RegisterObligationItem`、`RegisterGateCatalog`、`RegisterGateFinding`、`RegisterDutyPaymentGateRule`、`RegisterPayerRequirement`、`FormDutyCollaboration`、`ReceiveExternalFundsFact`、`VerifyReleaseGate`、`RecordCredentialGate`、`EstablishRestriction`、`ReleaseRestriction`、`FormFollowUpTarget`、`ProposeReplacement`、`RecordReplacementEffect`、`CloseCustomsCase`、`RederiveDutyVerifications`（重派生写出的核对版本仍走 `VERIFY_DUTY_PAYMENT` 那一形）。
- **SA**：`TriggerSellEvaluation`、`RecordChargeAdjustment`、`VoidStatement`、`ResolveDispute`、`RequestBuyEvaluation`、`ApportionCosts`、`ReleasePreAcceptanceControl`、`ConfirmCharge`、`RegisterSettlementAccount`、`ApplyPreAcceptanceControl`、`ReverseApplication`、`AdoptDutyPaymentVerification`、`FormSellCustomerCharge`、`JudgeChargeAttribution`。

### 已有指纹、本票不改写

- CC `FindingsDigest` / `GateVersionDigest` / `CredentialGateDigest`：注释写明改词形会让已入库门禁版本换指纹，要改另起一票。它们仍是未带形状版本的旧指纹。
- SA 领域 `freezeDigest`、`exposureDigest`：冻结与敞口请求的内部指纹，不是上列命令口。

### 作者自验（钉 `1176721c`）

`go build ./...` 与 `go vet ./...` 退 0。`go test -count=1 -p 1` 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，跑 NO / TF / CC / SA 全包、`internal/architecture`、反向依赖（`parcel-api`、`parcel-customs-register`、`parcel-dispatch`、`parcel-frontline-import`、`parcel-settlement-register`，以及 parcel-pricing / parcel-shipment / visibility-exception 里指向这四个上下文的适配器）——退 0。不自评。

### 判断项（交评审看）

1. 枚举进摘要用领域 `String()` 词形（NO 的收寄主张没有领域 `String()`，仍用数字）。旧的 `%d` 拼接与新文档不可比，靠版本前缀分开，不回算存量。
2. 未定形状且端点表已有行的口，本票没有关闭。关闭会改已放行口的答复，超出「译装不改答复」。
3. 结算侧应用包装在编码失败时 panic。这些文档只有字符串、整数和它们的切片，编码失败说明形状写坏了；空摘要会把两份不同内容读成重放。
4. 已存摘要带认不出的形状前缀（如更晚的版本）时，各口答该处读失败的既有未决（存储不可用那一格），不答内容冲突，也不答重放：读不懂这条记录与读不出来，同样是此刻判断不了（ADR-0014「版本不同不是冲突」）。不新增答复格，`adapters/http` 的答复集因此不变。

## 阻断修复记录（通道 3，分支 `mcp3-oc13`；待通道 2 复评、通道 1 重放）

阻断（通道 1 非作者评审，钉 `44f9c3c9`）：本票删掉了四个上下文原来的摘要算法，拿新的「版本:sha256」直接与库里已存摘要比。改动前入库的记录存的是不带版本的旧摘要，同一条命令重放会被答成内容冲突；ADR-0014 定摘要只在同一规范化版本内可比，ADR-0150 要演示租户的数据按真实租户对待。

做法（四笔同一形）：各口改动前的算法从基 `1121ba61` 原样恢复到各上下文 `application/unversioned_payload_digest.go`，只改函数名，函数体与基上逐字节相同（逐个对过）；领域加 `CompareStoredDigest`，按已存摘要选形状——带本上下文前缀的与本次摘要比，不带前缀的与本次命令按无版本那一版算出的摘要比，其余前缀答 `UnknownPayloadShape`（判断项 4）。存量不回写、不迁移，新记录照旧写新形状。拿摘要当键的口（CC 税费付款核对、处置执行核对）新形状键查不到时再按无版本键查一次，命中按在册那一行作答，不再落第二行、不另铸信封。

| 笔 | 提交 | 改了哪些口 | 定值用例 |
|---|---|---|---|
| NO | `ccc2a6c3` | 承接、执行、集运受理闸（六口共用）与领域拒绝后的复查、收寄 | 九口 |
| TF | `7711027c` | 监管承接（首查与并发输家读回）、委托、订舱、订舱应答、场外揽收执行、班期、运力池、有效交付首登、场外揽收首登与更正、交接首登、替代旅程；交接更正口只走版本键、不比摘要，未动 | 十二口 |
| CC | `04ff5484` | 外部结果、申报提交、原案内更正（当前版与并发赢家）、税费付款核对（落册前按无版本键点读）与重派生（谱系键改取在册那一行的键）、处置执行核对（按无版本键再查） | 七例（外部结果含放行层） |
| SA | `02c0f8a3` | 分摊、派生、垫付评估、追偿形成与调整、审核应付、供应商贷项、对账单发布、纳入（迟到费与调整共用 `commitInclusion`）、异议、资金事实采用与更正、映射、核销、供应商账单接收、索赔金额、应追偿、认可、索赔调整；重分摊、重派生、供应商预计成本不比已存摘要，未动 | 二十口 |
| 第 4 条 | `adf2e0ba` | 凭证登记口定 CCC-1 形状 | 往返两例，应用两例 |
| 预评阻断（CC） | `1c6eb24f` | 处置执行核对与税费付款核对的续办引用截指纹本身，不截形状前缀；连同非阻断一、三 | 续办两例，新记录形逐口 |
| 预评非阻断（NO） | `ca97e619` | 新记录写 NOC-1 形逐口断住；注释不数别处的口 | 新记录形逐口 |
| 预评非阻断（TF） | `bd81f6c7` | 新记录写 TFC-1 形逐口断住 | 新记录形逐口 |
| 预评非阻断（SA，同形补齐） | `5a33ad14` | 新记录写 SAC-1 形逐口断住；注释不数口 | 新记录形逐口 |

定值用例的形状：每口先跑一次命令落记录，把替身里那条记录的摘要改写成基 `1121ba61` 上旧代码对同一条命令落下的定值（不拿恢复后的函数现算），再同内容重放（必须答已有结果）、同身份异内容（必须答内容冲突；键口是「不是重放」）。同一份用例三方对照：基上绿；`44f9c3c9` 上逐口红——重放被答成冲突，CC 带指纹的键口（税费付款核对、重派生、处置执行核对）在那里真的多落了一行；修后绿。各上下文另有一例认不出的前缀答未决。

**第 4 条，做了**（`adf2e0ba`）。凭证登记口不存摘要（`customs_compliance.regulatory_credential` 无摘要列），原内容判据是 `sameCredential` 逐字段比。`domain.CanonicalizeCredentialRegistrationPayload` 用的就是那一组字段（机构、持有人、程序、期限两端、次数额度；凭证身份是查找键、不进摘要；额度未提供写 null），没有发明字段；登记口改为两侧都从凭证本体现算 CCC-1 摘要再比，所以没有旧摘要要认，不动迁移、`adapters/http` 与 `cmd/parcel-api`。同一组应用用例在 `44f9c3c9`（旧的逐字段判据）与修后各跑一遍，全绿——答复不变。本分支的基上没有 operator-channel/11 的接线；`origin/main`（`2babd238`）上 oc11 只改了 CC 的 `adapters/http`，与本笔不交。

### 作者自验（钉 `5a33ad14`；`adf2e0ba` 上同一组先跑过一遍，同为退 0、零 FAIL、两例 PASS）

`go build ./...`、`go vet ./...` 退 0；四个上下文与 `internal/architecture` 的 `gofmt -l` 无输出。`go test -count=1 -p 1` 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`（`docker ps` 报门禁库 `idp-parcel-postgres-gate` healthy）跑四个上下文全包、`./internal/architecture/...`、`./cmd/...`，以及 `go list` 反查出的生产反向依赖（`cmd` 之外是 parcel-pricing、parcel-shipment、visibility-exception 指向这四个上下文的适配器）——退 0，零 FAIL。真库判别：`-v` 单跑 `cmd/parcel-api` 的 `TestTheWiredCredentialAndDutyRegistrationsRecordAgainstARealDatabase`（凭证首登、重放、换有效期冲突，过真库读回）与 SA `adapters/postgres` 的 `TestFreezeScopesAreInvisibleToEachOther`，都是 `PASS` 不是 `SKIP`。不自评。

### 复评清单对照（通道 2 钉 `44f9c3c9` 独立列的受影响点）

「答冲突」「重复写入」两类逐口与上表对得上。CC 键口的连锁（重派生、SA 采用、放行门禁另立核对版本）随键口一并截住：核对不再另落一行，就不另铸信封、不被再采用一版。「核过、不受影响」各项本修复未动；原票四笔没碰任何适配器、迁移与 `cmd`（`git diff --stat 1121ba61..44f9c3c9` 对这几处为空）。

对不上的，取舍如下：

- **并发读回也改了**：NO `ConsolidateParcelsHandler.rejected`、TF `AcceptRegulatoryDispositionHandler.Handle` 与 CC `CorrectDeclarationHandler.Handle` 的并发输家读回，复评列为「到不了旧记录」，本修复仍改走 `CompareStoredDigest`。赢家若是新旧二进制并存时旧二进制写下的，它带的是无版本摘要，`==` 会把同内容答成冲突（`rejected` 答未受理）；赢家是同轮新写的时候两种比法等价，复评考虑的那条路答复不变。代价：这三处没有单独的旧摘要用例，竞态窗口要在替身上另开钩子才到得了。
- **顺带一格**（续办引用截到形状前缀）：初版只在票面如实改写、没改代码；通道 2 预评把它定为阻断，随后修了，见下一节。

### 预评处理（通道 2 钉 `04ff5484`，评 NO / TF / CC 三笔）

- **阻断，已修**（`1c6eb24f`）：`VerifyDispositionHandler.handOff` 与 `DutyPaymentReconciliationHandler.handOffVerification` 改用 `continuationHash`，截形状前缀之后的 8 位；无版本旧键没有前缀，截出来与改动前同一串。用例：处置执行核对在 CCC-1 新键上末段是 8 位十六进制、在无版本旧键上等于定值 `CONT-VERIFICATION/decision-1/9de48e94`（取基 `1121ba61` 上那一版指纹的前 8 位）；税费付款核对交接失败的续办引用末段是 8 位十六进制。两例在 `adf2e0ba` 上红（实得 `…/CCC-1:a0`、`…/CCC-1:34`），修后绿。
- **非阻断一，做了**（`1c6eb24f`、`ca97e619`、`bd81f6c7`，SA 同形补在 `5a33ad14`）：各上下文加 `TestNewRecordsAreStoredUnder{NOC1,TFC1,CCC1,SAC1}`，逐口断首次提交落下的摘要带本上下文前缀（CC 键口断键上那枚指纹）。另立一例而不塞进定值循环，是为了定值循环仍能原样放到基上跑绿、三方对照照旧成立；新例在基 `1121ba61` 上四个上下文都红，断得住。
- **非阻断二，未做**：「认不出的形状」仍各上下文只测一口。其余各口的 `UnknownPayloadShape` 分支都答该处读失败的既有未决，预评逐处读过；逐口补用例要给每例添一格未决答复，本轮不做，记在这里。
- **非阻断三，做了**：注释里数别处的东西改成点名或删数——NO 用例的「九个定形口」「集运六口」、CC `unversioned_payload_digest.go` 文件头与 CC 用例的「键上带指纹的两口」、CC 结果字段注释的「两格」、CC 端口注释的「只在……一处」，SA 文件头与用例的「三口」「前两口」「后一口」同形补齐。
