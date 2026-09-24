# 02 客户账户目录读口（`/commercial-customer-accounts`）迁到 ADR-0144

Category: enhancement
Status: ready-for-agent
Blocked by: 01
地盘：`internal/partycommercial/adapters/http/query_customer_accounts.go` 与其测试、它消费的读端口与 postgres 读面、`migrations/` 下 party-commercial 模块的
一条新迁移（索引）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定三至七；首批之一（规模先到）。

## 做什么

1. 按答复体字段点名本册的可排维与筛选维（缺省序照决定三；有状态即放出状态维），`q` 覆盖的文本列（标识、名称这类）写在读面头注里。
2. 读端口改为收查询对象、答「本页行 + 下一游标 + 总数」；租户仍在签名上，`limit` 非正仍拒（ADR-0077 Decision 五）。
3. postgres 读面按「排序维值 + 标识」取游标之后的行，`total` 与本页同一组条件；补一条与缺省序同形的索引。
4. 端点经 01 的共用件解码，答复加 `page`；Intake 不动——隔离形态的页大小仍是 `isolatedReadLimit`。

## 不做

- 不改登记写面；不改作用域与授权。

## 完成判据

- 真库用例（`SYN-` 数据）：翻页期间插入新登记，已翻过的页不重出、未翻的页不漏行；`total` 随筛选与 `q` 变；空册答空且 `total` 为 0；换条件拿旧游标答 400。
- 端点用例：未知键、集外排序维、词表外筛选值各答 400 带理由散文。
- 迁移按字节无 CR、无 BOM（`migrations` 行尾守卫）。
