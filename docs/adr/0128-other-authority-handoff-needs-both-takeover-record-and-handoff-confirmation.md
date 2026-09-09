# ADR-0128：`其他权威`下的安全交接要两种证据——治理接管记录（停写证据）先定归属，运行时交接确认（`AssessSafeHandoff`）后验交接，缺一不许交；缺格各落其未决；`ProductionOwnershipDecision` 上停写证据与交接评估分格；出向端口 `OtherProductionAuthorityChannel` 与治理读口分开；观察代数加`通道未配置`第六格，未配置适配器如实答它而不充作`查询不可用`

Status: Accepted（2026-09-09，通道 1 代裁——owner 授权自决口径（用户经 IDP 队列授权「你自决，目标是全部解决」），裁决原文在票 [wiring-baseline-remainder/01](../../.scratch/wiring-baseline-remainder/issues/01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md)「裁决」节四条；其中「出向端口未配置 → 观察答 `HandoffObservationQueryUnavailable`」一句于同日 14:18 由通道 1 改裁为 **B**（未配置自成一格），改裁经 IDP 队列接续单下达、票面「裁决」节同笔改口。本记录由通道 2 落文并实施（同通道三任接续，前两任的在途产出按判据对照后接手）。落文时读过：该票全文；UC-PS-001 步 3B、结果行「必须持久保留的业务记录」、`AT-PS-010`、`BD-PS-004` 各句；ADR-0055 与 ADR-0029 的 Context 与 Decision；ADR-0027 / 0025 / 0063 / 0091 在 README 的条目与被本仓代码注释引用的那几句；`internal/parcelshipment/domain` 的 `production_handoff.go` 全文与 `production_ownership.go` 决定记录一段；`application/submit_shipment_request.go`；`ports/ports.go`；`adapters/pilotgovernance/production_ownership.go` 里 `resolved` 装 `HandoffRef` 的那一段（只读）；`adapters/http/submit_shipment_request.go` 的归属视图；`cmd/parcel-api/assemble_submission.go`。未重读 pilotgovernance 的 `CONTEXT.md` 与接管记录写侧、UC-PS-001 全文；本记录因此不裁接管记录的语义（只消费它），也不裁 HTTP 面怎么呈现评估那一格）
Date: 2026-09-09

## Context

