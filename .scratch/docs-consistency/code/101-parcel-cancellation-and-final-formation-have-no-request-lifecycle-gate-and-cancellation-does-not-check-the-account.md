# 101 PS 取消与终局形成不看委托生命周期，取消命令也不核目标委托属信封账户：取消口一接上就可达

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code101
Blocked by: [code/36](./36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md)（已撤回委托读得回之后，闸门才有状态可看）
出处：code/36 作者核查；first-party-api 五口摸底。

## 现象

- `application.CancelParcelHandler.Handle`：先 `Requests.FindBySourceIdentity` 按来源身份加委托编号加成员核对（`memberOfCurrentVersion`）找委托，找不到答统一不可见结果；找到之后直接进取消幂等读、边界锁与取消形成，**不看委托的生命周期状态**。已撤回、已拒绝（自 ADR-0188 起可读回）、只是已提交的委托，其包裹都能被取消。
- `application.FormParcelFinalHandler`（以及经它的收寄采用、面单渠道服务终局判断路径）同样没有委托生命周期闸门：从未被接受的委托，也能为它的包裹形成终局。
- `CancelParcelCommand` 只有 `Identity`（来源身份）、`ShipmentRequestID`、包裹、请求方、原因、时刻，不带客户账户；`cancel_parcel.go` 没有客户账户引用。委托属不属信封账户，今天只能靠 Intake 把身份构造成"账户内唯一"。首发 1:1 下跨账户目标天然查无，但应用层没有第二道。

## 证据

- `internal/parcelshipment/application/cancel_parcel.go` 的 `Handle`、`CancelParcelCommand`、`memberOfCurrentVersion`；`form_parcel_final.go` 的 `FormParcelFinalHandler`。
- 文档：UC-PS-006（取消提交）、ADR-0254（取消边界）、ADR-0255 决定一（撤回后以撤回重提方向提交关联新委托，关联新委托只锚在原委托的终态上）、PS CONTEXT 委托生命周期（`已撤回`、`已拒绝`之后原委托不再推进）。
- 摸底报告「取消」一行：命令不带账户，须保证目标委托属信封账户；闸门票 G 立即做，是取消口硬前置。

## 三类

机制。无产品策略；无租户取值。

## 规模

S–M：两只编排各加一道生命周期闸门（读委托状态，按状态分格）；取消加账户核（委托的客户账户等于信封账户，不等答统一不可见结果，不泄露存在）；补各状态与跨账户的真库用例。

## 最小的诚实修法

1. 取消：找到委托之后、取边界锁之前，核委托生命周期——只有`已接受`的委托才能取消其包裹；`已撤回`、`已拒绝`、`已提交`各自答一个确定性业务格（客户可行动：已撤回/已拒绝无事可取消；仅已提交的应改用撤回），不进未决（ADR-0029）。
2. 取消：核目标委托的客户账户等于调用方信封账户（需要端口读委托的客户账户；PS 自己的委托对象带客户账户，核对放应用层，别靠 Intake 一处）；不等与查无同答统一不可见结果（AT-PS-090 的隔离面）。
3. 终局形成：对从未被接受的委托（`已撤回`、`已拒绝`、`已提交`）拒绝形成终局，答确定性格；不改`已接受`之后的既有路径。
4. 用例：各状态一例（取消、终局各一）；跨账户取消答不可见且与查无逐字相同；闸门在边界锁之前；并发撤回与取消（取消读到`已接受`、撤回随后提交——这一格由 ADR-0254 与撤回/接受链的现有串行保证，核一遍，不够就记进越权风险点）。

## 要裁的

- 三个非`已接受`状态各答什么格、是否要新的结果格：倾向复用`请求未受理`以外的新业务格（名字照仓内构词），因为 UC-PS-006 的`请求未受理`只说字段缺。落地时由作者定，写进票面。
- 终局形成的闸门放在 `FormParcelFinalHandler` 里，还是放在调它的三条采用路径各自入口：倾向放在它里面（单点），核它的所有调用方（含 `AdoptOnEffectiveDelivery` 那一路）不会被误拦。

## 完成记录

分支 `claude-code101`（基于 main `be198c8bc`）。

### 做了什么

