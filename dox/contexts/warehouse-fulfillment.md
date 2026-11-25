# 仓配履约（`warehouse-fulfillment`）

主数据登记册、商品（货主口与代录）、仓测尺寸重量、库存账、入库预报、收货确认、整箱库存、完结与少收、收货差异结论、待认领转记均按已进 main 的代码写（2026-11-20 入库一段及商品货主口/仓测各经一轮非作者评审并清零阻断后进 main）。

**「现状」、各「已实现」节，以及「别的上下文里已经为它留的缝」里写明代码里已有的句子，是现行规则。** 其余写的是设计意图与词，供要做时定位；那些句子都不能当作产品现在的行为。

## 现状

- 可选能力，首发不含。交接里点名的包已在 main。部署默认关。
- 已有代码：部署开关与依赖方向门禁；仓、库区、库位类型、库位、包材的登记和查阅；商品的货主口、运营代录、货主与运营查阅；仓测尺寸重量（按商品包装层级与按在库一箱，作业事实登记与货主/运营查阅）；库存账；入库预报的受理、告知版本、撤回、单据号、运营代录、运营查阅与货主口；收货确认（商品入库与整箱入库，支持引用已认领待认领到货）；整箱库存；完结与少收（整箱按箱）；收货差异结论（货主与代录）；待认领到货完整链路（一线直接登记、取消/完结收货转来、认领与撤销、处置规则修订册、按规则处置与运营查阅），以及下面各「已实现」节。路径、词与判定在那些节。
- 管理台顶栏支持在「国际小包」与「仓配系统」工作区之间切换；开关关着时顶栏不摆这个切换，仓配地址上说明本部署没有启用（管理台读仓库列表探一次，明确答 `WAREHOUSE_FULFILLMENT_NOT_ENABLED` 才收起，答不出照旧摆，共享的扫描设备与操作者授予不受影响）。仓配系统按岗位分七组十七页：总览（仓配工作台、仓库监控）、入库（入库预报、待认领到货、退货管理）、出库（出库单、波次与拣货、代理仓履约）、作业台（仓内作业台）、库存（实时库存、盘点与调整、加工与换标、建账导入）、计费（仓储计费）、基础资料（仓库与库位、货主与商品、仓配规则设置，另复用扫描设备与操作者授予）；操作通过上下文弹窗抽屉加实时级联名册下拉与自动单号完成，出库单、波次、入库预报、商品、建账导入、待认领到货、盘点、库存调整、作业差异、加工与换标指令、库存冻结、库内作业、退货预报与退货到仓各是有自己地址的单据页（代理仓履约的台账仍是弹窗），出库支持多选合并放波次，实时库存五视图均为结构化数据表，仓内作业台按收货/拣货车/复核打包/出库/库内五工位队列驱动全部 18 个作业事实口；拣货、复核打包与收货另有不带外壳的全屏工位 `#/station/pick`、`#/station/pack`、`#/station/receive`（选工位页 `#/station`），拣货逐格扫库位与商品、扫满按拣货口提交，复核打包逐件扫货、扫齐按复核口提交，再选包材称重、一笔提交整张单的各个包裹，收货扫预报后逐件或逐箱提交这一次收货，发货交接 `#/station/dispatch` 对已打包的单逐个包裹交出，上架与移库 `#/station/putaway` 先选货主再扫源库位、商品和目标库位，盘点清点 `#/station/count` 按库位录入实点，其余十种作业在 `#/station/more`，仓内作业台页只负责进入这些工位。仓配工作台是七张作业待处理卡加按等得最久排的异常行。波次、出库单、入库预报与盘点单可按现有读口打印。本产品网络已打包的包裹可打模拟的 USPS 面单，页上写明不能当邮资，也不充当出库确认所缺的面单本体。仓库监控在总览：左边只列有列表读口的对象，右边用和出库单同一套表，状态是徽章，仓和货主用名册下拉，点「拣货任务」或「这一格」往下看。目录有下一页就翻。库存格与整箱这一口不分页。演示种子（`495-access-warehouse-fulfillment.sh` 与 `910-warehouse-fulfillment.sh`）补齐演示租户全部仓配设备/授予/准入对照/权威区间、五册主数据、八项规则、商品、库存、入库预报、待认领、退货预报与到仓、进行中的盘点、待批库存调整与库内短少差异、待执行换标指令、全生命周期出库波次、一张已打包的本产品网络出库单、代理仓声明及仓储计费，全部取值只用 `SYN-`。
- 部署开关是环境变量 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED`，去首尾空白后恰为 `true` 才开，默认关。读法在 `cmd/internal/warehouseswitch`。`parcel-api` 与 `parcel-dispatch` 启动时各读一次并写启动日志。
- 开关关着时，仓配的端点（含 `/v1/warehouse/`）答 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`：在认证之前答，不读令牌、报文与册，不带 `outcome`。它与「接入渠道未配置」是两格，因为恢复动作是部署方打开开关。方法不对仍答 `405`。下面各口都受这一条管，不逐口再写。
- 库表在 schema `warehouse_fulfillment`，进同一份迁移计划。开关关着的部署也建表，表是空的。
- 派发里仓配的路只经 `registerWarehouseFulfillmentRoutes` 登记，开关关着时一路都不登记。开着时 `cmd/parcel-dispatch` 的 init 登记收费发生项消费者。
- 依赖方向门禁在 `internal/architecture/warehouse_fulfillment_dependency_gate_test.go`：别的上下文不得导入仓配，只留结算、计价、全程追踪各自的 `adapters/warehousefulfillment`。它不看测试文件，也不看库表与信封类型串。
- 搜「仓储」命中的是持久化仓储（repository）的意思，与本上下文无关。
- 用例、客户面、接入词与 PN-09 交接在 `docs/`，是历史设计稿。规则以本节与下面各「已实现」为准。

