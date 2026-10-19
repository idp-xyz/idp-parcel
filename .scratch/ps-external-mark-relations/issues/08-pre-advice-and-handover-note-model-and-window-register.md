# 08 送站预报与送站交接单的领域模型和存储，可验证交付范围的推导，预报有效时窗册

Category: enhancement
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-psxm08`，NO 迁移号 0013；与票 10 并行。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by: 无（ADR-0212 已接受）
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定一、二、三、五、九；NO CONTEXT「可验证交付范围」「送站预报」「送站交接单」与两段同名生命周期；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md)。
地盘：
- `internal/nodeoperations/domain/` 新文件：送站预报、送站交接单、交付范围依据引用、交付范围核对态、可验证交付范围的推导，及各自测试
- `internal/nodeoperations/ports/`：预报册、交接单册、预报有效时窗册的读写端口（新文件，不动 `ports.go` 里身份读口那一段，那归票 11）
- `internal/nodeoperations/adapters/postgres/` 新文件及测试
- `migrations/node_operations/`：取当时最大号的下一号（并行票也在取号，动手前再看一眼）
- 预报有效时窗册的登记写面与受控批量口：NO 今天没有登记 CLI，新起一个（形状照 `cmd/parcel-network-register`）；写面进 `cmd/parcel-api` 端点表一行，挂登记册配置写的操作者 Intake，照最近一份新登记册的先例（例如 ADR-0201 的周期安排册）
- `scripts/demo-seeds`：演示租户 `SYN-TENANT-01` 的有效时窗一行
- `docs/product/MECHANISM-INVENTORY.md`（重生成）

## 做什么

1. **送站预报**（ADR-0212 决定二）。
   - 字段：签发的预报标识、租户、客户账户、节点、预计送达时刻、客户包裹件号集合、可选的交付方线索、来源请求键与内容摘要、形成时刻。
   - 号码集合非空、不重复，逐字保存、不折算；号码的形状门与 PS 的 `NewExternalIdentifierNumber` 同一界：票 06 已进 main（`1734f2a0`），拒 NUL、UTF-8 字节数以 512 为界，照它，界值不另立一份。
   - 撤回只增一行，是终态。
   - 本票只立模型与册，不开提交口（票 09）。
2. **送站交接单**（ADR-0212 决定三）。
   - 字段：设备签发的标识、租户、节点、交付方线索、声称的客户账户集合（至少一个）、开立时刻。
   - 成员只增，每行记形成方式：收寄带它、或归入。
   - 关闭只增一行，冻结成员。
   - 绑定结论只增：（租户，交接单，客户账户）唯一，记途径（客户渠道或运营决定）与确认方；运营决定时另记交付方参与方与依据引用。
   - 「范围已成立」是派生：声称的客户账户全部已确认才成立，任一否认就不再成立。
   - 本票不开命令口（票 12）。
3. **交付范围依据引用与核对态**：依据引用是（种类，标识）两格，种类只有送站预报、送站交接单两个。核对态取值三种，未证的原因是封闭词表，照 ADR-0212 决定五：
   - 已证；
   - 未证，附原因：没有依据、依据不在册、依据不适用（节点不符、在时窗外、已撤回）、号码不在预报、交接单绑定未全部确认；
   - 未配置。
4. **可验证交付范围的推导**（纯函数，NO 领域）。
   - 输入：这件实物的依据（来源引用，或最后归入的交接单）、依据此刻的证据状态、时窗册在原收寄业务时间的生效行、收寄节点与业务时间、外部标识观察。
   - 输出：客户账户集合与核对态。
   - 预报按 ADR-0212 决定二那组条件逐条判，范围落到｛预报的客户账户｝。
   - 交接单按决定三判：全有或全无，范围是声称的整组客户账户。
   - 时窗册没有生效行时答未配置，不推定默认时长。
5. **预报有效时窗册**（ADR-0212 决定二末项，参数登记册 `PAR-NET-18`）。
   - 行：（租户，提前量，宽限量，生效区间，登记依据），只增。
   - 同租户生效区间重叠在登记时拒。
   - 登记写面与受控批量口同一用例；空册在推导里答未配置。
   - 演示租户在种子里登一行，证据记 `S`。

## 不做

- 预报的提交与撤回口（票 09）；交接单的开立、归入、关闭与确认口（票 12）；授权交付关系（票 10）。
- 收寄与重核接上推导、身份读口改形状（票 11）。
- 时窗按节点细分（ADR-0212 决定二：日后加列）。

## 完成判据

- [x] 领域用例：预报算数的各条件逐格（节点不符、时窗外、已撤回、号码不在列、号码逐字，带空白或大小写不同即不在列）；交接单只确认部分客户账户时不成立、任一否认不再成立、全部确认成立；一件实物最后归入的交接单优先于来源引用；时窗未登记答未配置，与未证分开。
- [x] 存储用例（真库）：预报、撤回、交接单、成员、关闭、绑定结论都只增；（租户，交接单，客户账户）的结论唯一，异结论撞键；时窗区间重叠在登记时拒。
- [x] 时窗册的写面与受控批量口：同一用例，重放答已有结果；未授予与未配置各答其格（ADR-0085）。
- [x] 演示种子登一行时窗，`seed.sh` 退 0。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、新 CLI、`cmd/parcel-api`、`migrations/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