- **取消**（`application.CancelParcelHandler`）：命令加必填账户字段；委托查找之后核"委托属信封账户"，不属与查无同答统一不可见；随后生命周期闸门（只放行`已接受`）；闸门在授权、取边界锁、取消库幂等读与边界上每一道读之前，被拦时什么都不落、不交下游。
- **终局形成**（`application.FormParcelFinalHandler`）：同一个闸门放在这一处（单点），位置在 code/98 加的边界锁之后、幂等读之前，读的是已取回的委托状态，不依赖 `Cancellations` 是否装配。
- **共用件**：`gateRequestLifecycle`（状态 → 放行/已终结/待决定，状态集合外响亮报错）与 `ownedByCustomerAccount`，都在 `cancel_parcel.go`。
- **消费方落点（ADR-0025）**：`finalconsume.Consumption` 为两个新终局格各给落点：`FinalRequestEnded` 入账收工（终态），`FinalRequestAwaitingDecision` 按未决回滚重投（`ErrFinalUndecided`，评审 N1 后）；取消口 HTTP 译装按 `Outcome().String()` 通用作答，新格答 200 加 `outcome` 名（与 `REQUEST_NOT_ACCEPTED` 等其他非 201 格同形），由真库装配用例钉；`labelfinal.Consumption` 经 `finalconsume` 自动落到。
- **文档**：UC-PS-006（决定矩阵两行、结果语义表三行、流程步骤 2、并发一条、验收 `AT-PS-150/151`、交接一行）、UC-PS-004（结果表两行、流程步骤 3、终局形成规则一条、验收 `AT-PS-152`）、PS CONTEXT（生命周期一条、取消规则一条）、AT 盘点（三行入①、`AT-PS-090` 补点名、文末追记）。

### 交接给首方取消 Intake（tops-20）

- **账户字段**：`application.CancelParcelCommand.CustomerAccount`，类型 `domain.CustomerAccountID`（PS 域类型；VE 的 `CustomerAccountReference` 属 VE 域，不跨上下文用），**必填**。首方 Intake 必须以认证信封里的客户账户填它（与构造 `Identity` 用的是同一份认证结果，但两者分开：`Identity` 是找委托的键，这个字段是"谁在问"）。
- 缺（零值或全空白）一律答 `CancellationNotAccepted`（`REQUEST_NOT_ACCEPTED`，HTTP 200 加 `outcome`），不是 404；不跳过账户核。
- 目标委托不属该账户：`ErrShipmentRequestNotVisible`（HTTP 404 `SHIPMENT_REQUEST_NOT_VISIBLE`），与查无、编号不符、成员出界逐字同答。
- 新结果格（取消口 `outcome` 词表新增两个，HTTP 200，不带凭据字段）：`CancellationRequestEnded` = `REQUEST_ENDED`（已撤回、已拒绝，客户无事可做）；`CancellationRequestAwaitingDecision` = `REQUEST_AWAITING_DECISION`（只是已提交，客户改用撤回）。`UnconfiguredIntake` 未改。
- 终局侧同名两格：`FinalRequestEnded`、`FinalRequestAwaitingDecision`（同样两个 `outcome` 名，非客户面）。

### 代决