入库、库内、出库、平台备货与代理仓各段的设计概览见 [scenarios.md](../scenarios.md)「仓配履约」。

## 已实现：主数据登记册

代码在 `internal/warehousefulfillment`。每册有登记、列表、修订史三个接口，都在操作员面：登记挂登记册配置写，列表与修订史挂主数据与运营查阅读。仓的当前修订另有货主列表。路径都以 `/warehouse-fulfillment-` 开头：

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
- 登记不判准入范围。持有登记册配置写的操作者能登记、也能停用任何一个仓，没有另开能力面。五册的受控批量口 `cmd/parcel-warehouse-register` 与管理台「仓库与库位」调同一套登记器；批量口一行一笔事务，已经落上的行重跑答 `ALREADY_REGISTERED`，开关关着整批拒绝，不是运营口。
- 查阅：当前修订的分页列表与一个对象的修订史。空册答空列表。本租户查不到的对象，修订史答空列表。
- 产品不带任何默认行。

各册自己的规则：

- **仓**。内容：名称、种类（`SELF_OPERATED` 自营仓或 `AGENT` 代理仓）、地址（国家码与正文）、业务时区、同址物流节点、是否停用。种类登记后不改。国家码不在内置目录、业务时区认不出，答 `CONTENT_INVALID`。业务时区不给就是没登记，不取默认值；认不认得出取决于运行环境有没有时区库。同址物流节点只是指名，不向 `network-routing` 核对。货主列表 `GET /v1/warehouse/reads/warehouses` 挂 `WAREHOUSE_READ`，读同一本仓册的当前修订，不带登记责任方；查询串自带租户、货主或登记责任方是坏查询。空册答空数组。
- **库区**。内容：名称、用途、是否停用。用途是封闭六种：`RECEIVING_STAGING`、`STORAGE`、`PICKING`、`DEFECTIVE`、`RETURN_STAGING`、`OUTBOUND_STAGING`，登记后不改。仓须已登记，停用的仓也算。
- **库位类型**。内容：名称、是否停用。各仓共用。
- **包材**。内容：名称、外廓的长宽高与一个长度单位、重量与一个重量单位、是否停用。各仓共用。四个数值在报文里是字符串，只认 `12` 或 `12.50` 这种写法，整数部分至多 9 位、小数部分至多 6 位，按写出来的位数数；写法不对是坏载荷。数值为零、单位不在内置目录或类别不对，答 `CONTENT_INVALID`。数值按数值比，`12.5` 与 `12.50` 是同一内容。不折算单位，`30 CM` 与 `0.3 M` 是两份内容。
- **库位类型与包材**在本租户一个仓都没登记时答 `NOT_CONFIGURED`，不落册；登记仓之后原样重交照常受理。
- **库位**。内容：所属库区、库位类型（可缺）、优先级（可缺的 32 位整数，数字越大越优先）、是否停用。没有与有是不同的内容，优先级 0 与没有优先级是两份内容。所属库区登记后不改。仓、同一个仓下的库区、库位类型须已登记，否则 `REFERENCE_NOT_FOUND`。新指名的库位类型已停用答 `REFERENCE_DEACTIVATED`；沿用上一笔修订里的同一个类型、它后来被停用了，照常受理。不核仓与库区是否停用。
- 停用的库区与库位不作移入目标。收货确认、整箱收货、库内移入、加工产出入库、开箱、拣货暂存、回库、退货入库、调增与建账导入碰到停用的库位或库区时不入账；从停用库位移出仍可以。停用的仓另按各口自己的规则。

