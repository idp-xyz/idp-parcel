# ADR-0093: 渠道账号使用授权不是商业版本，走自己的修订式登记册

Status: Accepted  
Date: 2026-09-02

## Context

[ADR-0039](0039-channel-account-use-authorization-requires-business-grant.md) 建了 `ChannelAccountUseAuthorization` 类型并守住「技术可用不顶替业务授权」，但它第四条明说「本记录不拥有凭据、不接真渠道适配器、不办 Outbox」。此后这个类型在 `internal/` 下除了自己的测试再无引用：没有表、没有端口、没有接入面。于是 CONTEXT 那几条硬句**没有执行器**——一个租户无法登记一笔授权，册上不可能有行，`parcel-shipment` 那侧「委托接受时固定渠道账号使用授权」只能恒为「不适用」或恒为未配置，而 CONTEXT 要的是显式记录「不适用」及原因。缺席不是判断。

产品侧已裁：首发**支持**客户自有渠道账号由运营企业代下单，因此要补执行器。

补的时候撞上一个前置问题。`.scratch/party-commercial-context-gaps/issues/` 下的票 01 与票 04 都把它写成「某某进不进 `CommercialObjectKind` 封闭集」，于是可选项只剩「新增一个 kind」与「复用 `AuthorizationRuleObject`」——两者都默认它是一个商业版本。

**它不是。** `CommercialObjectKind` 的定义是「遵循商业版本共同不变量的对象封闭集合」，而遵循那套不变量的表现是持有一个 `CommercialVersion`：抽验 `AuthorityGrant`、`CreditPolicy`、`SupplierAgreement` 三个集内成员，均以 `version CommercialVersion` 字段开头（实测于 `0c4b5a1`；此处的数是论点本身——若哪天出现一个不带 version 的集内成员，本 ADR 的依据就该重审，所以它必须锚住取证时的 SHA）。`ChannelAccountUseAuthorization` 不带 version，自带 `status` 与 `publishedAt`，即自己的生命周期。

封闭集自己的头注释已经把这种形状排除过一次，理由与此处一字同源：货主客户账户与责任法人刻意不在其中，因为它们走自己的生命周期，不是商业版本。渠道账号使用授权与被排除的那两个同族。

**但「持不持有 `CommercialVersion`」是症状，不是判据本身。** 它可以被「那就给它加一个字段」绕过去，而那样绕过之后集合的语义已经变了却没有任何东西报警。真正在分的是**生命周期是不是版本演进**：商业版本走草稿 → 已发布 → 已生效 → 已到期或已替代，一份新版本取代旧版本而旧版本留在册上；渠道账号使用授权走建立 → 撤销或自然到期，没有「第二版授权取代第一版」这回事——换了范围或期限是**另一笔**授权，换了被授权人更是。持有 `CommercialVersion` 只是前一种生命周期在代码里的落法。

（另需说清一处不成立的旁证：CONTEXT 里共用生命周期那句列举没有列渠道账号使用授权，但它同样没列服务产品、客户合同与供应商协议，而这三个都在集内。所以那句列举分不出族，不能拿来当依据。）

## Decision

**一、不进 `CommercialObjectKind`。** 渠道账号使用授权走自己的登记册，形状照参与方身份四表（`migrations/party_commercial/0015_party_identity_registry.sql`）的修订式登记：键为租户 + 登记标识 + 修订，修订从 1 起连续递增，绝不 `UPDATE`。

连带否掉了拓宽封闭集所需的那份新迁移（`commercial_version` 的 `object_kind` CHECK 上界），以及它跨 `valid()` 上界、`String()` 与枚举穷尽门禁的那次难逆转改动。

**二、登记标识不是渠道账号标识。** 同一个渠道账号可以在不同期间授给不同的被授权人，那是两笔授权而不是一笔的两个修订。拿账号当登记键会把它们挤成一条链，最新修订于是覆盖掉一段仍需追溯的历史。

