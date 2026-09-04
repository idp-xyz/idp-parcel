# 揽收登记的更正：新版本还是失效 + 替代

Category: enhancement
Status: resolved——MCP-3 2026-09-04，分支 `mcp3-tf08` 已验代码 tip `779f3c4b`（rebase 后基线 main `dc7d44e9`），见「完成记录」；领域问题已裁（见「裁决」节，MCP-3 2026-09-04，owner 授权自决）：**A 新版本**，段侧参与关系重派生另立票 [10](10-source-correction-rederives-participation.md)
Blocked by: 无（不阻塞 04–07）

## 裁决（MCP-3，2026-09-04）

**取 A「新版本」。** 更正形成一条新的 `OffsitePickup` 版本，回指被更正版本（`Corrects`），原版本与原判断不动；
`OffsitePickupRegistry` 仍**只插不改**；parcel-shipment 按版本幂等采用，新版本再采用一次是**正确行为**——PS 的
责任起点与正式承诺生效时间锚在 `OccurredAt` 上（`OffsitePickup.OccurredAt` 自注），发生时刻被更正时 PS 必须再判一次。

依据三条，都是已经写在仓里的话，不是新裁量：

1. `domain.PickupResultVersion` 的注释原句「**来源更正形成新版本不覆盖本版**」；`register_offsite_pickup.go` 对同键不同
   内容的处置注释原句「首登不顶替，**来源更正走新版本**」。B 路等于推翻这两句。
2. 本上下文已有五处更正形状同为 `Corrects` 回指 + 新版本新登记（`TransportHandover.Correct`、`EffectiveDelivery.CorrectProof`、
   `TransportMovementFact`、`LoadAssignment`、`TransportChargeOccurrence`），登记册一律只插不改（ADR-0097 窄口的同一结构
   判据：「回写为未发生」在这个口上表达不出来）。B 路要给登记册开改写口，是本上下文第一处例外，而例外没有理由。
3. CONTEXT 生命周期⑤「保留原段、原参与关系和原判断，形成失效或替代关系并重新派生」——「失效」在版本链里由「被后续版本
   回指」表达（`TransportHandover` 的汇总注释原话：被回指的那一代不计，它留在册上），不需要一个可改写的失效位。

**更正携带什么、不携带什么，照 `HandoverCorrection` 的取法**：只带「证据说了什么」——地点、控制依据、执行方、发生时刻
四格，加新版本号与更正时刻；**不带**「这是哪一次」——对象、任务、尝试三格沿用被更正版本，改了它们就是另一次揽收而不是更正。
每格完备性同首登（控制依据仍必备：更正不能把一次揽收更正成一次失败到访——那是另一种事实，走别的口）。更正时刻不得早于
被更正版本的登记时刻；沿用原版本号即覆盖，构造期拒绝。

**附问：更正改了控制证据或发生时刻，参与起点是同段新起点还是新段？——同段，但这半边不在本票。** 段是共同控制责任范围，
「承运责任变了才是另一段」（ADR-0103 主体判据；CONTEXT「可验证的实际承运责任或运输控制边界发生变化时，结束原参与并形成
下一段」）——更正证据或时刻不改变谁在控制，所以不是新段。参与关系今天以 `OFFSITE-PICKUP/<版本>` 为 `entryBasis`、以
`OccurredAt` 为 `enteredAt`（`JoinWithPickup`），按 CONTEXT ⑤ 应保留原参与、形成替代参与回指原参与并按新版本重派生起点；
**但这一层机制今天对交接更正同样没有**——`RegisterTransportHandoverHandler.Correct` 只落新版本并重交意图，不碰段登记册。
只在揽收一侧补它会让两种来源的更正在段上行为不一致，因此**段侧「来源更正 → 参与关系重派生」另立一票，覆盖交接与揽收
两种来源**；本票的 Correct 编排照交接那一侧的现状：落新版本 + 重交 PS 采认意图，不进段。

**本票范围（裁后重述）**：领域 `OffsitePickup.Correct(PickupCorrection)` → 端口 `OffsitePickupRegistry` 以新版本键 `Save`
（不加改写口）→ 编排 `RegisterOffsitePickupHandler.Correct`（读回前版 → 领域 Correct → 提交 → 重交意图；无前版则未受理，
更正不出无中生有的揽收）→ 端点 `/transport-fulfillment/offsite-pickup-corrections`（沿票 04 形状，共享接线文件占号）。
四层一次建设，走 TDD。

**能力边界**：读过本票、TF `CONTEXT.md` 全文、`domain/offsite_pickup.go`、`domain/transport_handover.go` 的 `Correct` 与
`HandoverCorrection`、`domain/actual_fulfillment_segment.go` 的 `JoinWithPickup`、`application/register_offsite_pickup.go`、
`application/register_transport_handover.go` 的 `Correct`；grep 过五处 `Corrects` 的落点。**没读** PS 侧采用揽收意图的编排
（`UC-PS-003` 揽收源链）——「新版本再采用一次是正确行为」是按 `PickupResultVersion` 自注与 PS 责任起点锚在 `OccurredAt`
推的，实施时若 PS 采用口对同对象第二版本有别的处置，以 PS 票面为准并回本票追记。裁的是**更正模型的形状**，不含任何取值。

