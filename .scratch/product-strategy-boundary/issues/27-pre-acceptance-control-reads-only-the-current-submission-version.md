# 27 受理前控制只认委托当前提交版本：已换代的版本不读申报、不形成冻结

Category: bug
Status: 完工，待评审与重放——2026-10-13 20:5x 通道 1：做什么三条落在 `mcp1-psb27`，代码 tip `03e8de14`、清点 `f47173d2`，见文末「完成记录」。第 1、2 条的代码是上一个通道 1 会话留下的未提交现场（八个文件 mtime 停在 20:22:56），本会话 20:4x 接手，自审后补了消费门那一格的纵向用例，分两笔提交。用户 20:4x 告知通道 2–5 空闲，评审门改为派非作者通道，认领时写的「推送方自审」作废；重放进 main 归通道 1。此前 in-progress——2026-10-13 20:1x 通道 1 认领（用户 20:0x 经队列告知「现在只有你可以工作了，请你自己独立全部完成」；此前两张派单 `task-28bd6f8c` → 通道 2、`task-3e3a24ac` → 通道 3 均作废，两边都没留下提交），分支 `mcp1-psb27`，基 `cd728610`（= origin/main），隔离 worktree `idp-parcel-mcp1-psb27`。作者与推送方同为通道 1，评审门照 [workflow.md](../../../docs/agents/workflow.md)「本仓不用子代理」那条由推送方自审，不算非作者评审。此前 ready-for-agent——2026-10-13 19:3x 通道 1 立（用户 19:2x 经队列点头「按你的建议开始吧」），出自[票 25](./25-pre-acceptance-estimate-formed-by-settlement.md)两段非作者评审：第一段 Spec ③（`task-c7cfb347`）与第二段 Standards 1（`task-386c6296`），原文摘在票 25 的 Comments。
Blocked by: 无
地盘：`internal/parcelshipment/domain`（`SubmissionVersionDeclarationFor` 与其用例；`FormNewSubmissionVersion` 只读不改）、`internal/parcelshipment/adapters/postgres` 的提交版本申报读口（`submission_version_declaration_view.go` 与其用例）、`internal/parcelshipment/application`（守卫若落在编排一层，只动受理判断里发起财务控制的那一段与其用例）、`cmd/parcel-dispatch` 的纵向用例（只新增）。**不碰** `internal/parcelshipment/adapters/settlementaccounting/`——那是[票 28](./28-psb25-review-tails-estimate-gaps-unit-folding-and-trace-as-of.md)的地盘，真要动先报通道 1。
出处：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定二「对委托当前提交版本的每一件声明包裹」、决定四「按委托当前提交版本读」；ADR-0045（新版本成为唯一待判断版本）；ADR-0094 决定一、二（`未形成`与`未配置`的恢复动作）。

## 现象（读码，钉 main `6e4d9a1d`）

- PS 领域 `ShipmentRequest.SubmissionVersionDeclarationFor` 的头注写「当前版本与此前各版都认，取哪一版由调用方指名」，PS 读口 `submission_version_declaration_view.go` 原样调它。
- PS `adapters/settlementaccounting` 的 `applyCommandFor` 把请求里的提交版本原样交给 SA，SA 施加编排据此要试算、形成估价、冻结。
- 续办经 `ListWaitingOnOperatorRegistration` 取 `current_submission_version_id`，这一路守住了当前版本。提交信封重投按信封里的版本走，而 `FormNewSubmissionVersion` 在`已提交`、未形成决定时允许换代（头注：新版本成为唯一待判断版本，旧任务转`已停止`）——这一路没守：旧版本的信封再投一次，会为一个不再待判断的版本读申报、占资金。已删的旧执行器 `estimation_amount.go` 遇版本换代即停。
- 今天采用册是空的，估价走不到读申报那一步，所以还没出错；[票 26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)给演示租户登了采用行之后就走得到。

## 做什么

