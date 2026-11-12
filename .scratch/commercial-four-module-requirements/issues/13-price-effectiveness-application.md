# 13 价格生效申请：接受之后发起现有的发布并记下依据，报价读面如实答生效到了哪一步

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 票 [12](./12-customer-acceptance-of-a-quotation.md)；票 [19](./19-settlement-requests-sell-evaluations-with-the-contract-declared-scope-list.md)（完成判据里「这位客户的一件包裹按新卡形成销售评价」要结算读合同的列表）；票 [16](./16-the-customer-contract-body-carries-the-sell-price-arrangement.md)（「这位客户用这套价」那一步是客户合同版本的销售价格来源，见 [ADR-0380](../../../docs/adr/0380-the-customer-contract-version-declares-ordered-sell-price-sources-and-settlement-hands-them-to-pricing-from-the-fixed-resolution.md)）
相关：票 [02](./02-price-card-preview-and-publication-show-overlap-with-registered-versions.md)（没有它，发布时看不到同范围已有在册版本）
地盘：至少有 `internal/partycommercial`（读时派生）、`internal/partycommercial/adapters/parcelpricing`、`internal/parcelpricing`（发布记下依据）、`apps/admin-web/src/pages/party`

对应要求：Quotation「价格生效申请」。

## 现象

取证钉 `535202926`。一位客户用上一套新的销售价，今天要运营在两处分别做完，两处互不知道：

- 价卡草稿的发布（`PublishPriceCardDraftHandler`）。发布命令只带租户与方案版本，记不下它是因为哪一条接受而发的（`PublishPriceCardDraftCommand`）。
- 结算账户登记销售计价范围。登了之后换不了：[ADR-0280](../../../docs/adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md) 决定二写「要换范围，与今天一样没有改口的路」。已经共用一个计价范围的客户接受了专属价，今天没有办法让它对他生效。

合同的价格政策上另有一处定价方案绑定，它不决定选卡（[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 决定一）。

## 要做的

下面按 ADR 草案第二稿列。落在哪一处见 Comments 里 2026-11-12 讲 ADR-0380 的那一条。

1. **不新立申请对象，不新立批准。** 接受之后，所引的若是草稿，由运营对它发起现有的发布；所引的已是在册价卡时没有这一步。
2. **发布记下依据**：发布多一格可缺的引用，指名那条接受。
3. **「这位客户用这套价」那一步照票 15 的结论做。** 本票不为它新立登记，也不代登租户取值。
4. **读时派生**：报价详情如实答生效到了哪一步——所引的价格内容是否已在册，这位客户是否已经用上；停在哪一步、等谁做什么。不另存一份会与各册不一致的状态。
5. **一道由租户登记的门**：有发出记录的草稿，发布要不要以一条有效的接受为前提。没登记不设门，读面如实答「发布以接受为前提：未配置」。
6. **重叠如实提示**（票 02）；收尾旧版照 [ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 由人做。
7. **管理台**：报价详情加「价格生效」一节，逐步显示到了哪里，并能跳到各自的页面去办。

## 要裁的

- 客户要求里的「申请」若本身要一道独立审批，与第 1 条相抵。这是 ADR 草案的越权风险点之一，由票 08 的评审定。
- 第 5 条的门没登记时不设门，与审批职责规则「没登记不放行」方向相反。理由是今天没有任何规则要求发布以接受为前提，没登记就拦会让现有的价卡发布全部停住。它算不算红线里要拒绝的默认值，由票 08 的评审定；票 [03](./03-signing-evidence-on-contract-and-agreement-versions.md) 的签署证据门是同一个问题，两处要一个口径（ADR 草案越权风险点里讲这道门的那一条）。
- 第 5 条的门放在哪一处。放在计价的发布口，计价要读参与方与商业才判得了；放在参与方与商业一侧，要由它去发起发布。ADR-0380 没有定，归票 08 的评审。
- 所引的价格内容被别人直接发布、没有带依据时，报价读面怎么答。

## 三类

读时派生与发布记依据是机制。第 5 条的门开不开，以及客户账户、结算账户与计价范围的登记，是租户取值。

## 完成判据

- 演示租户上一条已接受的报价：所引草稿发布之后，报价读出价格内容已在册；客户用上这套价之后读出已生效；缺任一步时读出停在哪一步。
- 发布记录里看得到作为依据的那条接受。
- 生效之后，这位客户的一件包裹照计费时的规则按新卡形成销售评价；报价没有被任何评价读到。
- 没登记那道门的租户，照今天的办法发布不受影响。

## Comments

- 2026-11-11：随 ADR 草案第二稿改。原稿立一个申请对象，把价卡草稿、合同版本与价格政策版本分别送进各自的流程。合同上的绑定与结算账户上的登记由哪一处说了算还没定（票 15），不该先在上面搭一层。改为落在现有的发布上，并加对票 15 的阻塞。
- 2026-11-12（非作者评审后）：评审指出第 5 条「没登记不设门」与红线「租户取值拒绝默认值」相抵，票 03 把同一处写进了「要裁的」而本票没写。补上读面答未配置，并把方向相反这一点写进「要裁的」。
- 2026-11-12：票 15 的结论是 ADR-0380，阻塞边改指它拆出的票 16。照它读本票：「要做的」第 3 条那一步，是客户合同版本的销售价格来源指名价卡所在的计价范围；合同已经指名这个范围时，生效只差价卡的发布，还没有指名时要发新一版合同。第 4 条「这位客户是否已经用上」读的是：他的委托采用到的那一版合同的来源列表里有没有这个范围，这个范围此刻有没有在用的卡。第 5 条的门放在哪一个发布上，ADR-0380 没有定，仍归票 08 的评审。「现象」里引的 ADR-0280 决定二那一句，对带销售价格约定的合同版本已由 ADR-0380 停用；但给一位已登记的客户换来源列表，在 `product-strategy-boundary/06` 解冻之前仍然走不通（ADR-0380 越权风险点 2），「老客户接受专属价之后生效」这条完成判据受它限制。
