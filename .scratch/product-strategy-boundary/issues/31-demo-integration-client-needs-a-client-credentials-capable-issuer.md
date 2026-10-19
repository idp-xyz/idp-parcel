# 31 演示租户的合成集成客户端：演示发行方要签得出客户端凭据令牌

Category: task
Status: ready-for-agent——2026-10-19 Claude Code 会话立并分诊（用户「你作为业务和系统专家自决……解决所有的问题」授权）。从 [30](./30-demo-tenant-registers-work-devices-and-work-fact-grants.md) 拆出：30 只做作业事实那一半。
Blocked by: 无
地盘：`scripts/demo-seeds/operator-channel/`（`dex.yaml`、`compose.yaml`、新取令牌脚本）；`scripts/demo-seeds/data/access/` 新增的集成客户端行文件；`scripts/demo-seeds/seed.sh` 只加调用行（照 30 与 `idp-parcel-9b` 的约定，加完把 SHA 告诉它）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、二；[ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)。

## 现状（实测，证据 `S`）

- 集成客户端各口（外部结果、监管凭证登记、外部资金事实登记与更正）在演示上答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，启动日志「Integration client channel not configured (ADR-0149)」（钉 `c68e4342`）。
- 种子里 `access_identity.integration_client` 零行；三种外部事实的准入对照与权威区间已登。
- 演示发行方 Dex 只配了管理台公共客户端，`grantTypes` 只有 `authorization_code` 与 `refresh_token`。
- 2026-10-19 探针：另起一只 `ghcr.io/dexidp/dex:v2.45.1`（与 `compose.yaml` 钉的同一版，端口 5557，不碰共用发行方）。加机密静态客户端 `syn-integration-client-01`，`grantTypes` 列 `client_credentials`，环境带 `DEX_CLIENT_CREDENTIAL_GRANT_ENABLED_BY_DEFAULT=true`。按客户端凭据许可取令牌，Dex 日志答 `unsupported grant type grant_type=client_credentials`。服务端 `oauth2.grantTypes` 列表是否还要另列这一授予，探针没核。

## 三类归法

- **机制**：集成客户端族的校验器、铸造器、注册册与授予都已在 main（`oidc.ClientVerifier`：签名、iss、aud、有效期，再核 RFC 8705 证书绑定；不带 `cnf` 的持有者令牌答未绑定）。本票不改代码。
- **租户取值（演示租户）**：合成集成客户端与其授予、演示部署的集成客户端发行方三件参数（`IDP_PARCEL_INTEGRATION_CLIENT_OIDC_ISSUER`、`_JWKS_URL`、`_AUDIENCE`）。全部 `SYN-` 值，证据只记 `S`。

## 做什么

1. **让演示发行方签出客户端凭据令牌。** 先补核服务端 `oauth2.grantTypes`；仍不行再换 Dex 版本（钉一个确实支持的确切版本，不用浮动标签）。受众要与操作者族不同（ADR-0149 决定三），写明它怎么落进 `aud`。
   - Dex 上走不通时，退路是演示部署另起一只只签客户端凭据的合成发行方。它与操作者族的发行方分开，`sub` 稳定，钥匙经 JWKS 公开。选哪条记进本票。
2. **登客户端与授予。** `integration-client-register` 登 `syn-integration-client-01` 对应的主体（发行方加 `sub`），`-grant` 授外部结果、监管凭证、外部资金事实三族。
3. **取令牌脚本与动线。** 照 `operator-token.sh` 的样子加一个脚本。动线「前置」补三件参数，「取证」补三族口越过渠道的期望答复。

## 不做

- 不改校验器、铸造器或答复形状。不为演示放宽证书绑定的核验。
- 不登真实客户端。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子，退 0。
- 照动线「前置」起进程，带集成客户端令牌：外部结果、凭证、资金事实各口越过渠道，答业务校验或业务结果，不再是 `403`。证据只记 `S`。走完删库。
