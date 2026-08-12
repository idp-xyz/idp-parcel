# ADR-0040: 商业版本身份键携带 TenantID，跨租户同号互不可见

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-014`：「跨租户引用参与方或规则 → 拒绝越界且不泄露另一租户内容。」

[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 把租户定为最高业务隔离边界。应用半边已在回指/重校验上把错租户与从未签发压成同形（与 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) / AT-PC-028 客户轴对称）。

对象半边此前缺轴：`commercialVersionKey` 只有 kind/objectID/version。两个租户可以合法共用同一业务对象号；登记册却会把第二次写入读成 `REPLAY`/`CONFLICT`，或让他租 `Lookup` 读到正文——都是泄露。

## Decision

**一、`CommercialVersion` / `CommercialVersionSpec` 携带 `TenantID`；`commercialVersionKey` 含租户。** `Register`、`Lookup`、选用候选与有效性更正键均按租户收窄。

**二、跨租户同 objectID+version 是两个独立身份。** 不得走 AT-PC-002 的 Replay/Conflict；对他租而言 `Lookup` ≡ 不存在，不泄露正文或冲突态。

**三、`ViewRevision` 按 (TenantID, Scope) 派生。** 另一租户写入不得推动本租户视图修订。

**四、与 AT-PC-028 客户轴分清。** 客户账户隔离仍在应用回指比对；本记录只把租户落到版本身份键。参与方/CustomerAccount 加租户字段另案（不做 F）。

**五、不把租户塞进 `CommercialScopeReference`。** 范围引用回答业务范围，不回答隔离边界。

## Consequences

- 构造草稿必须给出租户；夹具与 `Lookup` 签名升级。
- 完整 `AT-PC-014` 在应用半边 + 本对象半边齐备后可诚实挂 Covers。

## Alternatives considered

- **租户只在 Scope 字符串约定。** 否决：隔离靠约定会漏；且污染范围语义。
- **参与方同时加租户（F）。** 推迟：本票只关版本登记册键；参与方轴另开。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-014`
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)