## 从哪里来

票 [03](03-parcel-api-wiring-for-segment-orchestrations.md) 裁决 2 写「揽收含更正口」，票 [04](04-control-fact-entry-endpoints.md)
落地时发现 `RegisterOffsitePickupHandler` 只有 `Register`，应用层没有更正编排，端点表因此只挂了登记口。
MCP-3 2026-09-03 裁：**这不是端点层的事，是领域层尚未答的问题**，另立本票，不阻塞接线四票。

## 要裁的领域问题

CONTEXT 生命周期⑧「来源证据被更正 → 保留原段、形成失效或替代关系并重新派生」。揽收登记的更正
落到哪一种形状：

- **新版本**（同交接那一侧：新版回指前身、原判断不动、每格完备性同首次）——`OffsitePickup` 今天
  有 `Version`（逐成功对象签发的 `PickupResultVersion`），但没有 `Corrects` 链；parcel-shipment 的
  采用判断按版本幂等，新版本意味着下游要再采用一次。
- **失效 + 替代**（原登记标失效、另立一条替代登记并重新派生）——与生命周期⑧的措辞更贴，但
  「失效」在 `OffsitePickupRegistry` 上今天表达不出来（只插不改）。

两条路对段的影响不同：更正若改了控制证据或发生时刻，对象在段里的参与起点要不要跟着动？CONTEXT
「已经成立的实际履约段及履约参与关系不能被取消、删除或回写为未发生」——参与起点的更正是新段还是
同段新起点，与票 [02](02-actual-carrier-judgment-model.md) 的「不追溯覆盖未知期间」同族。

## 裁后要做的

领域（`domain.OffsitePickup` 的更正门）→ 端口（登记册的读回/落新版或失效口）→ 编排
（`RegisterOffsitePickupHandler.Correct` 或独立编排）→ 端点（`/transport-fulfillment/offsite-pickup-corrections`，
沿票 04 的形状）。四层一次建设，走 TDD。

## 边界

裁前不写代码。端点表照今天的样子不挂揽收更正口。

## 完成记录（MCP-3，2026-09-04）

分支 `mcp3-tf08`，开工基线 main `eba019a8`，收口前两次 rebase（`4cc1bc34` → `9e4e90bb` → `dc7d44e9`），零冲突。
**已验代码 tip `779f3c4b`**；分支 tip `0c3470ad` 只多一笔机制清点重生成（推送方在 tip 重生成时可丢）。不推，交 MCP-1
重放；分支 SHA 在重放后会换，票面留分支 SHA 作封存出处，main SHA 由推送方广播后对照。

各笔（分支 SHA · 范围）：

- `b29f9061` 票面认领。
- `d251c151` 领域：`OffsitePickup.Correct(PickupCorrection)`、`Corrects`/`CorrectedAt`、`RehydrateOffsitePickup`（`domain/offsite_pickup.go`、
  `domain/offsite_pickup_rehydration.go` + 两份测试）。
- `117be7b8` 迁移 `transport_fulfillment/0015_offsite_pickup_version_chain.sql`（主键纳入 `pickup_version`、加 `corrects_version`/`corrected_at`、
  链一致 CHECK、部分唯一索引 `offsite_pickup_one_first_registration` 与 `offsite_pickup_corrects_once`）+ `adapters/postgres/offsite_pickup_registry.go`
  （`FindByKey` 按回指派生当前版、`Save` 写链、读回走重建门）+ 真库用例。
- `e92208d4` 编排：`RegisterOffsitePickupHandler.Correct`、`CorrectOffsitePickupCommand`、`PickupCorrected`（`application/register_offsite_pickup.go`、
  `correct_offsite_pickup_test.go`；首登测试替身改按键存版本链）。
- `210b3cfc` `adapters/postgres/offsite_pickup_registration_handoff.go`：更正版本信封 ID 加版本段、载荷加 `pickupVersion`，首登 ID 与分区键不动。
- `9e980842` 端点 `/transport-fulfillment/offsite-pickup-corrections`（`adapters/http/correct_offsite_pickup.go` + 测试）、`PickupCorrectionIntake`/
  `PickupCorrectionHandler` 另立不动既有接口、`UnconfiguredIntake.IntakePickupCorrection`、`register_offsite_pickup.go` 加 `corrects` 响应格与更正 201。
- `766d3418` `cmd/parcel-api/assemble_offsite_pickup_correction.go`（+真库装配用例）、`endpoints.go`/`endpoints_test.go`/`unwired_orchestration.go`/`main.go`
  TF 那组各一处（MCP-6 释号后按 MCP-1 指令直接写）。
- `e0f70463` 立 draft 票 label-channel/24 与本目录 10。
- `779f3c4b` 双轴评审修复：登记册注释改按约束名引用不计数；真库装配用例补无前版 / 早于登记时刻 / 沿用版本号三格拒绝。

