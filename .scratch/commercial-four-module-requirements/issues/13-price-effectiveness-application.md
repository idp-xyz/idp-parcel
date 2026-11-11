# 13 价格生效申请：把已接受的报价送进价卡与合同各自的批准发布流程，并如实答停在哪一件

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [10](./10-quotation-object-entry-and-approval.md)、票 [12](./12-customer-acceptance-of-a-quotation.md)
相关：票 [02](./02-price-card-preview-and-publication-show-overlap-with-registered-versions.md)（没有它，申请时看不到同范围已有在册版本）、票 [04](./04-price-card-registration-verifies-the-direction-authorization-it-cites.md)
地盘：`internal/partycommercial`（申请与读时派生）、`internal/partycommercial/adapters/parcelpricing`、`cmd/parcel-api`、`apps/admin-web/src/pages/party`

对应要求：Quotation「价格生效申请」。

## 现象

取证钉 `11bfa1ffb`。没有报价对象。一位客户用上一套新的销售价，今天要运营自己在几处分别做完，各处互不知道：

- 价卡草稿的批准与发布（`PublishPriceCardDraftHandler`）。
- 客户合同版本与销售方向商业价格政策版本的待批准发布。
- 结算账户、以及它登记的销售计价范围（[ADR-0280](../../../docs/adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md)）。

## 要做的

1. **申请由运营在报价被接受之后发起**，同一报价版本至多一份，可重放。
2. **申请不自己发布任何东西，也不立第二套批准。** 它做两件事：
   - 把报价所引的价卡草稿指给 `parcel-pricing` 现有的批准与发布流程，并回指报价。价卡审批职责规则与新规范化版本的部署开关照旧适用。
   - 为这位客户备好客户合同版本与销售方向商业价格政策版本的待批准发布，定价方案绑定指向这份方案；批准与发布照旧走 `party-commercial` 现有的流程。
3. **租户取值不代登。** 相对方还没有货主客户账户、结算账户，或结算账户还没登记销售计价范围时，申请只列出缺哪几件与各自的登记入口。
4. **「价格已生效」读时派生。** 上面各件都已发布并生效、登记也齐了才答已生效；否则答停在哪一件、等谁做什么。不另存一份会与各册不一致的状态。
5. **范围对得上。** 报价所引草稿的计价范围，必须与这位客户的结算账户登记的销售计价范围相同；不同就如实答，不改任何一边。
6. **重叠如实提示。** 这个范围上已有同时适用的在册销售卡时提示，收尾旧版照 [ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 由人做。
7. **管理台**：报价详情加「价格生效」一节，逐件显示到哪一步，并能跳到各自的页面去办。

## 要裁的

- 客户要求里的「申请」若本身要一道独立审批，与第 2 条相抵。这是 ADR 草案的越权风险点之一，由票 08 的评审定。
- 备好的合同与价格政策草稿，正文从哪里来：报价上的条款能填到哪几格，其余留给操作者填，不给默认。
- 所引价卡草稿若在申请之前已被别人直接批准发布，申请怎么答。

## 三类

申请与读时派生是机制。客户账户、结算账户与销售计价范围的登记是租户取值。

## 完成判据

- 演示租户上一份已接受的报价发起申请：价卡草稿进到待批准，合同与价格政策的待批准发布各出现一份，都回指这份报价。
- 各件依次批准发布之后，报价读出「价格已生效」；缺任一件时读出停在哪一件。
- 生效之后，这位客户的一件包裹照今天的规则按新卡形成销售评价；报价没有被任何评价读到。
- 申请重放不产生第二份草稿；有真库用例。
