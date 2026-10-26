# 79 小包托运采用与面单终局各路把集运单元的信封当「目标委托还没到」一路重投

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code79（见文末「完成记录」）。此前 needs-triage——R2（上线后尽快）：整袋交接、整袋交付是常态，每一封对象是单元的揽收、交付、首次有效收寄信都会在 PS 那几路记未决、重投到失败预算耗尽，再落成发布失败，运维面上全是假的依赖故障。2026-10-21 协调方据票 [code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md) 隔离评审立（作者子代理代笔）。
Blocked by: 无（成员是否随单元采用那一半随票 [demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md) 再定）
上下文：PS（小包托运），面单渠道终局判断。
出处：票 code/75 完成记录越权风险点 6。

## 现象（读码，只记 `S`）

- PS 采用场外揽收（`internal/parcelshipment/adapters/transportfulfillment/adopt_on_offsite_pickup.go`）、采用有效交付（`adopt_on_effective_delivery.go`）、面单终局判断经首次有效收寄（`judge_on_carrier_first_effective_pickup.go`）各路的头注写明「不替它猜」：载运对象串原样进包裹反查，对象是集运单元时反查不中，落各自的 `ErrParcelTargetNotFound`。
- 这一格在各路的未决名单里（`offsitePickupUndecidedSentinels`、`effectiveDeliveryUndecidedSentinels`、`labelTransactionJudgmentUndecidedSentinels`，后者经 `carrierFirstEffectivePickupJudgmentUndecidedSentinels` 引用），理由是「委托可能还没落到已接受，重投会改变结果」。对象是单元时重投不会改变结果：单元永远没有已接受委托。于是每封这样的信一路重投到失败预算耗尽。
- 全程追踪那一侧（票 code/75）已按节点作业单元册先认单元、是单元即入账不派生；两侧口径不同。

## 三类

机制。规模 S–M。租户取值：无。

## 修法

- 各路在反查包裹之前，照全程追踪同一判据先问节点作业单元册「这一件那一刻是不是集运单元」（同串的包裹号按单元认，ADR-0214 决定二、ADR-0225 决定四）；是单元就确定性入账、不采用、不判终局，不进未决名单。PS 自己立消费侧端口（ADR-0025），翻译留在 PS，判据仍是节点作业那一本册，不另立串形或前缀规则。
- 单元册读不动是续办格，进各路未决名单；答了却译不回是确定性的，不进。
- 成员是否随单元被采用（整袋交付是不是袋里每件包裹的有效交付）随票 80 的单元控制规则再定，本票不投到成员上。

## 验收

- 三路各一条：对象是节点作业册上的一袋，信封入账、不采用、不判终局，不落未决；对象串长得像袋号却不在单元册上，照旧反查委托（反查不中仍是未决）。
- 派发分区：单元册读不动落 `dispatch.consumer_undecided`，答复不一致保持 `dispatch.publish_failed`。

## 既有票

- [code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md)、[demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)

## 完成记录（2026-10-26，Claude Code 作者子代理，分支 claude-code79，基于 main `1a991c35c`）

### 做了什么

- **PS 自立消费侧端口**：`internal/parcelshipment/ports/consolidation_unit_view.go` 的 `ConsolidationUnitView.IsConsolidationUnitAt(租户, 包裹号, 来源事实的业务发生时间)` 只答「这一件那一刻是不是节点作业单元册上的集运单元」，不认成员；答了却立不成本口形状的包住 `ErrConsolidationUnitAnswerUntranslatable`。适配器 `internal/parcelshipment/adapters/nodeoperations/consolidation_unit_view.go` 只翻译节点作业 `ConsolidationMembershipsAt` 的 `Unit` 那一格（同串的包裹号按单元认），不另立串形或前缀规则；租户译不过去、答「不是单元」却带成员（自相矛盾）是确定性的，册读不动原样交回。
- **三路入口先认单元**：`internal/parcelshipment/adapters/transportfulfillment` 的 `AdoptOnOffsitePickupAdapter`、`AdoptOnEffectiveDeliveryAdapter`、`JudgeOnCarrierFirstEffectivePickupAdapter` 构造多收单元册读口（缺即构造拒），在反查包裹之前问：是单元即返回 nil（消费门入账收工），不采用、不判终局、不留 PS 记录；共用的问法与两个哨兵在 `consolidation_unit.go`。时点取来源事实的业务发生时间（`OccurredAt`）。首次有效收寄的**失效版本**落在单元上同样收工（时点取 `JudgedAt`）；待确认版本不论对象是什么仍是交接口缺陷，响亮失败。
- **失败分格**：`ErrConsolidationUnitNotVisible`（册读不动，续办）进三路未决名单（`offsitePickupUndecidedSentinels`、`effectiveDeliveryUndecidedSentinels`、`carrierFirstEffectivePickupJudgmentUndecidedSentinels`），名单头注补一格与「不在名单里」一格；`ErrConsolidationUnitAnswerInconsistent`（答了却译不回）不进名单，保持 `dispatch.publish_failed`。`ErrParcelTargetNotFound` 仍在名单里（对象串长得像袋号却不在册上，照旧反查，反查不中仍是未决）。
- **装配**：`cmd/parcel-dispatch/assemble.go` 三个既有消费者构造器接新端口（各自 `carriedObjectUnitView(db)`），端口装配函数放新文件 `cmd/parcel-dispatch/assemble_consolidation_unit.go`（ADR-0222）；`cmd/parcel-api/**` 未动。
- **不加锁、不影响锁序**：单元那一问是读（`ReadExecutor`），在 `adapters/transportfulfillment` 入口，早于 `FormParcelFinalHandler` 里 code/98 的取消边界锁与取消视图、code/101 的生命周期闸门，也早于采用编排；是单元的信封一把锁都不取，不是单元的走原路径，锁序与端口头注原样不变。
- **文档**：UC-PS-003（一致性一条、`AT-PS-156`、现状缺口一行）、UC-PS-004（终局形成规则一条、`AT-PS-157`–`159`、现状缺口一行、「首次有效收寄被作废」一行补「单元不在此列」）、PS CONTEXT 规则一条；AT 盘点①节四行与文末一条追记（总览计数未重算）。`labelfinal/judge_parcel.go` 一行注释同步（不动逻辑）。

