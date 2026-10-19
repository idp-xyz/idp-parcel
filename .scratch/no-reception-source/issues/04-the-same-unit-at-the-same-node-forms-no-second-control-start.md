# 04 第四期：同一作业实物在同一节点已有当前收寄时并入、不另起控制起点；仅扫描的待确认由后来的明确接收或拒收续办

Category: enhancement
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（02、03 进 main 后开工），分支 `claude-nrs04`，NO 迁移号 0009；与 no-reception-identity/06 并行。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 03 进 main。功能上只依 02（来源文档给出待确认与拒收行的作业实物，前版机制给出续办）；与 03 同改收寄编排、PG 适配器与迁移号，按票号串行落。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「四、同一作业实物重复控制起点的挡法」与「同一作业实物按什么认」「仅扫描的待确认怎样续办」两问；ADR-0199 决定四。落地次序排在第三期（票 05）之前：翻转的前提里有一项就是本票的挡法。
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/ports/ports.go（记录种类加「并入已有收寄」、并入版本）
- internal/nodeoperations/adapters/postgres/reception.go 及其测试（按作业实物的咨询锁、找当前收寄）
- internal/nodeoperations/adapters/postgres/ 下读收寄册的各面：review_catalogue.go、parcel_containment_view.go、intake_identity_view.go 及其测试（只在需要按种类筛时改）
- migrations/node_operations/（记录种类 CHECK 扩一种、并入版本列、按（租户，作业实物，节点）的索引）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go 及其测试
- cmd/parcel-frontline-import/intake_import.go
- cmd/parcel-api 下新起真库用例文件（并发用例另起一份）
- docs/adr/0196-one-mark-on-two-units-is-an-identity-conflict-judged-by-the-associated-parcel.md（文末追加复核记录，决定原文不动）
- docs/adr/README.md（0196 一行标部分停用）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `91ea0883`，读码；核验探针在 `8d22729f` 实测过）

- 收寄键只有（租户，来源）。同一作业实物换个来源重扫，形成又一条收寄、又一个控制起点；探针实测同一作业实物两行、两个控制起点。UC-NO-002「一致性、幂等与并发」：「同一实物同一客户交付边界只能形成一个当前有效节点收寄判断和一个控制起点」。
- ADR-0196 决定一明说「同一件实物换个来源再收一次照常形成收寄」，用例 `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 钉的正是这一格；ADR-0196「能力边界」里冲突形成之后先到那件换来源再收照常答`节点收寄已形成`、交发布意图。
- 仅扫描的`收寄判断待确认`之后来了明确接收或拒收，两条判断互不相认，待确认没有续办。

## 做什么

照 ADR-0199 决定四：

1. **按（租户，作业实物）串行**：认同一实物之前取事务级咨询锁，所有分支都取；唯一识别那一支随后再取（租户，包裹）锁，次序固定先实物后包裹。键的派生与 no-reception-identity/03 的锁键口径对齐。
2. **认当前收寄**：同一实物在同一节点，有一条判断链的当前有效版本是形成或待识别、且控制没有转出。
3. **并入**：有当前收寄时，三种观察都落一行「并入已有收寄」（来源文档照存、指名并入的那一版、不带收寄与控制、不交意图），答 `EXISTING_RESULT`，`recordedOutcome` / `currentVersion` 是那一版的。
4. **续办**：当前有效版本是待确认时，后来的明确接收、拒收或又一次仅扫描照常形成新判断，以那条待确认为前版；待确认原行不改。`节点收寄未形成`之后的再次交付不指名前版。
5. **读面不误读**：复核目录、方向依据身份一格的读口、`ParcelContainmentView`、`FindOtherUnitIntakes` 逐面核过「并入」行：不带收寄与控制的行本就读不到的，用例钉住；要按种类筛的，改。
6. **改用例**：`TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 里「同一件实物换个来源再收一次照常形成收寄」那一段，改钉并入（同一节点）；另补不同节点照常形成收寄。冲突形成之后先到那件换来源再收，改钉并入到那件的冲突版。
7. **ADR-0196 复核记录**：决定一末句与「能力边界」那一句自本票落地起部分停用，范围只限同一节点上有当前收寄的情形（ADR-0199 决定四）；adr/README 的 0196 一行加「部分停用」注，照 0184 一行的写法。
8. **AT 点名**：`AT-NO-049`、`AT-NO-050`。

## 不做

