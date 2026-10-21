# 68 VE、TF 读初始路由时把重建不出来的坏行折成可等的未决，重投到耗尽

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code68。此前 ready-for-agent——R2（上线后尽快）：坏行是确定性错误，重投推不动，整封信封占着分区重投到 ABANDONED；与 code/52、code/60 在 NR 一侧修掉的是同一类病。2026-10-21 协调方据 code/60 隔离评审（N2）立。
Blocked by: 无
出处：code/60 隔离评审；NR 一侧已由 code/52（复核）、code/60（初始路由）让 `ports.ErrStoredRouteRecordUnreadable` 响亮报错。

## 现象

VE 由初始路由派生源事实、TF 由初始路由取计划段，两处读 `FindByKey` 出错时都包成 `fmt.Errorf("%w: %v", ErrInitialRouteNotVisible, err)`。`%v` 丢掉了 `ports.ErrStoredRouteRecordUnreadable` 的错误链，坏行于是进了各自的未决哨兵名单，按「路由还没形成、稍后再看」重投。

## 证据

- `internal/visibilityexception/adapters/networkrouting/derive_on_initial_route.go` 的 `HandleFormedInitialRoute`；`cmd/parcel-dispatch` 的 `veInitialRouteUndecidedSentinels`。
- `internal/transportfulfillment/adapters/networkrouting/planned_leg_source.go` 读 `FindByKey` 那一处。
- NR 端口 `ports.ErrStoredRouteRecordUnreadable` 的注释：坏行与「读不回」分开交，恢复动作是修数据、不等依赖。

## 三类

机制。

## 规模

S：两处各先 `errors.Is(err, ports.ErrStoredRouteRecordUnreadable)`，认出就响亮上抛、不包未决哨兵；各补一条用例（替身交坏行 → 响亮报错，不进未决）。

## 最小的诚实修法

1. VE、TF 两个适配器认出坏行哨兵就上抛（`%w` 保链），其余读不回照旧折未决。
2. 用例：坏行响亮、读不回仍未决。

## 完成记录（2026-10-21，作者会话，分支 claude-code68，基于 main `3ac24091b`）

### 做了什么

- VE `DeriveOnInitialRouteAdapter.HandleFormedInitialRoute`、TF `PrepareOnInitialRoute.HandleFormedInitialRoute`：读 `FindByKey` 先 `errors.Is(err, nrports.ErrStoredRouteRecordUnreadable)`，认出就以 `load initial route: %w` 上抛，不包 `ErrInitialRouteNotVisible`；其余读不回照旧 `%w: %v` 折未决。两处派发未决名单都只认各自的 `ErrInitialRouteNotVisible`，坏行于是落到硬失败格。
- TF 未决名单 `tfInitialRoutePreparationUndecidedSentinels` 的头注「不在名单里」补上这枚 NR 哨兵。
- 全仓扫过别的上下文读 NR 初始路由或复核册的地方：NR 端口在 NR 之外只有 VE、TF 这两处处理适配器读 `InitialRouteStore.FindByKey`；TF 的交付窗口取数侧读的是 NR 计划段窗口，已用 `%w`，那张表也不交这枚哨兵；PS 可达性适配器走 NR 编排，不读路由库；`cmd/parcel-api` 只用 NR 目录端口。NR 自己的入站适配器把复核、初始路由编排的错误都 `%w` 上抛。没有第三处要改。
- 文档：UC-VE、UC-TF 里没有讲这一格的现状注，没动。

### 代决

1. **坏行的包法用 `load initial route: %w`，不另立适配器哨兵。** 与 NR 编排侧 code/60 的 `load initial route: %w` 同形；另立一枚适配器哨兵只是给同一件事换个名字，派发名单本来就认不出它，处置（修数据）也看 NR 那枚就够。
2. **其余读不回保留 `%w: %v`，不改成双 `%w`。** 票面要「其余照旧」；改成双 `%w` 会让 NR 存储往后新加的任何哨兵自动穿过来，但同时也让它们挂着未决哨兵进名单，正是本票这类病，不如逐枚认。
3. **VE 未决名单 `veInitialRouteUndecidedSentinels` 的头注不补。** 它在 `assemble.go` 里，ADR-0222 约定既有哨兵名单与 `assemble.go` 不为一条新口径去改；名单本身不用动，逐路钉住它认不出坏行的那条用例已在测试里。TF 那份在特性文件里，补了。
4. **派发侧补了两条，分工不同。** 真库那条只看得到合并后的码：FanOut 取最响的一路，一路修好、另一路仍折未决也会落 `publish_failed`。所以另取两路生产处理适配器与生产未决名单逐路钉。

### 用例

- VE `derive_on_initial_route_test.go` 的 `TestAnUnrebuildableInitialRouteRowIsLoudNotUndecided`：替身交坏行，错误链带 `ErrStoredRouteRecordUnreadable`、不带 `ErrInitialRouteNotVisible`，不走到派生。读不回仍未决由既有的 `TestAnUnreadableInitialRouteIsContinuableUndecided` 守。
- TF `planned_leg_source_test.go` 的 `TestUnreadableOrUntranslatableRoutesFailWithTheirOwnSentinels` 加坏行一格：同上两条断言；读不回、读失败两格原样保留。
- 派发真库 `cmd/parcel-dispatch/initial_route_projection_test.go` 的 `TestAnUnrebuildableInitialRouteRowFailsLoudInsteadOfWaiting`：生产 NR 交接口入队，`initial_route.plan` 改坏后生产 wireDispatcher 一拍，不定稿，码面 `dispatch.publish_failed`，VE、TF 两路 inbox 都不入账。
- 同文件 `TestNeitherInitialRouteLaneListsAnUnrebuildableRow`：两路生产处理适配器接坏行替身，交回的错误不被各自的生产未决名单认出。
- 先跑红：修前四条全红——两处适配器答 `initial route judgment is not yet visible: ... does not rebuild`，真库那条码面 `dispatch.consumer_undecided`，逐路那条在 VE 一路就进了名单。

### 验证

- `go build ./...`、`go vet ./...` 干净；`gofmt -l cmd internal` 为空。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/visibilityexception/... ./internal/transportfulfillment/... ./cmd/parcel-dispatch ./internal/architecture/...`：退 0，29 个包 ok，2 个包无测试文件。
- 没跑全量；没重生成生成物。
