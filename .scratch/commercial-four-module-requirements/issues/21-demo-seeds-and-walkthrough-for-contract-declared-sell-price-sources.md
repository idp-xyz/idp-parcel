# 21 演示租户的种子合同声明销售价格来源，演示动线走通「专属价没谈到的线路落到标准价」

Category: enhancement
Status: draft——由票 [15](./15-one-authority-for-which-price-a-customer-uses-and-the-customer-price-structure.md) 拆出；[ADR-0380](../../../docs/adr/0380-the-customer-contract-version-declares-ordered-sell-price-sources-and-settlement-hands-them-to-pricing-from-the-fixed-resolution.md) 进 `main` 之后由用户激活
父票：[spec](../spec.md)
Blocked by: 票 [19](./19-settlement-requests-sell-evaluations-with-the-contract-declared-scope-list.md)、票 [20](./20-admin-web-contract-page-declares-sell-price-sources.md)
地盘：`scripts/demo-seeds`

对应要求：Contract、Pricing、Rating 三行在演示租户上的证据。证据记 `S`。

## 要做的

1. **种子合同声明自己的列表。** `SYN-TENANT-01` 的客户合同发新规范化版本，带销售价格来源。至少两位客户：一位只用一个范围；一位把自己的专属范围排在前面、公共范围排在后面。全部用 `SYN-` 取值。
2. **专属范围的卡只写谈下来的线路，但绑解得出全部线路分区的目录。** 目录解不出分区答的是目录未决，停在专属范围上、不回退（ADR-0380 越权风险点 4）；没谈的线路只在运费表里不写。一件走专属线路的包裹按专属卡计费；一件走没谈到的线路的包裹落到后一个范围，客户费用的解释写得出用的是哪一个。
3. **采用册的种子**：新形状合同的账户不再登计价范围；留一位旧规范化版本合同的客户，证明旧路照走。可行性先对着迁移与种子脚本核（ADR-0380「起草能力边界」写明没核）。
4. **受理前估价与预估客户费用同额**，在两位客户上各验一次。

## 要裁的

- 给一位已登记的客户换列表，要发新一版合同，而解析键上的锚点是登记下来的一个时刻（ADR-0380 越权风险点 2）。演示动线里「老客户接受专属价之后生效」这一步，在 `product-strategy-boundary/06` 解冻之前走不通；种子只能从开户起就把专属范围排在前面，卡以后再发。要不要解冻是用户的决定。

## 三类

种子是随产品发布的参考配置；每个取值是演示租户自己的，不是产品默认。

## 完成判据

- 干净库上重灌种子成功；上面第 2、4 条在灌完的库上各有一条可重复的核对。
- 管理台合同页上看得到两位客户各自的列表与每个来源此刻有没有在用的卡。

## Comments

- 2026-11-12：由票 15 拆出。
