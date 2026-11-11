# 15 申报的服务产品进售价卡选卡：接受前估价、正式 SELL 评价与试算

Category: enhancement
Status: ready-for-agent
Blocked by: 14

依据：[ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定二、三；ADR-0179 决定三（两处同一笔改）；ADR-0260 决定一（计价输入快照不带商业依据）。

## 要做的

- **接受前估价**：
  - 小包托运的 `SubmissionVersionDeclaration` 交出这一版申报的服务产品（可缺）。
  - 计价的 `ports.SubmissionDeclaration` 加同一格，适配器照搬。
  - `FormPreAcceptanceEstimatesHandler` 先读申报、再带产品选卡。改写「先选卡后读申报」那句注释。
- **正式 SELL 评价**：
  - 小包托运立一个读口：按（租户，包裹）答覆盖它的已接受委托申报的服务产品；没有已接受委托覆盖或没申报，答没有产品。
  - 计价在 `adapters/parcelshipment` 接它。
  - `FormEvaluationFromRequestHandler` 在 `SELL` 路上先读产品、再选卡；`BUY` 路照旧传零值。
  - 结算的评价请求不带产品，结算不改。
- **试算**：
  - `POST /pricing-estimates` 的载荷加可缺的 `serviceProduct`（字符串）。
  - `FormEstimateEvaluationsCommand` 带上它，传进 `LoadApplicable` 与冲突候选的解析。
- **快照**：
  - 计价输入快照记选卡用的服务产品，作可缺的一格（文档键 `serviceProduct`，不进评价的规范化文档，照 `addressNature` 的做法）。
  - 评价解释与评价查阅交出它。
- **装配**：`cmd/parcel-dispatch` 与 `cmd/parcel-api` 接上新读口。

## 完成判据

- 同一计价范围登一张不声明的卡与一张指名 FBA 产品的卡：
  - 申报 FBA 的委托，接受前估价与正式 SELL 评价都用指名卡；
  - 申报快递或没申报的，用不声明的卡；
  - 快照上记着产品。
- 试算带产品与不带产品各自选对卡。
- 只有指名卡、委托没申报产品：估价停在未配置，不挑卡。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

管理台（票 16）；演示（票 17）。
