# 15 申报的服务产品进售价卡选卡：接受前估价、正式 SELL 评价与试算

Category: enhancement
Status: in-progress
Blocked by: 14（已提交，未进 main；同在分支 `feat/goods-details-admin`）

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
  - `FormEstimateEvaluationsCommand` 带上它，传进 `LoadApplicable` 与冲突候选的解析；试算层内照旧不同方案并列（票 14）。
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

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/price-card-service-products`（工作树 `idp-parcel-goods-t14`）上的 `68a370754`，叠在票 14 的 `7ab94058d` 之上，与评审修订 `6622695b0`；未推送、未进 main（协调方已把 `68a370754` 拣入 `feat/goods-details-admin`）。
- 拣入：在 `feat/goods-details-admin` 上是 `f0412d370` 与评审修订 `8a675c5e2`，拣入时无冲突；拣入后带 PG 全量 `go test` 通过。
- 落点：
  - 小包托运：`SubmissionVersionDeclaration.RequestedServiceProduct` 交出这一版申报的产品。新读口 `ports.AcceptedServiceProductView`（`LoadAcceptedServiceProduct`，按租户与包裹），答案由聚合上的 `ShipmentRequest.AcceptedServiceProductFor` 给：取接受基线那一版提交的申报；没接受、不在基线成员里、没申报都答零值。postgres 实现复用 `findAcceptedRequestCoveringParcel`，不加列。
  - 计价消费侧（ADR-0025）：`ports.SubmissionDeclaration.ServiceProduct`，`SubmissionDeclarations` 照搬；`ports.AcceptedServiceProductView` 与适配器 `AcceptedServiceProducts`。小包托运原样保全的值若写不成产品引用（首尾空白），任何卡都指名不了它，折成零值，与没申报落在同一层。
  - 接受前估价：`FormPreAcceptanceEstimatesHandler` 改为先读申报、再按 `declaration.ServiceProduct` 选卡；「先选卡后读申报」那句注释改写。委托不可见与申报读不回都停在选卡之前；结果在这两格不再带价卡引用（结算一侧照旧，不读它）。
  - 正式 SELL 评价：`FormEvaluationFromRequestHandler` 新依赖 `ServiceProducts`（必备），SELL 路在选卡前按 `Sources.Parcel` 读产品，读不回答未决 `SERVICE_PRODUCT_UNAVAILABLE`；BUY 路不问、传零值。结算的评价请求不带产品，结算不改。
  - 试算：`EstimatePayload.serviceProduct`（可缺，写不成产品引用答 400），`FormEstimateEvaluationsCommand.ServiceProduct` 进 `LoadApplicable` 与冲突候选的 `ResolveInForce`。
  - 快照：`PricingInputSnapshot.WithServiceProduct` / `ServiceProduct`，文档键 `serviceProduct`，不进评价的规范化文档（照 `addressNature`）。评价解释首行写「price card selected for declared service product …」；试算答复的评价体交出 `serviceProduct`。计价这一侧没有入册评价的详情读口，评价查阅面今天只有试算答复这一处（评价目录行只是列面）。
  - 产品不进试算标识（`estimateIdentity`）：同一声明同一张卡的标识不变，受理前估价留痕（ADR-0179 决定六）对得上复算。
  - 装配：`cmd/parcel-dispatch` 的 `formEvaluationFromRequest` 接上新读口（BUY、SELL 两路同装）；受理前估价经既有 `SubmissionDeclarations` 自带产品。`cmd/parcel-api` 只装试算，不需要新读口，没有改动。
- 判据：
  - 真库（`service_product_selection_paths_test`）：同一计价范围一张不声明的卡、一张指名 FBA 的卡——申报 FBA 的，接受前估价、正式 SELL 评价、试算都用指名卡；申报快递或没申报的用不声明的卡；正式评价与试算的快照记着产品 ✅。
  - 接受前估价快照记着产品、选卡用的是申报的产品（应用层测试）✅。
  - 试算带产品与不带产品各自选对卡 ✅。
  - 只有指名卡、委托没申报：估价停在未配置、不挑卡（正式评价与试算同答）✅。
  - 小包托运读口真库：已接受委托申报的产品、没申报、仅已提交、他租户、从未声明的包裹 ✅。
- 未做：管理台（票 16）；演示（票 17）。协调方更正四：SELL 计价照旧逐包裹，没有按票的 SELL 评价。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
- 评审（非作者，2026-11-11）：无阻断；该改一处、细处若干，已改（`6622695b0`，同分支一笔，自足可拣）：
  - 该改：试算方向不是 SELL 却带 `serviceProduct`，原先照收、快照与解释会说卡是按产品选的，违 ADR-0378 决定一。现在 `FormEstimateEvaluationsCommand.accepted` 拒（未受理，卡集一次都不装），HTTP 载荷在 `serviceProduct` 一格答 400。
  - `AcceptedServiceProductFor` 在接受基线指向的提交版本缺席时报 `ErrInvalidAcceptedServiceProduct`，与 `AddressElementsFor`、`DeclaredMeasurementFor` 同一处置，不再答零值；postgres 读口照报 error。这一格没有专门的测试，前例那两处同样没有，造不出基线指向缺席版本的聚合。
  - 正式评价记不上快照产品改报 error（结构上到不了），不再折成 `RequestEvaluationNotAccepted`——那会让结算消费侧说成「译出的命令不被受理」。
  - `FormEvaluationFromRequestResult.ServiceProduct` 在未配置与适用冲突两格交回选卡用的产品；结算未形成交接的说明点名「for declared service product …」，没按产品选时说明与此前逐字相同。
  - 评审后全量门禁：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；带 PG 的 `go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。

