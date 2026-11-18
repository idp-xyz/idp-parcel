# 仓配履约（`warehouse-fulfillment`）

对照 main `99fc0037d`（2026-11-12）写成；「现状」一节 2026-11-18 对照 `9eef5267e` 改过。

**这份文件不是现行规则。** 本上下文的业务没有实现，「现状」之外写的是设计意图与词，供要做时定位；那些句子都不能当作产品现在的行为。

## 现状

- 可选能力，首发不含。用户 2026-11-18 点名继续，按 PN-09 开发交接逐包做。
- main 上只有部署开关与依赖方向门禁，没有仓配业务：`internal/` 下没有 `warehousefulfillment` 目录，`cmd/` 下没有它的进程、端点或登记口，派发里没有它的路。
- 部署开关是环境变量 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED`，去首尾空白后恰为 `true` 才开，默认关。读法在 `cmd/internal/warehouseswitch`。`parcel-api` 与 `parcel-dispatch` 启动时各读一次并写启动日志。今天开与关的行为相同：还没有任何端点或路读它。
- 派发里仓配的路只经 `registerWarehouseFulfillmentRoutes` 登记，开关关着时一路都不登记。它今天没有调用方。
- 依赖方向门禁在 `internal/architecture/warehouse_fulfillment_dependency_gate_test.go`：别的上下文不得导入仓配，只留结算、计价、全程追踪各自的 `adapters/warehousefulfillment`。它不看测试文件，也不看库表与信封类型串。
- 在 `internal/` 与 `cmd/` 里搜 `warehouse`，除上面几处外，命中的是测试夹具取值与一条被拒的 `"target_kind":"WAREHOUSE"` 用例，与本上下文无关。搜「仓储」命中的是持久化仓储（repository）的意思。
- 文档写到开发交接：用例、客户面、接入词与 PN-09 交接都在 main 的 `docs/` 里。库内用例只评过一部分，没评到的记在票 08；出库用例评过一轮，修订记在票 09。票 10 至 12 的非作者评审没交卷。

入库、库内、出库、平台备货与代理仓各段的设计概览见 [scenarios.md](../scenarios.md)「仓配履约」。

## 设计意图：管什么，不管什么

- 管 3PL 仓配：货主的商品、库存账、入库、库内作业、出库、退货入库、代理仓，以及仓储作业的收费发生项。回答「哪个货主的哪种商品，在哪个仓哪个库位、什么状态、有多少、被谁占着」。
- 库存只记数量、位置与状态。不记成本与价值，不管货主的采购、销售订单与补货计划。
- 商品库存与包裹是两种身份：打包完成、提交委托之前只有库存与任务，之后只有包裹。委托、包裹身份、面单交易归 `parcel-shipment`；本上下文只以货主名义提交委托。
- 货主就是 `party-commercial` 的货主客户账户，不另立货主档案；不同货主的库存互不相通。
- 仓、库区、库位是本上下文的身份。仓不是 `network-routing` 的物流节点，库位不是 `node-operations` 的作业位置；同址时只互相指名，不合并。
- 库存账只经库存流水改变，不另存可改写的余额；账实不符交人处理，批准后以新流水改账。
- 收费发生项只给「发生了多少」，不带金额。评价归 `parcel-pricing`，费用与应收归 `settlement-accounting`。
- 商品申报资料原样带进委托，不判断归类与合规（归 `customs-compliance`）。保税仓不进产品。
- 仓内的中间事实不进客户全程追踪；只有经本产品网络发出的出库确认交给 `visibility-exception`。
- 租户可以不用它，部署可以不启用它；别的上下文的规则不以它为前提。仓由租户登记，不建默认仓。

## 词

| 词 | 意思 |
|---|---|
| 货主 | 一个货主客户账户。商品、库存、入库预报、出库请求、收费发生项都按它分 |
| 仓 | 租户登记的一处存储与作业场所，分自营仓与代理仓 |
| 代理仓 | 合作方经营的仓。只收它报来的库存声明，出库转给它执行，库存不进库存账 |
| 库位 | 库存账记位置的那一格，属于一个库区 |
| 商品 | 货主名下按商品编码识别的一种货，身份是（租户，货主，商品编码） |
| 库存账 | 按货主、仓、库位、商品、库存状态等维记数量的账；整箱库存另按箱号记 |
| 库存流水 | 改变库存账的唯一方式，带原因与来源事实引用 |
| 库存状态 | 良品、待检、次品 |
| 占用 | 对可分配库存的数量占用，三种：预占、冻结、作业锁定。只减可用量，不改实物总量 |
| 可用量 | 可分配库存减去各项占用，是算出来的数 |
| 入库预报 | 货主交来的到货通知 |
| 收货 | 仓清点后确认的实收事实，确认即入账 |
| 出库请求 | 货主要仓发出的货，带发运方式：本产品网络、自备面单、自提 |
| 仓配出库提交 | 发运方式为本产品网络时，以货主名义向 `parcel-shipment` 提交委托 |
| 出库确认 | 货离开仓、交给下一程那一刻由仓内作业确认的事实 |
| 仓储作业收费发生项 | 仓里做了一件可收费的事的记录，只带作业种类与计量，不带金额 |

## 别的上下文里已经为它留的缝

只发现一处，在货物明细的来源标记上：

- `internal/parcelshipment/domain/goods_details.go` 的 `DeclarationSourceMark` 是封闭三格。`SourceMarkBoxList`（`BOX_LIST`）与 `SourceMarkCommodityDeclaration`（`COMMODITY_DECLARATION`）两格是给仓配出库留的。现在客户渠道交来的一律是 `SourceMarkThisDeclaration`。
- `internal/customscompliance/ports/customer_goods_details.go` 的 `CustomerGoodsCellSource` 逐格对应：`CustomerGoodsCellFromBoxList`、`CustomerGoodsCellFromCommodityDeclaration`。转写在 `internal/customscompliance/adapters/parcelshipment/customer_goods_details.go`。
- 前端 `apps/admin-web/src/pages/shipment-request/goods-view.ts` 按这三格显示来源。

没有发现的：

- 委托来源里没有「仓配出库」这一种。
- `internal/parcelpricing` 与 `internal/settlementaccounting` 里没有仓储作业收费发生项、仓储计费方案或对应的评价对象。
- `internal/visibilityexception` 里没有 `warehouse-outbound-confirmed` 这个类型词。
- 小包托运不发「委托待受控补充」信封，派发里也没有它的收工路。

## 要做它时先看

详细的历史设计在 [旧 CONTEXT.md](../../docs/domain/warehouse-fulfillment/CONTEXT.md) 与 [旧用例](../../docs/application/warehouse-fulfillment/)。两处都是设计稿，不是现行规则。
