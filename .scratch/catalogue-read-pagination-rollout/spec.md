# 目录读口分页、排序与筛选下推：ADR-0144 第二批——其余各册全部迁完

Category: enhancement
Status: in-progress——2026-11-06 通道 1 立（用户在 IDP 队列令「帮我全面解决这些问题吧」，指首批收口后其余列表读口仍答满一页就停）；子票全部 ready-for-agent
出处：[ADR-0144](../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定七「一处定义，逐册迁移」；首批 [catalogue-read-pagination](../catalogue-read-pagination/spec.md) 收口时写明「其余各册按同一形状逐册拆票，另立」，本 spec 即那一批。

契约只在 ADR-0144 一处定义；本 spec 与子票只引它的决定号，不复述参数名、答复形状与校验规则。

## 为什么

首批（2026-10-10 收口）只迁了客户账户与网络目录两册，之后客户追踪按运单号查（ADR-0356）顺带用了游标。其余列表读口今天仍是 `ORDER BY … LIMIT <页大小>`：
各上下文操作者 Intake 的页大小写死 200，答复里没有任何翻页信息，第 201 行起调用方取不到、也不知道它存在；管理台在这 200 行上做的客户端筛选可能答出假的「没有」
（ADR-0144 Context 原句）。parcel-shipment 等处的几个待办队列带 `truncated` 与计数，能说「还有」，但同样取不到后面的行。接入身份的操作者、工作设备两个列表反过来不设上限，
一次全取。

取证（钉 `a1274a6b8`，AST 枚举各上下文 `ports` 里带 `limit` 形参的接口方法，再按路由核对 `cmd/parcel-api` 的挂载）：对外挂载、按租户上列的读方法约 106 个，分在 11 个上下文；
首批 8 个（网络目录七族、客户账户）已迁。本数只作此刻取证，各子票按自己的地盘重数。

## 范围

**在内**：经 `cmd/parcel-api` 挂载、按租户上列一册行的 GET 读口，含一个路由按 `kind` / `family` 分派多册的那种（每个选择器取值各是一册）；管理台消费这些读口的列表页。

**不在内**：

- 单条读取（按标识取一行、按对象取修订链的 readback）。
- 有意不设上限的证据面：法人、业务参与方、产品渠道映射的修订链（「截断的历史不是历史」，见各读面头注）。
- 只给后台循环用、不经 HTTP 的内部队列（如结算的停滞评估、派送追问、案件编号碰撞重试）。
- 首方客户 API 的口：客户追踪按运单号查已按 ADR-0356 用游标；其余首方口不是按租户上列的目录。

## 横贯各子票的取舍（产品策略，ADR-0146；本 spec 定一次，子票只引不复述）

1. **缺省序保今天的次序。** 每册的缺省序取它今天 `ORDER BY` 的首列与方向，迁移后第一页与今天屏上看到的那一页同口径；待办队列今天是「最早的在前」，照旧。
   缺省序不是 `-registeredAt` 的，在该册声明头注写明为什么（ADR-0144 决定三）。
2. **行标识取登记表主键去掉租户那几段**，与排序同向作决胜键（决定三）。今天决胜键方向与首列不一致的，迁移后只在首列完全相同时次序不同，子票判断项里点名。
3. **可排维、筛选维从小放。** 可排维至少是缺省序那一维，另放操作者确有换序需要的一两维；有状态（或同类封闭词表）的册放出那一维作筛选维（决定四）；`q` 覆盖本册已上列的标识与名称列。
   不为「以后可能要」开维度——每开一维就多一个要索引、要校验的面（ADR-0144 候选乙的反方）。
4. **`total` 一律给精确值**（决定五缺省）。按包裹、交易、事件计量的册是越权风险点 3 最先撞上的那几册，子票判断项里点名，超出租户级规模时回 ADR-0144 改成 `null`。
5. **已带计数与 `truncated` 的待办队列保留这两格**（决定五「原有形状外加一格 `page`」）：计数仍是整个队列的积压数，不随筛选与 `q` 变；`truncated` 改为「本页之后还有」，
   即 `page.next` 不为 `null`——不带条件的第一页上与今天同义。
6. **可排维与行标识的列必须非空**：键集比较遇到 NULL 得 NULL，那一行会从后面各页静默消失。可为空的列不放出作可排维，或在读面里先导出成非空列。
7. **每迁一册补一条与缺省序同形的索引**（租户 + 排序维 + 标识，决定七）；登记表上已有同形索引的，子票写明是哪一条、不重建。
8. **端点先 Intake、后解码查询参数。** 渠道未配置时对一切请求同答 403，答复不随请求内容变（ADR-0055 决定三；parcel-shipment
   的未配置用例以查询串变体钉着这一条）；没有凭据的调用方也就探不出各册开放了哪些查询维度。配置了的渠道上坏参数答 400、理由散文进
   `detail`。首批两册当时取的是先解码、未配置时坏参数先答 400，与此相反，由 15 改齐。

## 每张迁移子票（02–13）的做法与判据

各票只列自己的册与特有事项，做法与判据在这里定义一次。

**做什么**

1. 每册在本上下文 `ports` 上声明可排维、筛选维、行标识与缺省序（取舍 1–3），声明头注写缺省序为什么、`q` 覆盖哪几列。
2. 读端口改为收查询对象、答 01 的统一结果类型；租户与作用域仍在签名上，`limit` 非正仍拒（ADR-0077 Decision 五）。
3. postgres 读面用 01 的落列声明取页，`total` 与本页同一组条件；补与缺省序同形的索引（取舍 7）。
4. 端点在 Intake 之后经本册声明解码（取舍 8），坏参数答 400、理由散文进 `detail`；答复加 `page`；带计数与 `truncated` 的队列照取舍 5。
   单条读取与有界查找的分支在解码之前分派走，不受影响；管理台与脚本已在用的查询参数登为选择器或筛选维，不能变成 400。
5. `cmd/parcel-api` 等处的占位与测试替身只改签名。

**不做**：不改作用域、授权与 Intake；不改写面与决定口；不改管理台（归 14）。

**完成判据**

- 真库用例（`SYN-` 数据）：每册翻页期间插入新行，已翻过的页不重出、未翻的页不漏行；`total` 随筛选与 `q` 变；空册答空页且 `total` 为 0。同一读面上同形的几册
  可以共用一组表驱动用例，但每册至少过一遍。
- 端点用例：未知键、集外排序维、词表外筛选值、换条件拿旧游标各答 400 带理由散文；渠道未配置时参数写对写错都答 403；答复带 `page`，
  `size` 取 Intake 页大小。
- 迁移按字节无 CR、无 BOM。

## 共用件的补件（子票 01）

首批两册各自手写了筛选维的 `= ANY`、`q` 的字面 `ILIKE`、「排序维值 + 标识」的键集比较与同向次序，写法逐字相同；第二批以百计，抄第三份起就会有一份抄错。
01 把这半落进 `internal/platform/cataloguepage`：各册只给「哪一维落到哪一列」，共用件拼出总数与取页共用的条件、键集比较、次序与页大小占位，并给出切页编游标的 `Finish`
与统一的结果类型——仍不含任何上下文的词，也不碰数据库连接（ADR-0144 决定七）。

## 子票

| 票 | 标题 | 地盘 | 状态 |
|---|---|---|---|
| [01](./issues/01-platform-sql-binding-and-result.md) | 共用件补件：落列声明、取页 SQL 片段、`Finish` 与统一结果类型 | `internal/platform/cataloguepage` | ready-for-agent |
| [02](./issues/02-parcel-shipment-reads.md) | parcel-shipment 九册 | `internal/parcelshipment`、`migrations/parcel_shipment` | ready-for-agent |
| [03](./issues/03-visibility-exception-reads.md) | visibility-exception 十四册 | `internal/visibilityexception`、`migrations/visibility_exception` | ready-for-agent |
| [04](./issues/04-parcel-pricing-reads.md) | parcel-pricing 各册 | `internal/parcelpricing`、`migrations/parcel_pricing` | ready-for-agent |
| [05](./issues/05-transport-fulfillment-reads.md) | transport-fulfillment 各册 | `internal/transportfulfillment`、`migrations/transport_fulfillment` | ready-for-agent |
| [06](./issues/06-node-operations-reads.md) | node-operations 八册 | `internal/nodeoperations`、`migrations/node_operations` | ready-for-agent |
| [07](./issues/07-network-routing-remaining-reads.md) | network-routing 其余四册 | `internal/networkrouting`（目录七族之外）、`migrations/network_routing` | ready-for-agent |
| [08](./issues/08-settlement-accounting-reads.md) | settlement-accounting 各册 | `internal/settlementaccounting`、`migrations/settlement_accounting` | ready-for-agent |
| [09](./issues/09-customs-compliance-reads.md) | customs-compliance 十六册 | `internal/customscompliance`、`migrations/customs_compliance` | ready-for-agent |
| [10](./issues/10-collection-remittance-reads.md) | collection-remittance 一册 | `internal/collectionremittance`、`migrations/collection_remittance` | ready-for-agent |
| [11](./issues/11-party-commercial-remaining-reads.md) | party-commercial 其余十八册 | `internal/partycommercial`（客户账户之外）、`migrations/party_commercial` | ready-for-agent |
| [12](./issues/12-pilot-governance-reads.md) | pilot-governance 四册 | `internal/pilotgovernance`、`migrations/pilot_governance` | ready-for-agent |
| [13](./issues/13-access-identity-reads.md) | access-identity 操作者与工作设备两册（今天不设上限） | `internal/accessidentity`、`migrations/access_identity` | ready-for-agent |
| [14](./issues/14-admin-web-pages-push-down.md) | 管理台各列表页改游标翻页、筛选与检索下推 | `apps/admin-web/src/pages/**` | ready-for-agent |
| [15](./issues/15-first-batch-onto-binding.md) | 首批两册改用 01 的补件、两份页类型并入统一结果类型 | 首批两册的读面与端口 | ready-for-agent |
| [16](./issues/16-derived-state-filter-dimensions.md) | 状态由读面派生的几册补状态筛选维（04、05、09 交回与评审提出） | 各册所在上下文的声明、读面与答复体 | ready-for-agent（Blocked by 04、05、09） |

走法：01–13、15 碰 Go / SQL，走[进 main 的步骤](../../docs/agents/workflow.md#进-main-的步骤)；14 的 diff 只在 `apps/admin-web/**` 与票面，走
[workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)，按上下文分笔、随各后端子票进 main 逐页接。

## 红线

- 契约以 ADR-0144 为准；实施中发现它写不通，回 ADR 改（或 supersede），不在代码里另立口径。
- 查询参数一律封闭集，集外即拒；页大小不开调用方参数（决定二）。
- 夹具与种子只出 `SYN-` 合成值；证据层级记 `S`。
