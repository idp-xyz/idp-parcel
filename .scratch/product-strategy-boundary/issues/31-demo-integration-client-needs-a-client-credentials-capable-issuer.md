# 31 演示租户的合成集成客户端：演示发行方要签得出客户端凭据令牌

Category: enhancement
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话立、分诊并实施，分支 `claude-psb31`。立票时（用户「你作为业务和系统专家自决……解决所有的问题」授权）。从 [30](./30-demo-tenant-registers-work-devices-and-work-fact-grants.md) 拆出：30 只做作业事实那一半。
Blocked by: 无
地盘：`scripts/demo-seeds/operator-channel/`（`dex.yaml`、`compose.yaml`、新取令牌脚本）；`scripts/demo-seeds/data/access/` 新增的集成客户端行文件；`scripts/demo-seeds/seed.sh` 只加调用行（照 30 与 `idp-parcel-9b` 的约定，加完把 SHA 告诉它）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、二；[ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)。

## 现状（实测，证据 `S`）

- 集成客户端各口（外部结果、监管凭证登记、外部资金事实登记与更正）在演示上答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，启动日志「Integration client channel not configured (ADR-0149)」（钉 `c68e4342`）。
- 种子里 `access_identity.integration_client` 零行；三种外部事实的准入对照与权威区间已登。
- 演示发行方 Dex 只配了管理台公共客户端，`grantTypes` 只有 `authorization_code` 与 `refresh_token`。
- 2026-10-18 探针：另起一只 `ghcr.io/dexidp/dex:v2.45.1`（与 `compose.yaml` 钉的同一版，端口 5557，不碰共用发行方）。加机密静态客户端 `syn-integration-client-01`，`grantTypes` 列 `client_credentials`，环境带 `DEX_CLIENT_CREDENTIAL_GRANT_ENABLED_BY_DEFAULT=true`。按客户端凭据许可取令牌，Dex 日志答 `unsupported grant type grant_type=client_credentials`。演示 `dex.yaml` 的服务端 `oauth2.grantTypes` 只列了 `authorization_code` 与 `refresh_token`，很可能就是这条拒因，探针没核。

## 三类归法

- **机制**：集成客户端族的校验器、铸造器、注册册与授予都已在 main（`oidc.ClientVerifier`：签名、iss、aud、有效期，再核 RFC 8705 证书绑定）。令牌不带 `cnf` 时答「未绑定」、不拒；拒不拒由铸造器看客户端行上要不要求绑定。本票不改产品代码。
- **租户取值（演示租户）**：合成集成客户端行与其授予、演示部署的集成客户端发行方三件参数（`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER`、`_JWKS_URL`、`_AUDIENCE`）。客户端行的 `certificateBoundTokenRequired` 显式填 `false`：Dex 签不出 `cnf`，这是演示租户对这台客户端的取值，不是放宽核验。全部 `SYN-` 值，证据只记 `S`。

## 做什么

1. **让演示发行方签出客户端凭据令牌。** 先补核服务端 `oauth2.grantTypes`；仍不行再换 Dex 版本（钉一个确实支持的确切版本，不用浮动标签）。受众要与操作者族不同（ADR-0149 决定三），写明它怎么落进 `aud`。
   - Dex 上走不通时，退路是演示部署另起一只只签客户端凭据的合成发行方。它与操作者族的发行方分开，`sub` 稳定，钥匙经 JWKS 公开，代码只落在 `scripts/demo-seeds/` 下。选哪条记进本票。
2. **登客户端与授予。** 一个客户端只代表一个外部方（`IntegrationClientBinding` 头注），所以登两台：合成报关服务商一台，授外部结果与监管凭证；合成银行一台，授外部资金事实。每台行上填主体（发行方加 `sub`）、`sourceIdentity`、`credentialRef`、`certificateBoundTokenRequired`（翻译器拒收缺席），再经 `-grant` 授予。
3. **取令牌脚本与动线。** 照 `operator-token.sh` 的样子加一个脚本。动线「前置」补三件参数，「取证」补三类口越过渠道的期望答复。

