# 安全交接评估有工厂无交接：`Other` 权威下编排只答阻断，从不「交给该权威」

Category: enhancement
Status: in-progress——2026-09-09 13:0x 通道 2 认领（通道 1 派单 task-0ad35a6d；分支 `mcp2-wbr01`，树 `D:/tops/idp-parcel-mcp2-wbr01`，基 main `74ef0da8`；ADR 号 0128 由派单给；PS 迁移若要用 **0021**，0020 已预给 wbr/08）。此前 ready-for-agent——「要先裁的一格」已由通道 1 代裁（2026-09-09，用户经 IDP 队列授权「你自决，目标是全部解决」），裁决见下「裁决」节；实施者按裁决落 ADR-0128（号由通道 1 给）与三层代码。此前 draft——只读取证（MCP-6，锚 `2efef58e`），PS 地盘归 MCP-2；交 MCP-1 派
Blocked by: 无（裁决已落；ADR-0128 由本票实施者按「裁决」节落文，不另等）

## 裁决（通道 1 代裁，2026-09-09；owner 授权自决口径：硬句不改、每个决定写理由、拿不准的点单列越权风险点）

**两件都要，先后固定：归属先定、交接后验；两件各占决定记录上自己的一格。**

1. **治理接管记录（停写证据）与运行时交接确认（`AssessSafeHandoff`）是两种证据，缺一不许交。** UC-PS-001 步 3B 与 `AT-PS-010` 的字面是
   「已明确其他当前权威」**且**「可安全交接」——前半答「谁是权威、前任停笔了没有」，后半答「这一次拟受理范围交过去了没有、对方确认了没有」。
   接管记录不能替代交接确认：它证明的是治理侧的授权状态，不证明这一笔范围到了对方手里；交接确认也不能替代接管记录：没有治理侧的停写证据，
   「其他权威」本身就没立住，投递出去的是一笔两边都可能写的范围（`PAR-GOV-05..07` 要防的正是双写）。
2. **先后：接管记录在前。** 归属决定为 `ProductionAuthorityOther` 必须先凭接管记录成立（今天 `resolved` 的路不动）；成立之后才进入交接步（投递 →
   观察 → `AssessSafeHandoff`）。理由：投递是对外动作、不可撤，只在归属已定时做；反过来「先投递、拿确认当归属证据」会让一个没有停写证据的
   权威凭一次应答成为权威，那是 ADR-0027 那族「解析标识 / 应答不得成为能力凭证」的反面。
3. **缺格落哪种未决**：接管记录取不到 → 仍是今天的 `OwnershipUnresolvedHandoffIncomplete` / `HandoffUnavailable`（归属未决，不投递）；接管记录在、
   交接观察不是完整确认 → 归属决定仍是 `Other`，结果行落「生产归属未决」，未决原因取 `SafeHandoffAssessment` 的原因格（部分确认 / 超时 /
   查询不可用 / 失败 / 通道未配置），续办引用取评估的 `ContinuationRef`；出向端口未配置 → 观察答 `HandoffObservationChannelUnconfigured`（自成一格）→
   同一条未决路，不冒充成功（ADR-0055 同形）。**此句 2026-09-09 14:18 由通道 1 改裁 B**：原句「观察答 `HandoffObservationQueryUnavailable`」与代码相悖——
   `validHandoffEvidence` 对`查询不可用`要求 `ConfirmedScopeDigest` + `ConfirmationRef` 在场，语义是「对方已确认但确认无法查询」；未配置根本没投递，塞进去
   要么放宽证据规则折叠两种恢复动作、要么编造引用，两条都不许 → 未配置自成一格，其余五格一字不改（ADR-0128 决定三，越权风险点 ③）。
4. **决定记录分格**：`ProductionOwnershipDecision` 上 `HandoffRef`（停写证据）保持原义不改名；新加评估结果一格（已确认带确认引用 + 生效时刻；未决带原因 +
   续办引用）。两格不合并——它们是两种证据，合并会让「有停写证据但交接失败」与「无停写证据」在记录上不可分。
5. **ADR-0128 落文**：本裁决是跨 PS 与 pilotgovernance 读口的协议取舍，且改结果行的形，按 AGENTS「难逆转技术或产品取舍 → 新 ADR」落
   `docs/adr/0128-*.md`（Status 行照 0126 / 0127 的「owner 授权自决」写法，越权风险点单列），README 加一行。

**越权风险点（单列，供 owner 复核）**：① 「接管记录在前」是从 `PAR-GOV-05..07` 防双写的目的推的，UC 3B 字面只写「且」没写先后；② 未决原因直接
沿用 `SafeHandoffAssessment` 的观察代数作为结果行的原因词，没有另立结果词表——若 UC 结果行要求的「安全续办引用」之外还要区分原因，那是另一格；
③ 观察代数五格改六格：实施时发现裁决 3「未配置 → `QueryUnavailable`」与代码的证据规则相悖，通道 1 改裁为未配置自成一格（`HandoffObservationChannelUnconfigured` /
`HandoffUnresolvedChannelUnconfigured`）；owner 若认为`查询不可用`该放宽到覆盖未配置，改的是 ADR-0128 决定三与 `validHandoffEvidence` 一处，其余不动。

