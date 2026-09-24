# 02 操作者族的凭据校验：OIDC 令牌校验器与 parcel-api 部署参数

Category: enhancement
Status: in-progress——2026-09-25 分支完工，待评审与进 main（完成记录见文末 Comments）。2026-09-25 通道 4 认领（用户令通道 4「继续完成」ADR-0151 那件，operator-channel/15 的实现要先过 03，03 要先过本票；动手前已告知 2026-09-24 22:35 按用户令承接 02、13、14 的通道 2），隔离 worktree `idp-parcel-mcp4-oc02`、分支 `mcp4-oc02`（基 `53d537cc`）。此前 ready-for-agent——2026-09-24 拆法经用户授权通道 4 自决认可
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：`internal/accessidentity`（`CredentialVerifier` 在操作者族上的生产实现）、`cmd/parcel-api` 部署形态参数。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二前两条、决定五第二条。

## 做什么

1. `parcel-api` 自己校验令牌：签名（按 JWKS）、`iss`、`aud`、有效期；不采信任何自报头部与 SPA 登录态。凭据本体只在校验那一刻存在，不落库、不进日志。
2. 部署形态参数：发行方地址、JWKS 端点、受众。未设即操作者渠道整族未配置，缺省朝拦；与 ADR-0049 一样全部必填、不给默认值。
3. 测试用进程内签发与 JWKS 替身；演示环境要接一个真 OIDC 实现作为发行方（选型在本票定，写进完成记录），不做绕过 OIDC 的「开发用」本地账号。

## 不做

- 操作者册（01）、信封与答复代数（03）。

## 完成判据

- 用例覆盖：合格令牌通过；签名错、`iss` / `aud` 不符、过期、缺令牌各自拒；JWKS 取不回答依赖故障而不是「凭据不对」；参数缺席时整族答未配置。

## Comments

### 完成记录 ← 通道 4 · 2026-09-25 · 分支 `mcp4-oc02`（待评审与进 main）

**逐笔落点**（分支上的 SHA，进 main 后另记 main 上的）：`8a3f8ef1` 认领；`0eece0b1` 校验器（`internal/accessidentity` 的 `OperatorCredential`、`OperatorCredentialVerifier`、`UnconfiguredOperatorCredentialVerifier`、新格 `ErrCredentialVerifierUnavailable`，与 `internal/accessidentity/adapters/oidc`）；`d8153b08` `cmd/parcel-api` 三件部署参数；`aae54dca` 机制清点重生成。

**完成判据逐条**（用例都在 `internal/accessidentity/adapters/oidc/verifier_test.go`，末条在 `cmd/parcel-api/assemble_operator_credential_test.go`）：
- 合格令牌通过：`TestQualifiedTokenYieldsTheOperatorSubject`（RS256）、`TestES256TokenFromAPublishedECKeyIsAccepted`、`TestAudienceListContainingThisAudienceIsAccepted`。
- 签名错：`TestTokenSignedByAKeyTheIssuerDidNotPublishIsRejected`；`iss` / `aud` 不符：`TestTokenIssuedForAnotherDeploymentIsRejected`；过期：`TestTokenOutsideItsValidityWindowIsRejected`；缺令牌：`TestMissingOrMalformedTokenIsRejected`。四者都答 `ErrCredentialRejected`。
- JWKS 取不回答依赖故障：`TestUnreachableKeySetIsADependencyFailureNotARejection`（503、宕机、内容不成形三种），断言是 `ErrCredentialVerifierUnavailable` 且**不是** `ErrCredentialRejected`。
- 参数缺席整族未配置：`TestOperatorChannelWithoutIssuerParametersAnswersNotConfigured`；设一半或不成形启动即拒：`TestOperatorChannelWithIncompleteOrMalformedIssuerParametersRefusesToStart`。
- 票面之外多证的：算法混淆（HS256 以公开的 RSA 公钥为密钥）、`none`、`crit`；公钥集缓存复用、轮换重取、陌生 `kid` 不逼每个请求都去打发行方、发行方宕着时缓存里的钥照验、取失败不每次重试；令牌本体经 `fmt` 各动词、`slog` 两种处理器、JSON 与错误信息都印不出（`TestOperatorCredentialNeverRendersTheToken`）。

