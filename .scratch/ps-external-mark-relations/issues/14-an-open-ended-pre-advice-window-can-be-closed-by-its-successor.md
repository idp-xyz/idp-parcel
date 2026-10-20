# 14 预报有效时窗册：不设终点的一行登下之后，这个租户再也换不了时窗

Category: bug
Status: resolved——2026-10-19 进 main（代码验证钉 `dca742e0`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-psxm14`，NO 迁移号 0020；与票 09 并行。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 08 评审遗留）
Blocked by: 08 进 main
父票：[08](./08-pre-advice-and-handover-note-model-and-window-register.md)
出处：票 08 完成记录「没做」末条与评审。
地盘：
- internal/nodeoperations/domain/ 时窗那一份及其测试
- internal/nodeoperations/adapters/postgres/ 时窗册及其测试
- migrations/node_operations/（取当时下一个空号）
- 时窗登记编排、在线写面与 `cmd/parcel-node-operations-register` 的 pre-advice-window 一支

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（读码）

时窗册严格只追加，同租户生效区间重叠由排他约束拒。生效区间终点可空；一旦登了终点为空的一行（演示种子就是这样登的），之后任何更晚起点的行都与它重叠、登不进——租户要换提前量或宽限，只能动库。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制）

照只追加的纪律加一种事实「时窗收口」：对当前终点为空的那一行追加一条收口，写明收口时刻与依据；收口之后，起点不早于收口时刻的新行照常登记。不取「登新行即隐式截断前一行」：隐式改写前一行的有效区间等于改历史，且一次笔误就会悄悄截断在用的时窗。收口是一次显式的登记写，走同一能力面（登记册配置写），在线口与 CLI 同一份编排。收口时刻早于前一行起点、对已有终点的行收口、重复收口都答坏输入或已有结果，照本册写口的现行答法。读「某时刻适用哪一行」时，把收口时刻当作那一行的终点。

## 完成判据

- [x] 领域与真库：开放终点的一行收口后，起点不早于收口时刻的新行登得进；收口前登不进（照旧）；收口时刻、已闭合行、重复收口各一格。
- [x] 推导：收口之后的业务时间不再落进前一行的时窗。
- [x] 在线口与 CLI 同一份编排，各一条用例。
- [x] Go 门禁照常；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm14`）

起点 main `4642eb2b`，完工后 rebase 到 `origin/main` `fec2a2a1`。证据层级 **S**：夹具全为 `SYN-` 合成值，跑在隔离测试库上。

**提交**

| 提交 | 内容 |
|---|---|
| `74ba98a5` | 收口的领域、端口、postgres 写口、迁移 `node_operations/0020`、收口编排、在线收口口、CLI 新族、parcel-api 装配，各层用例 |
| `9f25fe65` | 在 `74ba98a5` 上重生成机制清点 |
| 本笔 | 本完成记录 |

**落点**

- 领域 `internal/nodeoperations/domain/pre_advice_window.go`：新增 `PreAdviceWindowClosure`、`FormPreAdviceWindowClosure`，时窗行上新增 `Close`、`Closure`。用例在新文件 `pre_advice_window_test.go`。
- 端口 `ports/delivery_scope_registers.go`：`PreAdviceWindowRegister` 加 `ClosePreAdviceWindow`。
- 存储 `adapters/postgres/pre_advice_window.go`，用例在新文件 `pre_advice_window_closure_test.go`。
- 迁移 `migrations/node_operations/0020_pre_advice_window_closure.sql`。
- 编排 `application/close_pre_advice_window.go`。
- 在线口 `adapters/http/close_pre_advice_window.go`，Intake 方法加在 `operator_registry_intake.go`。
- CLI `cmd/parcel-node-operations-register` 新族；装配 `cmd/parcel-api/assemble_pre_advice_window.go`。

**收口事实与存储**

