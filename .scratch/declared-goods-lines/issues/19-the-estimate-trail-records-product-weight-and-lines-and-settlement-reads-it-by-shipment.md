# 19 估价留痕补记产品、计价重量与费用行，结算按委托交出读口

Category: enhancement
Status: in-progress
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`）

依据：[ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 决定一、二；ADR-0179 决定六（被部分停用的那一句）；ADR-0382 决定三（试算评价的输入快照记服务产品）。

## 要做的

- **留痕补记**（`internal/settlementaccounting`）：
  - 计价消费侧适配器 `PreAcceptanceTrials` 从每件试算评价里多留几格：
    - 选卡用的服务产品，取自输入快照，同一份估价里各件相同；
    - 计价重量（值与单位）；
    - 费用行（费用代码、增减方向、金额，最小币单位）。
  - 端口与领域值跟着带上。
  - 留痕表 `pre_acceptance_estimate_trace` 加一列服务产品（可空）；`trials` 的 jsonb 每件加 `pricingWeight` 与 `chargeLines` 两键（可缺）。新迁移，旧行不回填。
  - 重放判等把新格算进去。
- **读口**：`GET /settlement-pre-acceptance-estimates?shipmentRequest=<委托标识>`。走结算的运营读面（同 `/settlement-statement-lines?statement=` 那种按键读），租户与权限同费用、账单读口。答复形状如下，字段名钉死，管理台票 21 照它写：

```json
{"outcome":"<同 /settlement-statement-lines 那类按键读口的约定>",
 "estimates":[{
  "submissionVersion":"SUBV-…","form":"…","pricingScope":"…",
  "priceCard":{"plan":"…","version":"…","fingerprint":"…"},
  "serviceProduct":"…",
  "detailRecorded":true,
  "totalAmount":"1536","currency":"CNY","trialAt":"…","recordedAt":"…",
  "parcels":[{"parcel":"DPCL-…","evaluation":"EVAL-…","totalAmount":"384","currency":"CNY",
    "pricingWeight":{"value":"12.5","unit":"KG"},
    "chargeLines":[{"code":"BASE_FREIGHT","effect":"ADD","amount":"384.00","currency":"CNY"}]}],
  "occupations":[{"kind":"FUNDS_FREEZE","id":"…","status":"HELD","amount":"1536","currency":"CNY",
    "formedAt":"…","releasedAt":"…","convertedAmount":"0","convertedAt":"…"}]
}]}
```

  - `serviceProduct` 没申报或未记即不带。`detailRecorded` 为 `false` 是补记之前的旧行，那时 `pricingWeight` 与 `chargeLines` 都不带。
  - 金额照结算读面：`totalAmount`、`amount`、`convertedAmount` 是最小币单位的整数串（`minorAmount`），配 `currency`。费用行的 `amount` 不同：是评价原样的小数串，币种是那一行自己的（价卡币种，费用行不换算、逐行取整可选）。
  - `occupations` 是数组：组合策略在两本账上各占一笔。`kind` 取 `FUNDS_FREEZE` 或 `CREDIT_EXPOSURE`；`status` 冻结取 `HELD`、`RELEASED`、`CONVERTED`，信用暴露取 `RECORDED`、`RELEASED`、`CONVERTED`。部分扣费仍是 `HELD`/`RECORDED` 并带 `convertedAmount`，`convertedAt` 取最近一次。受限而没有占用即空数组；没发生的时刻不带。
  - 次序按试算时点。没有留痕答空的 `estimates`，不是 404。
  - 价卡字面引用拆成方案、版本与指纹。
  - 补记前记下、补记后重放的同一控制请求，按旧形状那几格比，不答冲突。补记不得挡住估价形成。
- **装配**：`cmd/parcel-api` 端点表挂上它。

## 完成判据

- 真库：补记几格写得进、读得回；旧形状的行读成 `detailRecorded:false`；同一控制请求重放答原结果，改了费用行答冲突。
- 读口：一票两版估价按时点列；冻结、信用暴露、两者都有、受限无占用几种都对；部分扣费；租户隔离；补记前的旧行重放不冲突。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

对客户交出；预估与实际并排（ADR-0379 越权风险点 2）；估价没形成时另存停因。演示段 `55` 的断言另由演示一侧接（随本票验收，见完成记录）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/estimate-trail-read`（工作树 `idp-parcel-goods-t19`，基线 `a73304a3e`）上的 `cf290b2ef`，未推送、未进 main。按协调方重钉的形状做（`outcome`、`occupations` 数组、金额整数串、费用行十进制原文），票面已同步。
- 拣入：在 `feat/goods-details-admin` 上是 `f8af921d7` 与评审修订 `cf9fdb50a`（与票 20 的修订同一笔），拣入时无冲突；最终分支头上的全量门禁见票 18 完成记录。
- 落点：
  - 领域（`domain/pre_acceptance_estimate.go`）：`PreAcceptanceTrial` 加 `WithPricingWeight`、`WithChargeLines`（至少一行）与两个可缺的读取；新值 `PreAcceptanceTrialPricingWeight`（值与单位原文）、`PreAcceptanceTrialChargeLine`（`PricingChargeCode`、`ChargeLineEffect` ADD/DEDUCT、金额十进制原文、`CurrencyCode`）。`PreAcceptanceEstimate` 加 `WithServiceProduct` / `ServiceProduct`。价卡字面的写法与拆法收在 `VersionReferenceLiteral` / `SplitVersionReferenceLiteral` 一处，计价消费侧的 `versionReferenceLiteral` 改调它。
  - 计价消费侧适配器（`adapters/parcelpricing/pre_acceptance_trials.go`）：`serviceProductOf` 取各件输入快照的产品，各件不同按译不了上抛；`withDetail` 补记进位之后的计价重量（`RoundedWeight`）与费用行，任何一格译不出只是那一格不记，费用行一行译不出整份不记，都不报错、不挡估价。
  - 端口：`PreAcceptanceTrialAnswer.ServiceProduct`；伴生读端口 `PreAcceptanceEstimateRead` 与读模型 `PreAcceptanceEstimateEntry`（`Occupations` 冻结在前，状态用各账本自己的词）。
  - 应用：`formEstimate` 在合计成形之后把产品记到估价上。
  - 迁移 `migrations/settlement_accounting/0057_pre_acceptance_estimate_trace_detail.sql`：加 `service_product`（可空、不许空白）与 `detail_recorded`（旧行一律 false，随即去掉默认值，写口必须显式写）；`detail_recorded OR service_product IS NULL`；索引 `pre_acceptance_estimate_trace_by_shipment_request`（租户、委托、试算时点）。
  - 留痕册（`adapters/postgres/pre_acceptance_estimate_trace.go`）：写口写产品、`detail_recorded = true`，`trials` 逐件加 `pricingWeight`、`chargeLines`；重放判等对新行比全部内容，对旧行只比旧那几格（去掉补记两键的 trials，不比产品），旧行不改写。`ListPreAcceptanceEstimates` 按整把键左接 `funds_freeze` 与 `credit_exposure`。
  - 读口 `NewQueryPreAcceptanceEstimatesEndpoint`：`outcome` 为 `PRE_ACCEPTANCE_ESTIMATES_LISTED`，没有留痕也是它、`estimates` 为空数组；`shipmentRequest` 缺在准入之后答 400；读口故障 500 `NO_ANSWER_FORMED`。
  - 装配：`cmd/parcel-api/endpoints_settlement_catalogue.go` 挂 `/settlement-pre-acceptance-estimates`（运营读面，探针 `?shipmentRequest=SYN-PROBE`），读写同一个 `NewPreAcceptanceEstimateTraces`；未接线占位 `unwiredPreAcceptanceEstimates`。
  - 测试种子：`pre_acceptance_conversion_test` 两处直插留痕的行补写 `detail_recorded false`（去默认值之后不写就进不去）。
