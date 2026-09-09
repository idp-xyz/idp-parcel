# `internal/platform/pgtest` 改模板库：先量四段各占多少，再让每用例 `CREATE DATABASE … TEMPLATE …`

Category: enhancement
Status: in-progress——2026-09-09 12:0x 通道 5 认领，分支 `mcp5-pgtest01`，基 `74ef0da8`。立票经过：2026-09-09 通道 1 代裁立票（用户授权自决）：MCP-5 09-08 提议、parallel-sessions「验证」节已把解法与不许走的路写死（模板库；不关 `fsync`、
不用事务回滚包裹），本票是那一段的票。**先量再改**，量不出「时间几乎全在夹具」就停下报数、不改
Blocked by: 无

## 缺口（parallel-sessions「验证」节的取证，锚 `6ae24ae2`）

`pgtest` 给**每个用例**一个物理库：`CREATE DATABASE` → 跑真实迁移计划 → `DROP DATABASE … WITH (FORCE)`。`internal/parcelshipment/adapters/postgres` 143 个用例总 45.2 秒、
中位 0.31 秒、最慢 0.42 秒，均匀到看不出哪个用例重——时间几乎全在建库 + 全套迁移 + 删库，不在用例正文。全仓含 DSN `-p 1` 一次 9–10 分钟，
推送方每票付一次；作者与评审各自再付。

## 取证

### 改前——逐用例迁移（量于 `788e226c`，基 `74ef0da8`）

量法：`pgtest` 内打点。设 `IDP_PARCEL_PGTEST_TIMING` 指向一份 TSV，每次 `Pool(t)` 在清理函数末尾追加一行「建库 / 迁移 / 用例正文 / 删库 / 合计」
（列定义见 `internal/platform/pgtest/timing.go` 的 `TimingVariable` 注释）；未设变量零行为变化。命令：带 DSN 与该变量
`go test -count=1 ./internal/parcelshipment/adapters/postgres/`，143 行 TSV，跑两遍。

环境：本机 55432 单实例 `postgres:16.14`（容器 `idp-parcel-postgres-gate`）。量前 `docker stats` CPU 2.5%，`pg_stat_activity` 客户端后端 1
（只有取数的 psql 自己），无遗留 `parcel_*` 库。量数窗口 14:16–14:18 先广播给通道 2/3/4/6 请他们别跑带 DSN 的包；两遍数字相差不到 1%，互证窗口内无干扰。

| 段 | 中位 ms（跑 1 / 跑 2） | 合计 ms（跑 1 / 跑 2） | 占合计（跑 1） |
|---|---|---|---|
| 建库 | 5.7 / 5.7 | 840 / 849 | 1.9% |
| 迁移 | 283.2 / 285.8 | 41,034 / 41,298 | 93.6% |
| 用例正文 | 7.0 / 6.9 | 1,029 / 1,034 | 2.3% |
| 删库 | 4.6 / 4.5 | 949 / 850 | 2.2% |
| 合计 | 301.3 / 304.6 | 43,852 / 44,030 | 100% |

`go test` 自报 43.9 s / 44.1 s。迁移 + 建删库合计占 97.7%，远过一半——停下条件不触发，做法第 2、3 步继续。最慢的用例正文 18.6 ms
（`TestListWaitingOnCustomerSupplementFiltersOrdersAndTranscribes`），没有任何一个用例的正文超过其合计的 7%：「时间几乎全在夹具」由数字坐实，
且几乎全在**迁移**那一段，建库与删库合起来只占 4%。

## 做法

1. **量**（单独一笔，只加脚本 / 测试辅助，不改 `pgtest` 行为）：把一个用例的耗时拆成「建库 / 迁移 / 用例正文 / 删库」四段，对 PS postgres 包全部用例取
   中位数与合计，数字与 SHA 写进本票「取证」节。四段怎么量由作者定（`pgtest` 内打点、或 `-v` 时间戳差），写出来。**若迁移 + 建删库合计不到总时长的一半**，停下，
   本票转 needs-info 报数，不做下面两步。
2. **模板库**：每个测试进程（每个包的 `TestMain` 或首次 `Pool(t)`）只迁移一次到一个模板库（名字带进程 / 包标识，避免并行包相撞），之后每用例
   `CREATE DATABASE <用例库> TEMPLATE <模板库>`（文件级拷贝），用完照旧 `DROP … WITH (FORCE)`；进程退出时删模板库。**隔离语义与「证的是随产品发出的那份 SQL」一字不变**
   ——模板库就是用那份迁移计划建的。改动只落在 `internal/platform/pgtest` 一个文件（或它的目录内），**`Pool(t)` 的签名不动**，任何一个调用方零改动。
3. **再量**：同一组用例重跑，四段数字并列写进「取证」节；`-p 1` 能不能放**不在本票裁**——parallel-sessions 写明「以 `ci.yml` 那种带 run 号的实测为据，不凭推」，
   本票只把两组数字交出来，放不放另立票。

## 不许走的路（parallel-sessions 已写死，此处只引不复述理由）

关 `fsync`；事务回滚包裹用例。

## 完成判据

四段耗时两组数字在票面；`pgtest` 模板库落地且 `Pool(t)` 签名不变；含 DSN 全仓 `go test -p 1 -count=1 ./...` 与改前同码（100 ok / 0 FAIL / 16 无测试 的口径）、
用时写进票面；`internal/platform/pgtest` 自己的测试盖「模板库只建一次 / 每用例库独立 / 进程退出模板库删掉」。

## 边界

不动任何迁移文件；不动 `compose.yaml`；不动 CI 分片。
