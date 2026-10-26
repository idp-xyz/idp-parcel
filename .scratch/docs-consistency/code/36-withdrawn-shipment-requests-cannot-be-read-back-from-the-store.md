# 36 PS 已撤回委托在真库上读不回：撤回决定不进快照，重复撤回与撤回重提关联走不通

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code36
Blocked by: 无
出处：文档自洽 06 修订时复核 UC-PS-005 `AT-PS-068`、`AT-PS-076` 与 [ADR-0255](../../../docs/adr/0255-a-submitted-request-is-regrouped-only-by-withdrawing-it-and-submitting-linked-new-requests.md) 的路径时读码所得，不在审计报告条目里；UC-PS-001、UC-PS-005「给开发的交接」。

## 现象

委托撤回之后，仓储按来源身份读它，重建门以 `ErrRehydrationStateNotSupported` 拒绝。读那份委托的每一条路——重复撤回、撤回后关联重提、查询——在真库上都报错。

## 证据

- `internal/parcelshipment/domain/rehydration.go` 的 `admitRehydratedState` 只放行`已提交`、`已接受`、`已拒绝`，`已撤回`答 `ErrRehydrationStateNotSupported`；头注写明撤回在规格里还没有字段可填（ADR-0061 决定一的`已撤回`那一半至今未推进）。
- `internal/parcelshipment/adapters/postgres/shipment_request.go` 头注与测试（`shipment_request_test.go` 里把状态改成`已撤回`后 `FindBySourceIdentity` 报错的那一例）钉住这一点。
- 读它的路：`WithdrawShipmentRequestHandler.resolvePreserved` 与 `existing`（`AT-PS-068`）；`SubmitShipmentRequestHandler.resolvePriorClaim` 以 `LinkWithdrawnResubmission` 建关联（`AT-PS-076`，ADR-0255 决定一）。两处读不回都上抛，HTTP 答 5xx。
- 应用层测试用内存仓储，读得回撤回，所以这一格在单元测试里看不出来。
- 文档：PS CONTEXT 委托生命周期「原提交、版本、判断、撤回决定和补偿关系继续保留」。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：快照规格加撤回决定一格；重建门照 ADR-0030 → ADR-0061 → ADR-0188 的先例开到`已撤回`（立一份推进记录）；仓储往返与坏数据判据；真库用例覆盖重复撤回与撤回重提关联。

## 最小的诚实修法

1. `RehydrateShipmentRequestSpec` 与快照文档加撤回决定（决定标识、授权、请求方、原因、决定时刻）。
2. 重建门开到`已撤回`，照 ADR-0188 的写法立一份推进 ADR-0061 决定一`已撤回`那一半的记录。
3. 真库用例：撤回后重复撤回答原撤回与原补偿关联；撤回后以撤回重提方向提交关联新委托成立。

## 要裁的

- 存量：首发前只有演示租户，`--reset` 重灌即可；若已有撤回行没带撤回决定，按坏数据拒绝还是推回，随推进记录定。

## 分诊

- 分级 R2：答错且挡路，生产装配下客户面撤回口仍未配置 Intake，首方族接上撤回口即生效，故在接上之前修。
- 存量：首发前只有演示租户，`--reset` 重灌即可。已撤回但没带撤回决定的存量行，重建时按坏数据响亮拒绝，不推回、不补默认值。种子里没有会撤回委托的步骤（`scripts/demo-seeds` 里的撤回是送站预报与运输履约采纳的撤回，不是委托撤回），无需改种子。
- 推进记录用预留号 ADR-0286，照 ADR-0188 的写法推进 ADR-0061 决定一`已撤回`那一半；前驱链 ADR-0030、ADR-0061、ADR-0188。
- 迁移号：PS 下一个号 0031 预留给本票；快照是 jsonb 文档，撤回决定作为文档字段进快照，库面没有要加的列与约束，是否占号见完成记录。

## 完成记录（2026-10-26，分支 claude-code36）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 PS owner 作出，待 PS owner 复核。推进记录 [ADR-0286](../../../docs/adr/0286-withdrawn-shipment-request-rehydration-by-snapshot-expressiveness.md)。

### 做了什么