### 代决（用户原话「你作为业务和系统专家自决」，协调方分诊授权）

1. **是单元的落点是「不留 PS 记录，只留 inbox 那一行消费记录」，没有复用各路的 `SOURCE_NOT_ADOPTED` 结果格**（偏离分诊 2 的字面）。依据：`SOURCE_NOT_ADOPTED` 是采用库、终局库里按（租户，包裹，种类，版本）键的持久记录，且随之交下游发布意图；单元没有委托、不是包裹，落那一格要么编出一份委托（`AdoptNetworkIntakeCommand`、`FormParcelFinalCommand` 都要委托），要么把非包裹写进包裹账、向下游交一份「某包裹未采用」的意图。分诊定「迁移不需要」也排除了另立单元登记表。与 code/75 代决 1（单元的事实入账不派生、不另留痕）同口径；消费门对该格是入账收工（`nil`）。备选「新立单元不采用登记」否（要迁移、要下游消费方，且无人读它）。依据名 `CONSOLIDATION_UNIT/<标识>` 因此没有落点，没有造。
2. **「答了却译不回」在本实现里可达的只有两种**：租户译不过去、答复自相矛盾（答「不是单元」却带着成员）。分诊 3 括号里的「册答『是单元』但标识译不成 PS 的值」不存在——单元标识就是问的那一串，PS 不另译。
3. **首次有效收寄的失效版本落在单元上收工**（票面没点，扩出）：单元没有据前版形成的终局，「重派生归 PS owner、尚未裁定」的显式未决（`ErrVoidedCarrierPickupRederivationUndecided`）与它无关，留着它单元的信封仍一路重投到失败预算耗尽，是同一类假依赖故障。对包裹的失效版本照旧停在那一格。
4. **不立 ADR-0291**：没有已接受 ADR 要求 PS 把单元串拿去反查包裹——三路头注的「今天不替它猜」是代码注释，不是决定；判据沿用 ADR-0214 决定二、ADR-0225 决定四，不改它们的口径，成员不随单元采用的理由归票 demo-chain-gaps/80。预留号未用。无迁移。
5. **面单交易、关闭 / 重开两路不问单元**：它们带的是 PS 自己面单交易册、继续尝试册里的包裹号，不是 TF 载运对象，核（`ParcelJudgmentCore`）不动；单元那一问只放在首次有效收寄一路的入口，核里的 `labelfinal.ErrParcelTargetNotFound` 因此只剩「委托还没落到已接受」一个意思。
6. **端口取 `DeclaredParcelID` 作对象类型**：与三路反查包裹用的是同一个翻译结果，不另立 PS 域类型；端口头注写明它是「尚未认出种类的对象引用原文」。

### 越权风险点

