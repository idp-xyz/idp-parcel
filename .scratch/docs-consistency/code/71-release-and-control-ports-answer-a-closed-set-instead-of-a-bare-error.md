# 71 释放端口与控制端口改答封闭集，分已收口、可重试未形成、确定未受理三格

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code71（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：今天 PS 的释放端口只交回 `error`，可重试与确定性失败分不开。确定性失败（提供方拒收请求、作用域映射变了）被当成补偿续办或内部重试，永远收不了口：撤回、拒绝那几路留一条没人推得动的补偿引用；接受链对被取代版本的释放（code/20）则会挡住新版本，一直重试到信封死信。2026-10-21 协调方据 code/20 隔离评审立。
Blocked by: 无
出处：code/20 隔离评审（越权风险点 1、N6）；code/20 完成记录「评审记录」。

## 现象

- `ports.PreAcceptanceControlRelease.ReleasePreAcceptanceControl` 只答 `error`。`pssettlement.PreAcceptanceControlAdapter` 把 `ReleaseRequestNotAccepted` 译成 `ErrUntranslatableAnswer`、把作用域未配置译成普通错误，到了 PS 编排都只是一个 `err != nil`。
- 各释放点一律折成 `ControlReleasePending`。释放点：`FormAcceptanceDecisionHandler.releaseIfRejected`、`RejectShipmentRequestHandler`、`WithdrawShipmentRequestHandler`、`DisposeShipmentRequestHandler` 的释放、接受链的 `releaseSuperseded`。
  - 撤回、拒绝那几路：确定失败留一条补偿引用，之后无人能推。
  - 接受链：确定失败让新版本一直停在内部重试。
- 控制端口一侧（`ApplyPreAcceptanceFinancialControl`）的业务答复已是封闭集，但适配器的确定性错误（`ErrUntranslatableAnswer`）同样经 `error` 交回，`AdvanceFinancialControlJudgmentHandler` 一律折成 `FINANCIAL_CONTROL_UNAVAILABLE` 内部重试。两个端口同一个缺口。

## 证据

- `internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go` 的 `ReleasePreAcceptanceControl`：`ReleaseRequestNotAccepted` 那一格的注释已写明「不是一种可重试的未决」，交回的却仍是一个 `error`，调用方分不出来。
- [ADR-0081](../../../docs/adr/0081-acceptance-judgment-is-envelope-driven.md) 决定三：封闭集合外的格保持 `publish_failed` 响亮，折进未决会重投到失败预算耗尽。释放这一侧今天正是把确定格折进了未决。

## 风险（并入本票，评审 N6）

释放时现查作用域：`FormControlScope` 在释放那一刻重新解析商业依据、再查账户目录，而不是取施加时用过的那一份。施加之后若结算政策回显或账户登记变了，释放查到的是另一个作用域，SA 在那个作用域下找不到这一笔，答`无可释放`，PS 当成已收口。原占用静默成孤儿。修法候选：施加时把作用域随控制结果存下、释放按存下的那一份认领，或释放查到的作用域与施加时不同即答确定未受理。本票落地时一并定。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：端口答复改形（三格），适配器逐格翻译，各释放点逐格分派；确定格响亮上抛，或落到人能处理的格，不进补偿续办。

## 最小的诚实修法

1. `ControlReleaseRequest` 的答复改成封闭集：`已收口`（已释放、无可释放）、`可重试未形成`（账本读不回或存不回、依赖答不出）、`确定未受理`（请求被拒收、作用域对不上）。
2. 适配器逐格翻译，不留兜底。
3. 控制端口同此：确定性的翻译失败不再折成 `FINANCIAL_CONTROL_UNAVAILABLE`。
4. 各释放点逐格分派：已收口不留引用；可重试照旧走 `ControlReleasePending`；确定未受理响亮上抛（照 ADR-0081 决定三）。
5. 接受链 `releaseSuperseded` 同改：确定未受理不再折成内部重试。
6. N6 那条作用域现查的风险按上一节定法。

## 要裁的

- 确定未受理在撤回、拒绝几路上是响亮上抛，还是另立一个等人处理的格：上抛会让已成立的撤回或拒绝答 5xx，落地时由 PS owner 选。接受链那一路照 ADR-0081 决定三上抛。

## 完成记录（2026-10-26，分支 claude-code71，rebase 于 origin/main `9733a08af`，下文 SHA 均为 rebase 后的）

### 做了什么