1. 先写 red：同一委托在`已提交`时换代，旧版本的提交信封再投一次，断言不为旧版本形成冻结或信用暴露、也不读它的申报。用例落在能复现这条路的最低一层（编排层或纵向真库，开工时核）。
2. 守住它。两处可落，选一处并写进判断项：
   - 读口一侧：`SubmissionVersionDeclarationFor`（或读口适配器）对非当前版本答不可见。SA 现有原因 `EstimateSubmissionNotVisible` 接得住，PS 映射里它落`未形成`。
   - 编排一侧：受理判断发起财务控制之前核信封版本是不是委托当前版本，不是就照旧任务`已停止`的现状收口，不发控制请求。

   取舍要答清两件：旧版本信封重投落哪一格、烧不烧失败预算（ADR-0094 决定一、二）；`SubmissionVersionDeclarationFor` 头注那句「此前各版都认」改不改、改成什么。
3. 当前版本那一路行为不变：既有用例全绿，经当前版本续办的纵向用例不动。

## 不做

- 不改 SA 施加编排、SA 原因集与 PS `notFormedOutcomeFor`（票 28 的地盘）。
- 不改 ADR-0179；读法与它对不上时停下报通道 1。
- 不碰 `scripts/demo-seeds`（票 26）。

## 完成判据