- **事实**：收口以那一行的起点指名那一行（同租户起点唯一），写明收口时刻与依据；登记时刻取时钟，不由登记方说。不取「此刻开着的那一行」：一份迟到或指错的收口会落到别的行上。
- **形成门**：收口时刻必须晚于那一行的起点。早于起点照裁定答坏输入；恰在起点会让半开区间变空，一并拒。比较在折成册上精度（微秒）之后做。
- **读法**：`EffectiveAt` 与 `Overlaps` 把收口时刻当终点。`EffectiveUntil` 仍是登记时给的终点。`SameContent` 不比收口，所以收口之后原样重登仍答已有结果，演示种子重灌不断。
- **存储**：迁移 0020 立表 `pre_advice_window_closure`。
  - 主键（租户，那一行的起点）：一行至多收口一次。
  - 外键指回 `pre_advice_window`。
  - CHECK：依据非空；收口时刻晚于起点，库面再守一道。
  - 已登的时窗行不改写。「只收登记时没有终点的行」跨表，由写口的插入谓词守。

**排他约束怎样与收口并存**

- 迁移 0020 撤掉 `pre_advice_window_no_overlap`。
  - 它比的是每一行登记时给的区间。开放终点那一行的区间永远是〔起点，无穷），行又不改写，它会拒掉此后每一个后继，正是本票的现象。
  - 排他约束读不到收口表。只加不删的路逐一试过，都绕不开这一点。
  - 删约束经用户在主会话答「允许」获准：auto mode 分类器先以修改共享资源拒过这一句，作者没有绕开，停下来交回会话请示。
- 不重叠改由时窗册的两个写口（登记、收口）交付。
  - 先取同一把以租户为键的事务级咨询锁 `pg_advisory_xact_lock`，再在下一条语句里按有效终点 `COALESCE(收口时刻, 登记时的终点)` 查重叠、插入。
  - 事务是读已提交，取锁之后的语句看得见先到那一笔已提交的行与收口。
  - 迁移 0013 头注说排他约束是「唯一不漏首插并发的那条路」，指的是行锁；咨询锁不要已有行，首插并发同样串行。
  - 票 08 的并发用例 `TestConcurrentOverlappingWindowsLandOnlyOnce` 在锁下照旧只放进一笔。
- 按时点读生效行（`FindPreAdviceWindowAt`）走同一个有效区间表达式。
  - 票 11 进 main 的身份判断经 `DeliveryScopeEvidence` 委托到它，所以收口之后的业务时间读到的是后继或没有生效行，不必另改。
- **改号**：认领时取 NO 迁移号 0014。推送方在用户获准之后把文件改名为 0020：main 上 NO 迁移已到 0018、另一会话占了 0019，0014 会落在已施加的迁移之前，不干净。

**在线口与 CLI 的形状**

- **在线口**：另立端点 `POST /node-operations-pre-advice-window-closures`，不和登记口按载荷区分。
  - 两种写各自封闭解码，并集载荷会让打错的键被当成另一种写的合法字段；两套 outcome 会让调用方先猜是哪一种写。
  - 挂与登记口同一只 `OperatorRegistryIntake`、同一个登记册配置写能力面，写开关换不了它。
  - 载荷封闭，三格必给：`windowEffectiveFrom`、`closedAt`、`basis`。
  - 答法：收口落下答 201 `PRE_ADVICE_WINDOW_CLOSED`；原样再收答 200 `EXISTING_PRE_ADVICE_WINDOW_CLOSURE`，回显那一行与它的收口。
  - 照裁定归坏输入的各格都答 400 `MALFORMED_REQUEST`：收口时刻不晚于起点、对登记时带终点的行收口、换内容再收、本租户没有那一行。换内容再收不归已有结果，免得操作员以为自己的时刻生效了。
  - 册读写不动答 500。
  - 登记口的回显在那一行收过口时多带一格 `closure`。
- **CLI**：`parcel-node-operations-register -kind pre-advice-window-closure -file <路径>`，输入是在线载荷原样嵌入、另加 `tenantId`。
  - 退出码：0 已收口或已有结果；1 坏输入（含上面各格，错误文本说是哪一格）；3 未决。
- 两口调同一个编排 `ClosePreAdviceWindowHandler`。

**共享文件里加的确切行**（都是只追加；rebase 时与 main 上别的会话加的行冲突，两边都保留，本票的行接在后面）