- 控制转出的生产接线（`ControlTransferAdapter.TransferOut` 今天只有测试调用；ADR-0199 能力边界：转出接上之前，离开节点再回来的实物会被并入旧收寄）。
- 作业实物标识在租户内是否唯一的核验（ADR-0199 越权风险点）。
- 身份核对读不动的翻转（票 05）。

## 完成判据

- [x] 编排用例：同一实物在同一节点已有当前收寄（形成、待识别各一），换来源再来明确接收、明确拒收、仅扫描：各落一行并入、不建控制、不交意图，答已有结果带那一版的种类与当前有效版本，册上只有一个控制起点（`AT-NO-049`）。
- [x] 编排用例：不同节点照常形成收寄；`节点收寄未形成`之后再来照常形成判断、不指名前版。
- [x] 编排用例：仅扫描待确认之后的明确接收与拒收各一条，新判断以待确认为前版，待确认原行逐字不变；重投待确认的来源答已有结果，`recordedOutcome` 是 `RECEPTION_UNDECIDED`、`currentVersion` 是新判断的版本（`AT-NO-050`）。
- [x] `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 已按上面改钉；no-reception-identity/01、03 其余用例照旧绿。
- [x] 真库并发用例（照 no-reception-identity/03 的做法，池外独立连接看 `pg_stat_activity` 的等锁，带超时）：同一实物两个来源两条连接并发送站，后到那次等在实物锁上，前者提交后它答已有结果，册上只有一个控制起点；与 03 的同包裹并发用例同跑不死锁。
- [x] 读面用例：复核目录、方向依据身份一格、`ParcelContainmentView`、`FindOtherUnitIntakes` 各一条，「并入」行不被读成收寄或关联。
- [x] ADR-0196 文末复核记录已追加、adr/README 0196 一行已加部分停用注；`git diff` 对 ADR-0196 只有追加行。（例外一处：Status 行照 adr/README「部分停用」约定加前向指针，那一行是改不是追加；决定原文与此前各节逐字未动，见完成记录「越出地盘的改动」。）
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nrs04`；在 `claude-nrs03` tip `3d6d971c` 上写成，收口前 rebase 到 main `d7283946`）

**提交**

| 提交 | 内容 |
|---|---|
| `f5b4e708` | 实施与用例：收寄编排认同一作业实物、并入与续办；收寄库按实物与节点的一问与实物锁；迁移 0009；HTTP 答复形状头注；前线导入明细写明并入 |
| `f25899f3` | ADR-0196 复核记录三，Status 行与 Links 的前向指针，adr/README 0196 一行的部分停用注 |
| `959fb8e3` | 机制清点在 `f25899f3` 的干净检出上重生成：node_operations 迁移 8→9、全库 231→232；nodeoperations 生产 51→53、测试 43→46 |
| 本笔 | 完成记录；AT 覆盖盘点 `AT-NO-049`、`AT-NO-050` 由②入① |

**落点**

- **收寄编排**：`internal/nodeoperations/application/receive_delivered_unit.go` 改 `Handle`、`receive`（多一个前版参数）、`existingResult`（并入行的重投答它并入的那一版），结果加访问器 `MergedInto()`；新文件 `receive_same_unit.go` 放 `currentIntakeAmong`、`undecidedToContinue`、`merge`、`mergedResult`。次序：幂等键认出重投与来源冲突 → 接收观察不在三态之内先拒 → `FindCurrentJudgmentsAtNode`（取实物锁）→ 有当前收寄即并入 → 否则有待确认即以它为前版 → 照旧分派三种观察。并入在身份核对之前认出，不问 PS。
- **端口**（`ports.go`）：记录种类封闭集只加 `RecordMergedIntoExistingIntake`（`MERGED_INTO_EXISTING_INTAKE`）一项；`ReceptionRecord.MergedInto`；`ReceptionStore` 末尾加 `FindCurrentJudgmentsAtNode` 与 `FindByJudgmentVersion`（后者与 `PendingIdentityLookup` 同名同形，生产上 `Receptions` 早已实现）。
- **收寄库**：`reception.go` 只在列清单、行扫描、写入与种类译码各加一格；按实物与节点的一问放新文件 `reception_unit_lookup.go`。新函数另起文件，是为了与并行的 no-reception-identity/06 少碰同一段。
- **HTTP**：`receptionResponse` 头注写明并入的答法，线上形状不变。
- **前线导入**：`intakeRowDisposition` 的已有结果一支在明细前加 `describeMerge`；去向照旧记重放。