## 不做

- 不改校验器、铸造器或答复形状。不为演示放宽证书绑定的核验。
- 不登真实客户端。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子，退 0。
- 照动线「前置」起进程，带集成客户端令牌：外部结果、凭证、资金事实各口越过渠道，答业务校验或业务结果，不再是 `403`。证据只记 `S`。走完删库。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-psb31`）

- 走的是退路：另起一只发行方，不换操作者族那只。
  - 补核过：v2.45.1 在服务端列上客户端凭据、再加特性开关，仍答 `unsupported_grant_type`。v2.46.0 不开特性开关也签得出。
  - 操作者族那只钉在 v2.45.1 有它的理由：v2.46.0 口令用户换刷新令牌会把存储锁死。所以不能整只升级。
- 发行方：`compose.yaml` 加第二个服务 `integration-issuer`（`ghcr.io/dexidp/dex:v2.46.0`，`127.0.0.1:5558`），配置 `integration-dex.yaml`。
  - 只开客户端凭据许可，不登口令用户。
  - 受众客户端 `idp-parcel-integration` 以 `trustedPeers` 认两台合成客户端。它们取令牌时带 `audience:server:client_id:idp-parcel-integration`，令牌的 `aud` 含本产品的集成受众，也含客户端自己。
  - 两族受众不同，一族的令牌过不了另一族的校验（ADR-0149 决定三）。
- 客户端与授予：
  - `data/access/05-integration-clients.json`：合成报关服务商、合成银行各一台。主体取这只发行方签出的 `sub`，要求证书绑定令牌填 `false`。
  - `06-integration-client-grants.json`：报关服务商授外部结果与监管凭证，银行授外部资金事实。
  - `seed.sh` 加两行调用。
- 取令牌脚本 `operator-channel/integration-token.sh`。README 加「集成客户端渠道」一节，组成表 `data/access/` 与 `operator-channel/` 两行补上新文件。动线「前置」补三件参数，「取证」那句改成能越过渠道的说法。
- 实走（`S`，一次性库 `c8_walk31`，走完已删）：
  - 灌种子退 0，两台客户端与三条授予都 `RECORDED`。
  - 带集成客户端三件起 `parcel-api`，日志出现集成客户端发行方已配置那一行。
  - 报关服务商打外部结果答 `200 SOURCE_NOT_ACCEPTED`，打监管凭证登记答 `400 MALFORMED_REQUEST`；银行打外部资金事实登记答 `400 MALFORMED_REQUEST`。都越过了渠道，空体走到业务受理或报文校验。
  - 对照组：银行打外部结果、报关服务商打资金事实，都答 `403 INTEGRATION_CLIENT_NOT_GRANTED`；操作者令牌与不带令牌都答 `401 INTEGRATION_CLIENT_CREDENTIAL_REJECTED`。
- 产品代码一处未改。

## 评审记录（2026-10-18，隔离子代理一次过两轴）

- 阻断 1 条，已修：`integration-token.sh` 提交时没带可执行位（本仓 `core.filemode=false`，工作树的权限没进 index），新检出里照 README 直接执行会答 `Permission denied`。已用 `git update-index --chmod=+x` 补上。
- 非阻断已修：
  - compose 头注改成两只 Dex，用法补 5558 的发现文档。
  - README 组成表 `operator-channel/` 一行补两份新文件。
  - 「族」只留给操作者族与集成客户端族，事实类型一律称「类」。
  - 受众客户端的注释不再说「不取令牌」，配置并没拦它，改说它不在册、取到的令牌答未授予。
- 评审另核实：v2.45.1 的二进制里没有客户端凭据许可；这只发行方不发刷新令牌，`offline_access` 答 `invalid_scope`，碰不到 v2.46.0 那条存储锁死。