## 已实现：商品与仓测

身份是（租户，货主客户账户，商品编码）。每笔修订是一份完整资料，从 1 起只追加；货主口与代录口写同一条修订序列。代码在 `internal/warehousefulfillment`。

- 运营代录：`POST /warehouse-fulfillment-goods-on-behalf`，挂运营决定「商品代录」（`GOODS_RECORD_ON_BEHALF`），准入两格都是这个字面量；货主客户账户在本租户查不到答 `404 OWNER_ACCOUNT_NOT_VISIBLE`，不带 `outcome`。货主写口：`POST /v1/warehouse/goods`，挂首方客户能力 `GOODS_MAINTENANCE`，准入两格都是这个字面量，来源请求键前缀 `goods:`；租户与货主取自信封，自带即坏载荷。
- 商品答复：`REGISTERED`（201），其余 200：`EXISTING_RESULT`、`SOURCE_CONFLICT`、`BASIS_STALE`、`IMMUTABLE_FIELD`、`IDENTIFIER_CONFLICT`、`CONTENT_INVALID`、`NOT_CONFIGURED`。判定次序：代录先核货主账户；然后本租户无仓答 `NOT_CONFIGURED`；按判重位置认重放与冲突；核基准、不可改的格、识别码、资料内容。
- 商品查阅：运营口挂主数据与运营查阅读（`GET /warehouse-fulfillment-goods`、`goods-current`、`goods-revisions`、`goods-by-code`）；货主口挂 `WAREHOUSE_READ`（`GET /v1/warehouse/reads/goods`、`goods-current`、`goods-revisions`、`goods-by-code`），不带登记责任方，也不带仓测的设备与操作者。找不到不答 404，别的货主商品同查无此商品。商品当前修订与列表逐包装层级附 `warehouseMeasurements`：已仓测给当前版，未仓测答 `measured: false` 并附货主声明来源 `OWNER_DECLARED`（没声明则为空，不给零或默认）。
- 仓测登记：`POST /warehouse-fulfillment-warehouse-measurements`，挂作业事实登记（范围种类 `WAREHOUSE`，引用须等于载荷仓；准入能力词 `WORK_FACT_REGISTRATION`，事实类型 `WAREHOUSE_MEASUREMENT`）。对象二选一：`GOODS_LEVEL`（商品 + 包装层级，按落册时当前修订核）或 `CARTON`（在库一箱，按到达服务端那一刻整箱账是否在库核）。答复：`RECORDED`（201），其余 200：`EXISTING_RESULT`、`CONTENT_CONFLICT`、`WAREHOUSE_NOT_FOUND`、`WAREHOUSE_NOT_ACCEPTING`（原因 `AGENT`；停用自营仓照记）、`NOT_ACCEPTED`（逐格指出不合）。只追加版本；当前版按发生时刻与设备序号定序，迟到的旧仓测照记不顶掉更新的一版。
- 仓测查阅：运营口 `GET /warehouse-fulfillment-goods-measurements`、`carton-measurements`；货主口 `GET /v1/warehouse/reads/goods-measurements`、`carton-measurements`（`WAREHOUSE_READ`，不带设备与操作者）。并发：商品修订对同一货主取事务级独占锁，记数取共享一侧；仓测按（租户，事实身份）取锁。没有：商品停用、审核、多级组合。

