# 02 装载分配登记口能在同一分配下另起并列首版：版本链成了森林，抢占变化入口要用的版本号，撤回之后还能起活跃新根

Category: bug
Status: resolved——2026-10-18 进 main（代码验证钉 `0a5afa74`），见「进 main 记录」。此前 in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 代决并认领（分支 `claude-tfa02`）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-load-assignment-registrations
地盘：
- internal/transportfulfillment/application/form_load_assignment.go 及其测试
- internal/transportfulfillment/ports/load_assignment.go
- internal/transportfulfillment/adapters/postgres/load_assignment_registry.go 及其测试
- internal/transportfulfillment/adapters/http/form_load_assignment.go 及其测试
- migrations/transport_fulfillment/（新迁移，序号取落地时的下一个）
- cmd/parcel-api/（新增真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取「应当」里的推荐取法：本口只形成一次分配的**首版**。该分配已有任何版本时，对指名的新版本号答新格「分配已形成」（`LOAD_ASSIGNMENT_ALREADY_FORMED`，HTTP 200，回显链头），不落库、不另起根。变化与撤回走领域 `ReviseMembers` / `Withdraw` 的入口（tf-unwired-seven/05 记着那两个入口没接，不在本票）。不取备选（把换版本号当变化）：那要给本口加前版格、把变化入口并进来，超出这一口的职责。修复方向四条照做；迁移取 `migrations/transport_fulfillment/` 下一个空号，建索引前先查存量并列首版，有就如实报出、不静默删。属机制，不涉租户取值。

越权风险点：代 TF owner 定了「本口只管首版」；owner 复核时看今天有没有调用方靠「换版本号」表达变化（演示种子、管理台、探针）。

## 现象
- **同一分配可起两个首版。** 先投 v1（班次 S1，分配时刻 08:00），再投 v2（换成班次 S2，分配时刻早一天多，成员也不同），两次都答 201 `LOAD_ASSIGNMENT_FORMED`。在册两版的 `corrects_version` 都是 NULL，班次与分配时刻各不相同。
- **抢占领域「变化」要用的版本号。** 领域「变化」要形成的版本（`corrects=v1`）如果恰好也用 v2，存进生产登记册会答「版本已在册」：这个版本号已被本口占掉。
- **撤回之后还能起活跃新根。** 用生产登记册存入领域 `Withdraw` 形成的撤回版（模拟撤回入口接上之后的情形），本口再投 v3，答 201：形成一个 `withdrawn=false`、`corrects_version` 为 NULL、分配时刻早于 v1 的活跃首版。
- 同版本异内容答 200 `LOAD_ASSIGNMENT_VERSION_EXISTS` 并回显原版。这一半照票 20 判据已够用，不在本票范围。

## 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。端点用生产构造 `tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, buildSegmentOperations(db).assign)`，准入走生产准入桥。

探针（各自同目录的 overlay.json）：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-authority/adv_authority_probe_test.go 的 `TestAdvLoadAssignmentRegistryTakesALinkedSuccessorAndTheFaceForksTheChain`（日志 run-adv.log）。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-authority2/adv2_authority_probe_test.go 的 `TestAdv2FaceFormsASecondRootForAnExistingAssignment`、`TestAdv2FaceFormsAnActiveRootOnAWithdrawnAssignment`（日志 run-adv2.log）。
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-load-assignment-registrations/adv-impl/adv_impl_probe_test.go 的 `TestAdvImplTheFaceFormsAParallelRootAfterAWithdrawnChain`。

## 应当
本口的写法与本上下文对「一次分配」的成文口径相左。成文口径是：
- `LoadAssignmentKey` 头注：「变化与撤回都形成新版本、原版本原样保留……所以「一次分配」在库里是一条版本链而不是一行会变的记录」；
- 迁移 0009：「assigned_at 在整条链上不变：变化与撤回沿用原分配时刻」；
- `LoadAssignment.Withdraw`：「已撤回的分配不再变化——后续要装载只能是新的分配」。

而 `FormLoadAssignmentHandler.Form` 的注释是：「换对象范围要换版本号，那是变化不是重投」。

推荐取法（待 TF owner 裁）：
- 本口只形成一次分配的**首版**。该分配已有任何版本时，对指名的新版本号答一个新格（如「分配已形成」，200，回显链头），不落库，也不另起根。
- 变化与撤回走领域 `ReviseMembers` / `Withdraw` 的入口。tf-unwired-seven/05 记着：「变化（`ReviseMembers`）与撤回（`Withdraw`）的**入口**没有接」。

备选：本口把「换版本号」当作变化。那就必须回指链头、沿用分配时刻，等于把变化入口并进本口，要改命令形状（加前版格），一并由 TF owner 裁。