M

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm08`）

起点 main `a2ff42f6`。证据层级 **S**：夹具与种子全为 `SYN-` 合成值，隔离合成库上跑。

**提交**

| 提交 | 内容 |
|---|---|
| `98e18f14` | 领域模型、端口、postgres 适配器、号码形状门、迁移 0013、两份棘轮基线，各层用例 |
| `9fb86716` | 时窗登记编排、在线写面与操作者登记册 Intake、受控批量口 `cmd/parcel-node-operations-register`、parcel-api 装配、演示种子一行 |
| `c3fdaf16` | 机制清点在 `9fb86716` 上重生成 |
| 本笔 | 本完成记录与 AT 盘点追记十四 |

**落点**

- 领域 `internal/nodeoperations/domain/`：`delivery_pre_advice.go`（送站预报、撤回、证据状态、`CustomerAccountReference`）、`delivery_handover_note.go`（交接单、成员、关闭、绑定结论、证据状态）、`delivery_scope.go`（依据引用、核对态、推导）、`pre_advice_window.go`（时窗行）。
- 端口 `internal/nodeoperations/ports/delivery_scope_registers.go`：`DeliveryPreAdviceRegister`、`DeliveryHandoverNoteRegister`、`PreAdviceWindowRegister`、`PreAdvisedNumberGate`，写口共用答案词表 `DeliveryScopeAppendOutcome`。`ports.go` 没动。
- 存储 `internal/nodeoperations/adapters/postgres/`：`delivery_pre_advice.go`、`delivery_handover_note.go`、`pre_advice_window.go`。
- 号码形状门 `internal/nodeoperations/adapters/parcelshipment/pre_advised_number_gate.go`。
- 编排 `internal/nodeoperations/application/register_pre_advice_window.go`；写面 `adapters/http/register_pre_advice_window.go` 与 `adapters/http/operator_registry_intake.go`；认证 `adapters/accessidentity/operator_registry_authenticator.go`。
- 迁移 `migrations/node_operations/0013_delivery_scope_registers.sql`。
- CLI `cmd/parcel-node-operations-register`；装配 `cmd/parcel-api/assemble_pre_advice_window.go`。
- 种子 `scripts/demo-seeds/data/node-operations/01-pre-advice-window-syn-tenant-01.json`。

**模型与存储形状**（票面留给实施定的格，逐条写下取舍）

- **送站预报**：字段照票面。来源拆成 `CustomerRequestSource`（来源客户端、来源请求键、内容摘要）：请求键只在一个客户端之内唯一，客户端单列，重放身份是（租户，来源客户端，来源请求键），与首方族「发行方 + 主体」拼成来源格同一理由。号码照交来的顺序存、逐字比，`Lists` 不折算空白与大小写。撤回是另一份事实（`DeliveryPreAdviceWithdrawal`，带撤回方客户账户与自己的来源），一份预报至多一份。证据状态 `DeliveryPreAdviceState` 的零值即不在册。
- **号码形状门**：PS 的 `NewExternalIdentifierNumber` 在 PS 领域，NO 的领域与端口不得跨上下文导入（`internal/architecture` 的 `TestBusinessModulesDoNotReachIntoEachOther`，ADR-0025「跨上下文翻译只落在 `internal/<consumer>/adapters/<provider>/`」）。所以立端口 `ports.PreAdvisedNumberGate`，实现落在 `adapters/parcelshipment`，问的就是入册门本身，界值只在 PS 一处。用例在门两侧二分出界、再拿 PS 入册门比同一处分界，不抄常量。`domain.NewPreAdvisedParcelNumber` 只拒全空白，给门的实现与仓储还原用，还原不再过门（理由同 PS 的 `RehydrateExternalIdentifierNumber`）。门今天没有生产调用方：提交口在票 09，届时编排先过门再形成预报。
- **送站交接单**：交付方线索必给（开立就在交付方面前，与收寄的交付方同类）。声称按标识定序、逐字不重复。成员记形成方式（`RECEPTION` / `ASSIGNMENT`）、事实身份与业务时间，重放身份是（租户，形成方式，事实身份）。关闭至多一次。绑定结论记结论、途径、作结论方（`Concluder`），运营决定另记交付方参与方与依据引用；运营决定只能确认，这两条领域与库面 CHECK 各守一道。「范围已成立」是 `DeliveryHandoverNoteState.ScopeEstablished()`，派生、不落列，不在声称之列的结论不算数。
- **依据引用与核对态**：依据引用（种类，标识），零值即没有依据。核对态三格：取值（已证 / 未证 / 未配置）、未证原因（封闭五格照 ADR-0212 决定五）、依据不适用的细分（节点不符 / 时窗外 / 已撤回）。细分单列，恢复动作虽同（补证据），复核的人看得出补哪一样；`NewDeliveryScopeCheck` 验三格搭配，供票 11 还原落册的核对态。
- **推导** `DeriveVerifiableDeliveryScope`（纯函数）：依据取 `UnitDeliveryScopeBasis.Effective()`，最后归入的交接单优先于来源引用。预报那一支的判序是：在册且同租户、节点、未撤回、号码逐字在列，最后才看时窗。时窗是唯一要读租户取值的一格，排在最后，未配置就只在它是唯一挡路的一格时答（`AT-NO-068`「其余条件都满足」）。号码本来就不在列时答未证，不把人指去登一个不解决问题的时窗。交接单那一支全有或全无，范围是声称的整组，不看号码、不看时窗。**另加一格**：交接单在别的节点开立，答依据不适用·节点不符。ADR 只在决定五的原因词表里列了节点不符，没限定只对预报；成员与归入都只发生在开立的节点上，收寄报了别处的交接单是扫错了依据。证据与依据标识对不上、或属别的租户，与不在册同答。
- **时窗册**：行是（租户，提前量，宽限量，生效区间，登记依据），另记登记时刻（取时钟，不进内容比对）。时长以整分钟为粒度，不得为负，为零照收（租户取值，产品不判合不合理）。生效区间半开、终点可空。算数区间〔预计送达 − 提前量，预计送达 + 宽限量〕两端都含。**只增，没有给开放前版落终点那条改写**：那会让「重叠在登记时拒」不再成立。代价是登下开放终点的一行之后，这个租户再也登不进更晚起点的行，要换值的租户先登带终点的行。领域 `PreAdviceValidityWindow` 头注写了这一条。
- **时刻**：形成处一律折成 UTC、截到微秒（库的精度），读回与重放比对逐格相等。
- **写口代数**：各写口撞键都不覆盖，答 `ALREADY_RECORDED` 并回显册上那一行。重放还是冲突由调用方比，册不替命令口定（预报口是票 09，交接单各口是票 12）。指名的预报或交接单不在本租户、撤回方不是预报的客户账户、结论的账户不在声称之列，几种同答 `TARGET_NOT_FOUND`，不泄露别的账户的东西在不在。向已关闭的交接单加成员答 `NOTE_CLOSED`；关闭之前已落的同一事实再投，照旧答已在册。关闭与加成员在交接单行上串行：加成员取 `FOR SHARE`，关闭取 `FOR UPDATE`。
- **最后归入**：只看 `ASSIGNMENT`，按业务时间取最后一次，同一时刻按落册先后（`member_sequence`）。离线补传的归入可能后落册、却发生得更早。
- **时窗重叠**：库面排他约束 `pre_advice_window_no_overlap` 拒。写口用无 conflict target 的 `ON CONFLICT DO NOTHING`，撞上读回起点最早的那一行；编排比内容，全同答已有结果，否则答重叠。

**迁移**：`node_operations/0013`。表：`delivery_pre_advice`（号码 `text[]`、非空 CHECK、来源唯一键）、`delivery_pre_advice_withdrawal`（主键即预报、来源唯一键、外键到预报）、`delivery_handover_note`（声称 `text[]` 非空）、`delivery_handover_member`（主键（租户，形成方式，事实身份），按交接单的索引，「最后归入」部分索引）、`delivery_handover_closure`（主键即交接单）、`delivery_handover_binding`（主键（租户，交接单，客户账户），结论与途径封闭、运营决定只确认、证据两格随途径在场）、`pre_advice_window`（主键（租户，起点），区间有序，排他约束）。另有 `CREATE EXTENSION IF NOT EXISTS btree_gist`：本上下文的迁移先于关务施加，干净库上走到这里时它还没装。号码不拆子表：判断只问「这个号在不在这份预报里」，没有按号码跨预报查的读口。

**登记 CLI 与写面**

- CLI `cmd/parcel-node-operations-register -kind pre-advice-window -file <路径>`，形状照 `cmd/parcel-network-register`。输入是在线口载荷 `nodeopshttp.PreAdviceWindowRegistrationPayload` 原样嵌入、另加 `tenantId`，译成命令只走载荷的 `Command`。退出码：0 已登记或已有结果，重灌种子不断；1 输入不合法，含形成门拒；2 区间重叠；3 未决。
- 在线写面 `POST /node-operations-pre-advice-window-registrations`。挂新立的 `nodeopshttp.OperatorRegistryIntake`，认证 `noaccess.OperatorRegistryAuthenticator` 以登记册配置写能力面铸信封、不判准入范围，答复格的翻译与查阅读共用。载荷 `leadMinutes`、`graceMinutes`、`effectiveFrom`、可选 `effectiveUntil`、`basis`，封闭解码，自报租户即坏载荷。两个时长缺席答坏载荷，不当零。首登 201，已有结果与重叠 200，都带 `outcome`（`PRE_ADVICE_WINDOW_REGISTERED` / `EXISTING_PRE_ADVICE_WINDOW` / `PRE_ADVICE_WINDOW_OVERLAPS`）并回显那一行。编排拒形 400，册读写不动 500。
- 共享文件里加的确切行（只追加，装配函数另起 `assemble_pre_advice_window.go`）：
  - `cmd/parcel-api/endpoints.go`：签名末尾 `preAdviceWindowRegistration nodeopshttp.PreAdviceWindowRegistrar,`；端点表末尾两行注释加 `{Pattern: "/node-operations-pre-advice-window-registrations", Handler: nodeopshttp.NewRegisterPreAdviceWindowEndpoint(operatorRegistries.nodeOperations, preAdviceWindowRegistration)},`。
  - `cmd/parcel-api/main.go`：`buildChargeOccurrenceRegistration` 之后一段 `preAdviceWindowRegistration, err := buildPreAdviceWindowRegistration(db)`（带一行注释与错误返回）；调用末尾 `preAdviceWindowRegistration,`。
  - `cmd/parcel-api/endpoints_test.go`：探测表末尾 `"/node-operations-pre-advice-window-registrations": {method: http.MethodPost, target: "/node-operations-pre-advice-window-registrations"},`；未接线装配末尾 `unwiredPreAdviceWindowRegistration{},`。
  - `cmd/parcel-api/assemble_operator_decisions_test.go`：`swappedRegistryFaces` 末尾 `"/node-operations-pre-advice-window-registrations",`。
  - `cmd/parcel-api/assemble_operator_decisions.go`（不在「只追加」名单里，改动是追加形）：`operatorRegistryIntakes` 多一格 `nodeOperations *nodeopshttp.OperatorRegistryIntake`（带一行注释）；`buildOperatorRegistryIntakes` 在 `buildOperatorOperationsReads` 之前建认证方与 Intake，之后 `reads.nodeOperations = nodeOperations`。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 领域：预报各条件逐格、号码逐字 | `internal/nodeoperations/domain/delivery_scope_test.go::TestAPreAdviceCountsOnlyWhenEveryConditionHolds`（节点不符、早于提前量一秒、晚于宽限量一秒、已撤回、号码不在列、多一个前导空格、大小写不同、不在册、别的租户、依据标识对不上、没带依据）；`delivery_pre_advice_test.go::TestAPreAdviceListsANonEmptySetOfVerbatimNumbers`、`TestAWithdrawalIsItsOwnFactAboutAPreAdvice`、`TestAValidityWindowAdmitsTheClosedSpanAroundTheExpectedArrival`、`TestValidityWindowsOverlapOnTheirHalfOpenEffectiveIntervals` |
| 领域：交接单部分确认不成立、任一否认不再成立、全部确认成立 | `delivery_handover_note_test.go::TestAHandoverNoteEstablishesScopeOnlyWhenEveryClaimIsConfirmed`；`delivery_scope_test.go::TestAHandoverNoteScopeIsAllOrNothing`；形状 `TestAHandoverNoteClaimsAtLeastOneDistinctCustomerAccount`、`TestABindingConclusionCarriesItsRoute`、`TestMembersRecordHowTheyJoinedAndClosureIsAFact` |
| 领域：最后归入的交接单优先于来源引用 | `delivery_scope_test.go::TestTheLastAssignedHandoverNoteTakesPrecedenceOverTheSourceBasis` |
| 领域：时窗未登记答未配置，与未证分开 | `delivery_scope_test.go::TestAnUnregisteredWindowAnswersUnconfiguredApartFromUnverified`；词表 `TestTheDeliveryScopeCheckIsAClosedVocabulary` |
| 号码形状门 | `internal/nodeoperations/adapters/parcelshipment/pre_advised_number_gate_test.go::TestThePreAdvisedNumberGateIsTheExternalIdentifierRegisterGate`、`TestThePreAdvisedNumberGateSharesTheByteBoundWithParcelShipment` |
| 存储（真库）：各份事实只增 | `internal/nodeoperations/adapters/postgres/delivery_scope_registers_test.go::TestPreAdvicesAndWithdrawalsOnlyAppend`、`TestHandoverNotesMembersClosuresAndBindingsOnlyAppend`、`TestTheLastAssignmentOfAUnitNamesItsHandoverNote`；无事务一律拒 `TestDeliveryScopeRegistersRefuseWritesOutsideATransaction` |
| 存储：结论唯一、异结论撞键 | `TestHandoverNotesMembersClosuresAndBindingsOnlyAppend`（先确认后否认答 `ALREADY_RECORDED` 回显已确认） |
| 存储：时窗区间重叠在登记时拒 | `TestValidityWindowsRefuseOverlapAtRegistration`、`TestConcurrentOverlappingWindowsLandOnlyOnce` |
| 写面与受控批量口同一用例、重放答已有结果 | 编排 `internal/nodeoperations/application/register_pre_advice_window_test.go::TestRegisteringAValidityWindowAnswersRegisteredExistingOrOverlapping`、`TestARegistrationThatFormsNoWindowNeverReachesTheRegister`；同一用例跨入口 `cmd/parcel-node-operations-register/main_test.go::TestTheRegisterAndTheOnlineFaceShareOneRegistration`；真库生产装配 `cmd/parcel-api/assemble_pre_advice_window_test.go::TestTheWiredPreAdviceWindowRegistrarRegistersAgainstARealDatabase`；退出码 `TestTheRegisterAnswersEachRecoveryActionWithItsOwnExitCode` |
| 未授予与未配置各答其格（ADR-0085） | 写面 `internal/nodeoperations/adapters/http/register_pre_advice_window_test.go::TestTheWindowRegistrationFaceAnswersEveryChannelGrade`、`TestTheWindowRegistrationFaceRefusesWhatItCannotTranslate`、`TestTheWindowRegistrationFaceAnswersEachOutcome`；认证 `adapters/accessidentity/operator_registry_authenticator_test.go::TestRegistryWriterAuthenticatesWithoutAnAdmissionCheck`；装配后的路由 `cmd/parcel-api/endpoints_test.go::TestEveryAssembledEndpointAnswersUnconfigured` 与 `assemble_operator_decisions_test.go::TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`（各多探一行） |
| 演示种子 | `cmd/parcel-node-operations-register/main_test.go::TestTheDemoWindowSeedRegistersOnARealDatabase`；`seed.sh` 实灌见下 |

**先红后绿**：红 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm08-red.log`。用例先写、生产代码一行未写时跑，八个包全部 `[build failed]`，现象原话是「undefined: nodeopsapp.PreAdviceWindowRegistrationResult」一类。日志里 `undefined` 共 63 行，各包报到「too many errors」即止，不是全部缺口。绿 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm08-green.log`。红之后改过一处用例写法，不改断言：号码门用例里一处把字符串误交给按长度判的助手，编译不过，改成直接过门。

**门**（在 `9fb86716` 的 Go 内容上跑，之后的提交不碰 Go 文件）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `-p 1 -count=1` 跑 `./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-node-operations-register/... ./migrations/... ./internal/architecture/...`，全 ok。全量 `./...` 没跑，留给推送方。两道棘轮各加一段理由：函数名棘轮加推导与核对态还原那几条，类型可达性加推导的输入输出与核对态、依据引用各类型，都等票 11 接上。两段都在父提交 `a2ff42f6` 上数过底数，见各段。

**演示种子实灌**：在门禁库上建临时库 `psxm08_seed_1792422279`，带 DSN 跑 `scripts/demo-seeds/seed.sh`（不带 `--reset`，迁移计划从零施加），退 0，末段「pre-advice-window: PRE_ADVICE_WINDOW_REGISTERED（2026-01-01T00:00:00Z 起，提前 1440 分、宽限 720 分）」。查表得 `SYN-TENANT-01` 一行。同一份再登答 `EXISTING_PRE_ADVICE_WINDOW`，退 0。用完删库，复查 `pg_database` 无此名。日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm08-seed-run.log`。没碰 `psb26_walk`。

