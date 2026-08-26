# 管理台骨架页的接线前沿盘点

回答一个问题：**二十三张骨架页，下一批该接哪几张，各自被什么拦着。**

**这份文档不是什么**：它不是排期，也不是接线许可；它不裁决任何页面的列面与呈现（那归各页自己的票，通则见 `apps/admin-web/README.md` 的列表页上列通则）。它只盘「后端供数面到位到什么程度」这一件事。

**当前接线态一律以 `apps/admin-web/src/page-registry.tsx` 的 `liveIds` 为准**——那是代码，不会与自己漂。本文按盘点当时的 `liveIds` 挑骨架页；某页接线后本文对它的判断即作废，不要拿本文当接线态的第二处登记。

## 判据：一页要接线，后端要齐四件

已接线那批的形状（`parcel-pricing` 是最完整的一份样板）逐件对出来是：

| 件 | 样板位置 | 缺了会怎样 |
|---|---|---|
| **表有行** | 迁移建表 + 库里真有数据 | 页面只能演空态，演不出内容 |
| **写入方** | 受控登记 CLI（如 `parcel-pricing-register`）或进程编排 | 表永远 0 行，灌不进去 |
| **装载口** | `ports/catalogue_read.go` 伴生读端口 + `adapters/postgres/operations_catalogue.go` | 有数据也取不出来 |
| **端点** | `adapters/http/query_*.go` + `isolated_read_intake.go` + `cmd/parcel-api/endpoints.go` 一行 | 页面无处发请求 |

四件里**「写入方」和「表有行」是两件事**：本盘最有价值的发现全在这条缝上——有几处写入方早就在了、库里也真有行，只是没人给它开读面。那种页是最便宜的下一批。

## 盘面

### 批 A · 库里已经有行，只差装载口与端点

这两页的表、写入方、数据三样今天全在，缺的只是读面那两件。**没有任何机制阻断物**，是纯增量。

| 页 | 数据在哪 | 写入方 | 缺什么 |
|---|---|---|---|
| **客户与合同** `party-contracts` | `party_commercial.commercial_version`（`CUSTOMER_CONTRACT` 类）+ `customer_contract_content` + `customer_contract_control_binding` | `parcel-commercial publish`（已在种子批里发过） | `ListCustomerContracts` 读端口与适配器方法；端点 |
| **供应商协议** `supplier-agreements` | 同一张 `commercial_version`（`SUPPLIER_AGREEMENT` 类） | 同上，CLI 的类别封闭集里就有这一类 | 一条种子（该类今天零行）+ 同上两件 |

两页共用同一张表、同一套装载方向（`OperationsCatalogue` 现有的 `ListServiceProducts` 是逐字可比的先例：按 `object_kind` 取版本壳，再左连接各自的正文册）。**建议合成一票做**——分两票会把同一个读法写两遍，而两遍之间没有任何理由不同。

一处待裁：客户合同与供应商协议要并进 `/commercial-policies` 的 `kind` 分派，还是各立入口。现有分派收的是「策略」类；合同与协议不是策略。倾向各立，但这归实现票裁。

### 批 B · 登记 CLI 已在、库里零行

**写入方已经建好了，只是种子没用它。** 灌一批种子就有真数据，之后同样只差读面两件。

| 页 | 表 | 写入方（已在，种子未用） | 额外缺件 |
|---|---|---|---|
| **阶段决定与暂停恢复** `stage-admission` | `pilot_governance.authority_interval` / `suspension_decision` / `resumption_decision` | `parcel-governance-register` 的 `authority-interval`、`suspend`、`resume` | `pilotgovernance` **整个 `adapters/http` 包不存在**（含未配置 Intake 与隔离读 Intake 一对）；`adapters/postgres` 五个全是写口 |
| （无页可归，见下节「导航缺口」） | `visibility_exception` 的六类目录表 | `parcel-ve-register` 的六个子命令 | 同上，VE 有 http 包但无目录查阅端点 |

`stage-admission` 页面所指的「阶段评审」与「接管」两格今天灌不进去：`parcel-governance-register` 自己写着那两类属第二批、未开。所以这一页接出来会是**三格有内容、两格如实说明未开**——那符合仓内纪律，但要在票面里先说清，别让人以为漏了。

### 批 C · 表在、装载口或写入方缺，要先补机制

