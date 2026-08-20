# 派发进程启动不验证数据库可用与 schema 就绪

Category: enhancement
Status: resolved

来自外部评估（基线 `49a2ab0`），协调岗已核实。

## 现象

`cmd/parcel-dispatch` 的 `assembleDispatcher` 建池用 `pgxpool.New`（惰性，不实际连库），
之后直接 `wireDispatcher`；全程没有 `Ping`，也没有调 bento 现成的只读 `CheckSchema`。
`Loop.Run` 对每拍失败只 `logger.Error` 后继续（注释：「一拍失败只记下来，不把进程拖死」）。

错误 DSN、库不可达、缺 bento schema 时，进程照常起来并常驻，以每拍报错的方式表现——
「部署错了」和「运行中依赖抖动」在观测上混成同一种日志。

## 期望

装配点在建池后做一次启动就绪检查，失败即返回错误让进程带明确原因退出：

1. `pool.Ping(ctx)` —— 库可达性。
2. `db.CheckSchema(ctx)` —— bento 框架 schema 就绪（bento 提供的只读检查，未被调用）。
3. 业务迁移版本检查：`internal/platform/migrate` 若已有现成读口就带上；没有就不造，
   不归本票扩。

`Loop` 对运行中瞬时故障的容忍行为保持不变——本票只分「启动时就坏」这一格。

## Comments

- 2026-08-20 MCP-1：外部评估四项可操作发现之一（其第 6 条），核实属实后立票。
- 2026-08-20 MCP-1：派 MCP-2，基线 `a097d7f`（非票头的审计基线 `49a2ab0`）。该票独占共享接线
  `cmd/parcel-dispatch/assemble.go`，在它合入前不再派第二张碰该文件的票。
- 2026-08-20 MCP-1：MCP-2 完工于 `093d53c`，父提交即当时的 `origin/main` tip `a097d7f`，快进直推、
  无需 cherry-pick。协调岗在 detached 验证树上独立复跑全部门禁：gofmt / go build / go vet /
  `git diff --check` 全过；全仓 `go test -p 1 -count=1 ./...` 绿，**含 PG**（3 分 28 秒，68 包 ok、
  零 FAIL）。真库确在场：跑前 `docker compose ps` 见 `127.0.0.1:55432->5432/tcp`，新增三用例经 `-v`
  复核全 `PASS` 无 `SKIP`，且 `docker inspect` 显示容器 `RestartCount=0`、全程未重启。已推
  `093d53c:main`，`assemble.go` 的独占解除。
- 2026-08-20 MCP-1：**本票有意留下一格缺口**——业务迁移是否施加齐全没有第三道检查，因为
  `internal/platform/migrate` 今天只导出施加计划的 `Run`（要独占连接、会建表，且该包明写自己
  从不在应用启动时运行），没有可用的只读状态读口。业务表缺失时进程仍起得来并按每拍报错表现。
  按票面「没现成读口就不造」办，要补先给 `migrate` 定出只读状态口，另立一票。
