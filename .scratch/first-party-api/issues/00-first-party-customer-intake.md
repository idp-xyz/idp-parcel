# 首方客户渠道 Intake：客户渠道册、首方铸造器、提交口先换并撤写开关

Category: enhancement
Status: ready-for-agent——2026-10-19 立。ADR-0139–0142 于同日由用户在 Claude Code 会话里答「接受」，按复核建议修订后接受。
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
