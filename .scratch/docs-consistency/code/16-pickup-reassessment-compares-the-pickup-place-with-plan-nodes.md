# 16 NR 场外揽收触发的复核把揽收地点当成计划节点来比，地点不是节点码，原计划就判失效

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code16（基于 main `14443ac03`）。原分级 R1（上线前必修）不变；此前 ready-for-agent——R1（上线前必修）：答错。揽收复核已在生产装配（`cmd/parcel-dispatch/assemble.go` 经 `adapters/parcelshipment/reassess_on_intake.go`）；揽收地点是客户场所、不是节点码，于是第一天起每一次场外揽收复核都让初始计划失效，包裹落无当前有效路由或被改路，与 `AT-NR-034` 期望的「保留原路由」相反。没有绕法（运营无法让复核不比节点）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 docs-consistency/05 已裁（[ADR-0249](../../../docs/adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)），裁向见文末「裁向」。原 Blocked by：docs-consistency/05 先裁 [audit/nr-no.md](../audit/nr-no.md) H2 的三件（揽收复核时位置与计划怎样比；揽收算不算可控节点；它与 CONTEXT「交给运输方的交接不建立可控节点」那一句的关系）。
出处：[audit/nr-no.md](../audit/nr-no.md) H2（审计归 B，建议修法写明「随后改 CONTEXT 这三句与 AT-NR-034，再立代码票」；无论裁向，今天的比法都要改。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，NR 代码在此期间未改）。

## 现象

揽收复核取揽收地点作复核位置，拿它去比计划的节点集合，不在里面就答原计划不再适用。计划段只有节点间段与交付段，没有「计划接货范围」或揽收段可比。`structuralAutoReroute` 又把揽收当成「位于可控节点」。

## 证据

- 文档：
  - NR CONTEXT「路由形成与选择」：「有效网络收寄结果形成后，必须依据当时已知的实际接货位置……重新校验。……场外揽收时使用 `transport-fulfillment` 的揽收交接结果，不要求尚未发生的到站或节点实测。」同节：「拒收、待确认与交给运输方的交接不建立可控节点，不复核。」
  - 「路由稳定性与受控改路」：「已执行前缀只由计划段链和当前可控节点切出；可控节点只认节点收寄或权威运输交接。」
  - UC-NR-003 `AT-NR-034`：「场外揽收地点与计划接货范围一致，尚未到节点 | 使用实际接货位置复核，不虚构节点实测或到站」。
- 代码：
  - `internal/networkrouting/adapters/parcelshipment/reassess_on_intake.go`：`triggerSpecFor` 以 `source.Place()` 作复核位置；`controlKindFor` 把 `psdomain.OffsitePickupSource` 译成 `nrdomain.TransportHandoverControl`。
  - `domain.ReviewPlanApplicability`（`domain/plan_review.go`）：位置不在 `plan.Nodes()` 里即 `PLAN_ORIGIN_MISMATCH/<地点>`，`PlanNoLongerApplicable`。
  - NR 没有 `AT-NR-034` 的测试。

## 三类

机制：复核按裁定的比法判适用性。产品策略：揽收地点与计划首节点的对应判法（例如服务区域）若裁为内置，属产品策略。租户取值：服务区域的具体划分归租户网络登记，未登记答未配置。

## 规模

M（按裁向，可能涉及计划模型加揽收段，则 L）。

## 最小的诚实修法

照 05 的裁定实现；不论裁向，下面两条都成立：

1. 揽收地点不得直接与计划节点码比；比不出时答「适用性看不清」一类的未决并留原因，不判失效。
2. 揽收是否算可控节点按裁定改 `controlKindFor` 与 `structuralAutoReroute`，并补 `AT-NR-034` 的用例。

## 要裁的

全部在 Blocked by 那一裁里，已裁，见下节。

## 裁向（2026-10-21，docs-consistency/05，ADR-0249）

三件都已裁，照 [ADR-0249](../../../docs/adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md) 实现：

