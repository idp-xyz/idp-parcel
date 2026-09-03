# ADR-0072：接入渠道与凭据验证归共享接入身份能力，登记册形状等真渠道证据

Status: 已接受（2026-08-21，MCP-3 受用户委托裁断。**部分停用**，2026-09-03：Decision 2 中「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。Decision 1、3 与其余各条不变）

## Context

墙/门审计票 01（W01/W02，接入渠道登记册与首个真渠道 Intake）被 MCP-6 复核（`0ec62ea`）判为不可按现状开工，阻断三条：

1. 票面第 1 件（预拟渠道登记表的列）正是 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) Alternatives 明文否决的方案——「渠道配置的形状取决于真实渠道是 API、标准文件还是门户，替它拟表就是替租户拟 `PAR-INT-01` 的样子」；解否决的前件「真渠道就位」不成立（`PAR-INT-01` 在参数登记册为「待提供」，本仓尚无租户）。
2. 凭据验证按 CONTEXT 硬句不归业务上下文：parcel-shipment CONTEXT 写明「共享身份认证与授权技术能力拥有凭据验证、通用授权策略和授权作用域签发」，PS 只消费已授权作用域；party-commercial CONTEXT 写明「渠道账号凭据和渠道接入的技术实现不属于本领域文档的决策范围」。而这个被点名的共享能力在本仓既无上下文目录也无迁移目录——它没有落点。
3. [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 的「结构先行、内容等参数」不可照搬：0068 成立靠的是结构半边由 CONTEXT 硬句定死；渠道登记册恰相反，没有任何 CONTEXT 为其形状背书，形状本身取决于渠道类型——那是一份实例证据，不是一个未填的值。

三条合起来是一件事：这份能力**归谁**是机制半边、现在就能裁；这份登记册**长什么样**是实例半边、必须等证据。此前两半被并在同一张票里，谁也动不了。

## Decision

1. **所有权**：接入渠道登记册、凭据验证、来源信封铸造（对 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 三级边界的入口断言）归一个**共享接入身份技术能力**，落点定为 `internal/accessidentity/`。它是技术能力而非业务限界上下文：不进 CONTEXT-MAP 业务地图，不拥有业务领域语言；它是 PS CONTEXT 已点名那个「共享身份认证与授权技术能力」的实现位。五个业务上下文一律只消费已铸造的来源信封与已授权作用域——此为既有 CONTEXT 硬句的落实，不是新边界。
2. **ADR-0055 的否决维持**：登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立。届时按实际渠道形状在 `internal/accessidentity/` 立册；替换点仍是 `assembleBusinessEndpoints` 逐端点换，路由层与处理器不动（ADR-0055 已预留）。
3. **载荷规范化摘要与本决定解耦**：PS 侧摘要的进出边界已由 PS CONTEXT 定死（信封元数据不进摘要、`requestEffectiveAt` 及其缺失状态进摘要），属机制半边，独立成票现在就做，按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 带规范化形状版本号；不等渠道，不被本 ADR 阻塞。

## Consequences

- 审计票 01 转 `needs-info`，重启条件 = `PAR-INT-01` 最低证据到位；W01/W02 记「按票裁定显式留待」——机制半边无事可做不是停滞，是这两堵墙诚实的当前形态。
- 审计票 12（治理登记进程入口）分辨表第三行所等的「那张新 ADR」即本文；其端点路照旧等实例证据，受控 CLI 路不受本 ADR 影响（治理登记是否属业务端点面由该票自裁）。
- 租户出现后的第一笔渠道工作有了明确顺序：按渠道形状立册 → 凭据验证与信封铸造 → 在装配点逐端点替换 Intake；全程不动业务上下文。
- 本 ADR 不设计登记册的任何列——那正是被维持的否决所禁止的。

## Alternatives

- **归 `internal/platform/`**：platform 承载无业务判断的技术件（outbox、httpapi、dispatch）；凭据验证与渠道适用范围携带租户隔离语义（ADR-0003），混入会让「技术件不判业务」的边界失守。否决。
- **分摊进五个业务上下文各自实现**：正面撞 PS/PC CONTEXT 硬句与「所有权清晰」红线。否决。
- **现在就按假想渠道形状立册（结构先行）**：ADR-0068 的前件不成立，等于替租户拟 `PAR-INT-01` 的样子。否决——维持 ADR-0055 原判。

## Links

- 确认维持：ADR-0055（其 Alternatives 中对运行时渠道登记表的否决）
- 部分停用本记录：[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)——Decision 2 的适用场景收窄为客户接入渠道；操作者渠道那一半在 Decision 1 划定的 `internal/accessidentity` 落点上立第一份生产实现
- 相关：ADR-0003、ADR-0014、ADR-0068
- 来源：`.scratch/syn-wall-door-audit/issues/01`（MCP-6 复核与三条阻断）、`.scratch/syn-wall-door-audit/issues/12`（ADR-0017 分辨表）
