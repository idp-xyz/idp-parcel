# 04 登记册配置写面逐口换操作者 Intake

Category: enhancement
Status: in-progress——2026-10-10 通道 5 收口核查（钉 `a8cf12ff`）之后通道 1 裁定（用户授权自决）：价卡登记口归本票，身份族 6 口另等 07；剩余三件见文末「剩余工作」。此前 in-progress——2026-09-25 通道 4 认领（用户令独立完成操作者渠道这条链），逐上下文分批进 main：第一批可见性八口、第二批关务七口、第三批网络七口、第四批计价四口、第五批商业参与方六口、第六批 TF 五口已换（见文末）；不在隔离名单上的口只剩价卡登记（属 price-card-import）。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 03（已 resolved）；[07](./07-admin-web-login-gate.md)——只挡身份族 6 口：换口要同笔撤隔离放行（ADR-0150 决定三），前提与 [15](./15-operation-decision-faces-take-operator-intake.md) 余下四口是同一件，演示环境接上发行方与合成操作者授予（2026-10-10 通道 1 补）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`cmd/parcel-api` 端点表里 ADR-0085 决定一那一族登记端点的装配行及其装配测试。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四与 Consequences；ADR-0091 Consequences 的逐口纪律。

## 做什么

1. 开工第一步：逐口归类端点表里挂 `UnconfiguredIntake{}` 的写行——属 ADR-0085 决定一那一族登记写面的归本票；作业事实登记、外部结果接收、客户委托命令归 [08](./08-isolated-release-of-main-chain-command-faces.md) / [09](./09-decision-production-channel-for-business-command-faces.md)（ADR-0100 决定四、五明文不覆盖）。归类表写进完成记录。
2. 本票那一族逐口换成操作者 Intake：每换一口，该口的「未配置即拒」测试改写为三格测试；未换的口答复不变。隔离读放行表与 `SYN-` 写开关零改动。

## 完成判据