**记录形状与迁移**

| 格 | 「并入已有收寄」那一行 |
|---|---|
| kind | `MERGED_INTO_EXISTING_INTAKE` |
| 键、content_digest | 这次送站的（租户，来源标识）与 NOC-1 摘要；同一来源重投照旧按指纹分重放与来源冲突 |
| source_document | 这次送站的 NRS-1 来源文档，三种观察原样 |
| merged_version | 并入的那一版判断：那件实物在那个节点上当前收寄所在判断链的当前有效版本 |
| judgment_version、predecessor_version | 空。并入不是判断，也不是后继 |
| intake、control | 空（0001 的在场规则照旧守） |
| candidates、service_markers、identity_conflict、refusal_reason、service_result_check | `[]`、`[]`、false、空串、空；拒收原因只在来源文档里 |

迁移 `migrations/node_operations/0009_reception_merged_into_existing_intake.sql`：重建 `reception_kind_closed`，扩出这一种；加列 `merged_version`；`reception_merge_shaped` 守「种类与并入版本同真同假，并入行的并入版本不空白、不带判断版本与前版、必带来源文档」；部分索引 `reception_judgments_by_unit_and_node`，键是（租户，`COALESCE(intake->>'unit', source_document->>'unit')`，`COALESCE(intake->>'node', source_document->>'node')`），只覆盖不是并入行、有判断版本的行。

**认当前收寄与续办的取法**

- 实物与节点：形成与待识别按收寄上的认，未收寄与待确认按来源文档认。ADR-0196 的冲突版来源文档里是后到那件，收寄上是先到那件，按收寄认才归在先到那件名下。
- 当前收寄：这件实物在这个节点上各链的当前有效版本里，种类是形成或待识别、控制没有转出（`PhysicalControl.Active()`）的那一条。本票之前同一件实物在同一节点各自形成过收寄时可能不止一条，取接收时间最晚的，同刻取来源标识在前的；这与 ADR-0196 冲突版依哪一行追加是同一个口径。
- 续办：链头是待确认的那一条。本票之前同一实物同一节点仅扫描过多次、各成一链时，取发生时刻最晚的，同刻取来源标识在前的，其余照留。新判断以它为前版，原行不改。`节点收寄未形成`不续办。
- **HTTP 并入那一格的答复**：200 `EXISTING_RESULT`。响应体各格照并入的那一版判断转写：`intakeVersion`、`unit`、`node`、`association`（形成格才有）、`controlBasis`、`candidates`、`serviceMarkers`、`serviceResultCheck`。`recordedOutcome` 是那一版的种类，`currentVersion` 是那一版所在链此刻的当前有效版本，首次并入时就是那一版。这次送站带的拒收原因不进答复，答复说的是并入的那一条。线上不另立「并入」一格：设备照 `outcome` 出队，并入与重投是同一种处置。应用层另有 `MergedInto()`，前线导入明细据它写「并入已有收寄 <版本>（本行来源已保全，未另起控制起点）」。重投并入行的来源时答法同首次并入：先按 `FindByJudgmentVersion` 读回那一版，读不回就不形成答案。

**旧行无判断版本的取法**（票 02 记下的那一格）

第一期之前落下的拒收行与仅扫描行没有判断版本，也没存作业实物与节点（迁移 0007 头注）。收寄库的这一问认不出它们属于哪件实物。本票的取法是不续办、不补签：后来的观察照首判办，不指名前版，旧行原样留着。补签版本等于在册上造一版当时并不存在的判断。由此留下的代价是：第一期之前只扫描过、本票之后才明确接收的实物，那条旧待确认不会被接续；重投那条旧来源照第一期的口径答已有结果，`currentVersion` 缺席。拒收行本来就不续办，不受影响。取法写在 `undecidedToContinue` 的头注与 `FindCurrentJudgmentsAtNode` 的端口注里。

**锁**

