# 15 查询值含 NUL 或非法 UTF-8 的读口一律答 400、不读库

Category: bug
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无
出处：票 [12](./12-customer-tracking-by-shipment-number.md) 复评（钉 `93dfe3f06`）——操作者族 `serveCurrentProjection`（visibility-exception `query_tracking_projections.go`）的 `parcel` 含 NUL 会进读，库拒收参数答 `500`。票 12 只修了首方客户追踪两口（`refuseUnstorableQueryValue`）。

## 要做的

1. 全仓清点：各上下文 HTTP 读口里，查询串或路径参数原样带进 SQL 参数的格（`internal/*/adapters/http`）。逐个探：含 NUL、非法 UTF-8 时今天答什么。
2. 凡答 `500` 的，改为在该口的入站解析处答 `400`、不读库（依据 ADR-0029：坏输入改了才会好，不能报成可重试）。答复体与该族既有坏请求同形。能共用一处检查就共用（例如 `internal/platform` 的 HTTP 共用件），不在每口各写一份；共用件放哪里照包布局与 architecture 测试。
3. 每个改过的口补 HTTP 用例（NUL、非法 UTF-8 各一，断言 `400` 且不读）；合法多字节 UTF-8（中文、全角、emoji）照收不误拒。

## 不做

- 不改任何口的契约形状、成功答复与既有坏请求的分格。
- 不做长度上限、字符集白名单之类的新校验——只拦库存不下的值。

## 完成判据

- 清点表（口、参数、修前答复、修后答复）写进完成记录。
- Go 受影响包加 `internal/architecture`、`cmd/...` 真库测试过；`go build ./...`、`go vet ./...` 退 0。
- 照「进 main 的步骤」交非作者评审。
