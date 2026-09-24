# 05 试算后端：计价试算编排、补齐读数共用件与 `POST /pricing-estimates`

Category: enhancement
Status: in-progress——2026-09-25 通道 3 认领，隔离树 idp-parcel-mcp3-est05、分支 mcp3-est05，基线 03509b46
Blocked by: 无
地盘：`internal/parcelpricing/application`（新试算编排；把 `EvaluatePricingHandler` 的补齐读数两步抽成共用件）、`internal/parcelpricing/adapters/http`
（新端点、载荷解码与响应）、`cmd/parcel-api/endpoints.go` 与其测试（端点表一行，**改前占号**）、机制清点重生成。
出处：[02 的裁决](./02-estimate-evaluation-entry.md)；行为以 [UC-PP-001](../../../docs/application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md) 为准，
取舍以 [ADR-0152](../../../docs/adr/0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md) 为准。

## 要做的

1. 补齐读数共用件：`completeSeriesReadings` / `completeCatalogueReadings` 两步抽出，`EvaluatePricingHandler` 改用它，既有测试零改动、行为一格不变。
2. 试算编排：受理 → `LoadApplicable` 取全部适用价卡 → 逐卡按目录绑定选分区给法造试算输入（确定性派生试算对象引用与评价标识，证据 `S`）→
   共用件补齐读数 → `domain.EvaluatePricing`。依赖里**没有**评价库写口与 `EvaluationHandoff`（ADR-0152 决定二的结构保证）。
3. 端点 `POST /pricing-estimates`：严格解码载荷（未知键拒，含任何身份格）；答案代数照 ADR-0152 决定七；状态码照 ADR-0022。
4. `parcel-api` 端点表一行，装配 `UnconfiguredIntake{}`；端点表测试随之；机制清点在干净检出上重生成、随笔提。

## 不做

- 真操作者 Intake（随 operator-channel/06 同批）；管理台页（票 06）；入册、交付、择优、报价（ADR-0152 决定八）。

## 完成判据

- UC-PP-001 验收示例 1–7 各有测试钉住（编排层用替身，端点层钉答案代数与状态码，结构保证用编译期断言或依赖形状测试）。
- `EvaluatePricingHandler` 既有测试零改动全绿。
- `go build ./...`、`go vet ./...` 全仓过；受影响包及其反向依赖 `go test -count=1`，`cmd/*` 带 DSN（parallel-sessions「全量只跑一次」一节）。
- 非作者评审（parallel-sessions「合入前独立评审」）；无空闲通道时推送方自审并如实写明。

## 形态

碰 Go 与 `cmd/parcel-api`，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路：隔离工作树与分支、改 `endpoints.go` 前占号、每笔提交后推分支。
