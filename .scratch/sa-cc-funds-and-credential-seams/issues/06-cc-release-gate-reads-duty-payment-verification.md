# 放行门禁核对不读税费付款核对：`VerifyReleaseGate` 的依赖里没有 `DutyVerificationStore`，「税费付款」那一道门禁今天只能由调用方口头交进来

Category: enhancement
Status: in-progress——2026-09-11 16:4x 通道 4 接单 task-18848394，树 `D:/tops/idp-parcel-mcp4-sacc06` 分支 `mcp4-sacc06` 基 main `620f7fed`（04 已进 main 同目录串接）；此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁，CC owner 口径）写入裁决：折法是登记进来的规则（三态各自接受集合，无默认；[ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定三）、门禁记录加一列核对版本引用（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/verify_release_gate.go` 四格结果 `GateVerificationRecorded / Existing / NotAccepted / Undecided` 在；`git grep -n DutyVerificationStore -- internal/customscompliance/application/verify_release_gate.go` **零**——门禁核对不读付款核对。
- 付款核对已有登记面：`ports.DutyVerificationStore`（mech/07 CC-c，`616646d`），`VerifyPayment` 写它。
- mech/07「没做」第 3 条后半：「步 10 门禁核对读付款核对——各一张」。

## 语言从哪里来

- CC `CONTEXT.md`：「放行门禁核对必须绑定当前有效的监管程序、明确申报范围、拟执行动作、适用监管边界、**税费付款核对**、限制、处置和其他前置条件判断。门禁满足不生成放行，也不能复用于其他动作或监管边界；门禁未满足也不能删除已经接收的放行结果。」
- UC-CC-009 范围节第 6 条：「按申报范围、拟执行动作和监管边界形成税费付款、限制、处置和其他前置条件的放行门禁核对，以及 `UC-CC-006` 外部放行回接。」

## 做法

1. `VerifyReleaseGateDeps` 加 `DutyVerifications ports.DutyVerificationStore`（读半边）；门禁核对形成时按（租户、申报范围、监管程序）取**当前**付款核对版本，把覆盖 / 差额 / 有效性三态折成「税费付款」那一道门禁的满足与否，并把核对版本引用记进门禁记录。
2. 没有付款核对 → 那一道门禁**未决**并指名等谁（不是「未满足」也不是「满足」）；三态里任一为「待确认 / 冲突」 → 同样未决。
3. 折法（哪些三态组合算满足）**不在代码里写死**——见「要裁的」第 1 条；裁前只做「有核对 → 记引用，无核对 → 未决」。

## 红线

- 门禁满足不生成放行（CONTEXT）；本票不碰 `receive_external_result.go`。
- 三态不得压成一组互斥总状态（CONTEXT「覆盖状态、差额状态和有效性状态分别表达」）——门禁记录带三态原值 + 引用，不带一个合成布尔。
- 真实程序的付款条件（何种差额可放行）属实例半边 `PAR-CUS-0x`，不写默认。

## 完成判据

1. `VerifyReleaseGate` 有读付款核对的路径：有核对 → 门禁记录带核对版本引用；无核对 → `GateVerificationUndecided` 并指名。
2. 应用层用例覆盖：有 / 无 / 待确认三条。
3. 真库：门禁记录往返带引用列（若要加列，新迁移序号在票面写明）。

## 地盘

`internal/customscompliance/application/verify_release_gate.go`、`internal/customscompliance/domain/`（`ReleaseGateVerification` 若要多一格引用）、`internal/customscompliance/adapters/postgres/`、`migrations/customs_compliance/`（若加列）。

## 要裁的

1. **三态怎么折成一道门禁**：（已覆盖 · 无差额 · 有效）才算满足，还是「超额」也算、「部分覆盖」按真实程序定——CONTEXT 只说分别表达，没说门禁怎么读；真实规则是实例半边，机制半边要裁的是「折法是登记进来的规则（门禁目录一行）还是编排常量」。归 CC owner。
2. **门禁记录要不要多一列核对版本引用**：ADR 层面是「引用还是快照」；本票倾向引用（CC 自己的表，同上下文内引用不违反 ADR-0013）。归 CC owner。

### 裁决

（1 由用户 17:0x 授权、通道 5 按 CC owner 口径代裁并落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定三；2 是 A 类由通道 1 推送方裁；通道 5 写入，2026-09-10 17:2x；task-b941ce87 分类、task-9a2ff746 落笔。拿不准的在 ADR「越权风险点」3 / 4。）

- **1 → 折法是登记进来的规则，不是编排常量；规则挂门禁目录既有登记册（`GateConditionRegistry` 那一族，范围 / 动作 / 边界三维键），这一道的目录行登规则而不登结论性认定。** 规则正文两种形之一：「税费付款不构成本动作在本边界的前置条件」，或三个接受集合——覆盖 ⊆ {无覆盖, 部分覆盖, 已覆盖}、差额 ⊆ {无差额, 不足, 超额}、有效性 ⊆ {有效, 失效}，三态各落在自己的接受集合内才满足。`待确认` / `冲突` 不可登记为接受，三态任一为它们时该道门禁未决并指名；没有付款核对版本同样未决；目录里没有这一道的规则行 → 答「规则未配置」诚实停点，不取任何默认折法。理由：CC CONTEXT「税费支付是否是放行前置条件，取决于当前监管程序的适用规则；本上下文不得统一假设『先税后放』或『先放后税』」与 AGENTS 红线「未确认参数保持可配置或显式未决」同时排除常量；「三态分别表达」→ 门禁记录带三态原值不带合成布尔。规则取值属实例半边 `PAR-CUS-0x`。做法 3「裁前只做有核对记引用 / 无核对未决」由此改为「有规则且有核对 → 按规则折；无规则 → 规则未配置；无核对 → 未决」。
- **2 → 加一列核对版本引用（引用，不快照）。** 同上下文内引用，[ADR-0013](../../../docs/adr/0013-pricing-owns-versioned-external-reference-series.md) 的「引用 vs 快照」判据只约束跨上下文的外部数值序列；本票红线本已写「门禁记录带三态原值 + 引用」。新迁移序号开工时重取、票面写明。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)「没做」第 3 条；UC-CC-009；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ④；`customs-gate-conditions` 读面（`cmd/parcel-api/endpoints.go`，门禁目录已有读口）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 16:4x · 通道 4：接单 task-18848394 开工，基 main `620f7fed`，按「裁决」1 / 2 实现（规则登目录行、门禁记录带三态原值 + 核对版本引用、迁移钉 0019）；本笔只改 Status。
