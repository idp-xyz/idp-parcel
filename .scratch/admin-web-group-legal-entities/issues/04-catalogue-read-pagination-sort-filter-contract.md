# 04 目录读口分页 / 排序 / 筛选下推的契约决策

Category: enhancement
Status: needs-info（归 owner：这是全部目录读口的契约形状，一决策一处定义，要先出 ADR 或设计交接）
Blocked by: 无
Type: grilling

## 问题

十个上下文的目录读口（`GET /commercial-*`、`/network-*`、`/pricing-*`……）今天都是一次拉全量，`isolatedReadLimit = 200`
是唯一上限，无 `page` / `sort` / 过滤参数；管理台 README「列表页上列通则」明写「过滤只在已取回的数据上做，不下推成新查询参数
——那要改端点契约」。演示种子量级下够用，主数据规模上去（一个租户几百个货主客户账户、上千条网络版本行）就不够。

## 要裁的

1. **分页形状**：偏移分页（`page`/`pageSize`）还是游标分页（`after=<登记时间,标识>`）。登记册按修订版本化、只增不改，
   游标分页在这类只增表上稳定（不会因为新插入而漏行或重行）；偏移分页操作者更熟。
2. **排序维**：封闭集（每册点名可排的列）还是任意字段。封闭集与 `MALFORMED_REQUEST`「查询参数不在服务端接受的封闭集合内」
   那条既有纪律同形。
3. **筛选下推**：只放状态一维，还是开放到每列。
4. **答复形状**：`{ outcome, entities, page: { total, next } }`——`total` 在大表上要不要给（COUNT 的代价）。
5. **一次改全部读口还是逐册**：`ListPageTemplate` 已留 `pagination` 槽，前端一次接上；后端十个上下文各自的读适配器逐个改。

## 不裁的

- 不改任何登记写面。
- 不改 `isolatedReadLimit` 的含义：它是隔离形态的上限，不是分页。

## 产出

一份 ADR（或若 owner 判为实施细节则设计交接）+ 拆成逐上下文实施票。本票在 owner 裁前不动代码。

## Comments
