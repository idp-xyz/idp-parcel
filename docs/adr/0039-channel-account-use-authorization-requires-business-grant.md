# ADR-0039: 渠道账号使用授权发布必须有显式业务授权，技术可用不能顶替

Status: Accepted  
Date: 2026-08-12

## Context

[UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md) `AT-PC-009`：「渠道账号技术可用但无业务授权 → 不发布账号使用授权。」

[CONTEXT](../domain/party-commercial/CONTEXT.md)：「取得账号凭据不等于取得业务使用授权」；「技术上能够访问账号不能替代该授权」；「渠道账号凭据和渠道接入的技术实现不属于本领域……不得把凭据等同于业务授权」。

今天领域只有 `ProductChannelMapping`（渠道产品候选）与接受侧 `AuthorityGrant`（人工复核／主动拒绝）。没有「渠道账号使用授权」类型，也就无法表达「技术可用但仍不发布」。

## Decision

**一、新增 `ChannelAccountUseAuthorization`。** 记录账号、授权双方、渠道产品、适用范围与有效期。它不是产品—渠道映射，也不是接受侧权限等级授权。

**二、发布入口同时询问技术存续与业务授权存续。** 业务授权未确认（含零值）→ `ErrChannelAccountBusinessUnauthorized`，**不**形成已发布的使用授权。技术可用不能单独让发布成功。

**三、字段不全与业务未授权分格。** 缺账号／双方／范围／期间 → `ErrInvalidChannelAccountUseAuthorization`；业务未授权是另一恢复动作（去取得持有人业务授权）。

**四、本记录不拥有凭据、不接真渠道适配器、不办 Outbox。** 机制半边到「能不能发布这条使用授权」为止。

## Consequences

- AT-PC-009 由红转绿。
- 调用方不得把「通道 ping 通」或映射候选当成使用授权已发布。

## Alternatives considered

- **复用 `AuthorityGrant`。** 否决：那是接受／主动拒绝的商业权限等级，不是渠道账号持有人的使用许可。
- **只在映射上加布尔。** 否决：映射回答候选渠道产品，不回答「这个账号此刻许不许用」。

## Links

- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-009`
- [CONTEXT 渠道账号使用授权](../domain/party-commercial/CONTEXT.md)