1. **比法：只复核硬约束，不比节点。** 揽收触发的复核只看选中候选的硬约束：被有效限制淘汰即失效，限制状态未知按「适用性看不清」（[ADR-0250](../../../docs/adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)，代码见 code/28），其余仍适用。揽收地点原样记入复核依据，不参与判断。上面「最小的诚实修法」第 1 条后半「比不出时答『适用性看不清』一类的未决」作废：比不出不是证据缺口，是比法不成立。
2. **揽收不建立当前可控节点。** 已执行前缀为空、全部计划段未执行（`onlyUnexecuted` 为真，不得出 `EXECUTED_PREFIX_AFFECTED`）；原计划仍适用即保留、结论 `STILL_APPLICABLE`，不判冻结（不落 `FREEZE_FORM_UNCONFIGURED`）、不走 `tryImprovementReroute`；失效线折自动条件时位于可控节点为假，只形成建议，阻塞 `NOT_AT_CONTROLLED_NODE`。原先无路由时照常可形成首个计划。
3. **与「交给运输方不建可控节点」的关系。** 两者都不建可控节点；揽收照复核（有效网络收寄的来源），节点交给运输方照旧不复核。揽收复核计入当前控制位置（复核册照常记触发业务时间）。

落到代码的建议形状：`ControlEvidenceKind` 加「场外揽收」一格（`controlKindFor` 把 `OffsitePickupSource` 译成它，不再译 `TransportHandoverControl`）；`ReviewPlanApplicability` 对这一格跳过节点比对；`JudgeExecutedPrefix` 对这一格答「尚无可控节点、全部未执行」的新等级；`structuralAutoReroute` 的 `atNode` 对这一格为假；`freezeOf` 前的分派对这一格直接提交仍适用。补 `AT-NR-034`、`AT-NR-084` 的领域、编排与收寄采用一路适配器用例。

## 完成记录（2026-10-21，分支 claude-code16）

用户授权原话「你作为业务和系统专家自决……解决所有的问题」；下列「代决」由作者会话代 NR owner 作出，待 NR owner 复核。照裁向（ADR-0249）实现，读码后没有发现与已接受 ADR 相抵之处。

### 做了什么

- 领域 `ControlEvidenceKind` 加 `OffsitePickupControl`（码值 `OFFSITE_PICKUP_CONTROL`），并加 `EstablishesControllableNode()`：只有节点收寄与交进接收节点的权威运输交接为真。适用性复核比节点、切已执行前缀、折「位于当前可控节点」、仍适用后判不判冻结，四处都问它，不各写一份名单。
- `ReviewPlanApplicability` 多收控制依据：建立可控节点的才拿位置比计划节点；揽收只看选中候选的硬约束（被有效限制淘汰即失效、限制状态未知照旧交回缺口、其余仍适用）。线索与空值答 `ErrInvalidPlanReview`。
- `JudgeExecutedPrefix` 对揽收答新等级 `ExecutedPrefixNothingExecuted`（前缀为空、剩余为整条计划），与线索那一格 `ExecutedPrefixNoControllableNode` 分开；`ExecutedPrefix.OnlyUnexecutedAffected()` 把「只改未执行部分」的折法收进领域。
- 复核编排 `reassessPlan`：仍适用且控制依据不建立可控节点时直接提交仍适用，不进 `freezeOf`、不进 `tryImprovementReroute`；`structuralAutoReroute` 的两个结构条件改问上面两处，揽收时「位于可控节点」为假、「只改未执行部分」为真，失效线上只形成建议、阻塞只有 `NOT_AT_CONTROLLED_NODE`。原先无路由那条线不动，揽收照常形成首个计划。
- 收寄采用适配器 `controlKindFor` 把 `OffsitePickupSource` 译成 `OffsitePickupControl`，不再译成 `TransportHandoverControl`。
- 与 ADR-0249 相反的注释一并改掉：`TriggerReassessment` 头注（「两类都是当前可控节点依据」）、`freezeOf`（「触发又只会是节点收寄或权威交接」）、`JudgeExecutedPrefix`、`ControlEvidenceKind` 头注。
- 测试：领域 `offsite_pickup_reassessment_test.go`（揽收立得起触发却不建立可控节点、揽收复核不比节点只看硬约束、揽收全部未执行且地点与节点同名也不切前缀）；编排 `offsite_pickup_reassessment_test.go`（`AT-NR-034` 仍适用且不落冻结未配置、不因更便宜的候选改路；`AT-NR-084` 失效只出建议；原先无路由形成首个计划；揽收复核计入当前控制位置；揽收与权威交接是不同触发）；收寄采用适配器 `reassess_on_pickup_test.go`（`AT-NR-034`、`AT-NR-084` 端到端，采用记录真经 PS 领域形成）。适配器两条先跑红（今天答 `REROUTED`），改后绿。
- 文档：UC-NR-003 `AT-NR-034`、`AT-NR-084` 的缺口注改为已补，`AT-NR-084` 另点名生产接法上还缺候选成本（demo-chain-gaps/115）与授权决定面（code/29）；「给开发的交接」去掉揽收复核一行、已实现一句补上。CONTEXT 与 ADR 由 docs-consistency/05 改过，未动。
- 装配、迁移、派发路由与 `internal/nodeoperations/**` 都没动。