**能力边界**：读过本票全文、票内引的 UC 3B / `AT-PS-010` / `BD-PS-004` 句、ADR-0027 / 0055 的相关决定；**未读** `production_handoff.go` 与
`production_ownership.go` 全文——裁的是「两件是否都要、先后、缺格落点、记录分格」四问，不裁端口方法签名与观察代数的字段。

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

1. **出向端口**（`ports/`）：向他方权威投递范围、取回确认或查询结果，答复落 `HandoffObservation` 六格（原五格 + 裁决 B 加的`通道未配置`）；生产装配里放未配置适配器（未配置即答 `HandoffObservationChannelUnconfigured` → 评估未决 `CHANNEL_NOT_CONFIGURED` → 结果行「生产归属未决」带续办引用，与 ADR-0055 那套「未配置即拒」同形，不冒充成功。原句写的是答 `HandoffObservationQueryUnavailable` → 归属未决 `HandoffUnavailable`，两处都按 B 改：那一格的证据形要求对方已给确认引用，未配置时没有；`HandoffUnavailable` 说的是接管**读口**没装，与出向**通道**没配置是两件事）。
2. **编排步**（`application/submit_shipment_request.go` 的 `Other` 分支，或拆成独立用例）：投递 → 观察 → `AssessSafeHandoff` → 已确认答「非本产品归属结束」返渠道中立关联；未决落「生产归属未决」带 `ContinuationRef`。
3. **决定记录**：UC 结果行要求携带安全交接结果 / 安全续办引用；今天 `ProductionOwnershipDecision` 只有 `HandoffRef` 一格（装的是停写证据），评估结果没有落点。

## 能不能归到已认可的留待

不能整条归。目标系统的协议、身份与交接证据是 `PAR-GOV-05..07` 实例半边（不在 r27 认可留待五项之列，但性质是实例）；端口、编排、评估、未配置适配器是机制半边，现在能做且不填任何实例值。

## 要先裁的一格

治理接管记录（停写证据，今天的路）与运行时交接确认（`AssessSafeHandoff` 建模的路）是**两种证据**。UC 3B 与 `AT-PS-010` 读起来是「已明确其他权威」（归属）**且**「可安全交接」（交接）两件，缺一都不许交；但接管记录先于投递还是投递确认可替代接管记录，CONTEXT 没有硬句。建议 `/domain-modeling` 一格先定：两者的先后、缺任一格时落哪种未决原因、决定记录上各占哪一格。可能落 ADR（预留号已尽，向 MCP-1 取号）。

## 完成判据（落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

1. `ports/` 有面向他方生产权威的出向端口，答复落 `HandoffObservation` 六格；生产装配放未配置适配器，未配置即答 `HandoffObservationChannelUnconfigured`（不冒充成功；原句「答 `HandoffObservationQueryUnavailable`」按 2026-09-09 14:18 通道 1 改裁 B 改口，理由见「裁决」3）。
2. 提交编排的 `ProductionAuthorityOther` 分支真调 `AssessSafeHandoff`：已确认答「非本产品归属结束」并返渠道中立关联，未决落「生产归属未决」带 `ContinuationRef`。
3. `ProductionOwnershipDecision` 上有评估结果与续办引用的落点，与 `HandoffRef`（停写证据）分格。
4. 剪基线行：先按头注三分成因（全仓 `AssessSafeHandoff` 只此一处声明才是第二种），在自己那笔的干净检出上两法同得记数、钉 SHA。
5. **若 1–3 之前先要改理由行**（今天那句「测试里调 9 次，包外零引用」既无调用方又带计数），改成：

   > 安全交接评估门，UC-PS-001 步 3B「其他权威时安全交接并返回渠道中立关联」与 `AT-PS-010` 的领域半边。**调用方是提交编排在生产归属决定为 `ProductionAuthorityOther` 之后的交接步**（投递范围 → 观察确认 → 形成评估）；那一层今天缺出向端口、编排步与决定记录落点三件，三件同票（wiring-baseline-remainder/01）落地那天这一条出名单。别把 `ProductionOwnershipDecision.HandoffRef` 读成它——那是治理接管的停写证据，不是一次交接的确认。

## 边界

- 本票不改代码、不改基线。基线行剪掉的时刻是编排步真调 `AssessSafeHandoff` 那一笔，剪时按头注纪律记数。
- 不碰 pilotgovernance 的接管记录语义——那是 PN-01 的记录能力，本票只是它的消费方。