**偏离裁决字面处，都写在代码注释里**：① 「读回前版」实施为「读回当前版并要求指名前版就是当前版」——登记册与 PS 都按键只读一个当前版，
链必须线性（一版最多被更正一次，库内索引兜底）；指名已被更正过的版本，同内容答已有版本、异内容答冲突。② 「更正时刻不得早于被更正版本的
登记时刻」在编排守（登记时刻是 `OffsitePickupRecord.RecordedAt`，不在领域对象上；不拿 `OccurredAt` 顶替——它是可更正的四格之一）。
③ 「新版本走既有表」走不通：0005 主键不带版本，故立 0015（MCP-1 同意）。④ 首登与更正版本共用一种内容比对锚，首登被更正后原内容重放
答冲突（`Register` 注释）。

验证（隔离 worktree 干净检出，钉 `779f3c4b`，基 `dc7d44e9`）：`gofmt -l .` 空；`go build ./...`、`go vet ./...` 退 0；无 DSN `go test -count=1 ./...`
退 0、96 ok / 0 FAIL；含 DSN `go test -count=1 -v ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...`
退 0，`--- PASS` 1429 / `--- SKIP` 0 / `--- FAIL` 0；探针一正一反：`TestAPickupCorrectionLandsAsANewVersionAndTheOriginalStays` 与
`TestACorrectedPickupVersionIsHandedOffAsItsOwnIntent` 带 DSN 为 PASS、不带为 SKIP。含 DSN 全仓 `go test -p 1 -count=1 ./...` 在前一基线
`9e4e90bb` 上的同内容 tip 跑过一次：95 ok / 0 FAIL（`dc7d44e9` 新进的是 VE 与 `cmd/parcel-api/assemble_claims*`，后者所在包已在上面含 DSN 重跑）。
真库用例覆盖派单点名的五格：更正落新版本回指前版且原行不动、无前版未受理、沿用版本号库面拒（主键）、更正时刻早于登记时刻拒、
重交意图带新版本（Outbox 第二份，ID 带版本段）。测试输入全为隔离合成，只记 `S`。

## Comments

- 2026-09-04 · MCP-3：**PS 采用口对同对象第二版本的处置，与裁决预期不同——今天不是「再判一次」。** 量到（main `4cc1bc34`）：
  `psinbox.OffsitePickupConsumer` 不读 `pickupVersion`；`AdoptOnOffsitePickupAdapter.HandleRegisteredOffsitePickup` 按（租户+对象+尝试）
  `FindByKey` 读到的是**当前版**（本票之后即链尾）；`AdoptNetworkIntakeHandler.Handle` 的采用键带版本，新版本不撞幂等，但随后
  `FindResponsibilityStart`（`AT-PS-049`）命中首登版本，更正版本落 `SOURCE_NOT_ADOPTED`，依据 `RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/<首登版本>`。
  `UC-PS-003`「一致性」节与 `AT-PS-050` 写的是「来源更正形成新的采用判断版本」，代码里没有分「同来源更正」与「另一来源竞争」的那一格。
  按 MCP-1 指令：**不改 PS**，立 [label-channel/24](../../label-channel-service-first-release/issues/24-source-correction-version-refused-as-second-responsibility-start.md)
  记事实不写方案。TF 侧照裁决落新版本并重交意图——链到 PS 采用口为止今天是断的，下一个人别以为已通。
- 2026-09-04 · MCP-3：裁决附问的段侧那半边立 [10](10-source-correction-rederives-participation.md)（draft，覆盖交接与揽收两种来源）。
- 2026-09-04 · MCP-1（接任）：**进 main 记录——分支 SHA → main SHA 对照。** 上一任 MCP-1 重放推出后未及广播即崩溃，对照由 MCP-3 逐文件核后转交
  （已验 tip `779f3c4b` 与 `origin/main` 在本票全部文件上 `git diff` 为空）；本任于 21:5x 以 `git cherry origin/main 779f3c4b dc7d44e9` 复核，九笔全为 `-`
  （patch-id 等价），`0c3470ad` 为 `+`，与下列一致：
  `b29f9061`→`91fab19d`（票面认领）· `d251c151`→`5746b45b`（领域）· `117be7b8`→`8db97256`（迁移 0015 + 登记册）· `e92208d4`→`ea6c6963`（编排 Correct）·
  `210b3cfc`→`8327bb90`（handoff）· `9e980842`→`58eb85ba`（端点）· `766d3418`→`54ae9fdc`（parcel-api）· `e0f70463`→`5ec41b0f`（两张 draft 票）·
  `779f3c4b`→`d52261b7`（评审修复）· `2c82caf0`→`27116c52`（票面转 resolved）。清点笔 `0c3470ad` 未重放，tip 上由 `3bbb1b08` 重生成（一并吸收 label-channel/23）。
  `git ls-remote origin main` 于 21:5x 为 `cc7f4e97`，含以上全部。分支 `mcp3-tf08` 指针保留（tip `2c82caf0`），其 worktree 已由 MCP-3 拆除（未 `--force`）。