**演示环境的发行方选型：Dex。** 单进程容器、配置文件即可登合成操作者（静态口令用户）与管理台公共客户端，支持 authorization_code + PKCE，按 RS256 签名、在 `<issuer>/keys` 发布 JWKS，不带数据库。比过的：Keycloak 要 JVM 加一只库，对演示太重；Zitadel 同样要库；Ory Hydra 不带登录界面，还得另写一个。接法就是本票的三件参数：`IDP_PARCEL_OPERATOR_OIDC_ISSUER` 取 Dex 的 issuer、`IDP_PARCEL_OPERATOR_OIDC_JWKS_URL` 取 `<issuer>/keys`、`IDP_PARCEL_OPERATOR_OIDC_AUDIENCE` 取管理台的客户端标识。**本票没有把 Dex 接进 compose**：在 03（信封）与 07（管理台登录门）之前没有任何东西消费发行方，接进去只会多一个空转的容器；那一步随 07 做。

**判断项**（留给评审）：
1. 另立 `OperatorCredentialVerifier` 而不实现 `CredentialVerifier`：后者拿登记行上的受控引用去核、只答真假，操作者族核验之前没有行可查，主体本身是核验的产物。ADR-0100 决定二写的「`CredentialVerifier` 在操作者族上的生产实现」按职责读，不按接口名读。
2. 不做 OIDC 发现，JWKS 地址单列一件参数（票面第 2 条本就列了三件）：校验路径上只有这一处出网。
3. 地址只要求 http(s) 绝对地址，不强求 https：演示环境发行方在 compose 网络内按服务名互访；生产用 https 属部署方。
4. 公钥集缓存没有过期时间，只在缓存里找不到令牌所指的 `kid` 时重取（两次取至少隔一分钟，失败也计入）。代价：发行方撤下的钥在本进程重启或下一次重取之前仍被信任——与 go-oidc 的缓存语义相同；要收紧就加一条最长缓存期，是一行常量的事。
5. `exp` 不放宽，`nbf` 放宽五分钟（与 go-oidc、Azure 身份库同）；`iss` 逐字比较不折叠末尾斜杠。
6. 令牌不带 `kid` 只在公钥集恰有一把钥时认（OIDC Core 第 10.1 节）。
7. 操作者族沿用 `ErrCredentialRejected` 与 `ErrAccessChannelNotConfigured` 两格，只新立依赖故障一格；它们映射成 `401` / `403` 是 03 那个 Intake 的事，本票不碰答复码。

**验证**（钉 `d8153b08`，代码 tip；`aae54dca` 只多一份清点）：`gofmt -l` 空；`go build ./...`、`go vet ./...` 全仓过；带 DSN 的 `go test -count=1 -p 1` 覆盖动过的包及其反向依赖（`internal/accessidentity/...`、`cmd/parcel-access-register`、`cmd/parcel-api`）加 `internal/architecture/...`，全部 ok；真库用例实跑（`accessidentity/adapters/postgres` PASS 6 / SKIP 0，`cmd/parcel-api` PASS 124 / SKIP 0）；`adapters/oidc` 另跑 `-race` 过。全仓带 DSN 全量留给进 main 那一跑。

**评审**：尚无非作者评审。作者按票面与 ADR-0100 自查过一遍，**不算非作者评审**。

**未做**：发行方接进演示环境（随 07）；`OperatorEnvelope` 与铸造（03）；`accessidentity/doc.go` 那段「本轮既没有登记册的表，也没有凭据形态」的改写（03 第 4 条）。
