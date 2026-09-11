# 34 lc/28 组合根的触发面：`buildLabelChannelOrchestration` 无生产调用方，`parcel-api` 从不构造这条链，构造期错误不在启动时暴露

Category: enhancement
Status: resolved——2026-09-11 11:3x（作者自标）通道 4 落地（task-c7390f8a；分支 `mcp4-lc34` 基远端 main `2c7326ef`，代码 tip `38f2322c`）：`run` 启动时装配 `buildLabelChannelOrchestration` 构造即丢 + 四处头注改口；完成判据 1–5 逐项见 Comments 末条；进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-11 11:2x 通道 4 认领（task-c7390f8a，通道 1 派单；隔离树 `D:/tops/idp-parcel-mcp4-lc34`）。此前 ready-for-agent——2026-09-10 22:0x 通道 4 立票（按通道 1 派单 task-d00c5556；lc/28 非作者评审 Spec 非阻断 ② / 判断题 (b) 的后继）。要裁的为零：本票只做「`main` 启动时装配、构造期错误 fail-fast」这一步；谁在什么业务时点发起一笔面单交易的建立（运营端点 / 进程内触发）是产品题，不在本票。只写票面未动代码；取证锚 main `9ddbafcf`
Blocked by: 无（[`28`](./28-channel-selection-composition-root-and-call-entry.md) 已进 main `784ad076`）

## 缺口（取证于 `9ddbafcf`，逐符号名）

- `cmd/parcel-api/assemble_label_channel.go` 的 `buildLabelChannelOrchestration(db)` 是面单渠道写链的组合根（lc/28 裁乙）。`git grep buildLabelChannelOrchestration -- cmd/ ':!*_test.go'` 只命中自身文件；`main.go` `run` 里其余每一只 `build*Orchestration` 都在启动时装配，唯独没有它。`labelChannelOrchestration.Flow` 今天只被 `assemble_label_channel_test.go` 调用。
- 后果（lc/28 评审 Spec ② 原话）：`parcel-api` 二进制从不构造这条链，构造期错误——`pcpostgres.NewChannelAccountUseAuthorizations` / `NewSupplierAgreementContents` / `pspostgres.NewChannelSelectionDecisions` / `outbox.NewStore` 任一失败——生产上不会在启动时暴露；`internal/architecture/production_wiring_ratchet_test.go` 只量 `domain` 导出工厂，不会为此变红。lc/28 完成记录原写「生产可达」，推送方按评审改读为「可装配、未触发」，触发面归本票。
- `main.go` `run` 自己的纪律（开池那段头注）：「部署坏了……要让进程带原因退出，而不是先挂上监听端口再让每个请求各报一次错」——这条链今天连这一道都没过。
- 顺带量到三处头注还在说旧话：`internal/parcelshipment/adapters/postgres/channel_selection_decisions.go` 头注「本适配器今天没有生产装配点：择优编排 SelectChannelCandidateHandler 自身尚无组合根（票 12 收口……）」——lc/28 之后已不成立（组合根就是 `buildLabelChannelOrchestration`，lc/28 改口时漏了这一处）；`cmd/parcel-api/unwired_orchestration.go` `unwiredLabelTransactions` 头注与 `cmd/parcel-api/assemble_channel_selection_decisions.go` 头注写「组合根已在、今天没有触发面」——本票落地后要改成「启动时已装配、今天没有触发面」。

## 做法

1. `main.go` `run` 在两张读面（`pspostgres.NewLabelTransactionViews` / `buildChannelSelectionDecisionRead`）之后调 `buildLabelChannelOrchestration(db)`，错误即 `return err`（照同函数其余 `build*` 的形）。产物今天没有消费者，两种落法行为等价、作者定：`if _, err := …; err != nil` 构造即丢，头注写明「只为 fail-fast，触发面另票」；或挂到端点装配那只结构体上留给触发面票取用。
2. 「缺口」末段三处头注改口。`channel_selection_decisions.go` 那句「尚无组合根」直接删——它引的票 12 收口那段已由 lc/28 结清——改指 `buildLabelChannelOrchestration`。
3. **不做**：运营端点（甲，lc/28 裁决 28-1.3「甲不立票」）；进程内触发（委托受理后自动、还是运营动作，属产品流程，今天无依据）；07 真适配器；三取数口的真实取数（实例半边）。

