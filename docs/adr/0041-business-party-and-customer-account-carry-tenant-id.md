# ADR-0041: 业务参与方与货主客户账户携带 TenantID

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) 要求货主客户账户独立保存租户与客户参与方等字段。[ADR-0040](./0040-commercial-version-key-carries-tenant-id.md) 已把租户落到商业版本身份键；参与方与账户此前仍无租户轴，跨租户可以把账户钉到他租参与方上。

## Decision

**一、`BusinessParty` 与 `CustomerAccount` 构造时必须携带 `TenantID`。** 身份仍是 PartyID / CustomerAccountID；租户是隔离边界，不是角色分类。

**二、账户绑定的客户参与方必须同租户。** 跨租户绑定拒绝，不建立账户。

**三、不把 Caller 探测或 AT-PC-028 客户轴塞进账户构造。** 本记录只定对象半边的租户必填与同租户绑定。

## Consequences

- `NewBusinessParty` / `NewCustomerAccount` 签名升级；夹具跟进。
- 与 ADR-0040 并列完成 AT-PC-014 在参与方/账户上的最小对象半边。

## Alternatives considered

- **只给账户加租户、参与方不加。** 否决：账户钉参与方时仍无法证明参与方属于本租户。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)
- [ADR-0040](./0040-commercial-version-key-carries-tenant-id.md)
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)