1. **`ConsolidationMembershipsAt` 对单元仍跑成员查询**（NO owner；同 code/75 越权风险点 3）：对象是未关闭单元时，单元行的成员行坏了（成员标识立不成作业实物标识）会让这一问报错，PS 读成读不动，那一封信一路记未决重投到失败预算耗尽。后续：NO 出一口只跑 EXISTS 的「是不是单元」，VE `IsUnitAt` 与本票的适配器都改接它（未立票，NO owner 复核时定）。
2. **每封揽收、交付、首次有效收寄信多一次单元册读**（运维、NO owner）：那一本读不动时，连纯包裹的这三路也整封未决——说不出是不是单元就不能当包裹反查。三路同一封信上别的路（VE 投影等）仍各自照跑，整封重投时已入账的按 inbox 跳过。
3. **单元册晚于运输履约事实到达**（NO、PS owner）：消费那一刻节点作业还没登这件单元，该串又不是已接受委托声明的包裹号时，反查不中落未决，重投到单元登册后即认单元，比 VE 一侧「按包裹投、事后不纠正」好；该串恰是某件已接受委托声明的包裹号、单元登册在后时，先按包裹采用（可能形成采用或终局），事后不纠正——ADR-0214 越权风险点 1「包裹号与单元号同串」的已知形状。
4. **成员不随单元被采用**（PS、TF、NO owner，票 demo-chain-gaps/80）：整袋场外揽收、整袋交付、整袋首次有效收寄之后，袋里每件包裹仍没有对应的采用或终局。本票不留痕，票 80 定下规则后回补靠 TF 册（对象是单元的揽收、交付、首次有效收寄登记）加 NO 单元册，没有 PS 侧登记可回扫。已在票 80 加一条指过来。
5. **存量不清理**（运维）：本票之前已重投到失败预算耗尽、落成 `dispatch.publish_failed` 的单元信封，行留在发布面上，不会自动重放；运维重放后即入账收工。
6. **CONTEXT-MAP 未补**（PS、NO owner）：`node-operations → parcel-shipment` 那一条只写了装袋事实，没写 PS 此刻另问单元册；边界清单不含 CONTEXT-MAP，留给 owner 复核时补一句（code/75 对 VE 一侧同样没补）。
7. **`labelfinal/judge_parcel.go` 改了一行注释**（范围外，仅注释）：原句「载运对象可能是集运单元，今天不替它判」与现行为相反。

### AT

- UC-PS-003 新立 `AT-PS-156`（场外揽收的载运对象是单元册上的一袋，含与已接受委托的包裹同串）；UC-PS-004 新立 `AT-PS-157`（有效交付同）、`AT-PS-158`（首次有效收寄同，含失效版本；待确认版本仍响亮）、`AT-PS-159`（三路问单元册：读不动 `dispatch.consumer_undecided`，答复不一致 `dispatch.publish_failed`）。取号前核过 `origin/main` 与各远端分支：`AT-PS-156` 起未被占。盘点分桶表①节加四行，`at-coverage-counts -check` 退 1（仅总览过期，不退 2）。
- 用例：适配器层 `internal/parcelshipment/adapters/transportfulfillment/consolidation_unit_test.go`（三路同组断言：单元收工且不反查、不在册的照旧反查并保持未决、读不动与译不回各分各格、时点取业务发生时间、失效与待确认版本、两路构造缺册即拒）；`.../adapters/nodeoperations/consolidation_unit_view_test.go`（判据只是册的一答、读不动不包译不回、自相矛盾与租户译不过去）；真库 `cmd/parcel-dispatch/carried_consolidation_unit_test.go`——三路各经生产消费者构造：一袋入账（inbox 一行，无采用、终局记录与下游意图）、与已接受委托的包裹同串按单元认、长得像袋号却不在册上照旧反查并落在各路未决名单、派发分区两格（名单取各路变量本身）。既有 `TestABagLike…FallsIntoTargetNotFound` 两条改名改注（原「今天不猜」口径）。证据一律 `S`。

### 验证

- 先写适配器层用例，在三路入口不问单元册时 5 个用例函数（含各子用例）红，实现后全绿。
- 变异（各自备份、跑完 `cmp` 还原）：①不问单元册（适配器层与真库袋、同串全红）；②是单元仍落未决哨兵（适配器层、真库全红）；③册读不动当作「不是单元」放行（读不动用例红）；④答复不一致进了未决名单（真库派发分区三路红）、读不动没进名单（三路红）；⑤答复译不回映射成读不动；⑥适配器不校验自相矛盾；⑦单元那一问放到反查之后；⑧失效版本不问单元；⑨时点取消费这一刻；⑩构造不拒缺册。十处都被杀。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`TMPDIR=… scripts/test-full.sh`（改动包、反向依赖、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...` 共 42 个包）退 0；新用例另跑 `-count=3`（适配器层两包、`cmd/parcel-dispatch` 相关用例）绿。
- **有效交付一路 code/98、101 之后的行为不回归**：对象是普通包裹的交付，既有真库用例原样绿——`TestAnEffectiveDeliveryOverAStandingCancellationIsConsumedAsARefusal`、`TestAnEffectiveDeliveryDoesNotGetPastAStandingCancellationOrderConflict`（取消在册拦下）、`TestTheWiredFinalAdoptionFormsNoFinalForRequestsThatAreNotAccepted`（生命周期闸门）、`TestARegisteredEffectiveDeliveryStopsAtUnconfiguredFinalRule`（单元册为空的生产装配上走到终局规则）；适配器层 `TestARegisteredDeliveryLooksUpTheUniqueAcceptedTargetAndFormsFinal` 形成终局原样绿。