## 红线

- 零行为变化：不新增端点，不动端点表 / 探针 / 放行表 / `isolated_read_test.go`；启动后没有任何请求能到 `Flow`。
- 六个实例半边缝仍全部显式未配置（lc/28 红线原句），不为 fail-fast 变绿种任何行。
- `main.go` 是共享接线文件：动手前在频道占号，只加自己那一块、不动邻行（`docs/agents/parallel-sessions.md`「共享接线文件」节）。

## 完成判据

1. `git grep -n buildLabelChannelOrchestration -- cmd/parcel-api/main.go` 一处非测试调用点；构造失败时 `run` 带原因退出。`cmd/parcel-api` 今天没有 `main_test.go`、没有「启动装配」夹具，**不为本票新造**——证据是 `go build` + `assemble_label_channel_test.go` 既有真库用例（它走的正是同一只装配函数）全绿。
2. 三处头注改口；`git grep -n '尚无组合根' -- internal/parcelshipment/adapters/postgres/channel_selection_decisions.go` 零命中。
3. 端点表 / 探针 / 放行表 / `isolated_read_test.go` 零 diff；`internal/architecture` 两道棘轮不响。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` `cmd/parcel-api`（带 DSN）+ `./internal/architecture/...` 绿；机制清点预期不变（不增删文件），变了就在干净检出重生成。
5. 完成记录逐笔 SHA。票 `28`「进 main 记录」里「票号待立」那句已在本票立票那一笔回填为 `34`，落地时不必再动 28。

## 地盘

`cmd/parcel-api/main.go`（一块，占号）、`cmd/parcel-api/assemble_label_channel.go` 文件头注若随之改口、`cmd/parcel-api/unwired_orchestration.go` 与 `cmd/parcel-api/assemble_channel_selection_decisions.go` 两处头注、`internal/parcelshipment/adapters/postgres/channel_selection_decisions.go` 头注；本票面；lc spec 子票表一行。**不动**端点表 / 探针 / 放行表 / `isolated_read_test.go`、`internal/parcelshipment/application/**`、`internal/architecture/*_baseline.txt`。

## 要裁的

无。「要不要让 `main` 启动时装配」评审（通道 3）倾向、作者判断题 (b) 提出、推送方列为候选后继，三方一致，本票只做这一步。触发面本身（谁发起、何时发起）是产品题，另议、无票号；届时它只需调 `labelChannelOrchestration.Flow`（`assemble_label_channel.go` 文件头注原句）。

## 参照

票 `28` Comments「评审 ← 通道 3」Spec 非阻断 ② 与判断题 (b)、「进 main 记录」；`cmd/parcel-api/assemble_label_channel.go` 文件头注（「今天没有运营端点、也没有进程内触发面调它」）；`cmd/parcel-api/main.go` `run`；`internal/architecture/production_wiring_ratchet_test.go` 头注；lc/12 收口两条 Comment（甲 / 乙代价）；`docs/agents/parallel-sessions.md`「共享接线文件」节。

## Comments

- 2026-09-10 22:0x · 通道 4（task-d00c5556，取证锚 main `9ddbafcf`）：立票，Status 直接 ready-for-agent（要裁的为零）。**只写票面，未动代码**；同笔把票 `28` 进 main 记录里「票号待立」回填为本票号（只改那一句）。能力边界：读过 `assemble_label_channel.go` 全文、`main.go` `run` 的开池段与全部 `build*` 调用、`unwired_orchestration.go` 的 `unwiredLabelTransactions` 头注、`channel_selection_decisions.go` 头注首句、架构棘轮头注、lc/28 全文、`cmd/parcel-api` 测试文件清单（无 `main_test.go`）；**没读** `endpoints.go` 那只装配结构体的形——做法 1 第二种落法（挂到结构体上）能不能不碰邻行未核，作者开工时看。「顺带量到」的 `channel_selection_decisions.go` 那句陈旧头注是本票新量到的，lc/28 评审没提。
- 2026-09-11 11:3x · 通道 4（task-c7390f8a；分支 `mcp4-lc34` 基远端 main `2c7326ef`）：**完成记录，转 resolved。** 逐笔（分支 SHA 只作此刻取证）：`6cf854a9` docs Status → in-progress；`38f2322c` feat `run` 启动时装配 + 四处头注改口（做法 1、2）；本笔票面 + lc spec 34 行。代码 tip `38f2322c`。
  **落法**：做法 1 两种落法取**构造即丢**——`if _, err := buildLabelChannelOrchestration(db); err != nil { return err }`，位置在 `buildChannelSelectionDecisionRead` 之后、`buildCancellationOrchestration` 之前（票面原句「两张读面之后」）。不挂到端点装配结构体：那要碰 `endpoints.go` 那只结构体的邻行（不在地盘、sa-cc/07 同期要动），且今天没有任何消费者会读那一格，多一个永不被读的字段只会让人以为有人在用。头注写明「只为 fail-fast、触发面另票」。
  **完成判据逐项**：
  1. ✓ `git grep -n buildLabelChannelOrchestration -- cmd/parcel-api/main.go` 恰一处（`main.go` `run`）；构造失败即 `return err`，与同函数其余 `build*` 同形，进程带原因退出。未新造 `main_test.go` / 启动夹具；证据是 `go build ./...` 退 0 + `assemble_label_channel_test.go` 既有真库两例（`TestTheProductionLabelChannelChainStopsHonestlyAtTheUnconfiguredSources` / `TestTheLabelChannelChainWalksToTheOutboundSeamOnceTheSeamsAreConfigured`）带 DSN `-v` PASS 2 / SKIP 0——它们走的正是同一只装配函数。
  2. ✓ 四处头注改口（票面说三处，`assemble_label_channel.go` 文件头「若随之改口」也改了，共四处）：`assemble_label_channel.go`「生产可达」→「已装配、未触发」+ 启动时装配只为 fail-fast；`unwired_orchestration.go` `unwiredLabelTransactions` 与 `assemble_channel_selection_decisions.go`「组合根已在、没有触发面」→「启动时已装配、没有触发面」；PS postgres `channel_selection_decisions.go` 删「尚无组合根（票 12 收口）」句、改指 `buildLabelChannelOrchestration`。`git grep -n '尚无组合根' -- internal/parcelshipment/adapters/postgres/channel_selection_decisions.go` 零命中。
  3. ✓ `git diff --stat 2c7326ef -- cmd/parcel-api/endpoints.go cmd/parcel-api/endpoints_test.go cmd/parcel-api/isolated_read_test.go internal/architecture/` 零行；`internal/architecture` 两道棘轮在带 DSN 的包测里过。
  4. ✓ `gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -count=1 ./cmd/parcel-api/ ./internal/architecture/...` 两包 ok（11:26:44→11:26:50，占号 / 释号已广播）。未跑全量（派单原句）。机制清点不增删文件，预期不变、未重生成。
  5. ✓ 本条逐笔 SHA。票 `28`「进 main 记录」的票号已在立票笔回填，本次未动 28。
  **零行为变化**：不新增端点，启动后没有任何请求能到 `Flow`；六个实例半边缝仍全部显式未配置，未种任何行。`buildLabelChannelOrchestration` 内部会再构造一次 `NewChannelSelectionDecisions` / `NewLabelTransactions` / `outbox.NewStore`——与 `buildChannelSelectionDecisionRead` / 其余 `build*` 各自构造自己的适配器同一纹样，构造函数只校验依赖、不开连接，不算重复装配。
  **评审**：按派单「不自评代替评审」，`/code-review` 双轴评审留给非作者；作者只对着基线看过 diff（五文件、代码 7 行加、余为注释）。
  **不做的**：运营端点（甲）；进程内触发；07 真适配器；三取数口的真实取数。
  **地盘外零改动**：`cmd/parcel-api/endpoints.go` / `endpoints_test.go` / `isolated_read_test.go`、`internal/parcelshipment/application/**`、`internal/architecture/*_baseline.txt`。共享接线 `main.go` 只加自己那 7 行、不动邻行；`assemble_label_channel.go`（lc/32 同期）与 `unwired_orchestration.go`（ve-disc/03 同期）只改头注行，与它们的块不重叠，已随释号广播提醒。
  **能力边界**：本票无新行为可 red，`/tdd` 不适用（判据 1 原句「不为本票新造夹具」）；`run` 的 fail-fast 分支未在测试里实跑（没有启动夹具，且票面明令不造），只以 `go build` + 同形的既有 `build*` 调用为据。