- `f9e4ca7d6` 代码、迁移与用例：
  - 端口 `ports.PreAcceptanceControlRelease` 交回 `ControlReleaseAnswer`：`SETTLED`（已收口：已释放、无可释放）、`NOT_FORMED`（可重试未形成）、`NOT_ACCEPTED`（确定未受理），非已收口带提供方原因。错误只留给形不成三格之一（提供方答复译不动包着新的端口哨兵 `ports.ErrUntranslatableProviderAnswer`；释放口没装齐）。
  - PS→SA 适配器：`ErrUntranslatableAnswer` 包着端口哨兵；释放逐格翻译（`releaseAnswerFor`，default 报错），账本读不回落`可重试未形成`、带 `SA-` 原因，`ReleaseRequestNotAccepted` 落`确定未受理`；施加时把作用域交回（领域新值 `domain.OccupationScope`，挂在 `FinancialControlResult` 上），释放按请求里存下的那一份认领、不再问 `FormControlScope`；没存下即交零值，由 SA 短路答未受理。
  - 判断账 `RecordFinancialControlResult` / 读回带三列作用域；迁移 `migrations/parcel_shipment/0030_control_occupation_scope_and_release_refusal.sql`：`acceptance_financial_control` 加三列（同进同出、`明确无控制`不带），新表 `control_release_refusal`（释放未受理册，主键 租户/委托/版本/控制结果）。
  - 新端口 `ports.ControlReleaseRefusalRegister` 与真库适配器 `pspostgres.ControlReleaseRefusals`（`Record` 要环境事务、`ListByShipmentRequest`）。
  - `occupationRelease` 逐格分派（`releaseFormed` → `occupationReleaseOutcome`：第一处可续办缺口 + 每一笔拒收），`settle` 给终局与暂停各路记册、交回补偿引用与「有一笔落册」；记不下册改留补偿，新原因 `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`。撤回、主动拒绝、授权处置、`FormAcceptanceDecisionHandler`（自动拒绝 `releaseIfRejected`、停进等待受控补充 `pauseForExternalResume`）结果加 `ControlReleaseNotAccepted()`，依赖加 `Refusals`。
  - 接受链 `releaseSuperseded` 遇拒收上抛 `application.ErrControlReleaseNotAccepted`（形不成三格之一时上抛原错误），不进 `acceptanceChainUndecidedSentinels`；`AdvanceFinancialControlJudgmentHandler` 遇包着端口哨兵的错误上抛，依赖调不通照旧 `FINANCIAL_CONTROL_UNAVAILABLE`。
  - `pspostgres.SavepointedControlRelease` 对非已收口与错误都退回 savepoint，开退收失败按`可重试未形成`交回。
  - HTTP：撤回、主动拒绝、授权处置三口答复加 `controlReleaseNotAccepted`。装配：parcel-api 三处（`assemble_withdrawal.go`、`assemble_review.go`、`assemble_disposition.go`）与 parcel-dispatch 形成决定处接释放未受理册；路由哨兵名单的「不在名单里」注释补两格。未动 endpoints.go、main.go、endpoints_test.go、routes.go。
- `b51dd4492` 管理台：三口响应类型加 `controlReleaseNotAccepted`，撤回页另起一句、处置命令摘要并列一格；摘要用例一条。
- `02fd8b7b5` 文档：新立 [ADR-0282](../../../docs/adr/0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md)；ADR-0132、ADR-0081 的 Status 索引注记与 Links 挂来向，ADR-0079 索引注记加读法注；PS CONTEXT 词条「控制项结果」补作用域随结果保存与释放三格、判断任务停止一条补拒收交人；UC-PS-001 接受链停在哪一步一段补拒收响亮失败、新立 `AT-PS-116`，UC-PS-005 结果语义与释放失败规则补拒收一格、新立 `AT-PS-115`，两份交接各补一行；AT 盘点两行入①、`AT-PS-035`/`AT-PS-068` 补点名。

### 代决（用户授权协调方「作为业务和系统专家自决」；协调方已裁的三条与本票的停下）