## 已实现：库存账

一格是（租户，货主，仓，库位，商品，计量单位，库存状态，贴标别名，入库批次，批号，效期）。余额不存，由流水现算。没有自己的端点。商品入库的收货确认调用记数用例。整箱不进这本账。

- 答复：`POSTED`、`ALREADY_POSTED`、`SOURCE_CONFLICT`、`REFUSED`。判重位置是（租户，原因，来源事实引用），重放排在校验之前。
- 判定：先锁再认重放。原因是开箱时答 `REFUSED`，种类 `REASON_NOT_SUPPORTED`。仓须已登记且是自营仓，停用的仓不拒。行的形状随原因的类。逐行收齐不合之处，整笔不落。取走之后余额不为负。收货、退货入库、建账导入的记入行，以及加工、拆解的产出行，在记数时形成入库批次。建账导入提交是 `POST /warehouse-fulfillment-stock-import-submissions`，查阅是 `GET /warehouse-fulfillment-stock-imports` 与 `stock-import`。提交不改账。批准与驳回是 `POST /warehouse-fulfillment-stock-import-approvals` 与 `stock-import-rejections`，授予与库存调整批准相同。职责规则没登记时批准答未配置，驳回不比这套规则。
- 并发：记数先取货主锁的共享一侧，再取（租户，货主，仓）的记数锁。商品修订问在库时取货主锁的独占一侧：修订先落，之后的记数照新修订判；记数先落，修订改跟踪批号或效期答 `IMMUTABLE_FIELD`。
- 没有：开箱；取走时不指名入库批次；占用；库存读口的端点。开箱仍由这本账拒绝，不因为整箱库存另有一本账就改口。

## 已实现：入库预报

身份是（租户，预报标识）。预报不是库存。受理、告知与撤回都不写库存账。