- 实物锁的写法是 `pg_advisory_xact_lock(hashtext('node_operations.reception.handling_unit'), hashtext(租户 || chr(31) || 作业实物))`，锁键口径与 no-reception-identity/03 的包裹锁相同，锁空间名不同。锁在实物上，不在节点上。读口无事务即拒（`RequireExecutor`）。
- 次序：三种观察都先取实物锁，唯一识别那一支随后在 `FindOtherUnitIntakes` 里取包裹锁。ADR-0196 给先到那件追加冲突版时不取那件的实物锁：那时已握着包裹锁，再取实物锁就是先包裹后实物，会与别的收寄交叉等待。
- **重核不取实物锁**（对 ADR-0199「重核同此」的读法，代决）。重核不形成控制起点，接收时刻与控制都照原判断，只取包裹锁。按号重核在一笔事务里会逐件重核多件实物，若每件都取实物锁，第二件起就是握着包裹锁再取实物锁，正好交叉。现在没有任何一处先取包裹锁再取实物锁，「次序固定」照样成立。代价：并发的重核或冲突追加可能在并入读完链头之后才接续那一条，那时并入行指名的是读到时的链头；答复里的 `currentVersion` 由提交前现读的链头给出，控制起点仍是一个。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 编排·并入（`AT-NO-049`） | `internal/nodeoperations/application/receive_delivered_unit_same_unit_test.go::TestTheSameUnitAtTheSameNodeIsMergedIntoItsCurrentIntake`（当前收寄是形成格与待识别格，各配明确接收、明确拒收、仅扫描，每一支都含重投并入的那一来源） |
| 编排·别的节点与未收寄之后 | 同文件 `TestTheSameUnitElsewhereOrAfterARefusalFormsAFreshJudgment` |
| 编排·续办（`AT-NO-050`） | 同文件 `TestAScanOnlyJudgmentIsContinuedByTheLaterObservation`（明确接收唯一识别、明确接收零候选、明确拒收、又一次仅扫描） |
| 三种观察都取锁、这一问读不动不形成答案 | 同文件 `TestEveryObservationAsksForTheUnitsCurrentJudgmentsFirst` |
| `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict` 改钉 | 同一节点换来源并入；冲突之后先到那件再来，并入它的冲突版（原判断种类待识别、不交意图、不再追加）。no-reception-identity/01、03 其余用例未改、全绿 |
| HTTP 面 | `internal/nodeoperations/adapters/http/receive_delivered_unit_same_unit_test.go::TestTheSameUnitAgainAnswersTheIntakeItWasMergedInto` |
| 真库面 | `cmd/parcel-api/reception_same_unit_test.go::TestTheWiredReceptionMergesTheSameUnitAtTheSameNode`（经 HTTP 端点，三种观察，另一个节点）、`::TestTheWiredReceptionContinuesAScanOnlyJudgment`（待确认原行按 `row_to_json` 逐字比对） |
| 真库并发 | `cmd/parcel-api/reception_same_unit_concurrent_test.go::TestConcurrentDeliveriesOfOneUnitSerializeOnTheUnit`：后到那次等在实物锁上，前者提交后答已有结果，一个控制起点、意图一封。`::TestConcurrentDeliveriesOnTheUnitAndTheParcelLocksDoNotDeadlock`：A 握着实物锁与包裹锁，B（同一件实物）等实物锁，C（贴同一个号的另一件）等包裹锁，三方都走完、没有死锁。观察走池外独立连接，查的是 `pg_locks` 而不是 `pg_stat_activity`，这样能按锁空间分出等的是哪一把。C 另开一只连接池，因为测试池只有两条连接。两条都与 03 的 `TestConcurrentDeliveriesOfOneMarkSerializeOnTheParcel` 同跑，连跑 5 次全绿 |
| 读面 | `internal/nodeoperations/adapters/postgres/reception_same_unit_test.go::TestAMergeRecordIsNotReadAsAnIntakeOrAnAssociation`：复核目录收寄册与待识别册、方向依据身份读口 `FindIntakeIdentities`、`FindOtherUnitIntakes`、`FindRecheckableByMark`（并入行的来源文档带同一个号）、`ParcelContainmentView`。各面都靠并入行不带收寄、关联与候选，或靠按种类筛，读不到它；没有一面要改 |
| 存储面 | 同文件 `TestCurrentJudgmentsAtANodeAreTheChainHeadsOfThatUnitThere`、`TestTheUnitLockSerializesOnlyTheSameTenantAndUnit`、`TestTheUnitLockedReadRefusesToRunOutsideATransaction`、`TestAMergeRecordIsReadBackUnchanged`、`TestTheReceptionTableRefusesAMisshapenMergeRecord` |
| 受控批量口 | `cmd/parcel-frontline-import/intake_same_unit_test.go::TestFrontlineImportContinuesAndMergesRowsOfTheSameUnit` |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs04-red.log`。钉在 `3d6d971c` 上，加未提交的用例与 API 骨架（种类、字段、端口方法、结果访问器、PG 空桩），编排、适配器与迁移都还没改。现象原话摘几条：并入那一格 `并入 outcome = "INTAKE_NOT_FORMED" err = <nil>, want EXISTING_RESULT`，明确接收那一支 `err = resolve parcel identity: … 并入不该再问 PS`；续办 `新判断 版本 = "intake-result/v2" 前版 = "", want 自己签一版、以待确认 "intake-result/v1" 为前版`；改钉的那条 `同一件实物换来源再来 outcome = "INTAKE_FORMED" err = <nil>, want EXISTING_RESULT——并入当前收寄`；HTTP `同一件实物再来 201 "INTAKE_FORMED", want 200 EXISTING_RESULT`；真库 `RECEIVED 再来 201 "INTAKE_FORMED" …` 与 `新判断 前版 = "" 控制 = true, want 以待确认 … 为前版`；库面 `violates check constraint "reception_kind_closed"`；锁 `同租户同一件实物·同一节点 err = <nil>, want 等在锁上直到 lock_timeout（55P03）`；并发 `没在时限内等到 map[node_operations.reception.handling_unit:1 node_operations.reception.parcel_association:1]`；前线导入 `第 4 行 factRef=SYN-SLIP-NRS04-3 → INTAKE_NOT_FORMED [已落地]，要 EXISTING_RESULT [重放]`。`TestTheSameUnitElsewhereOrAfterARefusalFormsAFreshJudgment` 修前就绿：它守的是本票不该碰的那一格。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs04-green.log`，含并发用例与 03 的同包裹并发用例同跑、连跑 5 次（rebase 之前）。门的输出：rebase 之前在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs04-gates.log`，rebase 之后在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs04-gates-rebased.log`（后者含并发用例同跑 5 次）。

