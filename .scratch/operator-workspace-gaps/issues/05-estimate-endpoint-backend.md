# 05 试算后端：计价试算编排、补齐读数共用件与 `POST /pricing-estimates`

Category: enhancement
Status: resolved——2026-09-25 通道 3 在隔离分支上做完、重放进 main，完成记录见文末（推送方自审，不算非作者评审）
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

## Comments

- 评审 ← 推送方自审（通道 3）· 钉分支 tip `4d938864` · 01:2x：01:28 在频道点名要非作者评审、截止 01:38 无应答，用户 01:4x 答「下一步的工作都你自己完成，没其他人了」。
  **推送方自审，不算非作者评审。** 主会话串行两轴：Standards——注释里数别处封闭集的计数（「状态五格」「四口」，AGENTS「改文档」计数与行号同构）、
  端点测试合计断言同义反复，已修；`pairedPurpose` 在应用层列出计算目的常量一条按判断保留（配对仍由领域 `PairsWithDirection` 一处判，CONTEXT
  写明取值集合与配对改动时一并重新确认）。Spec——验收示例 2（多卡并列）只半覆盖，已补。无阻断。

## 完成记录（2026-09-25 · 通道 3）

**落点**（`main` 上的 SHA；分支 `mcp3-est05` 上的 SHA 作封存出处并列）：`b3ffb066`（`17221c6c`）认领；`67022252`（`5f28e80e`）补齐读数共用件；
`47142a8a`（`31ce3707`）端口哨兵；`df479988`（`54032e35`）试算编排；`01c1c5a4`（`70dfba23`）端点；`e417d64d`（`f594605c`）`parcel-api` 接线；
`a9878a67`（`4d938864`）自审修复；`a9cccfec` 机制清点——分支上那笔 `f23bc51b` 重放时跳过，因 main 已前进，在重放后的干净检出上重生成。
重放基于 `d7e8ec71`（其间 main 前进的四笔是 operator-channel/15 之一，与本票只在清点上重叠）。

**判据逐条**

- ✅ UC-PP-001 验收示例 1–7：编排层 `form_estimate_evaluations_test.go`（单卡、多卡并列、缺邮编路线 / 缺分区、没有卡、同一方案两版重叠、未决两口、
  受理门、共用补齐、确定性、依赖形状、构造期拒法），端点层 `estimate_evaluations_test.go`（答案代数、状态码、严格解码拒身份格）。
- ✅ `EvaluatePricingHandler` 既有用例零改动全绿（重构后 application 包原有用例照跑）。
- ✅ `go build ./...`、`go vet ./...` 全仓；作者侧带 DSN `-p 1` 跑受影响包与 `go list` 反查出的反向依赖（`cmd/parcel-api` 124 PASS / 0 SKIP 等）。
- ✅ 推送方全量：在重放后的 tip `a9cccfec` 上 `gofmt` 净、build / vet 过，带 DSN `go test -p 1 -count=1 ./...` 121 包 `ok`、0 `FAIL`。
- ◑ 非作者评审：无空闲通道，推送方自审（见 Comments），如实不算。

**未验**：真操作者 Intake 路径（端点今天挂 `UnconfiguredIntake{}`，随 operator-channel/06 与回放同批换）；管理台页（票 06）。
