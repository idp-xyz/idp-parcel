# ADR-0178：客户合同这一册不收只有壳的版本；客户服务规则的壳指名客户合同时正文必须挂同一份合同；合同正文的货主客户账户是开放引用、不核在册

Status: Accepted（2026-10-12，用户经 IDP 队列通道 2 授权「请你帮我自决」；通道 2 起草，裁定依据见票 contract-tier-binding-followups/01「分诊」）
Date: 2026-10-12

## Context

票 customer-service-rule-contract-applicability/04、07 的事后非作者评审（两票 Comments「评审 ← 通道 2」）留下三处没人核的缺口，记在票
[contract-tier-binding-followups/01](../../.scratch/contract-tier-binding-followups/issues/01-unbound-contracts-and-contract-shell-with-product-body-pass-unchecked.md)：

- [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md) 决定二让闭包按键上的客户账户收窄客户合同候选，
  形状照 [ADR-0153](./0153-declared-service-product-identity-narrows-commercial-resolution.md)：没登账户的合同照旧参选。落得到这一格的有两种——经受控
  批文只发壳、没有正文的客户合同，与迁移 0037 之前写下的正文行。于是「自己没有合同的那一户」会唯一解到一份归属不明的合同，连带采纳挂在它上面的
  合同版客户服务规则与钉住它的规则包，即跨户串读。ADR-0153 里没指名产品的成员确实对范围内各产品都适用；客户合同在 ADR-0177 决定一之后账户必填，
  没登账户说的是「归属不明」，不是「对各户都适用」。
- 迁移 0037 头注写「账户在不在册由写入用例把门」；这一列唯一的写入路径是发布用例，它没有参与方身份读口，没有一处核账户在册。
- `ConsistentCustomerServiceRuleApplicability` 对「壳只指名了另一类」放行，于是壳指名客户合同、正文却挂服务产品的客户服务规则版本，发布前与点读后
  都过得去。[ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 之后「壳指名合同」就是合同层，这一形态自相矛盾。

管理台与 HTTP 发布载荷已要求客户合同正文在场（缺即点名 `customerContract.contractContent`）；只有壳的客户合同只可能出自受控批文。今天没有任何
租户的生产数据（与 ADR-0177 决定三同一前提）。

## Decision

**一、客户合同这一册不收只有壳的版本。** 发布用例对缺合同正文的客户合同版本答`未受理`，一行不写。这道门排在声明构造之后：挂错拥有者、重复键、
指名引用未发布、内容冲突这些输入问题先答各自的格；排在写入之前，所以一行不写。答`未受理`而不是错误，是因为恢复动作是改批文、补正文，与对账门
同一格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 按恢复动作分格）；答成错误，批文工具会把它读成技术失败、
叫运维重试。理由：货主客户账户只在正文里（ADR-0177 决定一），只有壳的合同说不出属于哪一户，违背 PC `CONTEXT`「合同版本必须明确关联…货主客户
账户」；收下它，就只能在选法里替它决定「归属不明算谁的」。

这是对 [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定二
「没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串」一句在客户合同一册上的**部分停用**。**适用场景**：只限客户合同这一册；其余各册
的壳照旧可以单独发布，ADR-0126 其余各条不变。

**选法不改。** ADR-0177 决定二照旧。发布口堵住之后，没登账户的合同只剩迁移 0037 之前的旧行；它们按 ADR-0177 决定五随 `seed.sh --reset` 重灌
消失，今天没有生产库需要迁移。

**二、客户服务规则版本的壳指名客户合同时，正文必须挂同一份合同。** `ConsistentCustomerServiceRuleApplicability` 不再对「壳指名合同而正文挂产品」
放行，发布写入前与点读之后两处同时生效（与 [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)
决定四是同一道核）。反向——壳只指名服务产品而正文挂合同——不在本记录：那一版归产品层（票 customer-service-rule-contract-applicability/02 判断项 1），
正文挂的合同由消费侧对到闭包采用的那一份（ADR-0176 决定四，票 04 判断项 3）。

**三、合同正文的货主客户账户是开放引用，不核在册。** 与 PC 发布正文里其余参与方引用同一纪律：客户服务规则的责任方、结算政策的客户相对方，发布
用例都不核在不在册。悬空的账户让这份合同对任何键都不参选，本该用它的那一户答`无适用依据`，不串户。要核，就得给发布用例接参与方身份读口，并让
发布排在身份登记之后（演示种子今天先发布、后登参与方），那是另一件事。迁移 0037 头注「账户在不在册由写入用例把门」一句由本条更正；迁移按字节
校验，文件不改。

## Consequences

- 落地位（票 contract-tier-binding-followups/01）：`internal/partycommercial/application` 的发布用例在声明构造之后加客户合同缺正文那道门；
  `internal/partycommercial/domain` 的 `ConsistentCustomerServiceRuleApplicability` 收紧；postgres 适配器写账户与装载账户两处注释说清不核在册与
  旧行；发布用例测试与 `cmd/parcel-commercial` 批文夹具里只有壳的客户合同补上正文，摘要按正文重算。
- 受控批文里只有壳的客户合同从此答`未受理`（退出码 2，要人改批文），而不是`已发布`。演示种子两份合同都带正文，不受影响。
- 三分类：三条都是机制，不涉租户取值。

## Alternatives considered

- **选法收窄：没登账户的合同对谁都不参选。** 否决于代价。在 `b9a01048` 上试改实测：受影响包里红 102 个顶层用例，横跨 PC、PS、VE 三个上下文与
  `cmd/parcel-api`、`cmd/parcel-commercial`——只有壳的合同在测试夹具里随处可见。而它比决定一多防的只有 0037 之前的旧行，那些行没有生产数据。
  同一基点上，决定一的门按本记录的位置实施，红 9 个顶层用例，集中在发布用例与批文两个包。
- **维持现状，在 ADR-0177 补记跨户那一面的代价。** 否决：受控批文一笔只有壳的合同，就足以让同范围没有合同的户采纳别户的条款，而堵住它只要一道门。
- **在 VE 补壳核（票 04 作者自审的提法）。** 否决：矛盾出在 PC 的版本本身，在提供方的一致性核上拒，发布时就拦下，比让每个消费方各补一道更对位。

## Links

- 票 [contract-tier-binding-followups/01](../../.scratch/contract-tier-binding-followups/issues/01-unbound-contracts-and-contract-shell-with-product-body-pass-unchecked.md)：三处缺口、分诊裁定与实施记录
- [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md)：决定一（账户必填）、决定二（选法收窄，本记录不改）、决定五（旧行重灌）
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：决定二在客户合同一册上被本记录决定一停用
- [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md)：壳指名合同即合同层；决定四的消费侧点核
- [ADR-0153](./0153-declared-service-product-identity-narrows-commercial-resolution.md)：没指名产品的成员照旧参选——本记录说明它为何不搬给客户合同的发布面
- [ADR-0104](./0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)：客户服务规则壳与正文一致性那道核的两处位置
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [PC CONTEXT](../domain/party-commercial/CONTEXT.md)：「合同版本必须明确关联…货主客户账户」
