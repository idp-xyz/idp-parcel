# 19 估价留痕补记产品、计价重量与费用行，结算按委托交出读口

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 决定一、二；ADR-0179 决定六（被部分停用的那一句）；ADR-0378 决定三（试算评价的输入快照记服务产品）。

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
{"estimates":[{
  "submissionVersion":"SUBV-…","form":"…","pricingScope":"…",
  "priceCard":{"plan":"…","version":"…","fingerprint":"…"},
  "serviceProduct":"…",
  "detailRecorded":true,
  "totalMinor":1536,"currency":"CNY","trialAt":"…","recordedAt":"…",
  "parcels":[{"parcel":"DPCL-…","evaluation":"EVAL-…","totalMinor":384,
    "pricingWeight":{"value":"12.5","unit":"KG"},
    "chargeLines":[{"code":"BASE_FREIGHT","effect":"ADD","amountMinor":38400}]}],
  "occupation":{"kind":"FUNDS_FREEZE","id":"…","status":"HELD","amountMinor":1536,"currency":"CNY",
    "formedAt":"…","releasedAt":"…","convertedMinor":0,"convertedAt":"…"}
}]}
```

  - `serviceProduct` 没申报或未记即不带。`detailRecorded` 为 `false` 是补记之前的旧行，那时 `pricingWeight` 与 `chargeLines` 都不带。
  - `occupation.kind` 取 `FUNDS_FREEZE` 或 `CREDIT_EXPOSURE`，`status` 取 `HELD`、`RELEASED` 或 `CONVERTED`。没有占用（受限）即不带这一格；释放、转扣的时刻没有即不带。
  - 次序按试算时点。没有留痕答 `{"estimates":[]}`，不是 404。
  - 价卡字面引用拆成方案、版本与指纹。
- **装配**：`cmd/parcel-api` 端点表挂上它。

## 完成判据

- 真库：补记几格写得进、读得回；旧形状的行读成 `detailRecorded:false`；同一控制请求重放答原结果，改了费用行答冲突。
- 读口：一票两版估价按时点列；冻结、信用暴露、受限无占用三种都对；租户隔离。
- 全量 `go test`（带 PG）与 `go vet` 通过。

## 不做的

对客户交出；预估与实际并排（ADR-0379 越权风险点 2）；估价没形成时另存停因。演示段 `55` 的断言另由演示一侧接（随本票验收，见完成记录）。
