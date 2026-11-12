# 20 委托详情交出受理判断记下的财务控制结论

Category: enhancement
Status: resolved——2026-11-12 进 main
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`）

依据：[ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 决定三；PS CONTEXT（小包托运不拥有金额、余额、冻结）。

## 要做的

- `/shipment-request-views` 的委托详情（`requestDetailBody`）加一格 `financialControl`：最近一次受理判断记下的接受前财务控制采用结果。形状与审核队列 `recordedJudgments.financialControl`（`reviewFinancialControlBody`）相同：outcome、resultId、basis、asOf 各格、jointPassCondition、occupationFormed、items（kind、order、conclusion、basis，以及受限项上的失败处置与责任）。
- 没判过即不带这一格。
- 列表行不带。
- 不带任何金额。
- 读的是审核队列已在用的那份受理判断记录，不另立读法。字段名钉死，管理台票 21 照它写。

## 完成判据

结论 `HELD`、`CREDIT_EXPOSED`、`RESTRICTED`（可用余额不足）、`NOT_APPLICABLE`（合同约定无控制，没有各项与条件）与没判过，在委托详情上各对；与审核队列同一形状（共用一个编码函数）；全量 `go test`（带 PG）通过。

## 不做的

金额（归结算，票 19）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/estimate-trail-read`（工作树 `idp-parcel-goods-t19`，基线 `a73304a3e`）上的 `17e239209`，叠在票 19 的 `cf290b2ef` 之上，未推送、未进 main。两笔不相依赖，可分开拣。
- 拣入：在 `feat/goods-details-admin` 上是 `99a32873c` 与评审修订 `cf9fdb50a`，拣入时无冲突。
- 落点：
  - 编码：财务控制那一段从 `recordedJudgmentsBodyOf` 抽成 `financialControlBodyOf`（新文件 `adapters/http/financial_control_body.go`），类型改名 `financialControlBody` / `financialControlItemBody`，JSON 键一个不动；零值答 nil、整键缺席。
  - 读模型：`ports.ShipmentRequestDetailRecord` 加 `FinancialControl domain.FinancialControlResult`，零值即没判过（与 `RecordedJudgments` 同一约定）。`ShipmentRequestViews.FindVisibleByID` 按当前提交版本调 `LoadRecordedJudgments` 用的同一个 `loadFinancialControl`，不另写 SQL。列表与按号、按标识集合的列表行都不读。
  - 委托详情：`requestDetailBody.FinancialControl`（`financialControl`，omitempty），`detailBodyOf` 经 `financialControlBodyOf` 填。
  - 复核详情：`request` 也复用 `detailBodyOf`，所以复核详情上 `request.financialControl` 与 `recordedJudgments.financialControl` 同在、逐字相同；`reviewCaseResponse` 的注释写明这一处例外。
- 判据：
  - HTTP：`HELD`、`CREDIT_EXPOSED`、`RESTRICTED`（`AVAILABLE_BALANCE_INSUFFICIENT`，结论与项上的依据都在）、`NOT_APPLICABLE`（合同依据在、没有条件、`items` 为空数组）四种在委托详情上各对，与复核详情 `recordedJudgments.financialControl` 逐字节相同 ✅；不带金额 ✅；没判过整键缺席 ✅；列表行不带 ✅。
  - 真库：四种结论与没判过照登记读回；同一版先受限后放行取最近那次；与 `LoadRecordedJudgments` 的财务控制一格逐字段相同 ✅。
  - 全量 `go test`（带 PG）通过 ✅。
- 未做：金额（票 19）；管理台（票 21）。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
- 评审（非作者，2026-11-11）：无阻断；改一处（`1e909e6ae`，与票 19 的评审修订同一笔，未推送）：
  - 复核详情只留一份财务控制：`serveReviewCase` 清掉 `request.financialControl`，复核页读 `recordedJudgments` 那一格。`FindVisibleByID` 与 `LoadRecordedJudgments` 是两条语句，两读之间记下一次判断会让两格不同。`reviewCaseResponse` 注释改写；上面落点里「复核详情上两格同在」一句以此为准。补测：读模型上带着结论，复核详情的 `request` 也不交，`recordedJudgments.financialControl` 仍与委托详情逐字节相同。
  - 评审后全量门禁（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
## 进 main 记录

- 2026-11-12 随 declared-goods-lines 链进 main，main 尖 `c5001b3de`；重放、改号（分支 0378、0380 落地为 ADR-0382、ADR-0383）、生成物与门禁的完整记录见 [25 号票](./25-an-incomplete-declaration-for-the-estimate-waits-for-customer-supplement.md)「进 main 记录」。