## 根因
- 幂等键（租户、分配、版本）只防同一版本重复，不防同一分配另起根；`LoadAssignmentRegistry` 也没有按分配读链头的口。
- 处理器注释与端口、迁移、领域对「换版本号」的说法不一致，而且没有任何票裁过。

## 修复方向（机制，按推荐取法）
1. `ports.LoadAssignmentRegistry` 加按（租户、分配）读链头的方法，并补 PG 实现。
2. `FormLoadAssignmentHandler.Form`：按键查不到时，再查该分配有无版本；有则答新格并带链头，不 `Save`。
3. 库面兜底并发：
   - 新迁移给 `transport_fulfillment.load_assignment` 加部分唯一索引「（tenant_id, assignment_ref）WHERE corrects_version IS NULL」，写法照 0014、0017、0020。
   - 建索引前先查存量里有没有并列首版；有就如实报出，不静默删。
   - `Save` 的 `ON CONFLICT DO NOTHING` 撞上这条索引时，编排按分配读回链头再答新格；不能按本次键读回，那样会读空。
4. HTTP：`writeLoadAssignmentOutcome` 给新格映射 200 并回显链头，时刻格式随票 31。

## 是否潜伏
- 同一分配另起并列首版：已可触发。生产装配不问授权映射，演示部署里配置员已有授予、对照与区间。
- 撤回之后起活跃新根：潜伏。撤回入口未接，今天只有直接写登记册才造得出撤回版。

## 完成判据
- [ ] 编排单测：已有 v1 时投 v2，答新格并回显 v1，不调 `Save`；同版本重放仍答 `LOAD_ASSIGNMENT_VERSION_EXISTS`，`TestASecondFormOfTheSameVersionKeepsTheOriginal` 照过。
- [ ] 真库回归：
  - v1 之后投 v2，答新格，行数不变；
  - 撤回链（经生产登记册存入撤回版）之后再投新版本，答新格；
  - 两份不同版本号的首版并发到达：恰一份 201，另一份答新格并回显赢家，库面只有一个首版。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件，生产端点构造 + 生产准入桥）：上一条三格各答其格。
