# 15 横切：ADR-0100 操作者渠道落地——操作者册、凭据校验、`OperatorEnvelope` 与端点逐个换真 Intake

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 经用户授权自决立（票 02 遗留：开发主线「按四项判据重定级」表「横切」行第一项与票 05 格 7、11、12、22，全仓没有实施票）；体量大，开工第一步是拆子票
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