- 归类表完整；已换各口对合成操作者答业务结果、对三格各答其格（带测试）；装配测试与端点表仍一一对照。
- 随 [ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三（2026-09-24 补）：已换各口若在隔离放行名单上（如 `/commercial-*` 身份族），同一笔撤下该口的隔离放行——隔离 Intake 上的方法与放行名单那一行，隔离放行用例改写为真渠道答复格用例。

## Comments

### 归类表 ← 通道 4 · 2026-09-25（取证钉 `8d515c4c` 的端点表）

**本票，且不在隔离放行名单上**（在未配置环境里换口没有可观察变化，可以先换）：
- 可见性 8 口：`/visibility-catalogue-{milestone-mapping,triage-rule,notification-policy,claim-eligibility,claim-authorization,disclosure-policy,exception-disclosure-rule,conflict-signal-rule}-registrations`——**第一批已换**。
- 关务 7 口：`/customs-{interpretation-rule,gate-catalog,candidate-port,declaration-path,case-requirement,duty-collaboration,duty-payment-verification}-registrations`——**第二批已换**。
- 网络 7 口：`/network-catalog-{node,connection,line,service-area,service-calendar,availability-adjustment,route-strategy}-registrations`——**第三批已换**。
- 计价 5 口：`/pricing-{price-card,reference-series,reference-catalogue}-registrations` 与 `/pricing-reference-series-{reviews,previews}`——**第四批已换其中四口**；价卡登记口的在线导入属 price-card-import 那一批（ADR-0101，通道 3 认领），不在本票换。
  **〔2026-10-10 通道 1 裁定，用户授权自决〕价卡登记口 `/pricing-price-card-registrations` 归本票，前一句按此更正（原句留痕）。** 它是 ADR-0085 决定一那一族
  登记写面，落在「做什么」第 1 条里；[price-card-import spec](../../price-card-import/spec.md)「本批自决的几格」第 4 格与此一致；price-card-import/04 的发布在
  用例层交 `RegisterPriceCard`、不经这一口，而 ADR-0101 决定一让 JSON 快照签留作受控批量口的在线镜像，所以这一口照第四批计价四口那样换操作者 Intake。
- 商业参与方 6 口：`/commercial-{service-product-form,product-channel-mapping,registration-number-type,channel-account-use}-registrations`、`/commercial-registration-number-type-deactivations`、`/commercial-channel-account-use-revocations`——**第五批已换**。
- TF 5 口：`/transport-fulfillment-external-carrier-credential-{registrations,applicability-changes}`、`/transport-fulfillment-effective-time-rule-registrations`、`/transport-fulfillment-carrier-master-document-{registrations,revisions}`——**第六批已换**。

**本票，但在隔离放行名单上**（换口要同笔撤放行，ADR-0150 决定三；等演示环境走通操作者渠道）：商业参与方身份一族 6 口（`/commercial-{business-party,legal-entity,customer-account,party-relationship,legal-entity-profile}-registrations`、`/commercial-party-identity-deactivations`），以及 `/customs-regulatory-credential-registrations`。

**不归本票**：商业发布五口与 `/pricing-evaluation-replays` 归 06；运营决定口归 15；外部资金事实两口归 11（集成客户端族，ADR-0151 决定四）；作业事实、外部结果、客户委托命令归 08、09、10、11（ADR-0100 决定四、五明文不覆盖）；`/pricing-estimates` 归 operator-workspace-gaps（ADR-0152）。

### 第一批进展 ← 通道 4 · 2026-09-25

- **译装只用 registrationjson 那一份**：可见性 `registrationjson` 的八个译装拆成受控批量口入口（签名不变，租户取批文）与在线入口 `…ForTenant`（租户取信封，批文带 `tenantId` 键即拒，`null` 也拒），两者共用同一个本体，CLI 行为不变。
- `visibilityhttp.OperatorRegistryIntake` 实现八口 Intake：先认证、再读批文（1 MiB 上限）交在线入口；登记写面的答复映射加三格（401 / 403 未授予 / 503）。防腐适配器在 `internal/visibilityexception/adapters/accessidentity`，以登记册配置写能力面铸信封、不带准入要求（ADR-0100 决定四）。
- 读 Bearer 令牌提成 `internal/platform/httpapi.BearerToken`，PS、TF 两处改用，不再各抄一份。
- `cmd/parcel-api`：铸造器改为 `buildOperatorMinter` 建一只、运营决定与登记写面共用；端点表八行换成操作者 Intake。发行方参数没设时八口照旧答 403 未配置。
- **判断项**：批文里的 `approvedBy` 在线口仍照受控批量口从批文取——它记的是批准人引用，不是提交者；认证出的提交操作者在快照里没有格（与 CLI 同）。要不要把提交操作者落册，另裁。
- **下几批要先做的一步**：关务的译装已在 `registrationjson`，照本批办；网络、计价、商业参与方、TF 的译装还在各自的登记 CLI 里，要先下沉成 `registrationjson` 包（照可见性当初下沉的先例），在线口才有「那一份」可用——不另写平行的解码。

### 第二批进展 ← 通道 4 · 2026-09-25

关务七口照第一批办：`registrationjson` 七个译装拆成受控批量口入口与在线入口 `…ForTenant`（门禁目录经 `gateKeyFrom` 取租户，改为先从租户来源取再交它，其余六个是通用改法），CLI 行为不变；`customshttp.OperatorRegistryIntake` 实现七口 Intake；`writeRegistrationIntakeProblem` 加三格；防腐适配器在 `internal/customscompliance/adapters/accessidentity`；端点表七行换成 `operatorRegistries.customs`。新增在线入口的测试（关务 `registrationjson` 此前没有包内测试，它的译装由受控 CLI 的测试钉）。

### 第三批进展 ← 通道 4 · 2026-09-25

网络七族：译装先从 `cmd/parcel-network-register` 的 `commandFor` 下沉为 `internal/networkrouting/adapters/registrationjson`（七个受控批量口入口加七个在线入口，共用本体；网络批文用 snake_case，在线口拒的是 `tenant_id` 键），CLI 改为调它、只包事务内调用，行为不变。其余照前两批：`networkhttp.OperatorRegistryIntake`、`writeRegistrationIntakeProblem` 加三格、防腐适配器 `internal/networkrouting/adapters/accessidentity`、端点表七行换成 `operatorRegistries.network`。装配测试另加一条，对已换的全部登记口证「只有查阅授予 → 403 未授予」「操作者册读不动 → 503」两格。

### 第四批进展 ← 通道 4 · 2026-09-25

计价四口：参考序列登记、预览、复核与参考目录登记。计价的在线线格式早已定好（ADR-0101 决定一的运营操作者面载荷：`ReferenceSeriesRegistrationPayload`、`ReferenceCataloguePayload`，注释写明「租户与登记责任方从 OperatorEnvelope 来，由 Intake 作为入参交进来」），本批只补那个 Intake：`pricinghttp.OperatorRegistryIntake`，登记责任方取认证出的提交操作者（发行方 + sub）。序列复核此前没有在线载荷，照受控批量口的复核文档去掉租户与复核人两格定了 `ReferenceSeriesReviewPayload`，复核人取认证结果——四眼门比的就是两个认证过的身份。受控批量口走领域快照重建（`Rehydrate…Registration`），与在线表单载荷本就是两种输入，不合并。`writeRegistrationIntakeProblem` 加三格；防腐适配器 `internal/parcelpricing/adapters/accessidentity`；端点表四行换成 `operatorRegistries.pricing`。

### 第五批进展 ← 通道 4 · 2026-09-25

商业参与方六口：服务形态、产品—渠道映射、注册号类型登记与停用、渠道账号使用授权登记与撤销。

- **线格式照本包在线口的既有惯例**（`IsolatedPartyIdentityIntake` 注释）：镜像受控 CLI 的批文、去掉整批的 `tenantId`——键在场即拒（`null` 也算），本口恰一项、别的口的项拒。管理台今天送的也是批文形状（`apps/admin-web/src/pages/party/api.ts`），只差去掉 `tenantId`。
- **译装先下沉**：`register-products` 与 `register-registration-number-types` 的批文外壳与逐项翻译（连同注册号类型的参考配置采用路径）下沉为 `internal/partycommercial/adapters/registrationjson`，CLI 只留逐项回显与事务；「每份已发布参考配置过领域构造门与样例」那条测试随翻译迁入该包。外壳的 `tenantId` 用 `json.RawMessage`：CLI 经 `DocumentTenant` 取整批租户，在线口见键即拒——形状只定义一处。
- **不重复**：在线口复用本包隔离形态的 `decodeClosedDocument` 与 `refuseSelfReportedTenant`；「恰一项」的判据从身份族外壳的方法里提成包内函数 `exactlyOneItem`，两边共用同一句拒绝。
- **使用授权族没有受控 CLI**，载荷只有在线一份（`channel_account_use_payload.go`）：两格存续按封闭集译、缺席译成未答交发布门判；撤销载荷装不下授权正文（ADR-0093 决定六）。
- 防腐适配器 `internal/partycommercial/adapters/accessidentity`；`writeRegistrationIntakeProblem` 加三格；三处接口注释的「真 Intake 未就位」改为指向真实现。
- **留给前端**（`apps/admin-web/**` 一人在 main 上做）：`api.ts` 在线登记口那段注释（「请求体形状此刻没有契约」「商业的译装在 package main 里」）已过期；操作者渠道配好之后请求体要去掉 `tenantId`。与 16 记下的管理台 `PAR-INT-01` 注释清扫同一张前端票办。

### 第六批进展 ← 通道 4 · 2026-09-25

TF 五口：外部承运凭证首登与改变适用关系、有效时间规则登记、总单首登与形成新版本。三本册子都是逐字段表单（ADR-0101 决定八），TF 没有登记 CLI，载荷只此一份，不涉译装下沉。

- 有效时间规则口复用既有的 `EffectiveTimeRuleRegistrationPayload`；凭证与总单四口照它的写法补载荷（字段与命令一一对应、只少租户一格，键名与本族应答同名）。词不过领域构造门：`change`、`revision` 认不出的词译成零值交编排答`未受理`（200），与本包「词不在集合内是形成了的业务答案」同一条判据；只有时刻解不出才是坏报文。
- 领域补 `ParseMasterDocumentRevision`：原注释写明「那一格随 Intake 一起来」，现在来了，注释随之改。
- `tfhttp.OperatorRegistryIntake` 五口共用一段「认证 → `decodeClosedPayload` → 载荷译命令」；认证在既有的 `transportfulfillment/adapters/accessidentity` 包里加 `OperatorRegistryAuthenticator`（登记册配置写面、不判准入），答复翻译复用运营决定口的 `answer`。三处接口注释的「就位前本包不带任何实现」改指真实现。
- **剩余**：隔离名单上的口（商业参与方身份一族 6 口、关务监管凭证）等演示环境走通操作者渠道再换，换口与撤隔离放行同笔（ADR-0150 决定三）；价卡登记口随 price-card-import。

### 收口核查（通道 5，钉 `a8cf12ff`）· 2026-10-10 · 通道 1 派单 `task-050c4117`，非作者独立取证

**结论：不收口，Status 不动。** 六批换下的口与三格测试、装配测试、ADR-0150 决定三都对得上；缺的是身份族 6 口未换、价卡登记口没有票认领，另有网络七口少一层
http 测试。逐条如下，交通道 1 定。取证全部实测于 `a8cf12ff`（该提交 CI run `38050118343` success）。

**判据逐条**

- ◑ **归类表完整**。端点表此刻挂 `UnconfiguredIntake{}` 的写行逐行对得上表里各类，只漏 `/claims`：它在归类钉的 `8d515c4c` 就已挂
  `visibilityhttp.UnconfiguredIntake{}`。按 `ClaimIntake` 的接口注释，它是 UC-VE-007 的客户索赔提交面，账户只能来自客户认证结果；ADR-0100 决定五不给客户
  业务命令面开操作者渠道，所以**不归本票**，本核查补记于此，不另立票。`8d515c4c` 之后端点表只多出 `/pricing-price-card-previews`、`/pricing-price-card-drafts`、
  `/pricing-price-card-draft-views`（price-card-import/02、03），都直接挂 `operatorRegistries.pricing`，与表里「价卡的在线导入属 price-card-import」一致；
  没有端点被撤。表里列为「本票，但在隔离放行名单上」的 `/customs-regulatory-credential-registrations` 此刻挂 `integrationClients.customs`（集成客户端族，
  ADR-0149、ADR-0151 决定四；[11](./11-integration-client-register-and-client-credentials.md) 已 resolved），隔离放行已撤，已不在本票。
- ◑ **已换各口对合成操作者答业务结果、对三格各答其格（带测试）**。六批各口此刻都挂 `operatorRegistries.*`，且全部列在
  `cmd/parcel-api/assemble_operator_decisions_test.go` 的 `swappedRegistryFaces` 里，由两条装配测试逐口钉：`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`
  （不带令牌答 401 `OPERATOR_CREDENTIAL_REJECTED`；授予齐备时走到译装，批文自报租户答 400 `MALFORMED_REQUEST`）与
  `TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`（只有查阅授予答 403 `OPERATOR_NOT_GRANTED`；操作者册读不动答 503
  `IDENTITY_DEPENDENCY_UNAVAILABLE`）。`swappedRegistryFaces` 与端点表上挂 `operatorRegistries.*` 的行只差 `/pricing-price-card-previews` 与
  `/pricing-price-card-draft-views`，两口属 price-card-import。「认证租户下译出命令」那一层，可见性、关务、计价、商业、TF 五族各有 http 包的
  `operator_registry_intake_test.go`；**网络七族没有**：`networkhttp.OperatorRegistryIntake`（认证 → 读批文、一兆上限 → `registrationjson` 在线入口）在本包
  无测试，只由上面两条装配测试与 `registrationjson` 的 `TestNetworkTranslationTakesTheTenantFromItsSource`、
  `TestOnlineTranslationRefusesASelfReportedTenantOnEveryNetworkFamily` 间接盖住，比另五族少一层。
- ✅ **装配测试与端点表一一对照**。`cmd/parcel-api/endpoints_test.go` 的 `TestEveryAssembledEndpointAnswersUnconfigured` 拿 `businessEndpointProbes` 与装配点
  双向对照（多装、重装、漏装各自红）；此刻端点表与探针表逐项对得上，不缺不多。
- ✅ **ADR-0150 决定三**。换下的各口没有一口在隔离放行名单上：端点表里它们直接挂 `operatorRegistries.*`，不经任何隔离变量；`assemble_isolated_write.go` 与
  `assemble_isolated_read.go` 也不点它们的名。

**缺口**

1. **商业参与方身份族 6 口未换**：`/commercial-{business-party,legal-entity,customer-account,party-relationship,legal-entity-profile}-registrations` 与
   `/commercial-party-identity-deactivations` 此刻挂 `isolatedPartyIdentity` 那组变量（ADR-0091 隔离写放行，未启用时是 `UnconfiguredIntake{}`），不是操作者
   Intake，「做什么」第 2 条对这一族没做完；`.scratch` 里没有别的票认领它们。归属仍是本票。前提与 [15](./15-operation-decision-faces-take-operator-intake.md)
   余下四口是同一件——演示环境接上发行方与合成操作者授予：[02](./02-oidc-credential-verifier-and-deployment-parameters.md) 的选型记录写明 Dex 接进 compose
   「随 07 做」，[07](./07-admin-web-login-gate.md) 票面此刻 ready-for-agent。15 已把这一格写进自己的 Blocked by，本票的 Blocked by 还只写着 03。
2. **价卡登记口 `/pricing-price-card-registrations` 没有票认领**：此刻挂字面量 `pricinghttp.UnconfiguredIntake{}`。本票归类表说它属 price-card-import 那一批；
   [price-card-import spec](../../price-card-import/spec.md)「本批自决的几格」第 4 格却说换真 Intake 归本票那一族，它 2026-09-25 的更正只管那一批自己的口；
   [price-card-import/04](../../price-card-import/issues/04-approval-and-publication.md) 的发布在用例层交 `RegisterPriceCard`，不经这一口；而 ADR-0101 决定一让
   JSON 快照签留作受控批量口的在线镜像，这一口还在用。两头互指，没有票接。price-card-import/04 正由通道 4 在分支 `mcp4-pci04` 上做，本核查未碰。
3. **网络七口 http 层 intake 少一层测试**（见判据第二条），归本票第三批。

**未改**：Status；父票 [psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 的子票表（该表没有状态栏）。

### 剩余工作（2026-10-10 通道 1 裁定后）

本票照旧 in-progress，余下三件，做完即可收口：

1. **商业参与方身份族 6 口换操作者 Intake**，同笔撤下这几口的隔离放行（ADR-0150 决定三）——等 [07](./07-admin-web-login-gate.md)：演示环境接上发行方与合成操作者授予
   之前换口，演示动线与管理台这几页会当场断。
2. **价卡登记口 `/pricing-price-card-registrations` 换操作者 Intake**，照第四批计价四口办——无阻塞，代码活，由通道 1 另派。
3. **网络七口的 `networkhttp.OperatorRegistryIntake` 补 http 层测试**，照另五族各自的 `operator_registry_intake_test.go`——无阻塞，属第三批，代码活，由通道 1 另派。

### 剩余第 3 件 ← 通道 1 · 2026-10-10

认领：通道 1，分支 `mcp1-oc04-nrhttp`，基 `1d67e27c`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp1-oc04-nrhttp`。本件原拟派通道 4；
用户 21:1x 告知通道 4 已 crash，改由通道 1 自己做。第 2 件由通道 2 在 `mcp2-oc04-pcr` 上做（`task-4a593223`）。两件各记在自己那一节；
Status 行由推送方在两件进 main 时一并改，免得两条分支各改同一行。