**机制清点**：`c3fdaf16`。node_operations 迁移 12→13，接入面端点 149→150，端口声明 508→512，基线口径缺口不变。

**越出地盘的改动**

- `internal/nodeoperations/adapters/parcelshipment/pre_advised_number_gate.go`（及用例）：号码形状门的防腐实现，理由见「模型与存储形状」。
- `internal/nodeoperations/application/register_pre_advice_window.go`、`adapters/http/operator_registry_intake.go`、`adapters/http/register_pre_advice_window.go`、`adapters/accessidentity/operator_registry_authenticator.go`（及各自用例）：票面地盘只写了「写面进端点表」，NO 此前没有登记册写面的 Intake 与认证方，写面要经过这几层。
- `cmd/parcel-api/assemble_operator_decisions.go` 与 `assemble_operator_decisions_test.go`：见上「共享文件里加的确切行」。
- `internal/architecture/production_wiring_baseline.txt`、`production_type_reachability_baseline.txt`：各加一段，理由在段内。
- `scripts/demo-seeds/README.md`：数据表加一行；`seed.sh` 编译列表加一行、登记一段。
- `.scratch/at-coverage-inventory.md`：追记十四。

**AT 盘点**：照扫描口径补进 UC-NO-002 的 `AT-NO-063` 至 `AT-NO-068` 与 UC-NO-006 的 `AT-NO-069` 至 `AT-NO-075`。本票测试点名的入①，都是半边：`AT-NO-063`、`AT-NO-068`、`AT-NO-070` 是推导半边，`AT-NO-074` 是领域半边。其余入②，见追记十四。

**未做**

- 完成判据末格「走 Go 那条路」：分支与作者自验已做；非作者评审与推送方在干净检出上带 DSN 跑全量，不归作者，该格未勾。
- 号码形状门、推导、核对态还原都还没有生产调用方：提交口与签发预报标识归票 09，收寄与重核接推导、来源文档带依据引用、判断版本记核对态归票 11，交接单各命令口与范围成立门铃归票 12。
- 预报标识的签发器没立：签发发生在提交那一刻，随票 09。
- 时窗册没有读面（查阅页与 `?registry=` 分派），票面不要求，管理台读归票 13。
- 开放终点的时窗行登下之后，这个租户不能再登更晚的行。要支持「后继给开放前版落终点」须另立决定，见「模型与存储形状」。