### 代决

1. **控制依据加一格，不复用权威运输交接。** 裁向给的形状照办；枚举值追加在末尾，码值不落库（复核册与未判登记都不存控制依据），没有迁移。
2. **「揽收地点原样记入复核依据」落在触发上，复核册不另存。** 触发原样保全地点、地点进触发指纹（换地点即触发冲突），触发关联 `intake/<租户>/<包裹>/<来源类型>/<来源版本>` 就是 PS 收寄采用记录的键，地点在那里按原样可查。节点收寄那一路今天也不在复核册存位置，失效时只在依据里点名；揽收这一路若在复核册加列，等于 NR 复制一份 TF 的位置事实，还要一次迁移。
3. **已执行前缀另立「全部未执行」一格，不放宽线索那一格。** 线索那一格答的是「不知道包裹在哪」，`OnlyUnexecutedAffected` 对它答假；揽收答的是「确知还没进任何节点」，答真。合成一格要么让揽收多出不实的 `EXECUTED_PREFIX_AFFECTED`，要么让日后线索也被当成只改未执行部分。
4. **揽收时选中候选限制状态未知，照旧答未决、回滚重投。** 这一格归 code/28（ADR-0250 改为入账未判）；本票不碰，code/28 落地后揽收复核同走那一格（code/28 票面「揽收复核（code/16 落地后）同走这一格」一句）。
5. **揽收与权威交接共用同一触发关联时答触发冲突。** 触发指纹含控制依据的码值，换格就换指纹。见越权风险点 1。
6. **不补派发真库用例。** SYN 垂直上的揽收采用今天停在收寄资格未证（`cmd/parcel-dispatch` 的 `TestARegisteredOffsitePickupStopsAtUnprovenIntakeEligibility`），生产路由走不到揽收复核；装配也没改。揽收复核在领域、编排与收寄采用适配器三层用例上钉住。

### 越权风险点

1. **改格之前落过的揽收触发指纹失配**（NR owner）。上线前只有演示租户；同一封信封重投由消费门跳过，到不了复核编排；只有携带同一揽收触发的另一封信封会答触发冲突，生产上今天没有工具发出这样一封。演示库 `scripts/demo-seeds --reset` 重灌即无旧指纹。
2. **复核册里看不见揽收地点**（NR owner）。只读 NR 复核册做审计时，要顺关联去 PS 收寄采用记录取地点。owner 若要复核册自带位置，加一列与一次迁移即可，判断不变。
3. **揽收在计划起点范围之外也判仍适用**（NR、TF owner）。即 ADR-0249 越权风险点 1，代码照它落地：偏差由包裹交进首个节点时的交接复核接住。
4. **AT-NR-084 的建议在生产接法上还形成不了**（NR owner）。复核今天不取候选成本（demo-chain-gaps/115），失效线上候选评估记未决，退回纯失效；建议之后的授权决定面归 code/29。本票只证复核这一半在成本到位时的走向。

### AT

- `AT-NR-034`、`AT-NR-084`：AT 盘点由 ④ 移入 ①，各点名领域、编排与收寄采用适配器用例；`AT-NR-084` 行内注明授权决定面归 code/29。总览计数未动，由协调方进 main 时重算。

### 验证（分支 claude-code16）

- `go build ./...`、`go vet ./...`、`gofmt -l`（全部已跟踪与新增 Go 文件）干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/networkrouting/... ./cmd/parcel-dispatch ./cmd/parcel-api ./cmd/parcel-network-register ./internal/parcelshipment/adapters/networkrouting ./internal/transportfulfillment/adapters/networkrouting ./internal/visibilityexception/adapters/networkrouting ./internal/architecture/... ./migrations/...`：21 个包 ok，退出码 0（`cmd/*` 与三个 `adapters/networkrouting` 是改动包的反向依赖）。
- 演示种子不读写复核依据的控制格，也不走揽收复核，未灌一次性库。
