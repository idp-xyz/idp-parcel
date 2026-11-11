# 12 客户接受报价：运营代录带证据，已有账户的客户经首方渠道自己接受

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [10](./10-quotation-object-entry-and-approval.md)
地盘：`internal/partycommercial`、`cmd/parcel-api`（运营命令面与首方客户面）、`apps/admin-web/src/pages/party`

对应要求：Quotation「客户接受」。

## 现象

取证钉 `11bfa1ffb`：没有报价对象，也就没有接受。首方客户渠道已经有补充、撤回、修订、取消、索赔这些客户命令面，账户取自凭据（[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)）。

## 要做的

1. **接受是相对方的决定。** 登记的是谁接受、何时、接受的是哪一版报价的内容摘要。
2. **入口一，运营代录**：必须带接受证据引用，即客户书面确认在受限证据库里的索引。登录操作人记为代录人，不是决定方。
3. **入口二，首方客户渠道命令**：只对已经是货主客户账户的相对方开；账户取自凭据，载荷里自报账户即拒；别的账户的报价与不存在的报价同答，不泄露。
4. **门**：报价版本是已发出；接受时刻不晚于有效期终点；调用方带的内容摘要与报价上记的一致。过期的不收，答已过期。
5. **拒绝同样登记**，原因可缺。
6. **同一报价至多一个版本被接受**；重放答原结果。
7. **不做**：接受之后自动生效。生效归票 [13](./13-price-effectiveness-application.md)。

## 要裁的

- **谁可以代录。** 是一条租户取值，还是授权规则里的一个新动作。草案借的是 [ADR-0116](../../../docs/adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 的分法，适不适用由票 08 的评审定。
- **有效期终点按哪个业务时区。**
- **接受证据的形状**是否与票 [03](./03-signing-evidence-on-contract-and-agreement-versions.md) 的签署证据共用。
- **已接受之后还能不能撤回。** 建议不能；要改就发新版本重新接受。

## 三类

入口与门是机制。谁可以代录是租户取值。

## 完成判据

- 运营代录不带证据引用不收；带了的接受成立并记下代录人。
- 首方渠道上，本账户的已发出报价能接受；别的账户的与不存在的同答。
- 过期的、未发出的、摘要不符的各答各的一格；重放答原结果。
- 证据文件不进产品库，只存索引。
