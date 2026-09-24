# 15 横切：ADR-0100 操作者渠道落地——操作者册、凭据校验、`OperatorEnvelope` 与端点逐个换真 Intake

Category: enhancement
Status: in-progress——跟踪容器：2026-09-24 通道 4 按通道 1 派单 task-2a0f590d（/to-tickets）拆为 [`.scratch/operator-channel/`](../../operator-channel/issues/) 下子票 01–14：拆法经用户授权通道 4 自决认可（01–08 转 ready-for-agent），丙轨决策 09 落成 ADR-0149 并续编实施票 10–14，切片计划见文末。此前：拆法待用户认可（01–09 draft）；needs-triage——2026-09-24 通道 4 经用户授权自决立（票 02 遗留：开发主线「按四项判据重定级」表「横切」行第一项与票 05 格 7、11、12、22，全仓没有实施票）；体量大，开工第一步是拆子票
Blocked by: 无——ADR-0100 已接受，这一格不等任何决定
地盘：`internal/accessidentity`（操作者册、凭据校验、信封铸造）与其迁移、`cmd/parcel-api` 端点表的 Intake 装配；各上下文的授予格按[票 07](./07-pc-authorization-coordinates-and-role-models.md) 的角色模型读。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md)；[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表「横切」行第一项原话：`internal/accessidentity` 没有操作者册、OIDC 校验与 `OperatorEnvelope`，端点表的命令行与目录读口在隔离开关之外一律挂 `UnconfiguredIntake{}`；[票 05](./05-demo-journey-criterion-evidence.md) 格 7、11、12、22 与「不在主路径上的命令面」实测全部答 `403 ACCESS_CHANNEL_NOT_CONFIGURED`。开发主线结论句把它列为机制缺口的头一件，「它挡着全部运营面」。

## 做什么

1. 按 ADR-0100 落操作者册：操作者主体绑定唯一租户，授予按能力面显式登记、可撤销、带生效区间；册的结构由产品定，行是租户取值。
2. 凭据校验与 `OperatorEnvelope` 铸造，信任锚与校验方式照 ADR-0100；租户 SSO 是发行方侧的联邦配置，属租户取值。
3. 端点表逐端点把 `UnconfiguredIntake{}` 换成操作者 Intake：先主链命令面（节点收寄、场外揽收、交接、移动、派送、交付、段关闭、外部结果、凭证登记、外部资金事实），再 `/shipment-requests/` 下的运营命令、商业发布与计价回放。
4. 参数登记册按 ADR-0100 增「运营操作者账户与授予」一行（租户取值：某租户的操作者主体、绑定与授予）；今天登记册里还没有这一行。

## 不做

- 客户侧接入渠道（ADR-0139 至 0142，Proposed，接受与否归用户）。
- 不登任何真实操作者，不带任何租户的 SSO 配置；演示租户用合成主体，证据只记 `S`。

## 完成判据

- 隔离环境里，主链命令面对合成操作者答业务结果而不是 `ACCESS_CHANNEL_NOT_CONFIGURED`，未登记或授予不覆盖的主体照旧被拒（带测试）；票 05 格 7、11、12、22 各有实测去处；登记册那一行已增。

## 切片计划（2026-09-24，通道 4，派单 task-2a0f590d，取证钉 `20e4c64b`）

**先更正本票一处。**「做什么」第 3 条与完成判据把主链命令面（节点收寄、场外揽收、交接、移动、派送、交付、段关闭、外部结果、凭证登记、外部资金事实）写进了操作者渠道的覆盖面。ADR-0100 决定四只覆盖登记册配置写面（ADR-0085 决定一那一族）与主数据目录查阅面（ADR-0077 那一族）；决定五明写「不给客户业务命令面开操作者渠道」，决定四末段说作业事实登记与外部结果接收「照旧被两项未决拦着」。原句保留，按下面三轨读。

**取证。** `internal/accessidentity` 只有客户渠道那一套：渠道登记、`CredentialVerifier` 接口、`SourceEnvelope` / `SubmissionEnvelope` / `WithdrawalEnvelope` 与 `Minter`；没有操作者册、OIDC 校验与 `OperatorEnvelope`，`migrations/` 下也没有 `access_identity` 模块。`cmd/parcel-api/endpoints.go` 的写行，除隔离提交口与 `/commercial-*` 身份族（ADR-0091 逐口放行）外，都挂字面量 `UnconfiguredIntake{}`；读行在隔离读入参非 nil 时换隔离行（ADR-0078）。

**三轨。**

- **甲 · ADR-0100 本体（01–07）**：操作者册 01 与凭据校验 02 可并行 → 信封与三格答复 03 → 登记写面 04、目录查阅面 05 → 商业发布与计价回放 06；管理台登录门 07 在 02、03 之后。
- **乙 · 隔离形态的主链命令面（08）**：不是操作者渠道，照 ADR-0091 逐口放行合成写。「隔离环境里主链命令面答业务结果」最早从这一步到，不等甲轨。
- **丙 · 生产上主链业务命令面的真渠道（09）**：要一份新 ADR，归用户或其授权的 owner。

**子票与阻塞边。**

| 票 | 标题 | Blocked by |
|---|---|---|
| [01](../../operator-channel/issues/01-operator-register-and-migration.md) | 操作者册、迁移首个模块、受控登记口与参数登记册一行 | 无 |
| [02](../../operator-channel/issues/02-oidc-credential-verifier-and-deployment-parameters.md) | OIDC 令牌校验器与 parcel-api 部署参数 | 无 |
| [03](../../operator-channel/issues/03-operator-envelope-and-answer-algebra.md) | `OperatorEnvelope` 与铸造、三格答复代数 | 01、02 |
| [04](../../operator-channel/issues/04-registration-write-faces-take-operator-intake.md) | 登记册配置写面逐口换操作者 Intake | 03 |
| [05](../../operator-channel/issues/05-catalogue-read-faces-take-operator-intake.md) | 主数据与运营目录查阅面逐口换操作者 Intake | 03 |
| [06](../../operator-channel/issues/06-commercial-publication-and-replay-take-operator-identity.md) | 商业发布批准链与计价回放端点接上操作者身份 | 04 |
| [07](../../operator-channel/issues/07-admin-web-login-gate.md) | 管理台登录门 | 02、03 |
| [08](../../operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md) | 隔离形态：主链命令面按 ADR-0091 逐口放行合成写（乙轨） | 无 |
| [09](../../operator-channel/issues/09-decision-production-channel-for-business-command-faces.md) | 决策：主链业务命令面的生产渠道与 ADR-0055 决定五两项未决（丙轨） | 无（已 resolved，落成 ADR-0149） |
| [10](../../operator-channel/issues/10-device-register-and-operation-fact-capability-face.md) | 设备登记与「作业事实登记」能力面 | 01、03 |
| [11](../../operator-channel/issues/11-integration-client-register-and-client-credentials.md) | 集成客户端册与客户端凭据校验 | 02 |
| [12](../../operator-channel/issues/12-signed-webhook-inbound.md) | 签名 webhook 入向 | 11 |
| [13](../../operator-channel/issues/13-command-payload-canonicalization-per-face.md) | 各命令口的载荷规范化形状（逐口） | 无 |
| [14](../../operator-channel/issues/14-admission-scope-read-and-grade.md) | 准入范围读口与「不在准入范围」一格 | 无（接进铸造随 10、11） |
| [15](../../operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) | 「运营决定」能力面：委托侧五个运营决定口与 TF 管理台上的决定与判断口换操作者 Intake（2026-09-24 随 ADR-0151 补立，2026-09-25 复核改定并补两个判断口） | 03、14 |
| [16](../../operator-channel/issues/16-stale-par-int-01-comments-on-operator-faces.md) | 过期注释：运营侧接入面的认证不再写「属 `PAR-INT-01` 待提供」（2026-09-25 随 ADR-0151 补立；前半已入库，余量通道 4 接手） | 无 |

**前沿**：01、02、08、13、14。要最早让隔离环境里的主链命令面答业务结果，走 08；要最早让合成操作者在登记写面与目录查阅面上走通，走 01 + 02 → 03 → 04 / 05；生产上的主链命令面要 10 或 11（外加 12）与 13、14 都到位，按口逐步开。

**与 [psb/07](./07-pc-authorization-coordinates-and-role-models.md) 的关系：不阻。** 本目录的授予是通道级的能力面授予（登记册配置写、主数据与运营查阅读），列由 ADR-0100 定死；psb/07 的角色模型是 party-commercial 的业务授权（谁能拒绝、撤回、修订、批准）。06 接商业发布时只把信封译成 PC 的「操作者主体引用 + 授予集」，审批规则的形态 ADR-0126 已定，所以 06 不等 psb/07。反过来，psb/07 定角色模型时应读 01 的授予结构、对齐「授予格从哪里来」——那是它的输入，不是阻塞边。

**参数登记册「运营操作者账户与授予」一行**归 01：册的结构在那张票定死，行是租户取值。本轮不动登记册。

**认可**：子票先以 draft 发出；同日用户在 IDP 队列授权通道 4「参考专业头部软件的做法，你来帮我自决吧」，据此认可拆法，01–08 转 ready-for-agent（`docs/agents/issue-tracker.md`「Draft and activate children」）。丙轨 09 一并裁决：落成 [ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)——一线作业事实走操作者渠道族的「作业事实登记」能力面（人 + 已登记设备），外部结果与资金事实走集成客户端族（客户端凭据，推送源经签名 webhook），ADR-0055 决定五两项按口逐一解；实施续编 10–14，均 ready-for-agent。
