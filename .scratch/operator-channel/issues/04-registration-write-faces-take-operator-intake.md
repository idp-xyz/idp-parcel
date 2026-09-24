# 04 登记册配置写面逐口换操作者 Intake

Category: enhancement
Status: in-progress——2026-09-25 通道 4 认领（用户令独立完成操作者渠道这条链），逐上下文分批进 main：第一批可见性八口、第二批关务七口、第三批网络七口、第四批计价四口、第五批商业参与方六口已换（见文末）。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 03
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
- 商业参与方 6 口：`/commercial-{service-product-form,product-channel-mapping,registration-number-type,channel-account-use}-registrations`、`/commercial-registration-number-type-deactivations`、`/commercial-channel-account-use-revocations`——**第五批已换**。
- TF 5 口：`/transport-fulfillment-external-carrier-credential-{registrations,applicability-changes}`、`/transport-fulfillment-effective-time-rule-registrations`、`/transport-fulfillment-carrier-master-document-{registrations,revisions}`。

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

