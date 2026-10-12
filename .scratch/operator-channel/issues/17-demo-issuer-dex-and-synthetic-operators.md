# 17 演示发行方：Dex 接进演示环境，合成操作者按它的标识另登

Category: enhancement
Status: in-progress——2026-10-12 通道 4 认领（通道 1 派单 `task-605064ea`，接替一直未接的 `task-be560cd8`），隔离 worktree `idp-parcel-mcp4-oc17`、分支 `mcp4-oc17`（基 `00bc85ce`）。此前 ready-for-agent——2026-10-12 通道 1 补立：[02](./02-oidc-credential-verifier-and-deployment-parameters.md) 完成记录写「那一步随 07 做」，而 [07](./07-admin-web-login-gate.md) 的地盘只有 `apps/admin-web/**`，这一步于是没有票认领；[04](./04-registration-write-faces-take-operator-intake.md) 第 1 件、[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口、[10](./10-device-register-and-operation-fact-capability-face.md) 换口那半与 07 的完成判据都在等它
Blocked by: 无（02、03 已 resolved）
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 甲轨
地盘：演示发行方的容器描述与 Dex 配置（进 `compose.yaml` 还是另起一份演示用 compose 文件，本票定）；`scripts/demo-seeds/data/access/`、`seed.sh` 里操作者册那一段、`scripts/demo-seeds/README.md`。不碰 `apps/admin-web/**`（07）、`internal/accessidentity` 与参数登记册（10 在改）、`cmd/parcel-api` 的代码。
出处：02 完成记录「演示环境的发行方选型：Dex」；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定二、决定五与 Consequences「演示多一个运行依赖」。

## 做什么

1. 演示环境起一个 Dex 作操作者族的发行方：镜像钉到精确版本（与 `compose.yaml` 钉 PostgreSQL 补丁号同理）；配置里登管理台的公共客户端（authorization_code + PKCE，回调指管理台本地开发源）与合成操作者的静态口令用户，至少覆盖 `data/access` 里的配置员与查阅员。`parcel-api` 对 `iss` 逐字比较（02 判断项 5），浏览器与 `parcel-api` 看到的必须是同一串发行方地址。接进 `compose.yaml` 就同笔改它的头注——那里今天写着它只为 PostgreSQL 集成门禁准备、不是部署描述符。
2. 合成操作者按 Dex 另登：照 `seed.sh` 操作者册那段头注办——册上的主体不能改指到新发行方，要按新发行方的标识另登一批。`issuer` 取 Dex 的 issuer，`subject` 取 Dex 对对应静态用户真签出的 `sub`（从一枚真令牌里读，不按配置推）；能力面与生效区间照合成发行方那批。合成发行方那批留不留由本票定，写理由。
3. `parcel-api` 在演示环境的三件部署参数写进 README 起进程那一段：`IDP_PARCEL_OPERATOR_OIDC_ISSUER` 取 Dex 的 issuer，`IDP_PARCEL_OPERATOR_OIDC_JWKS_URL` 取 `<issuer>/keys`，`IDP_PARCEL_OPERATOR_OIDC_AUDIENCE` 取管理台的客户端标识（02 完成记录原话）。产品侧照旧不给默认值（02 做什么第 2 条）。
4. 交给 07 的取值写进本票「交给 07」一节：发行方地址、授权端点、令牌端点、客户端标识、回调地址，以及管理台该带哪一枚令牌作 Bearer（访问令牌还是 ID 令牌，以 Dex 实签令牌的 `aud` 为准）。管理台怎么接、开发代理怎么配归 07。

## 不做

- 管理台代码与它的开发代理（`apps/admin-web/**`，07）。
- 任何口的换口与撤隔离放行：04 第 1 件、10 换口那半（做什么第 4 条与完成判据第二条）、15 余下四口各自做。
- 「作业事实登记」的授予与设备（10）、「运营决定」的授予（15）：随各票灌。
- 绕过 OIDC 的开发用本地账号（02 做什么第 3 条）。

## 完成判据

- 一条命令起得来演示发行方；发现文档与 `<issuer>/keys` 取得到。
- 干净库上 `seed.sh --reset` 全程零报错；操作者册里有按 Dex 的 issuer 加真 `sub` 登记的配置员与查阅员。
- 设了三件参数的 `parcel-api` 上，挑端点表里挂 `operatorRegistries.*` 的一个写口取证：配置员的 Dex 令牌答业务结果；查阅员的令牌答 403 `OPERATOR_NOT_GRANTED`；不带令牌答 401 `OPERATOR_CREDENTIAL_REJECTED`。取令牌与打口的步骤可重跑（脚本或 README 步骤），证据层级只记 `S`。