- 运营代录三个写口挂运营决定 `INBOUND_PRE_ADVICE_RECORD_ON_BEHALF`，准入两词都是这个字面量。`POST /warehouse-fulfillment-inbound-pre-advice-on-behalf` 代录提交；`POST /warehouse-fulfillment-inbound-pre-advice-notice-on-behalf` 代录告知；`POST /warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf` 代录撤回。代录提交时货主账户查不到答 `404 OWNER_ACCOUNT_NOT_VISIBLE`。
- 货主写口挂首方客户渠道，准入两词都是该口自己的字面量：`POST /v1/warehouse/inbound-pre-advices`（`INBOUND_PRE_ADVICE_SUBMISSION`）、`POST /v1/warehouse/inbound-pre-advices/arrival-notices`（`INBOUND_PRE_ADVICE_ARRIVAL_NOTICE`）、`POST /v1/warehouse/inbound-pre-advices/withdrawals`（`INBOUND_PRE_ADVICE_WITHDRAWAL`）。来源请求键是 `inbound-pre-advice:`、`inbound-arrival-notice:`、`inbound-withdrawal:` 加接入请求标识。
- 答复：`ADVISED`（201），其余 200：`NOTICE_UPDATED`、`CANCELLED`、`EXISTING_RESULT`、`SOURCE_CONFLICT`、`WAREHOUSE_NOT_FOUND`、`NOT_CONFIGURED`、`WAREHOUSE_NOT_ACCEPTING`（原因 `DEACTIVATED` 或 `AGENT`）、`CONTENT_INVALID`、`STATUS_MISMATCH`、`RECEIPT_STARTED`、`PRE_ADVICE_NOT_FOUND`、`NUMBERING_RULE_COLLISION`。
- 提交的判定：代录先核货主；按（租户，途径，来源，键）认重放，重放不取号；仓没登记、已停用或是代理仓按上列结果拒；明细有不合答 `CONTENT_INVALID`，不取号；同一事务里取号再落成已预报，告知第 1 版同笔写入。告知与撤回在一个仓都没登记时先答 `NOT_CONFIGURED`，不查预报在不在。已取消或已完结的告知答 `STATUS_MISMATCH`。已取消再撤答 `EXISTING_RESULT`；收货中或已完结的撤回答 `RECEIPT_STARTED`。
- 单据号类型 `INBOUND_PRE_ADVICE`，默认模板 `IB-{yyyy}{mm}{dd}-{seq:5}`，按日重置。日期用仓已登记的业务时区；仓没登记业务时区就用租户的单据编号时区；都没有则照常受理、不带号，无号原因 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`。撞号答 `NUMBERING_RULE_COLLISION`，不留预报。
- 运营查阅挂主数据与运营查阅读，不判准入：`GET /warehouse-fulfillment-inbound-pre-advices`、`GET /warehouse-fulfillment-inbound-pre-advice`、`GET /warehouse-fulfillment-inbound-pre-advice-by-number`。货主读口能力词都是 `WAREHOUSE_READ`，不判准入：`GET /v1/warehouse/reads/inbound-pre-advices`、`GET /v1/warehouse/reads/inbound-pre-advice`、`GET /v1/warehouse/reads/inbound-pre-advice-by-number`。找不到不答 404。别的货主的预报与查无此预报同答。读出带状态、告知版本、单据号或无号原因、累计实收、逐次收货、完结与少收、差异的现行结论；有待认领到货时多一格 `unclaimedArrival`。货主读口不带登记责任方，也不带收货的设备与操作者。
- 并发：提交锁判重位置；告知、撤回、收货与完结锁同一份预报。同一键并发至多一份预报。

## 已实现：收货确认

`POST /warehouse-fulfillment-receipt-confirmations`。挂作业事实登记，作业范围种类 `WAREHOUSE`，引用须等于载荷里的仓。铸造侧准入的能力词是 `WORK_FACT_REGISTRATION`，事实类型是 `RECEIPT_CONFIRMATION`。商品入库与整箱入库走同一口，按预报的入库方式分。

- 答复：`CONFIRMED`（201），其余 200：`EXISTING_RESULT`、`CONTENT_CONFLICT`、`CONTENT_INVALID`、`WAREHOUSE_NOT_FOUND`、`PRE_ADVICE_NOT_FOUND`、`DIVERTED_TO_UNCLAIMED`。
- 判定：先按事实身份认重放，重放不再入账。仓没登记、预报查不到、仓或行的形态与预报不符，整条不落。然后锁预报：已取消或已完结的，不核行上库位与别名，直接把原事实转成待认领到货、不入账。还能收货才整条核，有不合答 `CONTENT_INVALID` 并逐行指出全部不合（`issues`），一行都不落。形态相符且是整箱入库时改走整箱账，不进商品库存。商品入库核库位（须属于在用的收货暂存且库位未停用）；识别码与贴标别名按事实发生时刻生效的修订与有效区间解析，认得出的商品的计量单位、批号与效期按入账时的当前修订核。成立的行以收货流水入账，并形成这一次的差异：多收 `OVERAGE`、错货 `WRONG_GOODS`、破损 `DAMAGE`。待检不记差异。少收不在这一次。认不出的行不入账。没有任何成立的行也答 `CONFIRMED`，预报进入收货中。
- 并发：先锁事实身份，再锁预报，再让商品入账去拿货主锁与记数锁。同一份预报上的两次收货串行。
- 没有：不带预报的到货另立登记；上架。

## 已实现：整箱库存

与商品库存分开。一箱是一件，不分库存状态，余额由整箱流水现算。没有自己的端点。整箱收货写入收货暂存，一箱一个入库批次，不把箱单拆进商品库存。

- 这一段流水原因只有收货 `RECEIPT`。预报没列，或这份预报上已经收过的箱，不入账，记错货。要入账的箱号已经在这个货主这个仓的在库箱号里，不入账，也不形成差异。破损的入账箱另记 `DAMAGE`。
- 并发：先锁预报，再锁（租户，货主，仓）的整箱账，不拿商品库存的货主锁。
- 没有：开箱、取走、占用、整箱读口的端点。

## 已实现：完结与少收

`POST /warehouse-fulfillment-inbound-pre-advice-closures`，挂运营决定 `INBOUND_PRE_ADVICE_CLOSE`。准入的能力词与事实类型词都是这个字面量。不挂作业事实登记。

- 答复：`CLOSED`（201），其余 200：`EXISTING_RESULT`、`STATUS_MISMATCH`、`PRE_ADVICE_NOT_FOUND`、`NOT_CONFIGURED`。
- 判定：一个仓都没登记先答 `NOT_CONFIGURED`，不查预报。锁预报。已完结再完结答 `EXISTING_RESULT`，说明不同也不是另一份。已预报或已取消答 `STATUS_MISMATCH`。只完结收货中的。商品少收按基本单位，只留下大于 0 的；整箱少收是还没入账的预报箱号，一箱一行，数量是一箱。少收的种类是 `SHORTAGE`。有少收才另签一个差异标识，不写进这一次收货的差异行，也不改两本库存。
- 并发：预报锁。一份预报至多一次完结。
- 货主读口与运营查阅都带完结与少收。货主口不带完结操作者。没有少收就不出现少收行。

## 已实现：收货差异结论

货主口 `POST /v1/warehouse/receipt-discrepancy-conclusions`，能力 `RECEIPT_DISCREPANCY_CONCLUSION`，准入两词都是这个字面量，来源请求键前缀 `receipt-discrepancy:`。代录口 `POST /warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf`，决定 `RECEIPT_DISCREPANCY_RECORD_ON_BEHALF`，准入两词都是这个字面量。结论是 `CONFIRM` 或 `OBJECT`，不改库存。

- 答复：`CONCLUSION_RECORDED`（201），其余 200：`EXISTING_RESULT`、`CONCLUSION_HELD`、`DISCREPANCY_NOT_FOUND`、`NOT_CONFIGURED`、`SOURCE_CONFLICT`。
- 判定：一个仓都没登记先答 `NOT_CONFIGURED`。差异在这个货主名下查不到答 `DISCREPANCY_NOT_FOUND`。还没有结论就记下。现行是代录的：货主本人的结论取代它，不论相同还是不同，被取代的照留；代录不取代，相同答 `EXISTING_RESULT`，不同答 `CONCLUSION_HELD`。现行是货主本人的不再改。
- 并发：先锁差异标识；货主口再锁来源请求键。
- 收货形成的差异与完结形成的少收都能作结论。没有：开箱带来的差异。

## 已实现：待认领到货

两种来源共用一张表，只在操作者面按租户隔离，不进任何货主库存账，客户面没有读口。

- **直接登记**：`POST /warehouse-fulfillment-unclaimed-arrival-registrations`，挂作业事实登记（范围种类 `WAREHOUSE`，引用等于载荷仓；准入 `WORK_FACT_REGISTRATION` + `UNCLAIMED_ARRIVAL`；先认证再读载荷）。种分 `INBOUND`、`RETURN`，答复 `REGISTERED`（201）、`EXISTING_RESULT`、`CONTENT_CONFLICT`、`WAREHOUSE_NOT_FOUND`、`WAREHOUSE_NOT_ACCEPTING`（`AGENT`；停用自营仓照记）。观察与存放处逐字保存，线索不作认定，不向货主预报读口透出 `unclaimedArrival`。
- **收货转来**：取消或完结后到达的收货走收货确认口同笔落成 `DIVERTED_TO_UNCLAIMED` 与一件待认领到货；预报读口只对这类转来标 `unclaimedArrival`，不带内容与数量。收货确认载荷另带可选 `unclaimedArrivalId`：引用的到货须在 `CLAIMED` 且认给该预报，预报不能收货或对不上均答 `CONTENT_INVALID`、不另建新到货。
- **认领与撤销**：`POST /warehouse-fulfillment-unclaimed-arrival-claims`、`claim-revocations`，挂运营决定 `UNCLAIMED_ARRIVAL_CLAIM`（准入两格同字面量），按各自决定标识判重；认领向 `party-commercial` 核货主（查不到答 `404 OWNER_ACCOUNT_NOT_VISIBLE`），入库到货须指名同货主同仓可收货预报，退件不带预报；已有确认收货引用的认领答 `REVOCATION_NOT_ALLOWED`。
- **处置规则与处置**：规则修订走登记册配置写 `POST /warehouse-fulfillment-unclaimed-disposal-rule-registrations`（不判准入）与查阅 `GET /warehouse-fulfillment-unclaimed-disposal-rules`、`rule-revisions`，按（租户，仓）只追加，认领期限按经过秒数算、不依赖仓业务时区，方式逐字保存。处置走 `POST /warehouse-fulfillment-unclaimed-arrival-dispositions`（运营决定 `UNCLAIMED_ARRIVAL_DISPOSITION`），无规则答 `NOT_CONFIGURED`、不借用别仓，期限未到或方式不在规则答 `DISPOSITION_NOT_ALLOWED`，同方式重放答 `EXISTING_RESULT`，换方式答 `STATUS_MISMATCH` 并回显原处置。运营查阅：`GET /warehouse-fulfillment-unclaimed-arrivals`、`unclaimed-arrival`。没有：更正观察、向货主公开。退货入库已可引用已认领的待认领到货。

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

- 拒绝接受拆包：已拣出的生成回库任务，这一笔装箱记录标成已截单。整票一份委托时按请求回一次。操作者提交的库存调整仍不收待出库库区。
- 仓储计费方案在 `internal/parcelpricing`，采用、预计与确认在 `internal/settlementaccounting`。评价不进包裹那四种对象。确认依据 `WAREHOUSE_WORK_OCCURRED`。已确认费用的更正按差额结清；没采用实测差异自动调整答未配置，贷记会使费用为负时仍答待人处理。有采用行时按（发生项，版本）记下 SELL 请求，带结算账户、结算币种、计价范围与计价基准时点，评价标识指向这一次计价。
- 全程追踪已能收下 `warehouse-outbound-confirmed`。里程碑映射没登记，不编默认。
- 回补走批准口，不走库存调整那个提交口。

## 已实现：库内作业

- 四个登记口挂作业事实登记，范围种类 `WAREHOUSE`，引用等于载荷仓，先认证再读载荷：`POST /warehouse-fulfillment-internal-moves`（`INTERNAL_MOVE`，上架、移库与拣选位补货）、`form-conversions`（`FORM_CONVERSION`）、`condition-conversions`（`CONDITION_CONVERSION`）、`shortage-registrations`（`SHORTAGE`）。答复 `REGISTERED` 或 `SHORTAGE_REGISTERED`（201）、`EXISTING_RESULT`、`CONTENT_CONFLICT`、`CONTENT_INVALID`、`WAREHOUSE_NOT_FOUND`、`WAREHOUSE_NOT_ACCEPTING`。停用的仓照记，停用的库区与库位不作移入目标；代理仓不受理。
- 取得走的部分入账，其余各成一份待处理库存差异，写口不拒。没点名入库批次的从早到晚取。移入次品库区的同一笔转为次品。形态转换与带转入单位的补货只入账两头都折成整数个单位的部分，入库批次不另起。短少只记观察，不改账。上架没入账的数量可经 `POST /warehouse-fulfillment-job-discrepancy-approvals` 从源头补进目标位，短少经同一口在所在格调减；决定词与库存调整批准相同，驳回是 `job-discrepancy-rejections`。没登记批准职责时答未配置。形态转换与状态转换经同一口按原作业原因补入账：从源头的转出单位或状态取出，记入差异所在的转入单位或状态。整箱缺箱只在箱已在账上时从所在位移到报告库位，不新起入库批次；箱还不在账上时批准不成立。驳回不改账。占着的整箱仍不批。
- 次品转良品 `POST /warehouse-fulfillment-condition-restores`，运营决定 `CONDITION_RESTORE`，整份成立或整份不成立，库位须在收货暂存或退货暂存。答复 `RESTORED`、`EXISTING_RESULT`、`SOURCE_CONFLICT`、`NOT_VALID`。
- 运营查阅：`GET /warehouse-fulfillment-internal-jobs`、`internal-job`、`condition-restore`。冻结、盘点锁定、未分配预占和未拣完的分配都计入占用。整箱移动写 `-1` 与 `+1`，不新起入库批次。

## 已实现：加工、换标与开箱

- 货主提交 `POST /v1/warehouse/processing-instructions`、`relabel-instructions`，撤回在各自的 `/withdrawals`。加工与拆分共用加工这一词。货主与来源请求键取自信封。
- 运营发起 `POST /warehouse-fulfillment-processing-instructions`、`relabel-instructions`，作废是 `processing-instruction-voids`、`relabel-instruction-voids`。发起与作废共用 `PROCESSING_INSTRUCTION` 或 `RELABEL_INSTRUCTION`。作废须带原因。种类对不上这一口的答查无此指令。
- 执行与开箱挂作业事实：`processing-executions`、`relabel-executions`、`unboxings`。执行整笔入账，账上不够留在待加工。开箱按实点入商品账，入库时间沿用那一箱。

## 已实现：冻结

- 冻结与解冻挂运营决定 `STOCK_FREEZE`：`POST /warehouse-fulfillment-stock-freezes`、`stock-unfreezes`。只占可用量，不记流水。暂存、待检、余额不够或会挤占已有占用的整份不成立。解冻整份解除。库内入账取走时减掉还冻着的数量。库存读口 `GET /warehouse-fulfillment-stock` 与货主 `GET /v1/warehouse/reads/stock`：`view` 取 `cells`、`cartons`、`summary`、`holds`、`movements`。货主不带库位，也不带冻结的原因与操作者。仓的业务时区没登记时库龄答 `NOT_CONFIGURED`，数量照答。库内入账会减掉未分配的预占。货主 `POST /v1/warehouse/outbound-requests`，代录 `POST /warehouse-fulfillment-outbound-requests`，缺货处置 `POST /warehouse-fulfillment-shortage-policies`。不够又没登记缺货处置时请求停在待预占。尚未分配的可以拆，原预占放开。汇总可用量减掉预占，占用读口看得到；格上不减，预占不落库位。作业锁定按还在盘点中的数量计。放波次在分配策略没采用时答未配置；采用参考策略后，已预占的请求才落到库位。拣货把实拣的数量移进待出库，没拣的预占留下。拣货、出库复核与打包走作业事实口。复核数量相符才可打包，不符的停在已拣。打包按包裹序号记下包材与尺寸重量，包材文本不查册。本产品网络随后按（出库请求，包裹序号）以货主名义提交委托，来源是仓配出库；代办授予没登记答未配置，不建委托。已提交的截单先撤回；已接受的走包裹取消，成立才拆包，授权没登记或取消不成立则截单不成立、库存不动。自备面单与自提不提交。标了预取且包材、仓测、代办都齐时，放波次按包裹序号 1 交一份估算委托；缺了仍只分配。平台备货不预取。出库请求带 `platformStocking` 时，全部包裹打包后按（请求，提交轮次）交一份委托；没标的仍按包裹各交一份。预取过的包裹打包后沿用那份委托，不再另交；打包称重与估算不同时提出声明测量资料修订，已采用记已采用，未采用、未配置、不放行或尚未接受记待复核并挡住在线出库确认。货主补正经 `POST /v1/warehouse/outbound-requests/supplements` 代交新提交版本。资料版本把更正记成已采用或待复核，待复核不出库确认。包裹取消对上则标成已取消，对不上入账收工。开关关着时资料版本、包裹取消与出库确认没有订阅者。出库确认：面单本体没配置时在线的本产品网络答未配置，离线照记并消耗；自备面单数对上才确认；自提凭证没登记答未配置。

## 已实现：工作台查阅

运营查阅挂主数据与运营查阅读，不判准入。`GET /warehouse-fulfillment-settings-overview` 读八项规则是否已登记，未登记的格是 `configured: false` 或空列表，不填默认。`GET /warehouse-fulfillment-outbound-request` 读一张出库请求的行、预占、波次、拣货、复核、包裹、确认与回库。分页台账：`freezes`、`counts` 与 `count`、`adjustments`、`job-discrepancies`、`instructions`。`returns-overview` 读退货预报与到仓处置。`agent-overview` 读连接器、声明与回执；声明新鲜度与 `ReadStatement` 同一套：同一来源、仓、货主、商品且 `stated_at` 更晚为 `SUPERSEDED`，否则当前时刻不早于 `validUntil` 为 `UNKNOWN`，再否则 `VALID`。历史设计稿仍在 `docs/domain/warehouse-fulfillment/` 与 `docs/application/warehouse-fulfillment/`，不是现行规则。