**三、撤销是状态取值，自然到期不是。** 撤销没有任何东西可供导出——它是一次外部决定，必须记下来；自然到期由有效区间对时点导出，存成状态就是存一份会过期的推导。两者后果相同而要人做的动作相反（前者去重新取得授权，后者去续期），因此必须在类型上就分得开：`RevokedAt` 的第二个返回值是那道分界，到期的授权在那里报假。

**四、撤销自其自身时点起生效，不回溯。** 判据同 `SupplierAgreement.SupportsProcurementAt`。整段失效的写法会让事后复核把一笔撤销之前正当形成的交易判成当时就无授权，直接违反 CONTEXT 的「已经形成的交易仍保留当时有效的授权依据」。

**五、撤销时刻与撤销依据同在或同缺。** 只记时刻答不出凭什么撤，只记依据答不出从哪一刻起不能用——而后续新使用的判定要的正是那一刻。

**六、后继修订不得改换账号或授权双方。** 这条 SQL 守不住：主键只守键唯一，一条修订二把关系整个换掉照样入库，而它在册上仍像同一笔授权的后继，下游按登记标识取最新修订会取到一份从没有人授权过的关系。追加因此成了偷换的伪装。真要授给另一方或另一个账号，那是另一笔授权，另起登记标识。

## Consequences

- 票 01 与票 04 共同的前提（「这是个待归类的商业版本」）作废。票 04 的客户服务规则版本须按同一条判据重问一次，而**它很可能得出相反的答案**：草稿 → 已发布 → 已生效正是版本演进那一套，它大概率确实是商业版本。判据相同不等于结论相同——这正是为什么要问，而不是照本 ADR 类推。
- 本上下文自此有两族登记面：商业版本族（走发布登记册与商业解析）与自有生命周期族（参与方身份四表、产品—渠道映射、本册）。新增对象先判归哪一族，**判据是生命周期是不是版本演进**（有没有「新版本取代旧版本、旧版本留在册上」这回事），持不持有 `CommercialVersion` 只是它在代码里的表现，不要拿表现当判据——那样只需加个字段就能把语义改掉而无人报警。
- 代价是这一族不进商业解析。需要按解析取回授权的消费方得走本册自己的读面，不能指望 `resolve_commercial_basis` 那条路。这是与 ADR-0039 第四条一致的收窄，不是新增限制。
- `PublishChannelAccountUseAuthorization` 仍在生产接线棘轮基线上，直到本册的端口与适配器落地。

## Alternatives considered

- **新增一个独立 kind 进封闭集。** 曾被采纳，随后按上述取证推翻：它会让一个不持有 `CommercialVersion` 的对象混进以持有它为前提的集合，商业解析按 kind 去找正文时会落空，而落空的形态是「查无此行」——与「这一类确实没登记」不可分辨。
- **复用 `AuthorizationRuleObject`。** 否决，理由同 ADR-0039 对复用 `AuthorityGrant` 的否决：那是运营企业责任法人的授权治理，不是渠道账号持有人的使用许可。
- **给它补上 `CommercialVersion` 以便进集。** 否决：那是为了归类而改变对象本身。它的生命周期（发布、撤销）与商业版本的（草稿、已发布、已生效、已到期或已替代）不同型，硬套会多出几个永远走不到的状态。
- **不补，明确首发只支持运营企业自有账号。** 产品侧已否决：跨境小包里客户自有渠道账号代下单是主流商业形态。若他日重议，连带要改的是 CONTEXT 的 Language 词条、Rules 三条硬句、生命周期小节、Boundaries 给 `parcel-shipment` 与 `customs-compliance` 的两句，以及 ADR-0039 的适用性。

## Links

- [ADR-0039](0039-channel-account-use-authorization-requires-business-grant.md)：本 ADR 补它留下的执行器缺口，不改它任何一条决定
- [CONTEXT 渠道账号使用授权](../domain/party-commercial/CONTEXT.md)
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-009`