- 判据：
  - 真库：补记几格写得进、读得回，价卡拆回方案、版本、指纹 ✅；旧形状的行读成 `detailRecorded:false`、无产品无逐件明细 ✅；同一控制请求重放答重放，只改一行费用金额、换产品、去掉产品都答冲突，已落行不动 ✅；补记前的旧行跨迁移重驱答重放、换了评价仍冲突、旧行不被补上 ✅。
  - 读口：一票两版按试算时点（后一版先写）✅；冻结占用中且部分扣费、信用暴露已释放、组合策略两本账都占（冻结在前）、受限无占用空数组 ✅；租户隔离 ✅；没有留痕空列表 ✅；HTTP 层整份答复与钉住的形状逐键比对 ✅。
  - 适配器：申报了产品的委托带回产品、各件计价重量 1 KG 与 BASE ADD 费用行；没申报不带；费用行位数多于币种（12.505，合计按两位取整为 1251）照原文留下 ✅。
  - 全量 `go test`（带 PG）与 `go vet` 通过 ✅。
- 未做：演示段 `55` 改经本读口断言（演示一侧接）；对客户交出；预估与实际并排；SA CONTEXT、CONTEXT-MAP、UC-SA-002 的文档改动（ADR-0379 那一笔的事，不在本票代码里）。
- 与票面的细处：
  - 「计价重量」取进位之后的那一个（评价据它查表），不交派生前或抬高前的值。
  - 为分「补记之前」与「补记之后没申报产品」多加了一列 `detail_recorded`：两者在产品列上都是空，费用行也可能因译不出而整份缺，从别的列推不出来。
  - 价卡字面拆法：种类取第一个冒号之前，指纹取最后一个 `#` 之后，版本取其前最后一个 `@` 之后；拆不开按坏数据报错（500），不猜。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。
