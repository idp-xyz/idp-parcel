# 仓配履约（`warehouse-fulfillment`）

对照 main `99fc0037d`（2026-11-12）写成；「现状」与「已实现」两节 2026-11-19 对照 `2e2aabd7b` 改过。

**只有「现状」与「已实现：主数据登记册」两节是现行规则。** 其余写的是设计意图与词，供要做时定位；那些句子都不能当作产品现在的行为。

## 现状

- 可选能力，首发不含。用户 2026-11-18 点名继续，按 PN-09 开发交接逐包做。
- main 上有部署开关、依赖方向门禁，与仓、库区、库位类型、库位、包材各册的登记和查阅。别的都没有：商品、库存、入库、出库都没有代码；派发里没有仓配的路；没有受控批量口、登记 CLI、演示种子与管理台页面；货主的客户系统还查不了仓。
- 部署开关是环境变量 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED`，去首尾空白后恰为 `true` 才开，默认关。读法在 `cmd/internal/warehouseswitch`。`parcel-api` 与 `parcel-dispatch` 启动时各读一次并写启动日志。
- 开关关着时，仓配的端点答 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`：在认证之前答，不读令牌、报文与册，不带 `outcome`。它与「接入渠道未配置」是两格，因为恢复动作是部署方打开开关。方法不对仍答 `405`。
- 库表在 schema `warehouse_fulfillment`，进同一份迁移计划。开关关着的部署也建表，表是空的。
- 派发里仓配的路只经 `registerWarehouseFulfillmentRoutes` 登记，开关关着时一路都不登记。它今天没有调用方。
- 依赖方向门禁在 `internal/architecture/warehouse_fulfillment_dependency_gate_test.go`：别的上下文不得导入仓配，只留结算、计价、全程追踪各自的 `adapters/warehousefulfillment`。它不看测试文件，也不看库表与信封类型串。
- 搜「仓储」命中的是持久化仓储（repository）的意思，与本上下文无关。
- 文档写到开发交接：用例、客户面、接入词与 PN-09 交接都在 main 的 `docs/` 里。库内用例只评过一部分，没评到的记在票 08；出库用例评过一轮，修订记在票 09。票 10 至 12 的非作者评审没交卷。

入库、库内、出库、平台备货与代理仓各段的设计概览见 [scenarios.md](../scenarios.md)「仓配履约」。

## 已实现：主数据登记册

代码在 `internal/warehousefulfillment`。每册有登记、列表、修订史三个接口，都在操作员面：登记挂登记册配置写，列表与修订史挂主数据与运营查阅读。路径都以 `/warehouse-fulfillment-` 开头：

| 册 | 身份 | 登记 | 列表 | 修订史 |
|---|---|---|---|---|
| 仓 | （租户，仓编码） | `warehouse-registrations` | `warehouses` | `warehouse-revisions` |
| 库区 | （租户，仓编码，库区编码） | `zone-registrations` | `zones` | `zone-revisions` |
| 库位类型 | （租户，库位类型编码） | `location-type-registrations` | `location-types` | `location-type-revisions` |
| 库位 | （租户，仓编码，库位编码） | `location-registrations` | `locations` | `location-revisions` |
| 包材 | （租户，包材编码） | `packaging-material-registrations` | `packaging-materials` | `packaging-material-revisions` |

各册共同的规则：

- 每笔修订是一份完整内容。修订从 1 起连续，只追加。改资料、停用与再启用都是新修订。编码逐字保存，不折大小写、不去空白。
- 登记的答复：`REGISTERED`（201）、`ALREADY_REGISTERED`（同一修订号、内容相同，回显在册的那一笔）、`CONTENT_CONFLICT`（同一修订号、内容不同，在册的不变）、`NOT_ACCEPTED`、`NOT_CONFIGURED`。后四种都是 200。
- `NOT_ACCEPTED` 带原因：`REVISION_OUT_OF_SEQUENCE`（回显在册最新修订号，没有在册的是 0）、`IMMUTABLE_FIELD`、`CONTENT_INVALID`、`REFERENCE_NOT_FOUND`、`REFERENCE_DEACTIVATED`。后四种带 `field` 指出哪一格。
- 先判重放与冲突，再判别的。重放与冲突只比内容，不比登记责任方与登记时刻。
- 并发提交同一修订号的两笔，至多一笔落册，另一笔按内容答重放或冲突。靠库表主键守。
- 指名别的册的对象时只读不锁。各册之间没有外键。别的租户的对象一律按查不到答。
- 租户与登记责任方取自操作者信封。报文自带这两格、封闭集之外的词、必填格缺失或空白，都是坏载荷，答 `400`。
- 登记不判准入范围。持有登记册配置写的操作者能登记、也能停用任何一个仓，没有另开能力面。
- 查阅：当前修订的分页列表与一个对象的修订史。空册答空列表。本租户查不到的对象，修订史答空列表。
- 产品不带任何默认行。

各册自己的规则：

- **仓**。内容：名称、种类（`SELF_OPERATED` 自营仓或 `AGENT` 代理仓）、地址（国家码与正文）、业务时区、同址物流节点、是否停用。种类登记后不改。国家码不在内置目录、业务时区认不出，答 `CONTENT_INVALID`。业务时区不给就是没登记，不取默认值；认不认得出取决于运行环境有没有时区库。同址物流节点只是指名，不向 `network-routing` 核对。
- **库区**。内容：名称、用途、是否停用。用途是封闭六种：`RECEIVING_STAGING`、`STORAGE`、`PICKING`、`DEFECTIVE`、`RETURN_STAGING`、`OUTBOUND_STAGING`，登记后不改。仓须已登记，停用的仓也算。
- **库位类型**。内容：名称、是否停用。各仓共用。
- **包材**。内容：名称、外廓的长宽高与一个长度单位、重量与一个重量单位、是否停用。各仓共用。四个数值在报文里是字符串，只认 `12` 或 `12.50` 这种写法，整数部分至多 9 位、小数部分至多 6 位，按写出来的位数数；写法不对是坏载荷。数值为零、单位不在内置目录或类别不对，答 `CONTENT_INVALID`。数值按数值比，`12.5` 与 `12.50` 是同一内容。不折算单位，`30 CM` 与 `0.3 M` 是两份内容。
- **库位类型与包材**在本租户一个仓都没登记时答 `NOT_CONFIGURED`，不落册；登记仓之后原样重交照常受理。
- **库位**。内容：所属库区、库位类型（可缺）、优先级（可缺的 32 位整数，数字越大越优先）、是否停用。没有与有是不同的内容，优先级 0 与没有优先级是两份内容。所属库区登记后不改。仓、同一个仓下的库区、库位类型须已登记，否则 `REFERENCE_NOT_FOUND`。新指名的库位类型已停用答 `REFERENCE_DEACTIVATED`；沿用上一笔修订里的同一个类型、它后来被停用了，照常受理。不核仓与库区是否停用。
- 停用的库区与库位「不再作移入目标」还没有做：那要等入库与库内作业。

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