[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 步 3B 写「只有本产品是当前唯一权威……才返回未来建单可放行；**其他权威时安全交接并返回渠道中立关联**；……权威或交接无法确定时保持生产归属未决」；「必须持久保留的业务记录」要求保留「当前权威方、安全交接结果和渠道中立关联；归属未决时保留缺口与续办记录」；`AT-PS-010` 写「已明确其他当前权威**且**可安全交接时交给该权威并返回渠道中立关联，否则保持生产归属未决」；`BD-PS-004` 同指。

代码在 `74ef0da8` 上是这样的（票 wiring-baseline-remainder/01 取证，本记录落文时复核）：

- `internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 的 `resolved`：命中他方权威区间时读治理侧**接管记录**（`Handoffs.FindByInterval` → `takeover.StopEvidence()` 装进 `ProductionOwnershipDecisionSpec.HandoffRef`），取不到即 `OwnershipUnresolvedHandoffIncomplete`、没装接管读口即 `OwnershipUnresolvedHandoffUnavailable`。注释写明这是「原权威停止写入的证据」——它回答的是**归属**（谁是权威、前任停笔了没有），不是一次交接的确认。
- `domain/production_handoff.go` 早有 `AssessSafeHandoff`：按一次**交接尝试**的观察形成 `SafeHandoffAssessment`——观察代数五格（完整确认 / 部分确认 / 超时 / 查询不可用 / 失败），只有完整确认且范围摘要对得上才是 `SafeHandoffConfirmed` 并带确认引用与生效时刻；其余一律 `SafeHandoffUnresolved` 带原因，且未决必带续办引用（构造期拒绝没有续办引用的未决）。**它在全仓非测试代码里零调用**，是 `production_wiring_baseline.txt` PS 组的一条；`production_type_reachability_baseline.txt` 里七个类型「经它取得」随它躺着。
- `application/submit_shipment_request.go`：`DecideProductionOwnership` 之后过 `EvaluateFutureSubmissionGate`，不放行就 `blockedOutcome(decision)` 直接返回——归属为 `Other` 时径答 `OTHER_PRODUCTION_AUTHORITY`。**没有任何一步把范围交给那个权威**，也没有观察确认；`ports/` 里没有面向他方生产权威的端口。
- `ProductionOwnershipDecision` 只有 `HandoffRef` 一格（装的是停写证据），评估结果没有落点。

票面「要先裁的一格」：治理接管记录与运行时交接确认是**两种证据**。UC 3B 与 `AT-PS-010` 读起来是「已明确其他权威」（归属）**且**「可安全交接」（交接）两件，缺一都不许交；但接管记录先于投递、还是投递确认可替代接管记录，CONTEXT 没有硬句。四问——两件是否都要、先后、缺格落点、记录分格——由通道 1 按 owner 授权代裁；本记录是那四条裁决的落文，外加实施时补的端口形与观察代数形。

**实施中发现票面裁决 3 的一句与代码相悖。** 那句写「出向端口未配置 → 观察答 `HandoffObservationQueryUnavailable`」。而 `validHandoffEvidence` 对`查询不可用`的证据形要求 `ConfirmedScopeDigest` 与 `ConfirmationRef` 在场、`QueryRef` 缺席——它的语义是「对方已经确认过、只是此刻查不到确认状态」。出向通道未配置时什么都没投递出去，手上不可能有任何对方给的引用；要把它塞进那一格，要么放宽证据规则（把「重查确认」与「去配置通道」两种恢复动作折进一格），要么编造一个引用。实施方按派单纪律停下报通道 1，通道 1 改裁 **B**：未配置自成一格，其余五格一字不改。

## Decision

**一、出向端口 `OtherProductionAuthorityChannel` 单立，与 `ProductionOwnershipAuthority` 分开。** `ports.OtherProductionAuthorityChannel.DeliverAdmissionScope(ctx, ProductionHandoffDelivery) (ProductionHandoffObservation, error)`。投递携带尝试身份、完整拟受理范围与对方是谁，**不携带治理侧的停写证据**——那份证据是本方判归属用的，对方要确认的是这一笔范围，一起发出去等于让对方拿本方的归属依据当交接依据。答复落 `ProductionHandoffObservation`：取值照 `domain.HandoffObservation` 的封闭集合，证据格（范围摘要、确认引用、查询引用、生效时刻）随取值该有还是不该有由 `AssessSafeHandoff` 校验，端口不预判。**依赖调不通仍作为 error 返回，不折成`超时`或`失败`**：那两格说的是对方的交接答复，本方通道自己坏了是另一件事、恢复动作不同（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 的分格判据）。两口分开的理由：治理读口的适配器答「归谁」，出向通道对着那个「谁」交范围——两种证据、两个对方，合成一个端口会让治理读口的适配器被迫实现一个它答不了的方法。适配器单独成包 `adapters/productionhandoff`、不并入 `pilotgovernance`，同一理由。

**二、两件都要，先后固定：接管记录在前定归属，交接确认在后验交接；缺一不许交。** 归属决定为 `ProductionAuthorityOther` 必须先凭接管记录成立（今天 `resolved` 的路不动）；成立之后编排才进入交接步 `handOverToOtherAuthority`：投递 → 观察 → `AssessSafeHandoff` → `WithSafeHandoff` 记到决定上。理由（通道 1 裁决原文）：接管记录不能替代交接确认——它证明的是治理侧的授权状态，不证明这一笔范围到了对方手里；交接确认也不能替代接管记录——没有治理侧的停写证据，「其他权威」本身就没立住，投递出去的是一笔两边都可能写的范围（`PAR-GOV-05..07` 要防的正是双写）。先后固定的理由：投递是对外动作、不可撤，只在归属已定时做；反过来「先投递、拿确认当归属证据」会让一个没有停写证据的权威凭一次应答成为权威，那是 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 那族「解析标识 / 应答不得成为能力凭证」的反面。本产品自己承接、权威未决、本产品暂停三种决定都不投递。尝试身份与续办引用由决定派生（`PS-HANDOFF/<决定 ID>`、`CONT-PS-HANDOFF/<尝试 ID>`）而不签发：本上下文不持久化交接尝试，同一份决定重复走到这里必须得到同一次尝试，对方才能据以认领重放；续办引用不看观察结果就形成——它说的是「从这一次尝试续办」，原因由评估自己带。

**三、缺格各落其未决；观察代数加`通道未配置`第六格。** 接管记录取不到 → 仍是今天的 `OwnershipUnresolvedHandoffIncomplete` / `OwnershipUnresolvedHandoffUnavailable`（归属未决，不投递）。接管记录在、交接观察不是完整且范围相符的确认 → 归属决定仍是 `Other`，结果行落「生产归属未决」（`OutcomeOwnershipUnresolved`），未决原因取 `SafeHandoffAssessment` 的原因格（`SCOPE_MISMATCH` / `PARTIAL_CONFIRMATION` / `TIMED_OUT` / `QUERY_UNAVAILABLE` / `TARGET_FAILURE` / `CHANNEL_NOT_CONFIGURED`），续办引用取评估的 `ContinuationRef`。只有评估已确认，`Other` 才答「非本产品归属结束」（`OutcomeOtherProductionAuthority`），确认引用就是返回给调用方的渠道中立关联。**出向通道未配置**：`HandoffObservation` 加第六格 `HandoffObservationChannelUnconfigured`（`CHANNEL_UNCONFIGURED`），证据形与`超时` / `失败`同——三个引用与生效时刻一律缺席；未决原因加 `HandoffUnresolvedChannelUnconfigured`（`CHANNEL_NOT_CONFIGURED`）。生产装配（`cmd/parcel-api`，两种形态）放 `UnconfiguredOtherProductionAuthorityChannel`：对每一次投递不看范围、不看对方，一律答这一格、不带任何引用、不报 error（error 那格的恢复动作是重试，重试改不了一条没人配置的通道——[ADR-0063](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md) 决定四对「显式未配置实现」的分界），编排据以落「生产归属未决」并给出续办引用，不冒充成功——[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 那族「未配置自成一格」同形。它不充作`查询不可用`，理由见 Context 末段。真通道就位时在装配点替换，本类型随之退场。

**四、决定记录分格：`HandoffRef` 保持原义不改名，新加评估一格，两格不合并。** `ProductionOwnershipDecision.HandoffReference()` 仍是治理接管的停写证据（回答「前任停笔了没有」）；新加 `SafeHandoff()`（`SafeHandoffAssessment`，回答「这一笔范围交过去、对方确认了没有」——已确认带确认引用与生效时刻，未决带原因与续办引用），由 `WithSafeHandoff` 在决定成立之后记上、交回新值、原值不动。评估只能记到`其他权威`决定上，且必须是对同一范围、向同一对方的那一次；一次决定只记一次。这一格**不进 `ProductionOwnershipDecisionSpec`**：形成决定的一方（治理读口的适配器）拿不到它。两格不合并的理由：它们是两种证据，合并会让「有停写证据但交接失败」与「无停写证据」在记录上不可分。`EvaluateFutureSubmissionGate` 对记了评估的 `Other` 决定照样按`其他权威`阻断——评估不改归属身份，只补「交出去了没有」这一格。

**五、本记录不裁的。** pilotgovernance 接管记录的语义与写侧（PN-01 的记录能力，本票只是消费方）；HTTP 归属视图怎么呈现评估那一格（今天不渲染，见 Consequences）；通往他方权威的协议、身份与交接证据（`PAR-GOV-05..07` 实例半边）。

## 越权风险点（单列，供 owner 复核）

① 「接管记录在前」是从 `PAR-GOV-05..07` 防双写的目的推的，UC 3B 字面只写「且」没写先后。
② 未决原因直接沿用 `SafeHandoffAssessment` 的观察代数作为结果行的原因词，没有另立结果词表——若 UC 结果行要求的「安全续办引用」之外还要区分原因，那是另一格。
③ 观察代数五格改六格：票面裁决 3 原句「出向端口未配置 → 观察答 `HandoffObservationQueryUnavailable`」实施时发现与代码的证据规则相悖，通道 1 改裁为未配置自成一格。owner 若认为`查询不可用`该放宽到覆盖未配置，改的是决定三与 `validHandoffEvidence` 一处，其余不动。

## Consequences

- `application/submit_shipment_request.go`：`NewSubmitShipmentRequestHandler` 多一参 `ports.OtherProductionAuthorityChannel`；`Other` 分支多一步交接；`blockedOutcome` 对 `Other` 按评估状态分答。五处调用点随签名：生产装配与其测试、`cmd/parcel-dispatch` SYN-V0、`tests/bentocontract` 提交管线放未配置适配器（它们的归属替身都答本产品，通道走不到）；`adapters/http` 测试放给完整确认的替身——那份表驱动里 `OTHER_PRODUCTION_AUTHORITY` 一格按决定三只有确认才走得到。
- 基线：`production_wiring_baseline.txt` PS 组 `AssessSafeHandoff` 按成因**第二种**剪掉（`handOverToOtherAuthority` 是它的生产调用方；父提交 `6a80a6da` 上两法同得 4→3）；`production_type_reachability_baseline.txt` 「经 `AssessSafeHandoff` 取得」七条随它出名单（22→15）。两个数只对该检出成立，出处写在两份文件头注。
- **HTTP 面欠一格。** `adapters/http/submit_shipment_request.go` 的 `newOwnershipView` 今天渲染 `otherAuthority` / `handoffReference`（停写证据）/ `unresolvedReason` + `continuationReference`（只对归属未决的决定）/ `suspensionReference`，**不渲染评估那一格**。于是「`Other` + 交接未决」在 HTTP 上是 `OWNERSHIP_UNRESOLVED` 带对方与停写证据、却没有续办引用与未决原因；「`Other` + 已确认」是 `OTHER_PRODUCTION_AUTHORITY` 却没有确认引用（渠道中立关联）。UC 结果行要求两者都带。字段名、与既有 `continuationReference` 是合并还是分列，是 JSON 契约的形，归 PS 地盘另立票，本记录不替它选。
- 无租户故无迁移：交接尝试不持久化（决定二），`ProductionOwnershipDecision` 今天也不落库，`Other` 决定不建委托，评估一格不进任何持久化路径；若将来决定记录落库，评估一格随之，PS 迁移号按当时派单。
- `HandoffObservation` 与 `HandoffUnresolvedReason` 各多一格；今天消费这两个封闭集合的只有本上下文自己的 `String()` 与 `unresolvedReasonFor`，已全覆盖；将来任何跨上下文翻译按 [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md) 的全函数要求为第六格写落点。

## Alternatives considered

- **接管记录即交接，`Other` 直接答「非本产品归属结束」（`74ef0da8` 上的样子）。** 否决：它把治理侧的授权状态当成了范围已到对方手里的证据，结果行「安全交接结果」与「渠道中立关联」无从携带，`AT-PS-010` 的「且可安全交接」半句落空。
- **投递确认可替代接管记录（先投递，拿应答定归属）。** 否决：没有停写证据的一方凭一次应答成为权威，是 ADR-0027 那族「应答不得成为能力凭证」的反面；且投递不可撤，在归属未定时做出去的是一笔两边都可能写的范围。
- **出向通道并入 `ProductionOwnershipAuthority`。** 否决：治理读口的适配器要被迫实现一个它答不了的方法；两种证据、两个对方，合在一口会让「归谁」与「交过去了没有」在端口上不可分。
- **未配置答`查询不可用`（票面裁决 3 原句）。** 否决（改裁 B）：`查询不可用`的证据形要求对方已给确认引用，未配置时没有；放宽证据规则会把「重查确认」与「去配置通道」两种恢复动作折进一格，编造引用更不许。ADR-0052 / 0054 / 0055 三次治的都是这种折叠。
- **未配置适配器返回 error。** 否决：error 那格的恢复动作是重试（ADR-0029），重试改不了一条没人配置的通道；且编排对通道 error 原样上抛，提交会整个失败而不是落「生产归属未决」带续办引用。
- **评估合并进 `HandoffRef` 或替换它。** 否决：两种证据并一格，「有停写证据但交接失败」与「无停写证据」在记录上不可分，恰是决定四要拦的。
- **交接尝试身份由端口签发。** 否决：本上下文不持久化尝试，同一份决定重走一次会得到第二个尝试身份，对方无从认领重放；由决定派生则天然幂等。

## Links

- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：步 3B、结果行、`AT-PS-010`、`BD-PS-004`
- [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)：应答不得成为能力凭证——决定二「接管记录在前」的反面
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格——通道 error 不折成观察、未配置不答 error
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置自成一格的先例——决定三第六格同形
- [ADR-0063](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)：决定四「显式未配置实现」的分界——未配置适配器答一格而不是 error
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：翻译必须是全函数——第六格的消费侧落点
- 票 [wiring-baseline-remainder/01](../../.scratch/wiring-baseline-remainder/issues/01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md)：裁决四条与改裁 B、判据 1–5、完成记录