- 领域：`RehydrateWithdrawalSpec`（决定标识、授权引用、请求方、结构化原因、决定时刻）与 `RehydrateShipmentRequestSpec.Withdrawal`；`RehydrateShipmentRequest` 带回 `withdrawal`；`admitRehydratedState` 开到`已撤回`；新增 `withdrawnValidForRehydration`（边界已越过、撤回五格齐全、决定槽位与接受产物一概缺席、任务`已停止`且不在等待、任务处置不为`拒绝`），其余三个状态带着撤回（含半截）一律拒。头注改正。
- 仓储：快照文档加 `withdrawal` 一格；写入侧撤回在场时单独置上 `decisionFormed`（`AcceptanceDecision()` 对已撤回报告缺席，不能再拿它推边界）；读回逐格过领域构造门，残缺的撤回决定包着 `ErrInvalidRehydratedShipmentRequest` 上抛。`shipment_request.go` 头注与查阅读面、`parcel_target`、投影列等几处「重建门只开到……」的旧说法改正。
- 接受链：`AdvanceAcceptanceChainHandler.Handle` 在第一步之前见到`已撤回`即停（答`已决定`、状态`已撤回`、不带决定，与形成决定那一步撞上既有终局时的同一格，不新增结果格）。这一条不在票面，是开门的必然后果，见代决 4。
- 重建面门禁：`RehydrateWithdrawalSpec` 进 `rehydrationEntryIdentifiers`。
- 文档：ADR-0286；ADR-0061、ADR-0188 的 Status 注记与 Links、ADR-0030 的 Links 加前向指针；UC-PS-001、UC-PS-005「给开发的交接」里指向本票的两行改为已补；`.scratch/at-coverage-inventory.md` 给 `AT-PS-068`、`AT-PS-076`、`AT-PS-070` 补用例名并追记。PS CONTEXT 的规则句不挂票号、不用改，其头部只有固定指针。
- 没有迁移。快照是 jsonb 文档，撤回决定作为文档字段进快照，库面没有要加的列、约束或索引，故没有占预留号 0031，留给后来者。种子里没有会撤回委托的步骤（`scripts/demo-seeds` 里的撤回是送站预报与运输履约采纳的撤回），没有改种子，也就没有跑种子。

### 用例

- 真库（先红后绿，红都是「rehydration does not yet cover this shipment request state：WITHDRAWN」）：
  - `cmd/parcel-api` `TestASubmissionReadsBackAWithdrawnRequest`：原样重放被撤回的提交答既有结果；以撤回重提方向关联已撤回原委托的新提交成立并读回关联出处；方向不符答 `LINK_INELIGIBLE`；原委托仍是已撤回（`AT-PS-076`，ADR-0255 决定一）。
  - `cmd/parcel-api` `TestTheWiredWithdrawalReadsBackAnAlreadyWithdrawnRequest`：生产装配的撤回编排（真来源仓储、真委托仓储）读回已撤回委托，未决交回真实状态，重发同一撤回答原撤回。
  - `cmd/parcel-dispatch` `TestARepeatedWithdrawalReadsBackTheOriginalWithdrawalAndItsCompensation`：撤回成立而释放挂着补偿，重发同一撤回与换一个来源身份再撤，都答原撤回与原补偿引用，版本不动（`AT-PS-068`）。
  - `cmd/parcel-dispatch` `TestASubmittedEnvelopeConsumedAfterTheWithdrawalPlacesNoControl`：撤回之后才消费提交信封，不占资金、按处理完毕入账（`AT-PS-070`）。
  - 仓储 `TestAWithdrawnRequestRoundTripsWithItsWithdrawal`（快照落五格与边界标志、读回逐格相等）、`TestWithdrawnRowsWithoutOrWithBrokenWithdrawalAreInvalidOnRead`（九种缺失或残缺，补表达前的已撤回行是第一例）、`TestAStateColumnAheadOfItsSnapshotIsBadDataOnRead`（原「已撤回读回报不支持」那一例，改成坏数据）。
- 领域：`TestRehydratingAWithdrawnRequestKeepsItsWithdrawal`、`TestRehydrationRefusesImpossibleWithdrawnShapes`、`TestRehydrationRefusesAWithdrawalOnAnyOtherState`、`TestEveryNamedShipmentRequestStateIsAdmittedByTheDoor`（日后新增委托状态时变红）；`TestEveryShipmentRequestFieldIsClassifiedForRehydration` 去掉「必然缺席」一份名单。
- 应用：`TestAWithdrawnRequestIsNotJudgedAgain`（红在「apply-financial-control」被调用）。
- 查询读面（`ShipmentRequestViews`）直读文档、不经重建门，原来就交代`已撤回`；测试注释改正，未加撤回摘要，见越权风险点 3。
- `cmd/parcel-dispatch/control_release_refusal_test.go` 原先因读不回改从投影列取状态，现在直接经仓储读回并核撤回决定在场。