1. **停下—协调方裁 (A)—ADR-0282。** 动手前核出：协调方裁的「处置几路遇确定未受理另立待人处理格」收窄 ADR-0132 决定一「释放失败按既有补偿续办（`ControlReleasePending`，内部重试），不因去向不同而不同。」与决定四「续办方是系统（内部重试）」；「接受链响亮上抛」严格读是扩成 ADR-0081 决定三列的三格。照派单停下回报，协调方裁走 (A)、给号 ADR-0282，本票据此写 ADR 并挂停用指针。ADR 索引未重生成（本地跑过生成器、生成后 `-check` 退 0，已还原 README，不提交生成结果），由推送方生成。
2. **协调方已裁·撤回、拒绝、处置遇确定未受理的落法。** 业务事实照常落；释放这一侧记进释放未受理册（表 `control_release_refusal`，一笔占用一行，记客户账户、存下的作用域、原因、登记时刻）；结果与 HTTP 答复带 `controlReleaseNotAccepted`；不留补偿引用。同一判法扩到自动拒绝与停进等待受控补充两路：两路同在接受链那一轮里，上抛会让已成立的拒绝或暂停随整轮回滚——与终局三路同一处境。端口形不成三格之一时这几路也落册（原因 `CONTROL_RELEASE_ANSWER_UNTRANSLATABLE`）：UC-PS-005「也不得把释放失败改写为撤回失败」压过「集合外上抛」的通行纪律，见越权风险点 3。
3. **协调方已裁·接受链 `releaseSuperseded` 遇确定未受理响亮上抛。** 上抛 `ErrControlReleaseNotAccepted`，不进未决哨兵名单，落 `publish_failed`（`assemble_test.go` 失败分格表加两行钉住）；不记册（整轮回滚）。拒收与可续办缺口同时出现时也上抛。
4. **协调方已裁·N6：施加时把作用域随控制结果存下、释放按存下的那一份认领。** 存量（施加时没存作用域）不回退到现查，交零值给 SA，由 SA 答未受理，落终局各路的册或接受链的响亮失败。理由：现查正是 N6 要堵的那一格；首发前只有演示租户，存量随 `--reset` 重灌即消；留回退分支，释放路径就保留两种认领法。存下的作用域放在 PS 领域里用 `OccupationScope` 三段引用表达（本上下文只原样存、原样交回，不解释），不让 SA 的类型进 PS 端口（ADR-0025）。
5. **控制端口同此的落法。** 控制端口的业务答复已是封闭集，缺的是「译不动」与「依赖调不通」分不开。用端口级哨兵 `ErrUntranslatableProviderAnswer` 分：包着它的上抛，其余照旧 `FINANCIAL_CONTROL_UNAVAILABLE`；没有把依赖调不通改进`未形成`格，避免这张票改掉控制端口既有的依赖错误约定与 `FINANCIAL_CONTROL_UNAVAILABLE` 的来源。释放端口则把依赖答不出放进`可重试未形成`，错误只留给形不成三格之一——两口对「错误」的约定因此不对称，各在端口注释里写明。
6. **记不下册另立原因。** 拒收了又没记进册（册没接上或写不进）时留补偿续办，原因 `CONTROL_RELEASE_REFUSAL_NOT_RECORDED`，不与 `CONTROL_RELEASE_PENDING` 共用：续办引用由原因派生，两种缺口共用一条引用，按引用查回来的是另一种。册没接上照「写不进」处置（同 code/35 冲突册 `CONFLICT_REGISTER_UNAVAILABLE` 的先例）。
7. **释放未受理册只记不处理。** 册上只有按委托的读口，不立运营查阅面与处理动作（核对、补登作用域、重放）；那属另一件事，见越权风险点 1。
8. **ADR-0079 决定五不停用。** 决定本身（回指与作用域同出一次解析、释放命令不带回指）不变，末句描述的释放现查作用域的现状不再成立；按协调方意见只在其索引注记挂读法注。
9. **新立两个 AT 号。** 拒收那一格的预期结果与 `AT-PS-073`「只续办原释放请求」相反，不塞进旧行：UC-PS-005 立 `AT-PS-115`、UC-PS-001 立 `AT-PS-116`，盘点表各加一行入①。

### 越权风险点（待 PS owner 复核，SA owner 会签）

1. **册上的积压无人看见。** 只有按委托的读口，没有队列、告警、运营查阅面，也没有处理之后的收口动作；一笔被拒收的占用会一直占着客户的余额或额度，直到有人查到它。
2. **存量无作用域行一律交人。** 终局各路落册；接受链开头那一步响亮失败、那份委托的新版本判不了，信封耗尽失败预算后死信。人处理完之后新版本怎么重驱，本票没有立续办触发。首发前只有演示租户，`--reset` 即消。
3. **终局各路把「端口形不成三格之一」也落册而不上抛。** 编程错误因此不会让撤回答 5xx，只会在册上多一行 `CONTROL_RELEASE_ANSWER_UNTRANSLATABLE`，要有人去看册才发现。释放口没装齐（装配缺口）同落此格。
4. **暂停时被拒收的那一笔，补件后会挡住新版本。** 停进等待受控补充时释放被拒收，那一笔记册、暂停照旧；客户补件之后接受链开头再放它，再被拒收，链响亮失败、新版本不判——与决定三一致，但客户这一侧看到的是补件之后迟迟没有结论。
5. **记不下册时留的补偿引用仍无人推。** 终局各路的补偿引用本就不持久（code/72、73 已记），记不下册退回这一格。
6. **认领依赖结算侧不跨作用域迁移占用。** 存下的作用域是施加那一刻的；结算侧日后若合并账户、把冻结迁到另一个作用域，按存下那一份认领答`无可释放`，照样成孤儿（方向与 N6 相反）。今天结算侧没有这类迁移。
7. **控制端口译不动改为响亮。** 提供方一次形状变化会让走到控制那一步的委托成片落 `publish_failed`。code/72 那一格（再驱已暂停、占用已释放的那一版）由此从内部重试到预算耗尽改为响亮失败，`TestARedrivenPausedVersionStopsAtTheReleasedFreeze` 随之改期望。
8. **parcel-api 三处释放口今天就放得掉了。** 不再需要作用域源（code/20 越权风险点 5、code/73 越权风险点 2 那一格消失）；此前必然留补偿的释放现在真的到达 SA、真的释放——行为变化，非回退。
9. **释放未受理册的写入不隔 savepoint。** 册写入出 SQL 错误会让那一笔事务中止、整笔回滚（撤回答 5xx）；只有「没接上」与无事务两格退回补偿。主键冲突走 `ON CONFLICT DO NOTHING`，不在此列。