1. **三态两格而非三格**：已撤回、已拒绝合并为"委托已终结"（客户可行动同为什么都不用做）；已提交单列"委托待决定"（客户可行动是改用撤回）。按恢复动作分格（ADR-0029），不区分撤回与拒绝，因为区分不改变任何人的下一步。
2. **名字**：`REQUEST_ENDED`、`REQUEST_AWAITING_DECISION`。避开 `REQUEST_NOT_ACCEPTED` 的相近写法（如 `NOT_YET_ACCEPTED`），免得客户端作者读岔；取消与终局两个用例用同一对词。
3. **次序**：字段缺 → 委托查找 → 账户核（与查无并在同一个条件里，错误值同一个）→ 生命周期闸门 → 取边界锁 → 其余。账户核在闸门之前，别人的已撤回委托不露出自己的状态（有用例钉）。
4. **账户核对的是委托自己的账户**（当前提交版本来源身份上的客户账户），不是命令里 `Identity` 的账户：`Identity` 由接入面构造、查询键里已含账户，若比 `Identity` 等于没有第二道。该账户在各版本间恒等，所以取当前版本不会误拦；这条恒等的保证来自 HTTP 补充接入面（`customer_supplement_intake.go`）用同一份认证账户构造补充身份与 `Identity`，应用层（`FormNewSubmissionVersionHandler.Handle` 与领域 `FormNewSubmissionVersion`）没有再比较两者的账户。两个身份若不一致，只会让所有者被拦成"不可见"（过拦），不会越权放行。
5. **被拦不落记录、不交下游**：取消侧不落取消库任何行（含拒绝记录）；终局侧不落采用记录（也不是 `FinalSourceNotAdopted`）。理由：`已提交`委托之后会被接受，一份早先落下的记录会按（包裹、种类、来源版本）占住同一来源，被接受后同一来源再到只答已有结果、包裹永远没有终局（有用例钉"之后被接受即照常"）。
6. **终局闸门读已取回的委托状态，不再读一次**；位置按派单放在边界锁之后，实际与锁前锁后无关（见下论证）。
7. **状态集合外**（`ShipmentRequestStateInvalid` 一类）响亮报错，不放行：来路不明的聚合不得长出取消或终局。
8. **不立 ADR**（预留的 ADR-0289 未用）：闸门是把已接受文本落到代码——UC-PS-006 目标写明"已接受委托中的包裹"、PS CONTEXT 词条"包裹取消决定"为"针对已接受委托中的明确包裹"，UC-PS-006 目标与这条词条是闸门的依据，撤回与拒绝后原委托不再推进也由 CONTEXT 委托生命周期一条给出；ADR-0254 管的是闸门之后边界事实的先后，不受影响；ADR-0188（已拒绝可读回）只管读，未规定取消。新结果格按 ADR-0025/0029 加落点，不改任何已接受 ADR 的口径。
9. **不动装配**：`cmd/parcel-api/assemble_cancellation.go`、`cmd/parcel-dispatch/assemble.go` 都没改——两处已把 `Requests` 装进编排，闸门与账户核只读已取回的聚合，不需要新读口。

### 并发与不加锁的论证（已核）

- 状态转移只在领域聚合里发生：`SubmitShipmentRequest` 建为`已提交`；`WithdrawByCustomer`、`Decide`（接受或拒绝）、`RejectByAuthority`、`DisposeUnderAuthority`（拒绝去向，经 `formActiveRejection`）把它转到`已撤回`、`已接受`、`已拒绝`，**每一条都以 `decisionFormed == false` 且 `state == 已提交` 为前置**（已读 `withdrawal.go`、`acceptance_decision.go`、`active_rejection.go`、`authorized_disposition.go`）；`request.state` 只有这几处赋值，库里只有 `Save` 的乐观版本写 `state`。故撤回只对`已提交`、接受与拒绝由决定一次形成，三个目标状态既不回头也不互转。
- 所以闸门读到`已接受`、`已拒绝`、`已撤回`后答案不会被之后的提交翻回去；读到`已提交`的答案可能在读后变成别的，但那是合法时序，不是不一致：取消读到`已提交`答`REQUEST_AWAITING_DECISION`、随后客户的撤回提交，取消没有成立、什么也没落，无需特殊处理。放行的唯一情形是读到`已接受`，它之后的边界锁与读沿用 ADR-0254 与 code/77 的既有串行。
- 没有新锁、没有新写者，因此没有新的锁序或"事务外拒跑"面；取消与终局的既有锁序不变。

### 调用方核查（终局闸门不误拦）

`FormParcelFinalHandler` 的调用方只有两条：有效交付（`DeliveryOutcomeAdapter`，经 `AdoptOnEffectiveDeliveryAdapter`）与面单渠道终局判断（`JudgeLabelServiceFinalHandler` 的 `Adoption`，经 `labelfinal`）。两条的目标委托都来自 `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel`（只认当前已接受行），所以进来的委托天然是`已接受`，闸门是兜底；收寄采用链走的是 `AdoptNetworkIntakeHandler`，不经本编排。已跑的全部既有真库用例（含取消边界并发、有效交付并发、dispatch 纵向）都绿，没有误拦。

### 测试与变异