| 页 | 卡在哪 |
|---|---|
| **渠道产品目录** `channel-product-catalog` | `party_commercial.service_product_form` 有表、有 `SaveServiceProduct` 端口与真库适配器、有真库测试，**但全仓没有任何 cmd 调用方**——产品—渠道映射今天灌不进去。这一格与 `commercial_price_policy` / `commercial_settlement_policy` 是同病（后者已记在 `scripts/demo-seeds/README.md` 的已知边界），但那两张至少有页面在演空态，这一张没有 |
| **合规限制与监管税费** `customs-restrictions` | 四块内容分裂：放行门禁核对与结案义务两块的 CLI 与数据都在（见下节）；关务限制及解除的表与 `Restrictions.ListByScope` 读法都在、但写入方是案件编排（事务，被墙拦）；监管核定税费**无表** |
| **价格评价** `pricing-evaluation` | `parcel_pricing.evaluation` 零行，写入方是评价编排，取数依赖路由产出的费用——被墙三拦 |
| **路由计划与改路** `route-plans` | `network_routing.initial_route` 零行，被墙三（解析层缺席）直接拦 |
| **面单交易** `label-transactions`、**接受前人工复核** `acceptance-review` | `parcel_shipment` 全表零行，被墙一、墙二拦 |
| **节点作业查阅** `node-operations-review`、**运输履约查阅** `transport-fulfillment-review` | 两上下文表全零行；`adapters/http` 只有命令端点，无查阅面 |
| **关务案件与申报** `customs-cases` | `customs_case` / `declaration_submission` / `declaration_unit` 全零行，被墙拦 |
| **异常分诊** `exception-triage`、**异常案件** `exception-cases`、**索赔与追偿** `claims-recovery` | 案件与索赔项本体零行、被墙拦。三页的**规则目录半边**属批 B（VE 六类目录）。另有两条机制缝已各自立票：资格规则视图缺租户维、索赔材料归集面未建（`.scratch/ve-claims-read-seams/`） |
| **费用与计费** `charges-billing`、**对账单** `reconciliation`、**收付款核销** `settlement-application`、**经营核算** `operating-metrics` | `settlementaccounting` 表与写适配器都很齐，**但整个 `adapters/http` 目录不存在**；库全零行，写入方是事务链，被三堵墙一路拦到底 |
| **集团与法人** `group-legal-entities` | 无表、无领域类型。`CommercialObjectKind` 的注释明写「货主客户账户与责任法人刻意不在其中：它们是参与方身份、走自己的生命周期」——那个生命周期今天没有代码 |
| **业务参与方** `business-parties` | 有领域（`domain/party_relationship.go`）、无表、无写入方 |
| **口岸与申报路径** `customs-ports-paths` | 无表。关务侧十二个迁移里没有口岸或申报路径 |

### 批 D · 上下文尚未存在

| 页 | 状况 |
|---|---|
| **代收分户账** `cod-ledger` | `collection-remittance` 没有 `internal/` 包、没有 `migrations/` 目录、库里没有 schema、也没有 `CONTEXT.md`——它的唯一出处是 `docs/domain/CONTEXT-MAP.md` 的一行。这一页离接线最远，不属「补读面」范畴 |

## 两处不在页面清单上的发现

### 一 · 已登记但无页可看的行

关务侧种子灌进去的登记里，只有解释规则与建案要求两类被合规规则库页读到。另外四类**登记成功、库里有行、没有任何页面能看见**：结案义务目录与义务项、放行门禁目录与门禁认定；此外备案判定与提交权威两张也各有行。

这不是缺陷，是页面版图与登记版图没对齐。它对本盘的意义是：**这些行是免费的接线燃料**——写入方与数据都在，任何一张收下它们的页面都只差读面两件，成本与批 A 同级。要不要为它们开页（或并进 `customs-restrictions`）是导航裁决，不是机制问题。

同类还有商业侧的阶段内容声明一族（定案规则、取消权威、受理资格与允许来源），种子发布批灌过、页面读不到。

### 二 · VE 六类目录没有导航条目

`parcel-ve-register` 能登记里程碑映射、分诊规则、通知策略、索赔资格、索赔授权、披露策略六类目录。这六类都是**主数据**，按导航的分区口径本该在主数据区——但主数据区今天一个 VE 条目都没有，而追踪异常区那三页装的是案件不是目录。

所以这六类今天**无页可归**。补的是导航条目，不是接线。

## 建议的下一批

按「机制阻断物为零」排，只有两组：

1. **批 A 合一票**（客户与合同 + 供应商协议）——同表同读法，今天就能开工，做完两页转 live。
2. **批 B 的治理三格**（`stage-admission`）——要先建 `pilotgovernance/adapters/http` 包，工作量大于批 A，但阻断物同样为零；顺带把 VE 六类目录的读面按同一形状裁出来，因为两者缺的是同一样东西（有 CLI、有表、无查阅面）。

批 C 里除渠道产品目录与合规限制两页外，其余全部压在同三堵墙上（委托侧无入库通道、生产归属答不出、路由拿不到证据）。**墙不降，这些页接了也只能演空态**——降墙的判据不看本文，看 `.scratch/syn-wall-door-audit/issues/` 的对应机制票，动线上的表述见 `docs/design/synthetic-demo-journey-script.md` 第 5 步。

## 取证

盘于 `7ce41e4`，演示库为本机 `idp-parcel-postgres-gate`（`127.0.0.1:55432`），数据为 `scripts/demo-seeds/seed.sh` 的合成 `SYN-` 种子包（该脚本调用四个 CLI：计价、网络、关务、商业；**未**调用 `parcel-ve-register` 与 `parcel-governance-register`，这正是批 B 零行的原因）。

行数与「零行」断言都锚在上述 SHA 与那一份种子上——**数本身就是本文的论点**（「这一格有燃料」与「这一格是空的」的分界就是分批依据），换一份种子就要重取。逐表行数用 `information_schema` 全表扫得出，方法与 `docs/design/synthetic-demo-journey-script.md` 取证节同源。

代码侧断言（读适配器有无、`adapters/http` 目录有无、`SaveServiceProduct` 无 cmd 调用方、`CommercialObjectKind` 封闭集含合同与协议两类）为实读代码，同一 SHA。
