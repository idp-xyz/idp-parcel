# `internal/platform/pgtest` 改模板库：先量四段各占多少，再让每用例 `CREATE DATABASE … TEMPLATE …`

Category: enhancement
Status: ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：MCP-5 09-08 提议、parallel-sessions「验证」节已把解法与不许走的路写死（模板库；不关 `fsync`、
不用事务回滚包裹），本票是那一段的票。**先量再改**，量不出「时间几乎全在夹具」就停下报数、不改
Blocked by: 无

## 缺口（parallel-sessions「验证」节的取证，锚 `6ae24ae2`）

`pgtest` 给**每个用例**一个物理库：`CREATE DATABASE` → 跑真实迁移计划 → `DROP DATABASE … WITH (FORCE)`。`internal/parcelshipment/adapters/postgres` 143 个用例总 45.2 秒、
中位 0.31 秒、最慢 0.42 秒，均匀到看不出哪个用例重——时间几乎全在建库 + 全套迁移 + 删库，不在用例正文。全仓含 DSN `-p 1` 一次 9–10 分钟，
推送方每票付一次；作者与评审各自再付。

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
