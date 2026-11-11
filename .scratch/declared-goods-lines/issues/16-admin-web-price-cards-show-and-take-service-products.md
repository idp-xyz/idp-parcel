# 16 管理台：价卡录入与查阅带适用服务产品，试算可选产品

Category: enhancement
Status: ready-for-agent
Blocked by: 无（字段名在票 14、15 里钉死，可并行；联调等 14、15）

依据：[ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定一、二。

## 要做的

- **价卡编辑器**（`card-editor` 的基础一步与预览）：
  - 方向为 `SELL` 时多一格「适用服务产品」，可多选、可不选；候选取参与方与商业的服务产品目录。
  - 不选即方案文档不带 `serviceProducts` 键；方向不是 `SELL` 时这一格不出现、也不发。
  - 说明写「不选即接其余产品：同一计价范围里没有别的卡指名的那些产品」。
  - 导入模板（workbook）若有基础信息页，同样加这一列，可空。
- **价卡列表与详情**：
  - `SELL` 卡显示适用产品：列出指名的产品；不声明显示「其余产品（不指名）」。
  - `BUY`、`INTERNAL` 卡不显示这一格。
  - 导入面的内容预览同样显示。
- **试算页**：
  - 多一格可选的服务产品，发 `serviceProduct`。
  - 不选时说明「只按不指名产品的卡试算」。

## 完成判据

`tsc`、`run-tests` 与 `vite build` 三道门通过；表单与显示的纯函数各有单测（带与不带产品、方向切换时丢弃这一格）。

## 不做的

后端（票 14、15）。
