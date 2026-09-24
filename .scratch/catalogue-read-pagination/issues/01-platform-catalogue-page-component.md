# 01 `internal/platform` 共用件：游标编解码、查询参数校验、答复 `page` 拼装

Category: enhancement
Status: in-progress——2026-09-24 通道 2 认领（派单 `task-44d76717` ← 通道 3），隔离 worktree 分支 `mcp2-crp01`，基于本笔
Blocked by: 无
地盘：新包 `internal/platform/<名自定，如 cataloguepage>/` 与其测试。不碰任何上下文包。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定一、三、四、五、七。

## 做什么

1. **各册的声明形状**：一册把自己的可排维（含缺省序）、筛选维（维名、是否封闭词表及其词表）交给共用件；`q` 覆盖哪几列是 SQL 侧的事，不进声明。
2. **解码**：从查询串解出查询对象（游标、排序、筛选、`q`），按声明校验；集外键、集外排序维、词表外的筛选值、解不开或摘要不符的游标、超长的 `q`
   一律报成一个可映射到 `MALFORMED_REQUEST` 的错误，并带一句给操作者看的理由散文（形同各上下文 `problemDetail.Detail` 的用法）。
3. **游标**：编成不透明串；内容与摘要口径照 ADR-0144 决定一。
4. **答复 `page` 拼装**：照决定五。

## 不做

- 不写任何一册的 SQL，不改任何读端口签名（归 02、03）。
- 不开页大小参数（决定二）。

## 完成判据

- 包内测试钉住：游标往返；排序或筛选或 `q` 变了之后拿旧游标即拒；集外键即拒；集外排序维即拒；词表外筛选值即拒；`q` 去首尾空白、空即缺席、超长即拒；
  同一维重复给的解成多值。
- `go test ./internal/architecture/ -count=1` 过：共用件不导入任何上下文包。
- 完成记录写清包名与导出面，供 02 / 03 照用。
