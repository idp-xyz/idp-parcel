# 安全交接评估有工厂无交接：`Other` 权威下编排只答阻断，从不「交给该权威」

Category: enhancement
Status: draft——只读取证（MCP-6，锚 `2efef58e`），PS 地盘归 MCP-2；交 MCP-1 派
Blocked by: 本票「要先裁的一格」——治理接管记录与运行时交接确认是不是两件都要（`/domain-modeling`，可能落 ADR）

## 条目

`internal/parcelshipment/domain AssessSafeHandoff`（`production_handoff.go`）。基线理由行：「产出带校验的安全交接评估，测试里调 9 次，包外零引用」。

## 它是什么

`AssessSafeHandoff(spec)` 按一次**交接尝试**的观察结果形成 `SafeHandoffAssessment`：观察代数五格（完整确认 / 部分确认 / 超时 / 查询不可用 / 失败），只有完整确认且范围摘要对得上才是 `SafeHandoffConfirmed` 并带确认引用与生效时刻；其余一律 `SafeHandoffUnresolved` 带原因，且**未决必带续办引用**（构造期拒绝没有续办引用的未决）。这是 UC-PS-001 步 3B 那句「其他权威时安全交接并返回渠道中立关联……权威或交接无法确定时保持生产归属未决」的领域半边。

## 该有的调用方

- UC-PS-001 步 3B「试点准入控制」：「只有本产品是当前唯一权威……才返回未来建单可放行；**其他权威时安全交接并返回渠道中立关联**；……权威或交接无法确定时保持生产归属未决」。
- 结果行「非本产品生产归属」要携带「当前权威方和**安全交接结果**」；结果行「生产归属未决」要携带「**安全续办引用**」。
- `AT-PS-010`：「已明确其他当前权威且可安全交接时**交给该权威**并返回渠道中立关联，否则保持生产归属未决」。`BD-PS-004` 同指。

所以调用方是提交编排在生产归属决定为 `ProductionAuthorityOther` 之后的那一步：把完整拟受理范围投递给那个权威、观察确认、形成评估——已确认才答「非本产品归属结束」并返渠道中立关联，未决落「生产归属未决」带续办引用。

## 今天的样子

- `internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 的 `resolved`：命中他方权威区间时读治理侧**接管记录**（`Handoffs.FindByInterval` → `takeover.StopEvidence()` 装进 `ProductionOwnershipDecisionSpec.HandoffRef`），取不到即 `OwnershipUnresolvedHandoffIncomplete` / 没装接管读口即 `OwnershipUnresolvedHandoffUnavailable`。注释写明这是「原权威停止写入的证据」——它回答的是**归属**（谁是权威、前任停笔了没有），不是一次交接的确认。
- `internal/parcelshipment/application/submit_shipment_request.go`：`DecideProductionOwnership` 之后过 `EvaluateFutureSubmissionGate`，不放行就 `blockedOutcome(decision)` 直接返回。**没有任何一步把范围交给那个权威**，也没有观察确认；`SafeHandoffAssessment` 在 `application/`、`ports/`、`adapters/` 的非测试代码里零引用。
- 出向：没有面向他方生产权威的端口——`HandoffObservation` 那五格正是它该有的答复代数，而没有接口声明它。

## 三分

**支路未接**。缺三层：

1. **出向端口**（`ports/`）：向他方权威投递范围、取回确认或查询结果，答复落 `HandoffObservation` 五格；生产装配里放未配置适配器（未配置即答 `HandoffObservationQueryUnavailable` → 归属未决 `HandoffUnavailable`，与 ADR-0055 那套「未配置即拒」同形，不冒充成功）。
2. **编排步**（`application/submit_shipment_request.go` 的 `Other` 分支，或拆成独立用例）：投递 → 观察 → `AssessSafeHandoff` → 已确认答「非本产品归属结束」返渠道中立关联；未决落「生产归属未决」带 `ContinuationRef`。
3. **决定记录**：UC 结果行要求携带安全交接结果 / 安全续办引用；今天 `ProductionOwnershipDecision` 只有 `HandoffRef` 一格（装的是停写证据），评估结果没有落点。

## 能不能归到已认可的留待

不能整条归。目标系统的协议、身份与交接证据是 `PAR-GOV-05..07` 实例半边（不在 r27 认可留待五项之列，但性质是实例）；端口、编排、评估、未配置适配器是机制半边，现在能做且不填任何实例值。

## 要先裁的一格

治理接管记录（停写证据，今天的路）与运行时交接确认（`AssessSafeHandoff` 建模的路）是**两种证据**。UC 3B 与 `AT-PS-010` 读起来是「已明确其他权威」（归属）**且**「可安全交接」（交接）两件，缺一都不许交；但接管记录先于投递还是投递确认可替代接管记录，CONTEXT 没有硬句。建议 `/domain-modeling` 一格先定：两者的先后、缺任一格时落哪种未决原因、决定记录上各占哪一格。可能落 ADR（预留号已尽，向 MCP-1 取号）。

## 完成判据（落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

1. `ports/` 有面向他方生产权威的出向端口，答复落 `HandoffObservation` 五格；生产装配放未配置适配器，未配置即答 `HandoffObservationQueryUnavailable`（不冒充成功）。
2. 提交编排的 `ProductionAuthorityOther` 分支真调 `AssessSafeHandoff`：已确认答「非本产品归属结束」并返渠道中立关联，未决落「生产归属未决」带 `ContinuationRef`。
3. `ProductionOwnershipDecision` 上有评估结果与续办引用的落点，与 `HandoffRef`（停写证据）分格。
4. 剪基线行：先按头注三分成因（全仓 `AssessSafeHandoff` 只此一处声明才是第二种），在自己那笔的干净检出上两法同得记数、钉 SHA。
5. **若 1–3 之前先要改理由行**（今天那句「测试里调 9 次，包外零引用」既无调用方又带计数），改成：

   > 安全交接评估门，UC-PS-001 步 3B「其他权威时安全交接并返回渠道中立关联」与 `AT-PS-010` 的领域半边。**调用方是提交编排在生产归属决定为 `ProductionAuthorityOther` 之后的交接步**（投递范围 → 观察确认 → 形成评估）；那一层今天缺出向端口、编排步与决定记录落点三件，三件同票（wiring-baseline-remainder/01）落地那天这一条出名单。别把 `ProductionOwnershipDecision.HandoffRef` 读成它——那是治理接管的停写证据，不是一次交接的确认。

## 边界

- 本票不改代码、不改基线。基线行剪掉的时刻是编排步真调 `AssessSafeHandoff` 那一笔，剪时按头注纪律记数。
- 不碰 pilotgovernance 的接管记录语义——那是 PN-01 的记录能力，本票只是它的消费方。
