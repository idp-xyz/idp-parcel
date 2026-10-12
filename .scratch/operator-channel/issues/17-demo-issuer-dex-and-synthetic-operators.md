# 17 演示发行方：Dex 接进演示环境，合成操作者按它的标识另登

Category: enhancement
Status: 完工，待评审与重放——2026-10-12 通道 4：分支 `mcp4-oc17`（基 `00bc85ce`），代码与脚本 tip `968fe8fb`，完成记录见文末。此前 in-progress——2026-10-12 通道 4 认领（通道 1 派单 `task-605064ea`，接替一直未接的 `task-be560cd8`），隔离 worktree `idp-parcel-mcp4-oc17`、分支 `mcp4-oc17`（基 `00bc85ce`）。此前 ready-for-agent——2026-10-12 通道 1 补立：[02](./02-oidc-credential-verifier-and-deployment-parameters.md) 完成记录写「那一步随 07 做」，而 [07](./07-admin-web-login-gate.md) 的地盘只有 `apps/admin-web/**`，这一步于是没有票认领；[04](./04-registration-write-faces-take-operator-intake.md) 第 1 件、[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口、[10](./10-device-register-and-operation-fact-capability-face.md) 换口那半与 07 的完成判据都在等它
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

## Comments

### 完成记录 ← 通道 4 · 2026-10-12 · 分支 `mcp4-oc17`（待评审与重放）

**逐笔落点**（分支上的 SHA）：`1c5c4653` 认领；`01784385` Dex 容器描述、配置与取令牌脚本（`scripts/demo-seeds/operator-channel/` 的 `compose.yaml`、`dex.yaml`、`operator-token.sh`）；`4c9dc789` 票面「交给 07」；`7eff7028` 操作者册种子（`data/access/` 两份）与 `seed.sh` 操作者册那段；`9c851538` 取证脚本 `probe-operator-channel.sh` 与 README「操作者渠道」一节；`968fe8fb` 两个脚本入库为可执行。`968fe8fb` 同时更正 `9c851538` 提交信里「入库为可执行」那句：带 pathspec 的提交按工作树重新暂存这些路径，而本工作树 `core.fileMode=false`，模式取了 HEAD 的 `100644`，没进那一笔。本记录随其后一笔，只改票面。

**完成判据逐条**（证据层级都是 `S`）：

- ✅ **一条命令起得来演示发行方；发现文档与 `<issuer>/keys` 取得到。** `docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d --wait` 答 Healthy（健康检查打 `/dex/keys`）。发现文档的 `issuer` 为 `http://127.0.0.1:5556/dex`、`jwks_uri` 为 `<issuer>/keys`；`/keys` 上一把 RSA 签名钥，`alg` RS256。WSL 里与 Windows 侧（`curl.exe`）取到的是同一份发现文档。
- ✅ **干净库上 `seed.sh --reset` 全程零报错；操作者册里有按 Dex 的 issuer 加真 `sub` 登记的配置员与查阅员。** 55432 上的一次性库 `idp_mcp4_oc17_seed`：干净库上 `--reset`、再来一次 `--reset`，都退 0，日志里 error / fail / panic 零行。`access_identity.operator` 两行，即两个 Dex 主体，都绑 `SYN-TENANT-01`；`operator_grant` 三行；发行方为合成值的行数 0。tip `968fe8fb` 的干净检出上又用一次性库 `idp_mcp4_oc17_final` 重跑一遍，同样退 0。两只库都已删。两个 `sub` 读自两枚真令牌（`operator-token.sh … claims`），v2.46.0 与 v2.45.1 读出的是同一串。
- ✅ **设了三件参数的 `parcel-api` 上三格取证。** 写口 `POST /visibility-catalogue-notification-policy-registrations`（挂 `operatorRegistries.visibility`），脚本 `probe-operator-channel.sh`：配置员答 200 `REFUSED` / `VERSION_NOT_OVERWRITABLE`，查阅员答 403 `OPERATOR_NOT_GRANTED`，不带令牌答 401 `OPERATOR_CREDENTIAL_REJECTED`。工作树上连跑两遍、tip 的干净检出上再跑一遍，三次一致。反证：同一只库上起一个不设三件参数的 `parcel-api`，配置员那一枪答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，脚本退 1。票面之外另证：配置员登一条新策略 `SYN-VE-NOTIFY-OPERATOR-PROBE-01` 答 201 `REGISTERED`，库里那一行的 `tenant_id` 是 `SYN-TENANT-01`，同一条再登答 200 `VERSION_NOT_OVERWRITABLE`，查阅员登同一条答 403；批文带 `tenantId` 答 400 `MALFORMED_REQUEST`；签名末位改一个字符答 401。

**判断项**（留给评审）：

1. **Dex 不进仓库根的 `compose.yaml`，另起 `scripts/demo-seeds/operator-channel/compose.yaml`。** 根那份只为 PostgreSQL 集成门禁准备，`docker compose up -d` 起门禁库时不该顺带拉起发行方；演示发行方签出的主体登在演示种子里，跟着种子走。根那份的头注因此一字未动——做什么第 1 条「同笔改它的头注」以接进根那份为前提。
2. **镜像钉 v2.45.1，不钉最新的 v2.46.0。** v2.46.0 上本地口令用户一换刷新令牌就把存储锁死，此后 `/keys` 与新登录都不答；健康检查若打发现文档，容器还显示 healthy（发现文档不碰存储）。内存与 SQLite 两种存储都实测挂死。上游 [dexidp/dex#4209](https://github.com/dexidp/dex/issues/4209) 仍是 open，评论指 v2.46.0 的刷新重构把会话读挪进了更新事务。v2.45.1 内存存储实测：刷新答 200 并轮换，旧刷新令牌再用答 400，整套走完 `/keys` 仍答 200。健康检查因此改打 `/keys`。
3. **v2.45.1 的两样代价都如实写进配置注释与 README。** 它没有强制 PKCE 的开关（`oauth2.pkce` 是 v2.46.0 才有的键）：不带 `code_challenge` 的授权码流也放行，带了才核 verifier，错或缺都答 400 `invalid_grant`。它也没有拒收未知配置键的开关（v2.46.0 的 `DEX_CONFIG_DISALLOW_UNKNOWN_FIELDS`）：拼错的键会被静静忽略。这两样我在 v2.46.0 那一版里配过，换版本时删掉了，没有留着假装生效。
4. **合成发行方那批不再登。** 没有哪个发行方签得出 `https://syn-issuer-01.example.invalid` 这个 `iss`，那两个主体在册上与能登录的主体长得一样，却永远登不进来。仓里没有别处读这两份批文：引用那串的几处 Go 测试各用自己的常量。已灌过的库上它们留到下一次 `--reset`（README「已知边界」里「早先灌过的库上」那条）。新一批的绑定依据与授予号另起，旧库上不带 `--reset` 补登也不撞旧行。
5. **授权类型只开 `authorization_code` 与 `refresh_token`。** 不开 password：那等于让客户端自己收口令。取令牌脚本因此走与管理台相同的 authorization_code + PKCE 流（登录表单由脚本提交），换来的令牌与管理台拿到的同形。
6. **取证写口与批文的选法。** 原样重登种子里那条通知策略，答「版本不可覆盖」：这一格只有令牌解析出的租户下已经有这条策略才答得出，所以它同时证了租户来自操作者册的绑定；脚本也因此不往库里落新行，可以对演示库反复跑。201 那一格另在一次性库上手打取证（见上）。
7. **发行方地址写 `127.0.0.1`，不写服务名或 `localhost`。** 服务名在容器网络外解析不出；Dex 只发布在 IPv4 回环上，写死 `127.0.0.1` 就不依赖 `localhost` 先解析到哪一族地址。

**交给 07**：取值见上面「交给 07」一节，2026-10-12 20:3x 已发通道 5、同文抄通道 1；种子落地的 `7eff7028` 随后另报两边，附了不 `--reset` 共享演示库时只补登这一批的两条命令。

**验证**：只动了配置、种子、脚本与文档——`git diff --name-status 00bc85ce..968fe8fb` 只有本票面、README、两份种子、`seed.sh` 与 `operator-channel/` 下四份新文件，没有 `.go` 或 `.sql`。tip `968fe8fb` 的干净检出上照 CI「Mechanism inventory is current」那一步重生成机制清点：`docs/product/MECHANISM-INVENTORY.md` 零差，整树 `git status` 零行。没跑 `go build` / `go vet` / `go test` 全仓（无 Go 改动）；`seed.sh` 在干净检出上自己编了全部登记 CLI 与迁移助手，`parcel-api` 也从干净检出编过。

**未验与留意**：

- 管理台的真浏览器登录没走，归 07；Windows 侧只验了发现文档取得到。
- 此刻在跑的演示发行方容器起自本工作树：它挂载的是本树的 `dex.yaml`（与 `968fe8fb` 上那份 sha256 相同）。拆本树之前要从主树重起它，否则下一次容器重启找不到配置。

**评审**：尚无非作者评审，作者未自评。spec 指本票面，基线 `00bc85ce`，代码与脚本 tip `968fe8fb`。