- 第 1 条的 red 修前红、修后绿；去掉守卫的变异（`go test -overlay`）转红。
- 选定的那一处写成判断项。改了 PS 领域语义（头注那句）或定了旧版本信封落哪一格的，标「越权风险点 · 待 owner 复核（PS）」。
- 架构门禁、机制清点、受影响包组绿，口径照 [parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「全量只跑一次」。证据层级只记 `S`。

## 完成记录（通道 1 · 2026-10-13 · 分支 `mcp1-psb27` 基 `cd728610`，代码 tip `03e8de14`、清点 `f47173d2`）

做什么三条全部落地。编排守卫、新结果与消费门那一格是上一个通道 1 会话写下、没有提交的现场（八个文件 mtime 停在 20:22:56，那个会话的记录止于 20:20 那次重跑受影响包）；本会话 20:4x 接手，通读自审之后补了纵向用例和一处注释出处，按两笔提交。纵向用例是读过实现之后才写的，所以判据只取票面（旧版本不判、不占资金、消费门收口、新版本照旧驱动），不照实现的形状写；下面两组变异各自能把它打红。证据层级一律 `S`。

### 各笔

| SHA | 内容 |
|---|---|
| `9608ed40` | 认领：Status 行 |
| `b0d80f15` | 第 1、2 条：`AdvanceAcceptanceChainHandler` 在第一步之前按信封的来源身份读回委托，读回了而信封指名的提交版本不是当前版本，就答 `AcceptanceChainVersionSuperseded`（`版本已换代`），三步都不走；`AdvanceAcceptanceChainDeps` 多一只 `Requests`，缺它报 `ErrAcceptanceChainNotAssembled`；消费门 `advanceAcceptanceChainThrough` 把新结果折成本份投递处理完毕；`cmd/parcel-dispatch` 的生产装配与四条闭环夹具各补一行 `Requests`；编排层用例 `TestASupersededVersionIsNotJudgedAgain`，`TestAnUnassembledChainFailsLoudlyInsteadOfWaiting` 多一格 `requests` |
| `03e8de14` | 第 1 条纵向：新文件 `cmd/parcel-dispatch/superseded_submission_envelope_test.go`，真库 |
| `f47173d2` | 机制清点在 `03e8de14` 上重生成：`cmd/` 测试文件 121 → 122 |

### 完成判据

- ✅ 第 1 条的 red 修前红、修后绿；去掉守卫的变异转红。两层各一条用例：
  - 编排层 `TestASupersededVersionIsNotJudgedAgain`：委托已换代到 `version-2`，命令仍指 `version-1`。答 `VERSION_SUPERSEDED`；商业依据、可达性、财务控制三个权威一个都不问（`calls` 为空，可达性与控制替身各 0 次）；不交接受决定意图，不保存聚合，状态仍是`已提交`。
  - 纵向 `TestALateSubmittedEnvelopeOfASupersededVersionSettlesWithoutJudging`（带 DSN `-v` 为 `PASS`）：首版「委托已提交」信封铸出后，客户以首版为基准受控补充，首版信封随后才投到提交门。结果：这份投递入账（inbox 一行）；可达性权威 0 次；`acceptance_reachability_judgment` 与 `acceptance_financial_control` 零行；不入队接受信封；委托仍是`已提交`，当前版本是补充形成的那一版。同一封再投由账本跳过。新版本那一封照旧驱到接受，两本账这时各落一行，证明前面数出零的确实是链会写的那两本账。
  - 「不读它的申报」：申报只经财务控制端口那一路被读（SA 施加编排经 PS 读口 `submission_version_declaration_view.go` 取）。编排层控制替身 0 次、纵向控制账零行，控制没发，申报也就没读。
  - 修前红：两条用例都引了改前没有的符号（`AcceptanceChainVersionSuperseded`、`Deps.Requests`），在 `cd728610` 上编不过；能判读的红取自变异（`go test -overlay`，副本在本机 `/tmp/psb27-mut`）。m1 在守卫条件前加 `false &&`：编排层用例答 `DECIDED`，即旧版本一路走到了形成决定；纵向用例里可达性权威被问到 `SYN-VER-01`；两条都红。m2 把消费门那一格换成 `AcceptanceChainOutcomeInvalid`：纵向用例报 `unexpected acceptance chain outcome: "VERSION_SUPERSEDED"`，红。去掉 overlay，两条带 DSN `-v` 为 `PASS`。
- ✅ 选定的那一处写成判断项（下节 1–3）。定了旧版本信封落哪一格，已标越权风险点；PS 领域语义（`SubmissionVersionDeclarationFor` 头注）没改。
- ✅ 第 3 条，当前版本那一路行为不变：守卫只在读回委托且版本不等时生效，读不回照旧往下走。既有用例全绿（见「验证」）。经当前版本续办的四条闭环（受控补充、人工复核、运营登记、授权处置）只在装配里补了一行 `Requests`，断言一字未动。
- ✅ 架构门禁、机制清点、受影响包组绿，见「验证」。

### 判断项与越权风险点

1. **越权风险点 · 待 owner 复核（PS）**：守卫落编排一侧（票面第 2 条的后一条路），而且落在链的第一步之前，比票面写的「发起财务控制之前」早两步。原因是另两步也会出事：可达性那一步同样会为旧版本记判断；形成决定按命令里的版本读回判断，决定与接受基线却落在当前版本上。m1 下编排层用例答 `DECIDED` 就是后一格，只守财务控制挡不住它。PS 已有两格同形：复核完成的 `ManualReviewVersionSuperseded`、授权处置的 `AuthorizedDispositionVersionSuperseded`，都是`版本已换代`单列、不记到新任务上、不报错重试。
2. **越权风险点 · 待 owner 复核（PS）**：旧版本信封落「本份投递处理完毕」，不回滚、不烧失败预算。`版本已换代`是编排的第三种结果，不是 `ResumePath` 新增的一格；消费门按结果逐格折，仍不留 `default`（ADR-0094 Decision 一）。理由是重投换不回版本：新版本成为唯一待判断版本，旧任务转`已停止`（ADR-0045）；新版本由「新提交版本已形成」那一封驱动（ADR-0106 Decision 三）。回滚重投的代价也不止这一封的预算。同一分区 Outbox 只交出队头（bento `postgres/outbox` 的 `claimPartitions` / `loadActiveHead`；纵向用例实测，首版那一封定稿之前认领不到新版本那一封），旧信封回滚重投时，新版本的驱动随它一起堵着。判在编排、不判在消费门：ADR-0106 Alternatives 否决过「在消费门里判新版本是否已到达」，理由是那等于在适配层重建编排已有的判断、多一处对提交版本的读取。驱链的四扇门（提交、复核完成、运营登记完成、新提交版本已形成）都经 `advanceAcceptanceChainThrough`，同一格同一折法。
3. 判断项：`SubmissionVersionDeclarationFor` 头注「当前版本与此前各版都认，取哪一版由调用方指名」不改。读口照旧答调用方指名的那一版上申报了什么，那是真话；「只判当前版本」由链开头那一道核，不让读口代答。改读口一侧的话，SA 把换代读成 `EstimateSubmissionNotVisible`，PS 映射落`未形成`（`FinancialControlNotFormed`），其恢复动作是内部重试，旧版本那一封照样回滚重投到预算耗尽；而且读口只挡得住财务控制读申报那一下，挡不住判断项 1 那两步。
4. 判断项：读不回委托（`found` 为假或读出错）时，守卫不作答，照旧往下走。没有委托就谈不上换代；读不回委托那一格下游各步各有答法（等待运营登记那一支答`委托读不到`，形成决定那一步报委托查不到），在这里另答一次，同一种读失败就有了两种停法。喂不存在委托、走真图的装配用例（如 `TestAnUnconfiguredAcceptanceChainStallsAsUndecidedOnTheRealGraph`）因此照旧停在 `dispatch.consumer_undecided`，不必改。
5. 未覆盖：守卫读回委托之后、三步写账之前，另一事务提交了换代——这一窗没有验，也不在第 1 条要复现的路上（重投与迟到都是先后发生）。
6. 范围：越出票面地盘的有三处，都是为了接上新依赖与新结果。一是 `adapters/inbox` 的 `advanceAcceptanceChainThrough`：新结果不落格，就按集合外报错、投递回滚。二是 `cmd/parcel-dispatch/assemble.go` 的生产装配一行。三是四个闭环夹具文件各一行，而票面对 `cmd/parcel-dispatch` 写的是只新增用例。写这些文件时其余通道都空闲，`git worktree list` 只有共享树与本树。PS 领域、`adapters/postgres` 读口、`adapters/settlementaccounting/`（票 28 的地盘）一行没动。

### 验证（作者那层，go1.26.8，证据层级 `S`）

钉 `03e8de14`（worktree `idp-parcel-mcp1-psb27`，`git status --porcelain --untracked-files=all` 为空），20:47–20:51：

- 自 `cd728610` 起改过的 9 个 `.go` 无 CR；toolchain `gofmt -l` 为空（新文件补上末尾换行之后）。
- `go build ./...`、`go vet ./...` 全仓退 0。
- 改过的两个包（PS `application`、`adapters/inbox`）经 `go list` 反查反向依赖，连同 `cmd/parcel-dispatch` 共 12 个包（`cmd/parcel-api`、`cmd/parcel-dispatch`、PS `application` 与九个适配器）。带 `IDP_PARCEL_POSTGRES_DSN` 跑 `go test -count=1 -p 1 <这 12 个> ./internal/architecture/...`：12 ok（含 `internal/architecture`）/ 0 FAIL / 1 无测试文件（`adapters/adoptconsume`）/ 0 cached，退 0。
- 带 DSN `-v` 单跑：`TestASupersededVersionIsNotJudgedAgain`、`TestALateSubmittedEnvelopeOfASupersededVersionSettlesWithoutJudging`、`TestACustomerSupplementWaitIsResumedByTheNewSubmissionVersionEnvelope` 为 `PASS`，没有 `SKIP`。
- 分片核对 `scripts/ci/test-shards.sh check` 通过（153 个包，无重叠、无遗漏）。
- 机制清点在 `03e8de14` 上重跑生成器（`tools/mechanism-inventory`），差一行，成笔 `f47173d2`。
- 全量不在作者这层跑（[parallel-sessions.md](../../../docs/agents/parallel-sessions.md)「全量只跑一次」），由推送方在 tip 上跑。