- `cmd/parcel-api/endpoints.go`：
  - 签名末尾加 `preAdviceWindowClosure nodeopshttp.PreAdviceWindowCloser,`。
  - 端点表末尾加一行注释，和 `{Pattern: "/node-operations-pre-advice-window-closures", Handler: nodeopshttp.NewClosePreAdviceWindowEndpoint(operatorRegistries.nodeOperations, preAdviceWindowClosure)},`。
- `cmd/parcel-api/main.go`：
  - 在 `buildCancellationFaces` 那一段之后加一段（一行注释、`preAdviceWindowClosure, err := buildPreAdviceWindowClosure(db)` 与错误返回）。
  - 调用末尾加 `preAdviceWindowClosure,`。
- `cmd/parcel-api/endpoints_test.go`：
  - 探测表末尾加 `"/node-operations-pre-advice-window-closures": {method: http.MethodPost, target: "/node-operations-pre-advice-window-closures"},`。
  - 未接线装配末尾加 `unwiredPreAdviceWindowClosure{},`。
- `cmd/parcel-api/assemble_operator_decisions_test.go`：`swappedRegistryFaces` 里登记口那一行之后加 `"/node-operations-pre-advice-window-closures",`。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 领域与真库：收口后后继登得进、收口前登不进 | `domain/pre_advice_window_test.go::TestAClosureEndsAnOpenWindowAtItsClosingInstant`；`adapters/postgres/pre_advice_window_closure_test.go::TestAClosedOpenWindowAdmitsItsSuccessor` |
| 收口时刻 | `TestAClosureNeedsAnInstantAfterItsWindowStartsAndABasis`；库面 CHECK 在 `TestClosingAWindowAnswersByWhatTheRowAlreadyHas` |
| 已闭合行、重复收口 | `TestOnlyAnOpenWindowCanBeClosed`；`TestClosingAWindowAnswersByWhatTheRowAlreadyHas`；编排 `application/close_pre_advice_window_test.go::TestClosingAWindowAnswersClosedOrExisting`、`TestAClosureThatCannotEndItsRowIsRejected` |
| 并发与无事务 | `TestConcurrentClosuresAndSuccessorsLandOnlyOnce`、`TestClosingAWindowRefusesOutsideATransaction` |
| 推导：收口之后的业务时间不再落进前一行的时窗 | `domain/pre_advice_window_test.go::TestTheDerivationStopsUsingAWindowFromItsClosure`；读册半边 `TestAClosedOpenWindowAdmitsItsSuccessor`（收口时刻起读到后继） |
| 在线口 | `adapters/http/close_pre_advice_window_test.go::TestTheWindowClosureFaceAnswersEachOutcome`、`TestTheWindowClosureFaceRefusesWhatItCannotTranslate`；真库装配 `cmd/parcel-api/assemble_pre_advice_window_test.go::TestTheWiredPreAdviceWindowCloserClosesAgainstARealDatabase`；路由各格 `TestEveryAssembledEndpointAnswersUnconfigured`、`TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades`、`TestSwappedRegistryFacesAnswerFromTheOperatorChannel`（各多探一行） |
| CLI | `cmd/parcel-node-operations-register/main_test.go::TestTheClosureKindEndsAnOpenWindowSoItsSuccessorRegisters` |
| 在线口与 CLI 同一份编排 | `TestTheRegisterAndTheOnlineFaceShareOneClosure` |

**先红后绿**