**门**（rebase 到 main `d7283946` 之后在干净工作树 `959fb8e3` 上重跑一遍；rebase 之前在 `3d6d971c` 之上的工作树也跑过，两个基底只差三张 `.scratch` 票面，代码相同）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./cmd/parcel-frontline-import/... ./migrations/... ./internal/architecture/...` 全部 ok。没有跑全量 `./...`。机制清点在 `f25899f3` 的干净检出上重生成，与 rebase 之前生成的逐字相同。

**证据层级**：`S`。夹具全为 `SYN-` 合成值，没有租户取值，也不碰演示库。

**越出地盘的改动**

1. `cmd/parcel-api/assemble_reception_test.go::TestTheWiredReceptionAnswersHonestlyAgainstARealDatabase`：拒收那一支换成另一件实物 `SYN-UNIT-REFUSED-1`。原来是同一件实物先明确接收再拒收，本票之后第二次会被并入，不再是未收寄；这条用例要证的是拒收支真落库，所以换实物。
2. `cmd/parcel-frontline-import/intake_same_unit_test.go` 是新起的文件，地盘只列了 `intake_import.go`。
3. `internal/nodeoperations/adapters/http/receive_delivered_unit_same_unit_test.go` 给 `recheckStore` 补了 `FindCurrentJudgmentsAtNode` 方法。`recheckStore` 定义在 `recheck_pending_identity_test.go`，那个文件没动。
4. `ports.go` 里 `PendingIdentityLookup` 头注改了半句分立理由：收寄编排现在也按判断版本取一行。
5. ADR-0196 的 Status 行加了前向指针，这是部分停用约定要求的；「只有追加行」那一格的注里写明了。

**没做**

- 「走 Go 那条路」一格：非作者评审与推送方在干净检出上带 DSN 跑全量都还没做，格未勾。
- 票面「不做」三项照旧：控制转出的生产接线、作业实物标识在租户内唯一的核验、读不动的翻转（票 05）。
- 重核不取实物锁。这是上面「锁」一节的取舍，不是漏做。
- **给推送方的合并提醒**：
  - no-reception-identity/06 的迁移 0010 排在 0009 之后。它若重建 `reception_kind_closed`，词表要带上 `MERGED_INTO_EXISTING_INTAKE`；若新增种类，要与 `reception_merge_shaped` 相容（非并入行不带 `merged_version`）。
  - 06 若往 `ReceptionStore` 加方法，本票新加方法的那几只替身（编排用例与 HTTP 用例的 `receptionStoreDouble`、`recheckStore`）同样要补。
  - 06 的处置版接在链上、带收寄与控制。本票的 `FindCurrentJudgmentsAtNode` 会把它读成链头、并入它，这与「并入当前有效版本」同义，不用另改。
