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

## 交给 07

2026-10-12 通道 4 写，钉分支 `mcp4-oc17` 的 `01784385`（发行方的容器描述与配置在 `scripts/demo-seeds/operator-channel/`）。下面每一格都是对 Dex v2.45.1 实签令牌与实际答复量出来的，证据层级 `S`。管理台怎么接、开发代理怎么配仍归 07。

- **发行方地址**：`http://127.0.0.1:5556/dex`（发现文档的 `issuer` 逐字如此，不带末尾斜杠）。
- **授权端点**：`http://127.0.0.1:5556/dex/auth`；**令牌端点**：`http://127.0.0.1:5556/dex/token`；**JWKS**：`http://127.0.0.1:5556/dex/keys`（RS256，一把签名钥）。
- **客户端标识**：`idp-parcel-admin-web`，公共客户端，无密钥。
- **回调地址**：`http://localhost:5173/auth/callback`，只登这一个、逐字匹配；`http://127.0.0.1:5173/auth/callback` 实测答 400「Unregistered redirect_uri.」。
- **`parcel-api` 三件参数**：`IDP_PARCEL_OPERATOR_OIDC_ISSUER=http://127.0.0.1:5556/dex`、`IDP_PARCEL_OPERATOR_OIDC_JWKS_URL=http://127.0.0.1:5556/dex/keys`、`IDP_PARCEL_OPERATOR_OIDC_AUDIENCE=idp-parcel-admin-web`。
- **该带哪一枚令牌：访问令牌。** Dex 签的访问令牌本身就是 RS256 JWT：`iss` 为上面那串，`aud` 为单串 `idp-parcel-admin-web`，`sub` 与 ID 令牌相同（两个合成用户的两枚令牌逐项对过）。管理台 `accessTokenForRequest` 今天带的就是它，不必换。有效期 24 小时（`expires_in` 86399）。
- **scope**：管理台现用的 `openid profile email offline_access` 在发现文档里全有。`offline_access` 换来刷新令牌：刷新答 200 并轮换，被轮换掉的旧刷新令牌再用答 400 `invalid_request`——`oidc.ts` 的 `inFlightRenewal` 把并发续期收成一次，防的正是这一格。
- **PKCE**：S256 实测——verifier 错答 400 `invalid_grant`（`Invalid code_verifier.`），带了 challenge 却不带 verifier 同样答 400。这一版 Dex 不强制 PKCE（不带 challenge 也放行，发现文档列 `S256` 与 `plain`），管理台每次都带 S256，不受影响。
- **CORS**：令牌端点对 `http://localhost:5173` 答 `Access-Control-Allow-Origin: http://localhost:5173`，对别的源不答。所以令牌端点可以直连；若保留 `/oidc` 代理，目标换成 `http://127.0.0.1:5556`，`TOKEN_URL` 随之为 `/oidc/dex/token`。
- **登出**：发现文档里没有 `end_session_endpoint`，与 `oidc.ts` 只清本地会话的做法相合。
- **合成操作者**（口令都是合成值 `syn-demo-password`）：配置员 `syn-operator-config-01@syn-tenant-01.example.invalid`，`sub` 为 `ChZTWU4tT1BFUkFUT1ItQ09ORklHLTAxEgVsb2NhbA`，授登记册配置写与主数据与运营查阅读；查阅员 `syn-operator-viewer-01@syn-tenant-01.example.invalid`，`sub` 为 `ChZTWU4tT1BFUkFUT1ItVklFV0VSLTAxEgVsb2NhbA`，只授查阅读。两人进操作者册随本票改过的种子（`seed.sh` 操作者册那段）。
- **起法**：在仓库根 `docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d --wait`（本票进 main 之前文件只在分支 `mcp4-oc17` 上）。Dex 的状态只在内存：重启后此前签出的令牌与刷新令牌全部作废，要重新登录。`parcel-api` 只在见到陌生 `kid` 时重取公钥集，且两次取至少隔一分钟（校验器的 `minRefetchInterval`）：Dex 重启时若 `parcel-api` 一分钟内刚取过钥，新令牌会先答 401，过了这一分钟再试即好。
- **别升 v2.46.0**：本地口令用户一刷新令牌它就把存储锁死，之后 `/keys` 与登录都不答（上游 dexidp/dex#4209，未修），`compose.yaml` 的镜像那行注释有记。