### 代决

1. **缺撤回决定的存量行响亮拒绝，残缺同此**（协调方定口径，作者定判据）：撤回决定五格缺任一格、状态列写已撤回而文档是已提交形状、边界标志没越过，都是 `ErrInvalidRehydratedShipmentRequest`，不折成「不支持」，不推回，不补默认值。理由：授权、请求方与原因是当时的事实，没有第二个来源；UC-PS-005 明禁默认撤回授权。
2. **撤回决定自带快照格，不复用决定槽位或主动拒绝留痕格。** PS CONTEXT 明禁撤回与拒绝互相冒充，聚合已给撤回自带槽位，快照与之同形。
3. **任务处置与撤回的互斥格：处置`拒绝`的任务不可能是已撤回；处置`交客户补充`的任务可以是已撤回。** 来自 `WithdrawByCustomer` 要求决定边界未越过这一现状。
4. **接受链在已撤回委托上停下，且不新增结果格。** 审计读码（独立只读代理核过每个 `FindBySourceIdentity` 调用方）发现：门一开，撤回之后才消费的「委托已提交」信封会让链三步照常走，财务控制为已撤回的委托占下资金，而撤回已放过各版占用，新占用没有路径再放；形成决定还会耗决定标识。先写红用例（红在 `apply-financial-control`）再加守卫。结果取形成决定那一步撞上既有终局时的同一格（`已决定`、不带决定），消费门原有分支已覆盖，所以没有新结果格（ADR-0025）。这一条超出票面的三条，但是开门的直接后果，不修就是用一个泄漏换另一个失败。
5. **`ErrRehydrationStateNotSupported` 保留。** 它今天无状态落入，留着是为日后新增状态走 ADR-0030 的两步顺序；`TestEveryNamedShipmentRequestStateIsAdmittedByTheDoor` 守这一点。是否退役由 PS owner 定。
6. **不占迁移号 0031。** 理由见上；协调方若仍要一条迁移，需要说明库面要加什么。
7. **查阅读面不加撤回摘要。** 那是读面与 HTTP 接口契约的改动，且 ADR-0025 要求每个消费方落点，不在票面最小修法内。

### 越权风险点

1. **缺撤回决定的存量行响亮拒绝。** 若首发前有演示租户之外的已撤回行，读它们的编排都答读不回，只能 `--reset` 或人工补；没有补登路径，因为没有第二个来源可补。
2. **互斥格（代决 3）。** PS owner 若日后允许别的撤回时机，对账表要改。
3. **查阅详情不带撤回决定。** `ShipmentRequestDetailRecord` 与 HTTP 详情体只有接受或拒绝决定摘要；快照现在有这一格，补上它是读面与接口契约的改动。
4. **取消包裹（`CancelParcelHandler`）与终局结果（`FormParcelFinalHandler`）没有委托生命周期闸门。** 只核成员属当前版本，不核委托是否`已接受`。已撤回（同样已拒绝、已提交）委托的包裹走到它们时，先前被读回错误挡着（已撤回），现在由它们自己答。取消的授权角色在生产装配未配置、没有责任结果会为从未接受的包裹送达，所以今天是潜在缺口；不是撤回特有（已拒绝自 ADR-0188 起同样），已立 code/101（R1，首方取消口硬前置，Blocked by 本票）。
5. **人工复核完成与授权处置先问授权再核状态**（审计 A4）：已撤回委托上`任务已完结`那一格，在授权未配置或不可用时到不了。次序是原有的，与撤回无关。
6. **`ErrRehydrationStateNotSupported` 无人产出**（代决 5）。
7. **接受链守卫只看`已撤回`。** 已接受或已拒绝的委托再收到提交信封，仍是三步照走、末了撞上既有决定；这是原有行为，审计没有把它列为问题，本票不动。

### AT

`AT-PS-068`、`AT-PS-076`、`AT-PS-070` 补用例名（`.scratch/at-coverage-inventory.md` 追记）。没有新立 AT 号，`at-coverage-counts -check` 退 0。

### 验证（分支 claude-code36，已 rebase 到 origin/main `27d77d97f`）

- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出。
- `TMPDIR=… scripts/test-full.sh`：范围 = 改动包（`internal/parcelshipment/domain`、`adapters/postgres`、`application`、`ports`）+ `go list -test` 算出的反向依赖（共 51 个包，含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-commercial`、`cmd/parcel-settlement-register`、`tests/bentocontract` 与各上下文里读 PS 的适配器）+ `./internal/architecture/...` + `./migrations/...`，全绿（退出码 0）。
- 新用例另跑 `-count=3`（领域、应用、仓储、parcel-api、parcel-dispatch 各自的新用例）全绿。新写者与锁都没有（本票只读写既有快照列与既有链），所以没有并发用例，也没有锁、去重、封顶这类变异。
- 变异（备份、改、跑、从备份拷回并 `cmp` 核一致，共 12 处，全部被杀）：重建门又不放行`已撤回`（领域、仓储、parcel-api、parcel-dispatch 四处红）；读回丢撤回决定；写入丢决定边界标志；写入丢撤回原因；撤回重提关联不认已撤回（领域、parcel-api 红）；其余三个状态带半截撤回放过（`formed` 代替 `anyRecorded`）；已撤回的任务不必停止；已撤回的撤回决定残缺放过；已撤回带决定放过；已撤回不必越过边界；重复撤回丢原撤回（parcel-dispatch、应用层红）；接受链不拦已撤回（应用、parcel-dispatch 红）。
- `go run ./tools/at-coverage-counts -check` 退 0（没有新立 AT 号，总览不过期）；`go run ./tools/adr-index -check` 退 1，只是生成物 `docs/adr/README.md` 过期（协调方进 main 时统一重生成），本地跑过生成器确认 ADR-0286 的 Status 注记能被索引读出，随后原样拷回并核一致，没有提交生成物。
- 没有改迁移与种子，没有跑种子（种子里没有会撤回委托的步骤）。
- 没有碰 `cmd/parcel-api/assemble_submission.go` 与 `internal/parcelshipment/adapters/pilotgovernance/production_ownership.go`（另一会话在改）；`cmd/parcel-api` 只新增了一个测试文件。

### 评审记录与修订轮

独立评审通过，合入前要求补下列小项，已在本分支另加提交处理，不改业务逻辑：

- **N1 半截撤回只钉了一格。** `TestRehydrationRefusesAWithdrawalOnAnyOtherState` 的半截样本原来只有原因一格；改为决定标识、授权、请求方、决定时刻、原因各造一个「只剩这一格」的样本，挂在`已提交`、`已接受`、`已拒绝`上。变异核验：`Withdrawal.anyRecorded` 的五格逐个删掉各跑一次，五次全红；每次从备份拷回并 `cmp` 核一致。
- **N2、N3 注释里的计数与「今天」。** 重建门状态数的写法（「四个状态」「今天四个状态都已开门」「今天只开到`已提交`」）改成现行规则句「每个有名字的委托状态」。范围：`rehydration.go` 的 `ErrRehydrationStateNotSupported`、`RehydrateShipmentRequestSpec`、`admitRehydratedState` 头注，`ShipmentRequests`、`ShipmentRequestSummaryRecord` 头注，`rehydration_gate_test.go` 的哨兵说明串，`revision_test.go` 与 `shipment_request_test.go` 的同类句；`SourceDataVersionRecords` 头注「聚合重建门今天只开到`已提交`」自 ADR-0061 起就不实，改为现行规则（每个有名字的委托状态都经重建门放行，哨兵只给没有快照表达力的新状态）。与重建门状态数无关的「今天」不动。
- **N4 ADR-0188 Status 注记引文。** 注记里决定一末句的引文带省略号，改成决定一末句的逐字整句，`grep -F` 核过确在原文；只动 Status 注记，正文不动。
- **守卫说明。** 接受链守卫是未加锁的先读后判：ADR-0286 决定六末尾补一句，窗口内的竞争由 PS 申报视图对已撤回的状态过滤（估价读不到已撤回委托，冻结形不成）和 code/99（撤回、拒绝、处置与接受链取委托行锁与控制作用域锁次序相反）兜住，不为本票补并发用例；`AcceptanceChainOutcome` 类型头注补一句，已撤回的委托也折进`已决定`且不带决定（守卫在 `Handle` 内）。
- **N5 ADR-0286 章节名与近期先例不同。** 不改：照 ADR-0188 的章节名，按派单。
