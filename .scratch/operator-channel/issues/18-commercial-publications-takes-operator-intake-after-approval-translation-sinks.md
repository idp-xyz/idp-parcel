# 18 批文译装下沉后 `/commercial-publications` 换操作者 Intake

Category: enhancement
Status: resolved——2026-10-13 22:34 通道 3：重放进 main，远端 main = `c38c9307`，进 main 记录见文末（用户裁定本票免非作者评审、作者直接重放）。此前完工，待评审与重放——2026-10-13 22:2x 通道 3，代码 tip `07bfa7ee`（分支 `mcp3-oc18`、基 `b3c8bd46`），清点 `c55b2c12`，完成记录在文末。此前 in-progress——2026-10-13 22:06 通道 3 认领（派单本卡 ← 通道 1，`task-42392427`），分支 `mcp3-oc18`，基 `b3c8bd46`（开工前 `ls-remote` 核远端 main 仍是它）。此前 ready-for-agent——2026-10-12 20:2x 通道 1 立（派单方裁定），出自 [06](./06-commercial-publication-and-replay-take-operator-identity.md) 完成记录判断项 1：06 把发布路径四口、词表读口与计价回放口换上了操作者渠道，`/commercial-publications` 留在字面量 `UnconfiguredIntake{}` 上，作者报「另立下沉票，或扩本票地盘」待裁，派单方裁另立，即本票。
Blocked by: 06（本票接在 06 的 `OperatorRegistryIntake` 与认证适配器交回的 `OperatorIdentity` 上；06 进 main 之前不开工）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-commercial` 里把受控批文译成 party-commercial 命令的那一段（下沉出去、受控 CLI 改调下沉后的那一份）；下沉的落点在 party-commercial 的适配器包里，照 [04](./04-registration-write-faces-take-operator-intake.md) 第三、五批「译装先下沉成 `registrationjson`、在线口不另写平行解码」的先例定；`cmd/parcel-api` 端点表里 `/commercial-publications` 那一行与它的装配测试。
出处：06 判断项 1；[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定五；[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四。

## 做什么

1. 批文译装下沉：受控 CLI 与在线口共用同一份译装，不另写平行解码；受控 CLI 对同一份批文的答复不变。
2. `/commercial-publications` 换操作者 Intake（挂 `operatorRegistries.commercial`）。这一口收的是受控批文的在线镜像（声明摘要 + 批准三格 + 角色确认），批准照旧从批文取、不从信封来——操作者的批准路径是 ADR-0126 决定三的待批准载体，06 已换（06 判断项 1 原话，该判断项标着待 owner 复核，复核改判就照改判做）。
3. 三格测试同 06 的回放口：未带令牌 401、授予不足 403、操作者册读不动 503；授予齐备而载荷自报租户 400；发行方未配置仍答 403 未配置。

## 不做

- 发布路径四口与词表读口（06 已换）。
- 审批职责规则的取值与登记入口；角色模型里有哪些授予格（[psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）。

## 完成判据

- 下沉前后受控 CLI 对同一份批文的答复相同（带测试）。
- `/commercial-publications` 三格各答其格，授予齐备时答业务结果（带测试，`cmd/parcel-api` 装配测试带 DSN）。

## 完成记录（2026-10-13 通道 3）

分支 `mcp3-oc18`，基 `b3c8bd46`，代码 tip `07bfa7ee`，清点 `c55b2c12`。证据层级一律 `S`（隔离合成：演示种子 `SYN-TENANT-01` 与 `SYN-OC18-*` 夹具）。各笔都单独编得过、各自推过分支，未 rebase。

各笔：

- `76c4a957` 票面认领。
- `65df0720` 判据一先于下沉：在基上的译装上钉受控 CLI `publish` 的整份答复（`cmd/parcel-commercial/publish_answer_test.go`）——演示种子在空库上首发与重放两遍的逐项回显与退出码（带 DSN），以及在译装里被拒的各份批文与译得过、停在连库之前的那一份（不连库）。期望串取自基上的译装，所以这一笔在基上就绿；它是下沉的特征化测试，判别力见验证里的变异。
- `43b98da4` 译装下沉：`cmd/parcel-commercial/translate.go` 里发布批文的外壳与逐项翻译（各族声明、封闭集名称镜像、时长子集）整份迁入 `internal/partycommercial/adapters/registrationjson/publication.go`；CLI 只留整批解码与「第 N 项」定位（`publishCommandsFromJSON`）和解析键登记。`PublicationItemDocument.TenantID` 改用 `json.RawMessage`；两入口 `PublicationCommand`（受控，取项上的 `tenantId`）与 `PublicationCommandForTenant`（在线，取认证结果）共用一个本体。`CommercialKindFrom` 导出供解析键登记用；时长子集那条测试随 `parseISODurationSubset` 迁入；生产接线基线里 `DelegatedByLegalEntity` 那句注释改指新落点（基线条目未动）。
- `04ab97c6` `commercialhttp`：`OperatorRegistryIntake.IntakeCommercialPublication`，先认证、后解载荷，恰一项、项上 `tenantId` 在场即拒；`CommercialPublicationIntake` 接口注释改指真译法。
- `07bfa7ee` `cmd/parcel-api`：`/commercial-publications` 那一行挂 `operatorRegistries.commercial`、注释随改；`swappedRegistryFaces` 加这一口；06 钉「仍答未配置」的 `TestCommercialPublicationsStaysUnconfiguredOnTheOperatorChannel` 换成真库用例。
- `c55b2c12` 机制清点重生成（干净检出钉 `07bfa7ee`）。
- 本笔：完成记录与 Status。

顺手收了 06 评审记下、随下一张碰这几份文件的票办的两条：Standards N1（`assemble_publication_operator_test.go` 文件头改用两条测试的符号名）与 N3（`OperatorRegistryIntake` 注释去掉「载体三口」的计数）。N2 在 parcel-pricing 的 `operator_read_intake.go`，本票没碰那份文件，未收。

### 完成判据逐条

- ✅ 下沉前后受控 CLI 对同一份批文的答复相同（带测试）。`TestTheControlledPublishAnswersTheDemoSeedAlike`（带 DSN：演示种子首发每项 `PUBLISHED_EFFECTIVE`、重放每项 `REPLAYED`，各声明落点逐格，退出 0）与 `TestTheControlledPublishRefusesTheSameApprovalDocumentsAlike`（不连库：集合外取值、领域构造门、未知键、缺项、未发布的引用、坏 JSON 各格的诊断与退出 1）在 `65df0720`（下沉前）与下沉之后各笔上都绿，`publish_answer_test.go` 自 `65df0720` 起没改过。既有 `translate_test.go`、`translate_citation_gate_test.go` 照旧经 `publishCommandsFromJSON` 跑。例外一格见判断项 3。
- ✅ `/commercial-publications` 三格各答其格，授予齐备时答业务结果（带测试，`cmd/parcel-api` 装配测试带 DSN）。
  - 三格与外壳自报租户：这一口进了 `swappedRegistryFaces`——`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`（未带令牌 401 `OPERATOR_CREDENTIAL_REJECTED`，外壳带 `tenantId` 400 `MALFORMED_REQUEST`）、`TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`（只持查阅授予 403 `OPERATOR_NOT_GRANTED`，操作者册读不动 503 `IDENTITY_DEPENDENCY_UNAVAILABLE`）。
  - 发行方未配置仍答 403 未配置：`TestEveryAssembledEndpointAnswersUnconfigured` 照旧绿。
  - 业务结果（真库）：`TestTheOperatorChannelPublishesTheControlledDocumentMirrorAgainstARealDatabase`——真铸造、真 PC 认证适配器、操作者 Intake 与真库发布编排连起来；授予齐备时受控批文的一项（不带 `tenantId`）发在信封绑的 `SYN-TENANT-01` 下答 201 `PUBLISHED_EFFECTIVE`，再发答 200 `REPLAYED`；项上自报 `SYN-TENANT-02` 答 400 `MALFORMED_REQUEST`，库里那个租户下零行。
  - Intake 层：`commercialhttp` `TestPublicationMirrorTranslatesAsTheControlledBatchWithTheAuthenticatedTenant`（两口对同一项译出同一条命令，`reflect.DeepEqual`：租户是认证出的那一个，批准三格与角色确认是批文里的），`TestPublicationMirrorRefusesSelfReportedTenantsAndAnythingButOneItem`（项上 `tenantId`、项上 `null`、外壳 `tenantId`、零项、两项、集合外类别、多余键都答请求畸形），`TestPublicationMirrorAuthenticatesBeforeReadingTheBody`（令牌不过、未授予、身份依赖读不动、渠道未配置下请求体一个字节都不读）。
- ✅ 做什么第 2 条「批准照旧从批文取、不从信封来」：上面的 `reflect.DeepEqual` 一条钉住——受控入口的批准格只能来自批文，在线口译出的命令与它逐字段相同。

### 判断项（越权风险点 · 待 owner 复核）

1. **06 判断项 1 未见复核改判，照原判做。** 取证于 `b3c8bd46`：06 票面那组判断项的标题仍是「越权风险点 · 待 owner 复核」，06 票面、psb/15 与 `docs/adr/` 里都没有对它的复核记录。本票照原判：认证出的操作者只决定租户，不进批准格。owner 若改判「批准者也从信封来」，要改的是 PC 发布用例的批准语义，不在本票地盘。本票没有改 PC 的 application 与 domain。
2. **下沉落点与共用层。** 受控 CLI 与在线口共用 `registrationjson` 的外壳（`PublicationBatchDocument`、`PublicationItemDocument`）与逐项本体，两入口只差租户来源；本体里租户仍在类别之后、对象标识之前取，一项同时错几格时报回的是同一格。整批解码与「第 N 项」定位留在 CLI；「恰一项」与「项上自报租户即拒」两道门在 `commercialhttp`——照 04 第五批的分工。解析键登记不下沉：它译成 parcel-shipment 的登记面，放进 party-commercial 的适配器包会让 PC 依赖 PS；它要的类别名镜像经导出的 `CommercialKindFrom` 共用，两种批文对同一个类别名只有一种读法。
3. **受控 CLI 答复里有一格措辞变了，只在批文用错 JSON 类型时可见。** 解码器的诊断带 Go 类型名：项上直属各格用错类型时，诊断里的 `publicationItemDocument` 变成 `PublicationItemDocument`；`"tenantId": 5` 从整批的形状错（「发布批不是本入口的形状：…」）变成该项的错（「第 1 项：tenantId 不是字符串：…」）。退出码 1、技术失败、一个字节没写都不变；嵌套各格的类型没导出，措辞不变。这一类没进答复表，`publish_answer_test.go` 文件头写了理由。要逐字不变就得让外壳类型不导出，在线口便没有「那一份」形状可用——与 04 第五批导出 `ProductBatchDocument` 时同一取舍。
4. **线格式取批文外壳 `{"items": [一项]}`，不取裸的一项。** 照本包在线口的惯例：镜像受控 CLI 的批文、恰一项；管理台今天送的也是批文形状。发布批文的租户在每一项上、外壳上没有那一格，所以「键在场即拒」拒的是项上的 `tenantId`；外壳上带它是未知键，同样答请求畸形。

### 留给前端（`apps/admin-web/**` 一人在 main 上做）

- `apps/admin-web/src/pages/party/api.ts` 在线登记口那段注释说「商业的译装在 cmd/parcel-commercial 的 package main 里，两口只锁得到同一个登记用例」，已过期：发布批文的译装此刻也在 `registrationjson`。操作者渠道配好之后，发布快照要去掉项上的 `tenantId`，否则答 400。与 04 第五批记下的 `api.ts` 注释清扫同一张前端票办。

### 验证（作者那层，钉 `07bfa7ee`，隔离工作树 `idp-parcel-mcp3-oc18`）

- 改动与新增的 `.go` 经 toolchain `gofmt -l` 零行，无 CR、无 BOM。
- `go build ./...` 退 0；`go vet ./...` 退 0。
- `go test -count=1 -p 1`，`IDP_PARCEL_POSTGRES_DSN` 已设：`./cmd/parcel-commercial`、`./cmd/parcel-api`、`./internal/partycommercial/adapters/registrationjson`、`./internal/partycommercial/adapters/http`，`go list` 按生产导入与测试导入反查这两个包多出的 `internal/partycommercial/adapters/accessidentity`，外加 `./internal/architecture/...`，全部 ok，退 0。`cmd/parcel-api` 与 `cmd/parcel-commercial` 整包 `-v` 零 `--- SKIP`。
- `-v` 单跑：`TestTheOperatorChannelPublishesTheControlledDocumentMirrorAgainstARealDatabase`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`、`TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`、`TestEveryAssembledEndpointAnswersUnconfigured`、`TestTheControlledPublishAnswersTheDemoSeedAlike` 均 PASS，不是 SKIP。
- 判别力（`go test -overlay`，变异副本放 `/tmp/oc18-ov`，工作树不动）：
  - 受控入口的租户写死成 `SYN-TENANT-01`（变体能编译）→ `TestTheControlledPublishRefusesTheSameApprovalDocumentsAlike` 红；
  - 规则分类一律译成 `SHIPMENT_INVARIANT`（变体能编译）→ `TestTheControlledPublishAnswersTheDemoSeedAlike` 红；
  - 在线口退回受控入口、租户取项上 → `TestPublicationMirrorTranslatesAsTheControlledBatchWithTheAuthenticatedTenant` 与 `TestTheOperatorChannelPublishesTheControlledDocumentMirrorAgainstARealDatabase` 红；
  - 不再拒项上的 `tenantId` → `TestPublicationMirrorRefusesSelfReportedTenantsAndAnythingButOneItem` 与上面那条真库用例红；
  - 端点表这一行退回字面量 `UnconfiguredIntake{}` → `TestSwappedRegistryFacesAnswerFromTheOperatorChannel`、`TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades` 红。
- 机制清点在 `07bfa7ee` 的干净检出上重生成（`c55b2c12`）。
- 作者不自评：非作者评审由通道 1 另派，重放进 main 由通道 1 做。（完工报之后用户改裁，见下节。）

## 进 main 记录（2026-10-13 通道 3）

用户 22:2x 裁定（回复本通道的原生提问）：全部由本通道独立完成，本票无需评审，直接重放。据此本票没有非作者评审记录，[parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「合入前独立评审」那道门按用户裁定免去，重放由作者通道自做。

通道 3 在隔离树 `idp-parcel-replay-oc18-mcp3` 把七笔原样重放到 main `8a096657` 之上——`76c4a957`→`a349c053`、`65df0720`→`6999eac1`、`43b98da4`→`9fec5ebb`、`04ab97c6`→`6b7d5b93`、`07bfa7ee`→`c52537cd`、`c55b2c12`→`9c9f6bca`、`90ebce1a`→`c38c9307`；分支上的 SHA 作封存出处，本地分支指针改名 `merged/mcp3-oc18`。main 自 `b3c8bd46` 起只多 `8a096657` 一笔纯 `.md`（psb/29 立票），与本票各文件不重叠；重放出的各文件对分支 tip `90ebce1a` 逐字相同（`git diff 90ebce1a c38c9307` 只剩那份 `.md`）。

验证在 `c38c9307`：改动的 `.go` 经 `gofmt -l` 无输出；清点在该 tip 重生成零差；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` 139 ok / 0 FAIL / 14 无测试 / 0 cached（22:31:17→22:33:51）；`-v` 探针 `TestTheOperatorChannelPublishesTheControlledDocumentMirrorAgainstARealDatabase`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`、`TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`、`TestEveryAssembledEndpointAnswersUnconfigured`、`TestTheOperatorChannelRecordsEnvelopeSubjectsOnThePublicationCarrier`、`TestTheControlledPublishAnswersTheDemoSeedAlike`、`TestTheControlledPublishRefusesTheSameApprovalDocumentsAlike` 都 PASS 非 SKIP。22:34 共享树 ff 到 `c38c9307`（`.cursor/` 两份改动与未跟踪的 `Untitled` 原样未动）后 `push c38c9307:main`，远端 main = `c38c9307`。
