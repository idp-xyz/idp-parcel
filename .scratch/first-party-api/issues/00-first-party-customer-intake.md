# 首方客户渠道 Intake：客户渠道册、首方铸造器、提交口先换并撤写开关

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-fpa00（2026-10-19 立；ADR-0139–0142 于同日由用户在 Claude Code 会话里答「接受」，按复核建议修订后接受）。
Blocked by: 无
地盘：`internal/accessidentity`（客户渠道册、首方铸造器）、`migrations/access_identity/` 下一号、`cmd/parcel-access-register` 新子命令、`cmd/parcel-api` 新 `assemble_*.go` 与 `endpoints.go` 提交口一行、`scripts/demo-seeds`（access 种子与演示发行方配置）、参数登记册 `PAR-INT-13` 的证据格不动（无租户）。
出处：[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 二至四、六、七与 Consequences；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三。

## 做什么

1. **客户渠道册**：一行 =（客户端主体〔发行方 + `sub`〕、租户、客户账户、凭据引用、是否要求证书绑定、登记依据），绑定一次登定、不可改指；生效区间与撤销挂在授予上，授予按客户面能力登记。迁移取 `migrations/access_identity/` 现有最大号的下一号；`parcel-access-register` 加登记与授予两个子命令，形状照集成客户端那两个。
2. **首方铸造器**：复用 `adapters/oidc.ClientVerifier` 校验令牌，主体取（发行方, `sub`）；以已核验主体查客户渠道册，查到即绑定成立；再核证书绑定、授予、准入范围，经 `seal` 铸首方族的来源信封。顺序照 `MintIntegrationClient`（ADR-0139 Decision 二）。答复照 ADR-0139 Decision 四的各格与错误码。
3. **部署参数**：首方族自带一组三件（发行方、JWKS、受众），全设或全不设，只设一部分即启动拒；受众必须与操作者族、集成客户端族都不同。
4. **提交口先换**：`/shipment-requests` 挂首方 Intake，同一笔撤下隔离提交放行（写开关的最后一行）；若读开关此时已随 operator-channel/53 退场，同一笔删掉两个开关（ADR-0139 Decision 六、ADR-0150 决定三）。其余客户面各口逐口换，另立票。URL 主版本 `/v1` 前缀随 ADR-0140 的实施票加，不在本票。
5. **演示**：种子登一个合成客户端（绑 `SYN-ACCOUNT-01`）与提交能力的授予；演示发行方配置加首方受众与该客户端；`scripts/demo-seeds/submit-one-shipment.sh` 改为先取客户端令牌再提交。

## 完成判据

- 单元与真库用例：令牌无效 / 证书绑定不符 401；不在册或无生效授予 403；生产写不在准入范围 403；依赖读不动 503；正例铸出首方信封、提交形成 `SUBMITTED`；他族令牌（操作者、集成客户端）在首方口一律拒。
- 装配测试：三件参数只设一部分启动拒；首方族未配置时提交口答 `ACCESS_CHANNEL_NOT_CONFIGURED`；隔离提交放行已撤。
- 演示实走（`S`）：合成客户端取令牌经首方口提交一单，答 `SUBMITTED`，后续动线不变。

## 规模

L

## 完成记录（2026-10-19，作者子代理，分支 `claude-fpa00`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。本票不属 TF 链方案，权威是 ADR-0139（决定二至四、六、七、八）、ADR-0140、ADR-0149（凭据机制）、ADR-0150 决定三。未立新 ADR：实现中没有遇到 ADR-0139 没裁的难逆转取舍（ADR-0211 未用）。前一位作者被用量上限中断，本次在其提交上续做，未推倒重来。

### 做了什么

1. **客户渠道册**（`internal/accessidentity/customer_client.go`、`adapters/postgres/customer_client_registry.go`、`migrations/access_identity/0010_customer_channel_register.sql`）：行 =（客户端主体〔发行方 + `sub`〕、租户、客户账户、凭据引用、是否要求证书绑定、登记依据），主键是主体，一次登定、不可改指；授予按客户面能力登记、带生效区间，撤销另表、只增不改。登记口答复代数同集成客户端册（`RECORDED` / `ALREADY_REGISTERED` / `CONTENT_CONFLICT` / `CLIENT_BOUND_TO_ANOTHER_TENANT` / `CLIENT_NOT_REGISTERED` / `GRANT_NOT_REGISTERED`），写口无事务即拒。迁移号 0010，未加决定种类，不动决定种类那条 CHECK。
2. **首方铸造器**（`customer_client_envelope.go` 的 `CustomerClientMinter`）：`CustomerClientVerifier`（`adapters/oidc`，包一只 `ClientVerifier`）先验签名、`iss`、`aud`、有效期，不读册 → 以已核验主体查客户渠道册 → 核证书绑定 → 核授予 → （要判的口）判准入范围 → 由册上那一行拼 `ChannelRegistration` 经 `seal` 铸 `SubmissionEnvelope`。顺序照 `MintIntegrationClient`。首方族的 `RequestKeyDerivation` 取载荷里的接入请求标识。`doc.go`、`credential.go`、`registration.go`、`mint.go` 头注照 ADR-0139 Consequences 改写。
3. **部署参数**（`cmd/parcel-api/assemble_customer_client.go`）：`IDP_PARCEL_CUSTOMER_API_OIDC_ISSUER` / `_JWKS_URL` / `_AUDIENCE` 三态——全不设即整族未配置答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，设一部分或值不成形启动拒，受众与操作者族、集成客户端族已设的那件相同也启动拒。
4. **提交口先换**：`/shipment-requests` 挂 `CustomerSubmissionIntake`（`internal/parcelshipment/adapters/http/customer_submission_intake.go`，由原隔离提交 Intake 改写而来），同笔删去隔离提交 Intake、`isolatedWriteAdmittedCommandLines` 与它的启动日志格；提交端点的失败映射并入 `writeIntakeProblem`，答复按 ADR-0139 决定四分格（`401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`、`403 CUSTOMER_CLIENT_NOT_GRANTED`、`403 OUTSIDE_ADMISSION_SCOPE`、`503 IDENTITY_DEPENDENCY_UNAVAILABLE`、`403 ACCESS_CHANNEL_NOT_CONFIGURED`）。读开关（operator-channel/53）此刻仍在，所以两个开关都不删，见代决 6。
5. **CLI**：`parcel-access-register` 加 `customer-client-register` / `customer-client-grant` / `customer-client-revoke`。
6. **演示**：演示发行方 `integration-dex.yaml` 加受众静态客户端 `idp-parcel-customer-api`（`trustedPeers` 只列首方客户端）与合成货主系统 `syn-customer-client-01`；种子 `data/access/07-customer-clients.json`（绑 `SYN-ACCOUNT-01`）、`08-customer-client-grants.json`（`SHIPMENT_REQUEST_SUBMISSION`），`seed.sh` 追加两行；新 `operator-channel/customer-token.sh`；`submit-one-shipment.sh` 先取令牌再带 Bearer 提交，载荷带 `accessRequestId`；README「首方客户渠道」一节、`docs/design/synthetic-demo-journey-script.md` 起后端那段与开关矩阵随改。
7. 立两张后续票：[04](./04-ps-production-ownership-directory-before-the-write-switch-retires.md)（PS 生产归属目录，写开关退场的前件）、[05](./05-admin-web-submission-page-has-no-customer-credential.md)（管理台提交页没有首方凭据）。

### 代决

1. **客户面能力词表一口一词**，首发只有 `SHIPMENT_REQUEST_SUBMISSION`；还没换口的客户面各口不进词表，随各口换上首方 Intake 的那一笔加，`CustomerCapability` 枚举与迁移里那条 CHECK 同笔改。粒度（按口还是按用例归并）留给票 01。
2. **来源格由主体推出**：来源信封的来源 = `发行方#sub`（`CustomerClientSubject.channelSource`），册上不设来源一列。客户端本身就是来源，另登一格会让它与主体各说各话；发行方要在里面，因为 `sub` 只在发行方之内唯一。
3. **首方族推导口**：来源请求键 = 载荷 `accessRequestId` 去两端空白，此外一字不动；提交与撤回读同一格、不加前缀。缺席即 `ErrRequestKeyNotDerived`，答 `400`。
4. **首方提交载荷形状** = 管理台提交页的草案形状加一格 `accessRequestId`，封闭解码（未知字段即拒：自报的租户、账户、来源都落在这一类）。正式字段名与版本发布归 ADR-0140 的契约生成票。
5. **拟受理范围快照** =（租户、客户账户），引用 `customer-account:<租户>/<账户>`，摘要带形状版本 `PS-CUSTOMER-ACCOUNT-ADMISSION-SCOPE/1`。一个客户端只绑一个客户账户，这个账户就是这次提交能确定的完整范围。换算成治理坐标归票 04 的生产目录。
6. **写开关不删，只撤命令面放行**：读开关（operator-channel/53）没退场，ADR-0150 决定三「最后一口那一笔删两个开关」还没到时候。另外写开关还承担 ADR-0091 的墙二（给提交编排注入合成治理坐标），而 PS 生产归属目录还没立，此刻删它，演示提交就停在 `OWNERSHIP_UNRESOLVED`。所以本票只删墙一，墙二随票 04 接手后，与读开关在同一笔里删。
7. **受众撞车即启动拒**：ADR-0139 决定二只说「必须不同」。实现成：首方受众与另两族任一件已设的值相同，进程启动即拒；另两族没配置的不比。
8. **证书绑定判在授予之前，答 401**：照 `MintIntegrationClient` 同一处的做法。
9. **核验方分型**：出示物复用 `IntegrationClientCredential`（凭据机制共用），核验方接口另立 `CustomerClientCredentialVerifier`，不复用集成客户端族的接口——共用一个接口时，装配点把集成客户端那一只接到首方铸造器上也能编译通过。
10. **CLI 多一个撤销子命令**（票面写的是登记与授予两个）：授予「可撤销」是 ADR-0139 决定二的一部分，集成客户端那组也是三个子命令。
11. **演示发行方共用**：首方族与集成客户端族落在同一只 Dex（`integration-issuer`）上，受众不同。ADR-0139 决定二说三族落不落在同一个发行方实例上是部署决定，演示环境取共用。

### 越权风险点（待 owner 复核）

1. 能力词 `SHIPMENT_REQUEST_SUBMISSION` 的名字和「一口一词」的粒度，进了库上的 CHECK 和演示种子；票 01 若要归并，改的是 CHECK、枚举和已登的授予行。
2. 来源串 `发行方#sub` 进了 PS 的来源身份，也进了判重空间。owner 若要按登记的显式来源名（例如客户系统名）出来源，就要给客户渠道册加列、加迁移，已落的委托来源串也不会跟着变。
3. 推导口不加前缀：同一个接入请求标识先用于提交、再用于撤回，会被答成接入冲突，而不是两次独立请求。ADR-0139 决定五没有裁这一点。
4. `accessRequestId` 这个字段名与提交载荷形状，在 ADR-0140 的契约生成落地之前就由本票定了；契约票若改名，客户系统和演示脚本要一起改。
5. 拟受理范围快照的形状由本票自定，进了决定记录和门禁里的范围摘要；票 04 的目录若要别的范围形状，形状版本要升，旧记录与新记录的摘要不可比。
6. 管理台的「提交委托」页从本票起必然答 `401`，演示动线里「管理台提交」这一步断了，演示提交改走 `submit-one-shipment.sh`。保留与否（「操作者代客提交」是另一个用例，ADR-0139 决定三）归票 05 待裁。
7. 生产形态（不设写开关）下，任何租户经首方口提交都停在 `OWNERSHIP_UNRESOLVED`，直到票 04 立起 PS 生产归属目录。首方口在生产形态下还不能落成`已提交`。
8. 受众撞车只比已设的那几件：另一族后来再配上一个同值受众，要等进程重启才会被拒，在线换参数时没有防护（部署参数本来就只在启动时读）。

### 用例与完成判据对照

- 令牌无效 / 证书绑定不符 401、不在册或无生效授予 403、依赖读不动 503、正例铸出首方信封：`internal/accessidentity/customer_client_envelope_test.go`、`customer_client_test.go`、`adapters/oidc/customer_client_verifier_test.go`、`internal/parcelshipment/adapters/accessidentity/customer_client_authenticator_test.go`、`adapters/http/submit_shipment_request_test.go` 的分格用例。
- 生产写不在准入范围 403：提交口按 ADR-0139 决定八不在铸造侧判准入，公开方法走不到这一步；铸造器的准入那一步由 `customer_client_admission_test.go` 钉住，留给撤回等各口。
- 他族令牌在首方口一律拒：`cmd/parcel-api/assemble_customer_client_test.go` 的 `TestOtherFamiliesTokensAreRefusedAtTheSubmissionLine`。
- 真库正例提交形成 `SUBMITTED`：同文件 `TestACustomerClientSubmitsThroughTheFirstPartyLineAgainstARealDatabase`。
- 装配：三件只设一部分或受众撞车即启动拒（`TestCustomerChannelParametersRefuseToStartWhenPartialOrSharingAnAudience`）；首方族未配置时提交口答 `ACCESS_CHANNEL_NOT_CONFIGURED`，与字面量未配置逐字节同（`TestSubmissionLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset`）；隔离提交放行已撤（`isolated_write_test.go` 的 `TestNeitherIsolatedSwitchOpensTheSubmissionLine`）。
- 册：`adapters/postgres/customer_client_registry_test.go`（重放与不可改指、跨租户拒、区间与撤销、无事务即拒）；CLI：`cmd/parcel-access-register/customer_client_test.go`。

### 验证

- `go build ./...`、`go vet`（相关包）、`gofmt -l` 干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1`：`./internal/architecture/`、`./internal/accessidentity/...`、`./cmd/parcel-access-register/`、`./cmd/parcel-api/`、`./internal/parcelshipment/...`、`./internal/platform/migrate/...`、`./scripts/demo-seeds/...` 全 ok。未跑全量，未重生成机制清点（进 main 时统一做）。
- 与 `origin/main`（`1167b4d9`）试合并（`git merge-tree`）无冲突。
- **演示实走（`S`）**：一次性库 `fpa00_demo` 上 `seed.sh` 退 0（客户渠道行与提交授予都答 `RECORDED`）。另起一只私有 Dex（同一份 `integration-dex.yaml`，issuer 串仍是 `http://127.0.0.1:5558/dex`，发布在 5560 端口，没有重启其他会话在用的共享发行方容器）。`customer-token.sh claims` 读到 `sub=ChZzeW4tY3VzdG9tZXItY2xpZW50LTAx`、`aud` 含 `idp-parcel-customer-api`，与种子行一致；集成客户端请求首方受众时，发行方答 `invalid_scope`。`parcel-api` 带首方三件（JWKS 指 5560）与读写两个开关起；`submit-one-shipment.sh` 答「已提交 SHR-WB6GU2Z35LK3FDTNQKUSEH6IYI 来源键 SYN-ACCESS-REQUEST-20261019T122651Z；列表与详情都读到 SUBMITTED」。同一进程上的探针：不带令牌、带集成客户端令牌都答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`；用 `customer-client-revoke` 撤掉提交授予之后再跑脚本，答 `403 CUSTOMER_CLIENT_NOT_GRANTED`。用完已删库、删容器。

### 偏离与待裁

- 票面第 4 条「写开关的最后一行」：本票撤的是命令面放行（墙一），写开关本身因墙二与读开关仍在而保留（代决 6），删除挪到票 04 与 operator-channel/53。
- 票面第 1 条「登记与授予两个子命令」：实做三个，多了撤销（代决 10）。
- 前一位作者留下的 `customer-token.sh` 缺可执行位，`submit-one-shipment.sh` 因此取不到令牌，本次补上；接线基线里摘要函数接法的那段注释仍指已删的隔离提交 Intake，一并改指首方提交 Intake。
- 待裁：票 04（生产归属目录与能力词、事实类型词）、票 05（管理台要不要保留代客提交）、票 01（能力粒度与 1:N 绑定）。