- [ ] 迁移：干净库上 `migrate` 顺序跑通；存量检查语句与结果写进完成记录。
- [ ] `go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...`，全部 ok。
- [ ] 新增生产文件时，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`。
- [ ] 在票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验、非作者评审（两轴分开）、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa02）

提交：`4e812d38`（端口与 PG 读链头）、`a33ce961`（编排与 HTTP 新格）、`98838cfd`（迁移与并发读回）、`0a748baf`（真渠道回归）、`4d4aee8e`（机制清点）。

- **落点**
  - 端口 `LoadAssignmentRegistry.FindChainHead`（租户、分配）：取此刻未被任何版本回指的那一版；PG 实现两步读（先取链头版本号，再走 `FindByKey` 装回）。本表库面不守「一版至多被回指一次」，并列链尾取最后登记者，注释写明。
  - `FormLoadAssignmentHandler.Form`：按键查 → 按分配读链头 → 登记。新格 `LoadAssignmentAlreadyFormed`（`LOAD_ASSIGNMENT_ALREADY_FORMED`）交回链头、不 `Save`；读链头失败答未决。`Save` 答已登记而按本次键读不回时改按分配读链头答新格。「换对象范围要换版本号，那是变化不是重投」那句失实注释已改。
  - HTTP：新格走既有非 201 分支答 200 并回显链头，`writeLoadAssignmentOutcome` 的注释补列这一格。
  - 迁移 `migrations/transport_fulfillment/0021_load_assignment_one_root_per_assignment.sql`：先查存量并列首版，有就 `RAISE` 逐个列出并拒绝迁移，再建部分唯一索引 `load_assignment_one_root_per_assignment`。
- **各格对应用例**
  - 编排（替身，替身也模拟了首版索引）：`TestAnotherVersionOfAFormedAssignmentAnswersAlreadyFormed`（v1 后投 v2，答新格回显 v1，不 `Save`）、`TestAWithdrawnAssignmentAnswersAlreadyFormedWithTheWithdrawal`、`TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner`、`TestAChainHeadReadFailureIsUndecided`；`TestASecondFormOfTheSameVersionKeepsTheOriginal` 照过。
  - 真库适配器：`TestTheChainHeadIsTheVersionNoOtherCorrects`（无版本读空、首版、变化后、撤回后、跨租户不串）、`TestASecondFirstVersionOfOneAssignmentIsAlreadyRegistered`（第二个首版答已登记，版本行与成员行都不落）。
  - HTTP 适配器：`TestFormLoadAssignmentAnswersAlreadyFormedWithTheChainHead`。
  - 真渠道（`cmd/parcel-api/load_assignment_single_chain_test.go`，生产端点构造加生产准入桥）：
    - `TestTheLoadAssignmentFaceAnswersAlreadyFormedForASecondVersion`：答新格，行数不变；本口没占的 v2 留给领域变化，照常登记。
    - `TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain`：撤回版经生产登记册存入，再投 v3 答新格回显撤回版；同版本重放仍答 `LOAD_ASSIGNMENT_VERSION_EXISTS`。
    - `TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner`：赢家在事务里挂住，观察到本口那笔插入挂在锁上再放行，确定落在索引那条路。
    - `TestConcurrentFirstVersionsThroughTheFaceFormExactlyOne`：四轮两首版并发经本口到达，每轮恰一份 201、另一份答新格回显赢家，库面一个首版。
- **先红后绿**
  - 真渠道四例先在 `59403e6f` 上跑，全红：第二个首版、撤回后再投、并发落败一方、并发对的两份，全都答 201 `LOAD_ASSIGNMENT_FORMED`。现象一与现象三复现。
  - 适配器：`FindChainHead` 打桩回空时红（「只有首版时链头应是 v1：found=false」）；0021 落地前第二个首版 `Save` 答已存（outcome=1）。
  - 编排：改处理器前三例红（v2 与撤回后两例答 `LOAD_ASSIGNMENT_UNDECIDED`，读链头失败一例答 `LOAD_ASSIGNMENT_FORMED`）；并发落败例在加按分配读回前答 `LOAD_ASSIGNMENT_UNDECIDED`。
  - HTTP 适配器例改前答 201。
  - 改后全绿；真渠道四例 `-count=5` 稳定。
- **复现不了的格**：无。现象二（版本号被占）没有单独跑出红：修复前首例在更前一步就红了。修复后由首例尾段覆盖。
- **迁移号**：`transport_fulfillment/0021`。落地时 origin/main 上该目录最高是 0020，本地各分支也没有 0021。
- **存量检查**：语句是迁移里的那条 `SELECT tenant_id, assignment_ref, count(*) … WHERE corrects_version IS NULL GROUP BY tenant_id, assignment_ref HAVING count(*) > 1`。
  - 本机 gate 库（55432）：c8_walk6、psb26_mcp4d、psb26_walk 均 0 行，无并列首版；postgres 库与演示库 idp-postgres/parcel_syn 没有该表。
  - 临时库上造一对并列首版跑 0021：`RAISE` 列出 `SYN-TENANT-01/SYN-LA-FORK（2 个首版）`，索引未建；删掉并列根后重跑，索引建成。
  - 干净库：pgtest 模板库按序施加全部迁移，上列测试全绿。
- **门**：改动 `.go` 跑 `gofmt -l` 无输出；`go build ./...` 与 `go vet ./...` 退 0；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api ./internal/architecture/... ./migrations/...` 全 ok；机制清点已重生成，单独提交。全仓全量未跑。
- **顺带发现（不在本票，未改）**
  - 领域 `LoadAssignment.Withdraw` 在变化版上撤回时，照抄前版的 `revisedAt`，产出的行撞 0009 的 `load_assignment_revision_xor_withdrawal`。撤回入口接上（tf-unwired-seven/05）之前要修。
  - 响应形状没有撤回格：链头是撤回版时，调用方从本口读不出它已撤回。本票未加字段。
- **未做**：票 15 补记下的回写行（越出本票地盘，留给会话）；非作者评审；推送方全量。
- **证据层级**：`S`（隔离合成，`SYN-` 夹具）。
- 评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `15174149`（基线 `59403e6f`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。编排先按键查（原样重放仍答 `LOAD_ASSIGNMENT_VERSION_EXISTS`），键不在再读链头 `FindChainHead`，分配已有版本即答 `LOAD_ASSIGNMENT_ALREADY_FORMED` 并回显链头、不落库；读链头失败答未决。迁移 0021 先查存量并列首版、有就列出并拒绝迁移、不代删，再建部分唯一索引；撞索引时按分配读链头。真渠道现象一、三复现后转绿，并发两条用例稳定。
  - **Standards**：阻断无，非阻断无。
  - **票外发现（建议另立）**：(1) 领域 `LoadAssignment.Withdraw` 撤回变化版时照抄前版 `revisedAt`，存库会撞 0009 的 `revision_xor_withdrawal`，撤回入口接上之前要修。(2) 链头是撤回版时答复里没有撤回标记，调用方看不出分配已撤回。

### 进 main 记录（2026-10-18，会话 idp-parcel-9b）

- 分支 `claude-tfa02` rebase 到远端 main 之上，快进推送；迁移用 `transport_fulfillment/0021`。验证钉 `0a5afa74` 之后远端只多一笔纯 `.md`（`4c2b0daa`，票 03、05 裁定），rebase 后代码与已验那份逐字相同，未重跑。
- 推送方全量一次：钉 `0a5afa74` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