### 迁移

`migrations/parcel_shipment/0030_control_occupation_scope_and_release_refusal.sql`（开工前与 rebase 后两次核：main、origin/main 与各在途分支 PS 最大号都是 0029）。演示种子在一次性库 `c71seed` 上灌入退 0（「种子灌入完成」），新表与三列在库，用后 `DROP DATABASE c71seed WITH (FORCE)`。

### AT

- 新立并入①：`AT-PS-115`（UC-PS-005）、`AT-PS-116`（UC-PS-001）。`AT-PS-035`、`AT-PS-068` 行补点名。总览计数未改（`at-coverage-counts -check` 只报过期、不报缺行），由推送方重算。
- 用例：编排 `internal/parcelshipment/application/control_release_refusal_test.go`（六路逐格、译不动、记不下册、作用域随请求、两版各落一格、重放、接受链两格、控制端口一格）；适配器 `pre_acceptance_control_test.go`（释放三格、按存下作用域认领两格、无作用域即未受理、施加交回作用域、端口哨兵）；真库 `pspostgres` 册往返与无事务拒、作用域三列往返与 CHECK、savepoint 三格；派发真库 `cmd/parcel-dispatch/control_release_refusal_test.go` 三条与失败分格表两行；装配真库 `cmd/parcel-api/control_release_refusal_test.go` 两条（经生产操作者 Intake 与 HTTP 答复）。

### 验证（rebase 后，代码同 `02fd8b7b5`）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l cmd internal migrations tests tools` 无输出；改动文件无 CR、无 BOM。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh` 范围：改动包的反向依赖（`go list -test` 算，42 个，含 parcel-commercial、parcel-frontline-import、parcel-settlement-register、tests/bentocontract 与各上下文的 PS 适配器）加 `./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`、`./internal/settlementaccounting/...`、`./internal/visibilityexception/...`（第二次 rebase 带进 VE 改动，一并跑）：退 0，66 个包 ok、4 个无测试文件、0 FAIL（含 PBC-08 负面证据门禁）。
- 新用例与改动用例 `-count=3`（第一次 rebase 之后跑；第二次 rebase 只带进 VE 与文档改动）（PS `application`、`adapters/settlementaccounting`、`adapters/postgres`、`cmd/parcel-api`、`cmd/parcel-dispatch`）：ok。
- 变异（第一次 rebase 之后逐个施加，各跑一遍编排、适配器、parcel-api、parcel-dispatch，含真库）：
  - M1 确定未受理折回 `ControlReleasePending`：编排五条、parcel-api 一条、派发两条变红；
  - M2 接受链拒收折成内部重试：编排两条、派发一条变红；
  - M3 释放仍现查作用域：适配器两条、parcel-api 两条、派发两条变红；
  - M4 控制端口译不动折回 `FINANCIAL_CONTROL_UNAVAILABLE`：编排一条、派发一条变红。
  恢复后绿。新用例先在旧折法（即上面四种变异）上跑过，红在同一批用例上。
- `go test ./tools/...` ok；`go run ./tools/adr-index`、`./tools/at-coverage-counts` 只报生成物过期（推送方生成），机制清点本地重生成会动 PS 一行与迁移数，未提交。
- admin-web（worktree 内离线装依赖，lockfile 未动；最后一次 rebase 带进 `apps/admin-web/src/pages/party/**` 与 .scratch 改动、不碰 Go，Go 测试未重跑，`go build ./...` 退 0）：`tsc -b --noEmit` 退 0，`run-tests.mjs` 667 pass / 0 fail，`vite build` 退 0。

### 未决

- ADR 索引、AT 盘点总览计数、`docs/product/MECHANISM-INVENTORY.md` 未重生成，由推送方生成。
- 释放未受理册的查阅面与处理动作、接受链响亮失败之后的续办触发：未立票，见越权风险点 1、2，留待协调方判是否立票。
