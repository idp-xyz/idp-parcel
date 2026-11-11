# 17 演示：FBA 产品用自己的按票计费售价卡

Category: enhancement
Status: ready-for-agent
Blocked by: 14、15

依据：[ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md)；票 12 完成记录「价卡没另登」一条。

## 要做的

- **种子**：演示租户在 `SYN-SCOPE-01` 登一张指名 `SYN-PROD-CN-SG-FBA` 的 SELL·客户费用卡。
  - 卡按票计费：合成取值，票级聚合用今天已支持的形态。
  - 规范化版本 `PPC-7`，演示的部署开关已开。
  - 快递那张 `SYN-PLAN-CN-SG-01` 不动，照旧接其余产品。
- **演示段 `55`**：断言 FBA 货件的接受前估价与正式 SELL 评价用的是 FBA 卡，同一趟的快递小包用的是 `SYN-PLAN-CN-SG-01`。
- **README**：删掉「FBA 沿用快递卡」那几处，改写为两张卡各计各的价。

## 完成判据

`up.sh <库> --reset` 退 0；`55`、`50`、`10-main-chain.sh` 各退 0；读种子数据的测试包通过；证据记 `S`，参数登记册不动。