- 评审（非作者，2026-11-11）：无阻断；改了以下几处（`1e909e6ae`，同分支一笔，与票 20 的评审修订同一笔，未推送）：
  - 补记不挡估价（ADR-0379 决定一）：`serviceProductOf` 遇各件产品不同时答空、不再报 `ErrUntranslatablePreAcceptanceTrial`；`formEstimate` 里 `WithServiceProduct` 记不上就跳过。上面落点里「各件不同按译不了上抛」一句以此为准。补测：适配器内测（`pre_acceptance_trials_internal_test.go`）各件不同不记产品、每件照样有合计；应用层产品空白时控制照常执行、冻结照常占用、留痕无产品。
  - 重放判等按数比：已补记的行经 `comparableTrials` 比，费用行金额与计价重量值换成 jsonb 数值（jsonb 数值按 numeric 比）；件序与行序照旧算数；不是十进制写法的原文照原文比。评审只点了金额，计价重量值理由相同，一并按数比。补测：金额 12.50 与 12.5、重量 12.5 与 12.50 是重放，两行换序、金额或重量变了是冲突，已落行留首写原文。
  - 补测：同一委托同一提交版本在两个结算作用域（同一账户的 CNY 与 USD）各一份，两份都读回、按试算时点排、凭 `currency` 分得开，冻结只接在自己作用域那一份上。答复不加字段。
  - 不改：`convertedAmount` 没有转出时照发 `"0"`（评审第 5 条），管理台已把 `"0"` 读作没转出。
  - 评审后全量门禁（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432）：`gofmt -l` 无输出、`go vet ./internal/... ./cmd/...`、`go build ./...` 通过；`go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/... -count=1` 退 0（170 个包 ok）。

## 演示断言（2026-11-11）

- 提交：分支 `feat/demo-estimate-read` 上的 `4fc4cd758`（基线 `99a32873c`；工作树 `idp-parcel-goods-t19demo`），未推送、未进 main。只动 `scripts/demo-seeds/demo-run/`（`55-fba-supplement.sh` 与 `README.md`）。
- 拣入：在 `feat/goods-details-admin` 上是 `3708373ba`，另有 `c1154dcbb` 把演示 README 里「管理台还没有页」一句随票 21 改掉。
- 断言：`55` 在售价一节经本读口按委托读回接受前估价，判过的每一版恰一份。
  - 每份核：价卡方案、选卡用的产品、`detailRecorded`、件数、每件有 `pricingWeight` 与非空的 `chargeLines`、各件合计等于整份合计。
  - 占用：演示账户的受理前财务控制策略 `SYN-FIN-CONTROL-01` 先 `PREPAID_FREEZE`、再 `CREDIT_CHECK`，两条都要过，所以每份恰两笔、冻结在前，金额都是整份合计。
    - 被接受的那一版：`FUNDS_FREEZE/HELD`、`CREDIT_EXPOSURE/RECORDED`。收寄形成的预估客户费用不转出它们，`convertedAmount` 为 0。
    - FBA 补充前那一版被新版本换下，两笔都是 `RELEASED`。
- 结果：`up.sh demo_goods12 --reset`（`:19380`）退 0；依序 `55`、`50`、`10-main-chain.sh` 退 0、0、0（31 s、6 s、81 s）。`55` 的估价几行：
  - `接受前估价 FBA 货件 第 1 版 → SUBV-O2542BHU4HWC6D7D7VYNRGLC7M plan=SYN-PLAN-CN-SG-FBA-01 product=SYN-PROD-CN-SG-FBA total=1536.00 CNY（4 件，各件计价重量与费用行齐） FUNDS_FREEZE/RELEASED CREDIT_EXPOSURE/RELEASED`
  - `接受前估价 FBA 货件 第 2 版 → SUBV-MOVMXKLLDS7EKWTSBOXEBAEGKY plan=SYN-PLAN-CN-SG-FBA-01 product=SYN-PROD-CN-SG-FBA total=1536.00 CNY（4 件，各件计价重量与费用行齐） FUNDS_FREEZE/HELD CREDIT_EXPOSURE/RECORDED`
  - `接受前估价 快递小包 第 1 版 → SUBV-NK6DJGNVHKLB3QBQCBWURQHDQM plan=SYN-PLAN-CN-SG-01 product=SYN-PROD-CN-SG-EXPRESS total=78.00 CNY（1 件，各件计价重量与费用行齐） FUNDS_FREEZE/HELD CREDIT_EXPOSURE/RECORDED`
- 读回的明细：FBA 每箱 `pricingWeight` 16 KG、一行 `BASE_FREIGHT ADD 384`；快递小包 1 KG、`BASE_FREIGHT 73` 加 `REMOTE_AREA_SURCHARGE 5`。
- 读种子数据的 Go 测试包（同票 17 那一串，带 PG）全部 ok。`down.sh` 已收，库 `demo_goods12` 保留。
- 去掉的说法：`55` 头注与演示 README 里「接受前估价没有读口、本段不断言」。演示 README 改写为经本读口逐版读回，并注明管理台还没有这页。票 17 记录里的同一条在文档工作树，不在 README 里，没动。
- 证据记 `S`，参数登记册不动。