- 先红后绿：新应用层用例先对"只有字段与枚举骨架"的代码跑出红（取消三态、先拒后受理、跨账户、缺账户、终局三态、取消机制未装配、先拒后受理），再实现转绿。
- 新用例：`internal/parcelshipment/application/request_lifecycle_gate_test.go`（取消三态与已接受、被拦什么都不碰、授权读不动也先答闸门、先被拦后被接受照常取消；跨账户与查无逐字同答、别人的已撤回委托只答不可见、账户缺席答未受理；终局三态与已接受、取消机制与锁都不装配时同样拦、成员核验先于生命周期、先被拦后被接受照常形成）；真库 `cmd/parcel-api/assemble_cancellation_lifecycle_test.go`（生产装配加 HTTP：四态、跨账户 404 同字节、账户缺席）与 `cmd/parcel-dispatch/final_lifecycle_gate_test.go`（生产 `parcelFinalAdoptionChain`：四态，加消费门落点）。
- 既有两个 `cmd/parcel-api` 真库取消用例原先种`已提交`委托，现改种`已接受`（它们要走到授权与边界读，闸门之后`已提交`走不到）。
- 变异（每个都从备份拷回并 `cmp` 核一致）：去掉取消闸门；账户为零值时跳过核（去掉零值守卫并在比较里放行零值）；账户核比较换成只比租户；去掉终局闸门；终局闸门依赖 `Cancellations != nil`；终局的`已提交`改为落持久不采用记录；取消闸门挪到取边界锁之后；取消闸门挪到授权之后。应用层八处全部被杀；取消闸门、租户比较、零值跳过、终局闸门四处在真库层同样被杀。
- 跑过：`scripts/test-full.sh ./internal/parcelshipment/... ./internal/visibilityexception/... ./tests/bentocontract/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 全绿；新用例 `-count=3` 绿；`go build ./... && go vet ./...` 干净，`gofmt -l` 无输出；`at-coverage-counts -check` 退 1（只是总览过期，三个新号都有分桶行）。

### 越权风险点

- 动了边界清单之外的 `internal/parcelshipment/adapters/finalconsume/finalconsume.go`：新终局格不加落点会落进 `default` 报 `ErrUnexpectedFinalOutcome`，消费门回滚重投、永远收不了工，所以是必要的（ADR-0025）。
- 取消口 HTTP 状态：新格答 200（沿用通用译装，未改 HTTP 适配器代码）。若首方取消 Intake 另有口径（如把`REQUEST_AWAITING_DECISION`译成 409），在 Intake/适配器一侧调，不影响应用层格。
- `FinalRequestAwaitingDecision` 在终局消费门按未决回滚重投（`ErrFinalUndecided`，已由两条调用方适配器登记进各自的未决名单），不入账：委托之后可能被接受，入账会把交付或面单信封不可逆地丢弃。今天的两条调用方都先走"当前已接受"反查，到不了这一格，它是兜底；若将来有调用方不经反查就送来源，早于接受到达的来源会一直重投到委托被接受或被撤回、拒绝（后者答 `FinalRequestEnded`，入账收工）。
- 一处不对称不在本票范围：UC-PS-003 的收寄采用（`adoptionRefusal`）对非已接受委托会落持久不采用记录并按 ADR-0279 作答，对`已提交`委托同样会占住来源。那是已接受 ADR 的既有口径，没有改动，也不另立票。
- AT 号取 `150–152`：first-party-api/10 已把 `AT-PS-140`–`143` 立给 UC-PS-002 的首方修订，`125–129`、`130–137` 也已有在途占用；取号前核过各在途分支与 main，没人占 `150–152`。
- 取消的账户核依赖"各提交版本的来源身份账户恒等"这条。这条保证来自 HTTP 补充接入面 `customer_supplement_intake.go` 用同一份认证账户构造补充身份与 `Identity`，应用层没有另核（`FormNewSubmissionVersionHandler.Handle` 与领域 `FormNewSubmissionVersion` 都不比较两者的账户）；不一致只会让所有者被拦成"不可见"（过拦，不会越权放行）。若将来出现允许改账户的路径，或补充身份不再由这个接入面构造，这里要改成读库里的账户列。

### 评审记录与修订轮

- 独立评审结论：合入前修 B1、N1–N3，已在同一分支上以新提交修完。
- **B1 改号**：first-party-api/10（分支 `claude-fpa10`）已把 `AT-PS-140`–`143` 立给 UC-PS-002 的首方修订，与本票原取的 140–142 撞号；本票三个号改为 `AT-PS-150`（取消的目标委托不是已接受）、`AT-PS-151`（目标委托不属信封账户与账户缺席）、`AT-PS-152`（终局形成的目标委托不是已接受），对应关系不变；盘点三行、UC-PS-006 与 UC-PS-004 验收行、文末追记与本票引用同步。测试未引这三个号。
- **N1 终局消费门**：`FinalRequestAwaitingDecision` 单独成一支，返回 `ErrFinalUndecided`（按未决回滚重投，与"反查不到已接受目标"的 `ErrParcelTargetNotFound` 同属等依赖，ADR-0029 按恢复动作分格）；`FinalRequestEnded` 是终态，仍入账收工。`finalconsume_test.go` 补两格用例，`cmd/parcel-dispatch` 的 `TestTheWiredFinalAdoptionFormsNoFinalForRequestsThatAreNotAccepted` 的已提交一格改期望 `ErrFinalUndecided`；UC-PS-004 终局规则一条补消费门两格。UC-PS-004 结果表里"不进未决"说的是应用层结果格，不变。
- **N2 管理台词表**：`CancellationOutcome` 与 `cancellationOutcomeViews` 补 `REQUEST_ENDED`（委托已终结）、`REQUEST_AWAITING_DECISION`（委托待决定，请改用撤回），并补 `presentation.test.ts`；`REQUEST_NOT_ACCEPTED` 一格的缺失项补上信封客户账户（与 UC-PS-006 结果表一致）。
- **N3 完成记录**：代决第 4 条与末条越权风险点不再写"新版本形成都拒绝跨账户"（`FormNewSubmissionVersionHandler.Handle` 与领域 `FormNewSubmissionVersion` 都不比较账户），改为保证来自接入面构造、应用层未另核、不一致只过拦；代决第 8 条依据改指 UC-PS-006 目标与 PS CONTEXT「包裹取消决定」词条，不再把转述当 ADR-0255 原文引。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code101` 由作者 rebase 到 origin/main `aa65d3ce7` 后快进，tip `31a20265f`。无迁移、无 ADR（依据：闸门是把 UC-PS-006 目标、PS CONTEXT「包裹取消决定」词条与 ADR-0255 决定一已写的「已接受」落到代码，不改任何已接受 ADR 的口径）。
- 协调方分诊：R1，首方取消口（用户定的上线范围）的硬前置。只有`已接受`的委托才能取消其包裹、才能为其包裹形成终局；已撤回/已拒绝答 `REQUEST_ENDED`、已提交答 `REQUEST_AWAITING_DECISION`，确定性业务格；`CancelParcelCommand.CustomerAccount`（`domain.CustomerAccountID`）必填，零值答 `REQUEST_NOT_ACCEPTED`，不属该账户与查无同答 404 不可见；闸门不依赖取消库是否装配。
- 隔离评审（Sonnet）一条阻断、三条建议：B1 AT 号 140–142 与在途分支 first-party-api/10 撞号，协调方裁本票改号为 AT-PS-150–152（PS 的 AT 号 144–149 归首方五口、150 起归 PS 应用层票）；N1 终局消费门对「委托只是已提交」从入账收工改成未决重投（今天走不到，但入账收工是不可逆地丢弃信封，与反查不到已接受目标时的既有归类 `ErrParcelTargetNotFound` 不一致）；N2 管理台取消结果词表补两个新词；N3 完成记录一处不实的代码事实（账户恒等的保证来自首方补充接入面构造，应用层未另核，不一致只会过拦）。均由 Sonnet 子代理收掉；评审另核实账户核确有第二道（取委托当前提交版本来源身份上的账户，不取命令里 `Identity` 的账户）、闸门位置在授权与边界锁之前、委托状态单向转移不需要新锁。
- 协调方单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；admin-web tsc、730 个用例、vite build 通过。无迁移，种子未灌。
- 去向：无新票。已把接口（账户字段、两个新 outcome）转给负责首方五口接线的会话。`finalconsume_test.go` 的 `resultOf` 用 `reflect`+`unsafe` 给未导出字段赋值（该结果类型没有导出构造器），仅测试用；若 owner 嫌脆可给结果类型补一个导出的测试构造器。
