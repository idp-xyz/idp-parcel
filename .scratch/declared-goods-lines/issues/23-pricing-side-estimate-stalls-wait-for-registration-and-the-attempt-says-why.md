# 23 价卡一侧答不出的估价等运营登记、计价参数登记后续办；处理尝试记下原因与产品，委托详情从落处读

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0380](../../../docs/adr/0380-an-estimate-the-price-card-side-cannot-form-waits-for-operator-registration-and-the-attempt-records-why.md) 决定一至四；ADR-0094 决定三至五（等运营登记的落库、续办触发同笔落地、先存等待）；ADR-0229（计价「参数已登记」信封）。

## 要做的

- **分法**：小包托运的结算消费侧适配器（`adapters/settlementaccounting/pre_acceptance_control.go` 的 `notFormedOutcomeFor` 一带）把下列结算原因改归「未配置、等运营登记」：
  - `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`
  - `ESTIMATE_PRICE_CARD_CONFLICT`
  - `ESTIMATE_TRIAL_WITHOUT_TOTAL`
  - `ESTIMATE_CURRENCY_MISMATCH`
  - `ESTIMATE_NOT_POSITIVE`
  - `ESTIMATE_TRIAL_NEGATIVE`

  其余原因照旧。受理判断沿既有的等运营登记那一路先存等待、处理尝试同笔落库。
- **续办**：
  - 新 inbox 消费门接收 `parcel-pricing.pricing-parameter.registered`。消费方自己写这个串，组合根的对照用例钉住两串。
  - 只认 `SELL` 方向；按租户重驱停在等运营登记的委托，走与结算账户、估价采用登记续办同一个重驱口。
  - 在 `cmd/parcel-dispatch` 装配，并补进路由与消费者名的登记。
- **处理尝试另记两格**：
  - 结算的未形成原因码（如 `ESTIMATE_PRICE_CARD_NOT_CONFIGURED`，不加 `SA-` 前缀，另一格就是「来自结算」）；
  - 这一版申报的服务产品（取小包托运自己的提交版本，没申报不记）。
  - 迁移给 `parcel_shipment.acceptance_processing_attempt` 加两列（可空），旧行不回填。
- **委托详情**：
  - `acceptanceTask` 的 `lastAttemptReason`、`lastAttemptContinuation`、`lastAttemptedAt` 改从 `acceptance_processing_attempt` 读这份委托当前任务最近一条。
  - 另交 `lastAttemptSettlementReason` 与 `lastAttemptServiceProduct`（可缺）。字段名钉死，管理台票 24 照它写。
  - 聚合文档里的处理尝试列表不再当读源。它没有生产写入；能删就删，删不动的说明为何留着。

## 完成判据

- 真库：
  - 没有在用价卡的估价 → 委托停在等运营登记，处理尝试落库、带原因码与服务产品；
  - 登一张售价卡（交「参数已登记」）→ 委托被重驱、解开后接受；
  - `BUY` 方向的信封不重驱。
- 估价采用登记后重驱到「没有在用价卡」的那一路不再回滚：新的等待落库，上一次「未配置」的尝试被新的一条取代为最近一条。
- 依赖读不回照旧内部重试、不落库。
- 委托详情的「上次处理」在等运营登记时有值，并带两格。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

申报不全改等客户补充（票 25）；按原因或计价范围收窄重驱（ADR-0380 越权风险点 2）。
