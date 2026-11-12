# 货物明细与专线型 FBA 头程：实施

Category: enhancement
Status: in-progress

依据：[ADR-0373](../../docs/adr/0373-a-shipment-requests-goods-details-are-invoice-lines-with-a-packing-allocation-kept-verbatim-and-replaced-as-a-whole.md)（委托的货物明细）、[ADR-0374](../../docs/adr/0374-dedicated-line-fba-headhaul-is-an-ordinary-shipment-request-one-platform-shipment-one-request-one-carton-one-parcel.md)（专线型 FBA 头程是普通委托）；决定过程见票 [01](./issues/01-decide-goods-details-order-references-and-fba-headhaul.md)。

用户 2026-11-10 原话「问题是fba我们现在就要实现的」。这是用户点名的新能力，不受[上线线冻结清单](../launch-line-freeze-2026-10-27.md)约束（冻结清单管的是 10-27 时的存量票池）。

## 目标

客户经首方 API、运营经管理台，能下一份专线型 FBA 货件：一个平台货件一份委托、一箱一个包裹，委托带货物明细行与平台货件号，每箱带平台箱标号与装箱分配；能查、能改货物明细，关务能按行取用。国际小包同样用上多行货物明细。

## 不在本轮

- 仓配平台备货整份提交（ADR-0374 决定五）：`internal/` 下还没有仓配履约的代码，随仓配实现切片做，届时另立票。
- 货物特性与危险品、送仓预约、进口商、平台收货差异：两份 ADR 已划在边界外。

## 子票

| 票 | 内容 | Blocked by | 通道 |
|---|---|---|---|
| [01](./issues/01-decide-goods-details-order-references-and-fba-headhaul.md) | 文档：两份 ADR 与 CONTEXT/UC 改动 | — | 文档票，走「进 main 的步骤」 |
| [02](./issues/02-goods-details-and-platform-identifiers-domain-model.md) | 领域：货物明细、装箱分配、平台号声明，`PSC-1` 同号加可缺键 | 01（用户点名越过） | 后端 |
| [03](./issues/03-submit-carries-goods-details-and-platform-identifiers.md) | 提交链路：首方 API 收、落库、重放与冲突、接受基线保留 | 02 | 后端 |
| [04](./issues/04-shipment-views-show-goods-details-and-platform-identifiers.md) | 查阅：委托视图返回货物明细与平台号；管理台详情与号码界面词 | 03 | 后端 + 管理台 |
| [05](./issues/05-admin-web-fba-shipment-and-multi-line-goods-entry.md) | 管理台：FBA 货件下单页，小包下单改多行货物 | 03 | 前端（main 上直接做） |
| [06](./issues/06-amend-goods-details-as-a-shipment-level-data-group.md) | 资料修订：委托级资料组「货物明细」 | 03 | 后端 |
| [07](./issues/07-customs-reads-goods-lines-by-anchor-shipment-and-line.md) | 关务读口：按（资料版本锚，委托，行号）取行 | 03 | 后端 |
| [08](./issues/08-demo-seeds-walk-a-synthetic-fba-shipment.md) | 演示种子：`SYN-TENANT-01` 走通一票 FBA 货件 | 03、05 | 种子与脚本 |
| [09](./issues/09-cross-field-condition-executor-for-acceptance-rule-packages.md) | 接单规则包的必填与跨字段条件执行器（`BD-PS-006`），随它发布 FBA 头程提交条件参考配置 | — （ADR-0376 已定） | 后端；票 08 补出 |
| [10](./issues/10-controlled-supplement-carries-platform-identifiers-before-acceptance.md) | 受控补充收平台号，受理前补报或更正 | — | 后端 |
| [11](./issues/11-customer-contract-need-not-name-an-acceptance-rule-package.md) | 客户合同不必指名接单规则包 | — | 后端 + 管理台 |
| [12](./issues/12-demo-fba-product-stops-for-missing-box-labels-and-resumes-on-supplement.md) | 演示：FBA 产品缺箱标号停下，补齐后受理 | 09、10、11 | 种子与脚本 |
| [13](./issues/13-one-account-prices-each-service-product-on-its-own-card.md) | 同一账户的不同服务产品各用各的售价卡：决定（ADR-0378） | 14–17 | 文档票 |
| [14](./issues/14-a-sell-price-card-names-its-service-products-and-selection-has-two-tiers.md) | 售价卡声明适用服务产品，选卡分两层 | — | 后端 |
| [15](./issues/15-the-declared-service-product-reaches-sell-card-selection.md) | 申报的服务产品进售价卡选卡：估价、正式评价与试算 | 14 | 后端 |
| [16](./issues/16-admin-web-price-cards-show-and-take-service-products.md) | 管理台：价卡录入与查阅带适用产品，试算可选产品 | 14（模板一格；可并行写） | 前端 |
| [17](./issues/17-demo-fba-product-is-priced-on-its-own-card.md) | 演示：FBA 产品用自己的售价卡（逐箱计价） | 14、15 | 种子与脚本 |
| [18](./issues/18-the-pre-acceptance-estimate-trail-is-readable.md) | 受理前估价的留痕可读：决定（ADR-0379） | 19–21 | 文档票 |
| [19](./issues/19-the-estimate-trail-records-product-weight-and-lines-and-settlement-reads-it-by-shipment.md) | 估价留痕补记产品、计价重量与费用行，结算按委托交出读口 | — | 后端 |
| [20](./issues/20-shipment-detail-shows-the-recorded-financial-control-conclusion.md) | 委托详情交出受理判断记下的财务控制结论 | — | 后端 |
| [21](./issues/21-admin-shipment-detail-shows-pre-acceptance-financial-control.md) | 管理台运单详情显示「受理前财务控制」 | —（联调等 19、20） | 前端 |

03 定下 API 形状之后，04、05、06、07 可以并行。