- 红日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm14-red.log`，分两段：
  - 第一段：用例先写、生产代码一行未写时跑。六个包全部 `[build failed]`，现象原话是「undefined: domain.PreAdviceWindowClosureSpec」一类。
  - 第二段是行为红：领域、端口与写口都写好、迁移只建了收口表时跑 postgres 包。`TestAClosedOpenWindowAdmitsItsSuccessor` 与 `TestConcurrentClosuresAndSuccessorsLandOnlyOnce` 报「conflicting key value violates exclusion constraint "pre_advice_window_no_overlap" (SQLSTATE 23P01)」，这就是要撤约束的证据。
- 绿日志：rebase 前 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm14-green-prerebase.log`，rebase 后 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm14-green.log`。

**门**（rebase 后在 `74ba98a5` 的 Go 内容上跑，之后的提交不碰 Go 文件）

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN `-p 1 -count=1` 跑 `./internal/nodeoperations/... ./cmd/parcel-api/... ./cmd/parcel-node-operations-register/... ./migrations/... ./internal/architecture/...`，全 ok。
- 全量 `./...` 没跑，留给推送方。
- 两道棘轮基线不用动：新的领域工厂与类型都有生产调用方。

**机制清点**：`9f25fe65`。nodeoperations 生产文件 105→107、测试 110→114、应用编排 14→15、http 适配器 15→16；node_operations 迁移 17→18；接入面端点 169→170。

**越出地盘的改动**

- `internal/nodeoperations/ports/delivery_scope_registers.go`：端口加收口写口，答法词表的注释顺带写进收口。
- `adapters/http/operator_registry_intake.go`：加 `IntakePreAdviceWindowClosure`。
- `adapters/http/register_pre_advice_window.go`：回显加 `closure`。
- `cmd/parcel-api` 的 `assemble_pre_advice_window.go` 与其用例，以及上面四份共享文件。
- 票 08 的 `adapters/postgres/delivery_scope_registers_test.go`：两段用例注释里「库面排他约束」的说法改掉，断言没动。
- 迁移 0013 没动，它的校验和已落册。

**未做**

- 完成判据末格「走 Go 那条路」：分支与作者自验已做；非作者评审与推送方全量不归作者，该格未勾。
- 收口还没写进 NO CONTEXT、ADR-0212 决定二与参数登记册 `PAR-NET-18` 那一行。那几处写的「同租户任一时刻至多一行生效，重叠在登记时拒」仍成立；收口是这本册的写法机制，裁定只在本票。要不要补一句，留给评审定。
- 演示种子没有加收口：种子登的那一行仍开放，要换值时经本票的口收口即可。
- 收口时刻早于此刻（追溯收口）不另设限：会改变按原业务时间重核的结果。裁定没限，时窗行本身的起点也照收过去的时刻，同一口径。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `44c5ccab`）

两轴合审，0 阻断。

- **Spec**：裁定落到。收口是独立事实（主键为租户与那一行的起点，一行至多一次，外键指回时窗行，收口时刻晚于起点），已登的行不改写；`EffectiveAt`、`Overlaps` 与推导读收口后的有效终点；只收登记时终点为空的行，已闭合行、换内容再收、本租户无此行各答坏输入，原样再收答已有结果。在线口另立端点与登记口同一能力面、同一 Intake（两种写各自封闭解码，理由在头注），CLI 新族与在线口同一份编排。
- **删排他约束（用户 2026-10-19 答「允许」）**：排他约束只能比同一行的列、读不到收口，开放终点那一行会拒掉一切后继；迁移 `node_operations/0020` 撤去它，不重叠改由两个写口先取同一把按租户的事务级咨询锁、再按有效终点查重叠，首插并发同样串行，票 08 原有并发用例与新并发用例在锁下都过。迁移原占 0014、编号落在已施加的 0015–0018 之前，推送方改为 0020。
- **评审补（推送方，已做）**：ADR-0212 文末追加复核记录记收口与守法的改变，NO CONTEXT 与参数登记册不另写（同一处权威）。
- **未做（不另立票）**：演示种子没登收口（种子那一行照旧开放终点，需要时经新口收口）。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-psxm14` rebase 到远端 main `f91f29a4`（另一会话的 infra-speed/01 等已进）快进推送；机制清点重生成零差。
- 第一次推送方全量（钉 `28e6707f`，经共用库 55432、`-p 1`）有 9 行 FAIL，全是 parcelpricing 两个包的 `connection reset by peer`（握手阶段），与本票无关：同一时段另一会话的全量也压在 55432 上。单包重跑与整套重跑（`int41-fulltest-2`）都 0 FAIL。排查结论见本会话与用户的对话：库未崩、未拒连，重置在经 Docker 端口代理的连接层；另一会话随后落的 `scripts/test-full.sh` 给每一跑起专用一次性实例、走 Unix socket，根除这一类。
- 推送方全量（以本记录为准）：钉 `dca742e0` 的隔离 detached 检出，`bash scripts/test-full.sh`（专用一次性 PostgreSQL、Unix socket、`-p 8`）退 0——有测试的包 153 个 ok、0 FAIL。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
